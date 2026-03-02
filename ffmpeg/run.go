package ffmpeg

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"vid-tui/model"
)

// ffmpegThreads returns the number of threads to pass to ffmpeg.
// Capped at 8: libx264 with veryslow/slow preset allocates large per-thread
// buffers and on high-core machines can exhaust RAM and trigger the OOM killer.
func ffmpegThreads() string {
	n := runtime.NumCPU() - 2
	if n < 1 {
		n = 1
	}
	if n > 8 {
		n = 8
	}
	return strconv.Itoa(n)
}

// RunWithProgress executes ffmpeg with -progress pipe:1 and reports progress
// as a fraction (0.0–1.0) on the returned channel.
// The error channel receives at most one value when the process completes.
func RunWithProgress(ctx context.Context, args []string, totalDuration float64) (<-chan float64, <-chan error) {
	progressCh := make(chan float64, 1)
	errCh := make(chan error, 1)

	go func() {
		defer close(progressCh)
		defer close(errCh)

		// Create a pipe for progress output
		pr, pw, err := os.Pipe()
		if err != nil {
			errCh <- fmt.Errorf("create pipe: %w", err)
			return
		}
		defer pr.Close()

		// Build final args with progress output to our pipe.
		// -threads limits CPU usage so the system stays responsive.
		fullArgs := append([]string{"-loglevel", "error", "-progress", "pipe:3", "-nostats", "-threads", ffmpegThreads()}, args...)

		var stderrBuf bytes.Buffer
		cmd := exec.CommandContext(ctx, "ffmpeg", fullArgs...)
		cmd.Stderr = &stderrBuf
		cmd.ExtraFiles = []*os.File{pw}

		if err := cmd.Start(); err != nil {
			pw.Close()
			errCh <- fmt.Errorf("start ffmpeg: %w", err)
			return
		}
		pw.Close()

		// Parse progress output in background
		scanner := bufio.NewScanner(pr)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "out_time_us=") {
				val := strings.TrimPrefix(line, "out_time_us=")
				us, err := strconv.ParseFloat(val, 64)
				if err == nil && totalDuration > 0 {
					progress := (us / 1_000_000) / totalDuration
					if progress > 1.0 {
						progress = 1.0
					}
					// Non-blocking send, drop old values
					select {
					case progressCh <- progress:
					default:
						select {
						case <-progressCh:
						default:
						}
						select {
						case progressCh <- progress:
						default:
						}
					}
				}
			}
		}

		err = cmd.Wait()
		if err != nil && ctx.Err() == nil {
			msg := strings.TrimSpace(stderrBuf.String())
			if msg != "" {
				errCh <- fmt.Errorf("ffmpeg: %w\n%s", err, msg)
			} else {
				errCh <- fmt.Errorf("ffmpeg: %w", err)
			}
		} else if ctx.Err() != nil {
			errCh <- ctx.Err()
		}
	}()

	return progressCh, errCh
}

