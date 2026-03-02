package ffmpeg

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"vid-tui/model"
)

// probeResult maps ffprobe JSON output.
type probeResult struct {
	Format struct {
		Duration string            `json:"duration"`
		Size     string            `json:"size"`
		Tags     map[string]string `json:"tags"`
	} `json:"format"`
	Streams []struct {
		CodecType    string `json:"codec_type"`
		CodecName    string `json:"codec_name"`
		Width        int    `json:"width"`
		Height       int    `json:"height"`
		RFrameRate   string `json:"r_frame_rate"`   // e.g. "60/1" — max frame rate
		AvgFrameRate string `json:"avg_frame_rate"` // e.g. "30/1" — actual average
	} `json:"streams"`
}

// parseFrameRate parses an ffprobe rational rate string like "30000/1001" to float64.
func parseFrameRate(s string) float64 {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) == 2 {
		num, _ := strconv.ParseFloat(parts[0], 64)
		den, _ := strconv.ParseFloat(parts[1], 64)
		if den != 0 {
			return num / den
		}
		return 0
	}
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

// ProbeFile probes a single video file and returns its metadata.
func ProbeFile(path string) (*model.VideoFile, error) {
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		path,
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe %s: %w", path, err)
	}

	var result probeResult
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("parse ffprobe output for %s: %w", path, err)
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	vf := &model.VideoFile{
		Path: path,
		Size: info.Size(),
	}

	// Parse duration
	if result.Format.Duration != "" {
		vf.Duration, _ = strconv.ParseFloat(result.Format.Duration, 64)
	}

	// Parse creation time
	if ct, ok := result.Format.Tags["creation_time"]; ok {
		vf.CreationTime = parseCreationTime(ct)
	}

	// Find video and audio streams
	for _, s := range result.Streams {
		switch s.CodecType {
		case "video":
			if vf.Codec == "" {
				vf.Codec = s.CodecName
				vf.Width = s.Width
				vf.Height = s.Height
				// Detect VFR: r_frame_rate is the codec's max rate; avg_frame_rate
				// is the actual average. When they diverge by >2% the stream is VFR
				// and will cause timestamp discontinuities when concat-copied.
				r := parseFrameRate(s.RFrameRate)
				avg := parseFrameRate(s.AvgFrameRate)
				if r > 0 && avg > 0 && r/avg > 1.02 {
					vf.IsVFR = true
				}
			}
		case "audio":
			vf.HasAudio = true
		}
	}

	return vf, nil
}

// VerifyResult summarizes stream presence and any decode errors found.
type VerifyResult struct {
	HasVideo     bool
	HasAudio     bool
	DecodeErrors string // non-empty when ffmpeg reported frame-level errors
}

// VerifyOutput checks an output file for stream integrity.
// It always runs a fast ffprobe header check. If fullDecode is true it also
// runs "ffmpeg -f null" which decodes every frame and surfaces corruption.
// expectAudio=true treats a missing audio stream as an error.
func VerifyOutput(path string, expectAudio, fullDecode bool) (*VerifyResult, error) {
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-print_format", "json",
		"-show_streams",
		path,
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("verify probe %s: %w", path, err)
	}

	var pr probeResult
	if err := json.Unmarshal(out, &pr); err != nil {
		return nil, fmt.Errorf("parse probe output: %w", err)
	}

	vr := &VerifyResult{}
	for _, s := range pr.Streams {
		switch s.CodecType {
		case "video":
			vr.HasVideo = true
		case "audio":
			vr.HasAudio = true
		}
	}

	if !vr.HasVideo {
		return vr, fmt.Errorf("%s: no video stream", path)
	}
	if expectAudio && !vr.HasAudio {
		return vr, fmt.Errorf("%s: missing audio stream", path)
	}

	if fullDecode {
		var stderr bytes.Buffer
		dec := exec.Command("ffmpeg", "-v", "error", "-i", path, "-f", "null", "-")
		dec.Stderr = &stderr
		_ = dec.Run()
		if s := strings.TrimSpace(stderr.String()); s != "" {
			vr.DecodeErrors = s
			return vr, fmt.Errorf("%s: decode errors detected:\n%s", path, s)
		}
	}

	return vr, nil
}

// VerifyAndRepair runs VerifyOutput and, on failure, attempts RepairOutput then
// re-verifies. Returns nil if the output is valid or was successfully repaired.
func VerifyAndRepair(path string, expectAudio bool) error {
	vr, err := VerifyOutput(path, expectAudio, false)
	if err == nil {
		return nil
	}
	if repairErr := RepairOutput(path, vr, expectAudio); repairErr != nil {
		return fmt.Errorf("verification failed (%v) and repair unsuccessful: %w", err, repairErr)
	}
	if _, verifyErr := VerifyOutput(path, expectAudio, false); verifyErr != nil {
		return fmt.Errorf("repair attempted but output still invalid: %w", verifyErr)
	}
	return nil
}

// RepairOutput attempts to fix common issues detected by VerifyOutput.
//
//   - Missing audio stream: injects a silent AAC track (stream-copies video, near-instant)
//   - Decode errors: re-muxes the container with -c copy (fast, fixes some structural issues)
//   - No video stream: cannot be repaired; returns an error immediately
//
// vr may be nil if VerifyOutput could not read the file at all. The file at path
// is replaced in-place on a successful repair.
func RepairOutput(path string, vr *VerifyResult, expectAudio bool) error {
	if vr == nil {
		return fmt.Errorf("cannot repair: file is unreadable")
	}
	if !vr.HasVideo {
		return fmt.Errorf("cannot repair: no video stream in output")
	}
	if expectAudio && !vr.HasAudio {
		return injectSilentAudio(path)
	}
	if vr.DecodeErrors != "" {
		return remuxContainer(path)
	}
	return nil
}

// injectSilentAudio adds a silent AAC audio track to a video-only file.
// The video stream is stream-copied (not re-encoded), so this is fast.
func injectSilentAudio(path string) error {
	tmp := path + ".repair.tmp"
	cmd := exec.Command("ffmpeg",
		"-i", path,
		"-f", "lavfi", "-i", "anullsrc=r=44100:cl=stereo",
		"-c:v", "copy",
		"-c:a", "aac", "-b:a", "128k",
		"-map", "0:v:0", "-map", "1:a:0",
		"-shortest",
		"-y", tmp,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("inject silent audio: %w\n%s", err, strings.TrimSpace(stderr.String()))
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace with repaired file: %w", err)
	}
	return nil
}

// remuxContainer re-muxes the file with stream copy, which can resolve some
// container-level issues (e.g. corrupt moov atom, wrong duration metadata)
// without re-encoding any media data.
func remuxContainer(path string) error {
	tmp := path + ".repair.tmp"
	cmd := exec.Command("ffmpeg",
		"-i", path,
		"-c", "copy",
		"-y", tmp,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("remux: %w\n%s", err, strings.TrimSpace(stderr.String()))
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace with remuxed file: %w", err)
	}
	return nil
}

// parseCreationTime handles various ffprobe timestamp formats.
func parseCreationTime(s string) time.Time {
	s = strings.TrimSpace(s)
	formats := []string{
		"2006-01-02T15:04:05.000000Z",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