// RunSimple executes ffmpeg synchronously without progress reporting.
func RunSimple(args []string) error {
	fullArgs := append([]string{"-loglevel", "error", "-threads", ffmpegThreads()}, args...)
	cmd := exec.Command("ffmpeg", fullArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// BuildRotateArgs constructs ffmpeg arguments for rotating a single file.
func BuildRotateArgs(input, output string, rotation int, hw HWAccel, creationTime string) []string {
	var args []string
	args = append(args, "-noautorotate", "-i", input)

	if rf := RotationFilter(rotation); rf != "" {
		args = append(args, "-vf", rf)
	}

	args = append(args, EncoderParams(hw)...)
	args = append(args, "-fps_mode", "cfr") // convert VFR to CFR for stable playback
	args = append(args, "-pix_fmt", "yuv420p")
	args = append(args, AudioParams()...)
	args = append(args, "-avoid_negative_ts", "make_zero")
	args = append(args, "-movflags", "+faststart")

	if creationTime != "" {
		args = append(args, "-metadata", "creation_time="+creationTime)
	}

	args = append(args, "-y", output)
	return args
}

// BuildReencodeArgs constructs ffmpeg arguments for re-encoding without rotation.
func BuildReencodeArgs(input, output string, hw HWAccel, creationTime string) []string {
	var args []string
	args = append(args, "-noautorotate", "-i", input)
	args = append(args, EncoderParams(hw)...)
	args = append(args, "-fps_mode", "cfr") // convert VFR to CFR for stable playback
	args = append(args, "-pix_fmt", "yuv420p")
	args = append(args, AudioParams()...)

	if creationTime != "" {
		args = append(args, "-metadata", "creation_time="+creationTime)
	}

	args = append(args, "-avoid_negative_ts", "make_zero")
	args = append(args, "-movflags", "+faststart")
	args = append(args, "-y", output)
	return args
}

// BuildNormalizeArgs constructs ffmpeg args to re-encode a single file to a
// target resolution (with rotation and scale/pad applied). Used to normalize
// files one-at-a-time before stream-copy concat, keeping only one file in
// memory at a time instead of decoding all inputs simultaneously.
//
// hasAudio indicates whether this specific file has an audio stream.
// wantAudio indicates whether the output should contain audio (true when at
// least one file in the batch has audio). When wantAudio is true but hasAudio
// is false, a silent anullsrc track is synthesized so all normalized segments
// have a matching audio stream for the concat demuxer.
func BuildNormalizeArgs(input, output string, rotation, targetW, targetH int, hw HWAccel, hasAudio, wantAudio bool) []string {
	silentAudio := wantAudio && !hasAudio

	var args []string
	args = append(args, "-noautorotate", "-i", input)
	if silentAudio {
		args = append(args, "-f", "lavfi", "-i", "anullsrc=r=44100:cl=stereo")
	}

	var vfParts []string
	if rf := RotationFilter(rotation); rf != "" {
		vfParts = append(vfParts, rf)
	}
	vfParts = append(vfParts, ScalePadFilter(targetW, targetH))
	args = append(args, "-vf", strings.Join(vfParts, ","))

	args = append(args, EncoderParams(hw)...)
	args = append(args, "-fps_mode", "cfr") // convert VFR to CFR for stable playback
	args = append(args, "-pix_fmt", "yuv420p")

	if wantAudio {
		args = append(args, AudioParams()...)
	}
	if silentAudio {
		// Explicit maps required when mixing a real video input with a lavfi source.
		// -shortest stops at end of the (finite) video stream; anullsrc is infinite.
		args = append(args, "-map", "0:v:0", "-map", "1:a:0", "-shortest")
	}

	// Shift timestamps so DTS starts at 0, removing the MP4 edit list that
	// libx264/nvenc write for B-frame delay. Without this, the concat demuxer
	// mishandles the per-segment edit list offsets and produces timestamp
	// discontinuities (audio reset to 0) at segment joins during stream-copy concat.
	args = append(args, "-avoid_negative_ts", "make_zero")
	args = append(args, "-movflags", "+faststart")
	args = append(args, "-y", output)
	return args
}

// BuildConcatArgs constructs ffmpeg args for the concat filter path.
func BuildConcatArgs(concat *ConcatResult, output string, hw HWAccel) []string {
	var args []string

	for _, input := range concat.Inputs {
		args = append(args, "-noautorotate", "-i", input)
	}

	args = append(args, "-filter_complex", concat.FilterGraph)
	args = append(args, concat.MapArgs...)
	args = append(args, EncoderParams(hw)...)
	args = append(args, "-pix_fmt", "yuv420p", "-level", "5.1")

	if concat.NeedsAudio {
		args = append(args, AudioParams()...)
	}

	args = append(args, "-movflags", "+faststart")
	args = append(args, "-y", output)
	return args
}

// TotalDuration calculates the sum of all file durations.
func TotalDuration(files []*model.VideoFile) float64 {
	var total float64
	for _, f := range files {
		total += f.Duration
	}
	return total
}

// GetCreationTime extracts the creation_time metadata from a file.
func GetCreationTime(path string) string {
	out, err := exec.Command("ffprobe",
		"-v", "error",
		"-show_entries", "format_tags=creation_time",
		"-of", "default=nw=1:nk=1",
		path,
	).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
