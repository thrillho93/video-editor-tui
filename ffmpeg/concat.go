package ffmpeg

import (
	"fmt"
	"strings"

	"vid-tui/model"
)

// ConcatResult holds the constructed filter graph and input arguments.
type ConcatResult struct {
	Inputs       []string // -i args (file paths)
	FilterGraph  string
	MapArgs      []string // -map args
	NeedsAudio   bool
}

// FirstEffectiveDimensions returns the effective width and height of the first
// file after applying its configured rotation. Used as the target resolution
// when normalizing clips one-at-a-time.
func FirstEffectiveDimensions(files []*model.VideoFile, cfg *model.WizardConfig) (int, int) {
	if len(files) == 0 {
		return 0, 0
	}
	f := files[0]
	rot := cfg.Rotation
	if cfg.PerVideoRotate {
		rot = cfg.FileRotations[f.Path]
	}
	return EffectiveDimensions(f.Width, f.Height, rot)
}

// BuildConcatFilter constructs the complex filter graph for concatenating videos.
// It handles mixed resolutions, codecs, per-file rotation, and missing audio.
func BuildConcatFilter(files []*model.VideoFile, cfg *model.WizardConfig) *ConcatResult {
	// Use the first file's effective dimensions as the target resolution.
	// All clips are expected to be from the same source, so this avoids
	// upscaling. Computing max W and H independently would create a square
	// target (e.g. 1920x1920) when mixing landscape and portrait clips.
	var targetW, targetH int
	allSameRes := true
	firstRes := ""
	anyHasAudio := false

	for _, f := range files {
		rot := cfg.Rotation
		if cfg.PerVideoRotate {
			rot = cfg.FileRotations[f.Path]
		}
		ew, eh := EffectiveDimensions(f.Width, f.Height, rot)
		if targetW == 0 {
			targetW, targetH = ew, eh
		}
		res := fmt.Sprintf("%dx%d", ew, eh)
		if firstRes == "" {
			firstRes = res
		} else if res != firstRes {
			allSameRes = false
		}
		if f.HasAudio {
			anyHasAudio = true
		}
	}

	var filterParts []string
	var inputs []string

	for i, f := range files {
		inputs = append(inputs, f.Path)

		// Build per-input video filter chain
		var vfChain []string

		// Rotation
		rot := cfg.Rotation
		if cfg.PerVideoRotate {
			rot = cfg.FileRotations[f.Path]
		}
		if rf := RotationFilter(rot); rf != "" {
			vfChain = append(vfChain, rf)
		}

		// Scaling if resolutions differ
		if !allSameRes {
			vfChain = append(vfChain, ScalePadFilter(targetW, targetH))
		} else {
			vfChain = append(vfChain, "setsar=1")
		}

		filterParts = append(filterParts, fmt.Sprintf("[%d:v:0]%s[v%d]", i, strings.Join(vfChain, ","), i))

		// Audio
		if anyHasAudio {
			if f.HasAudio {
				filterParts = append(filterParts, fmt.Sprintf("[%d:a:0]acopy[a%d]", i, i))
			} else {
				filterParts = append(filterParts, fmt.Sprintf("anullsrc=r=44100:cl=stereo[a%d]", i))
			}
		}
	}

	// Build concat input string
	var concatInputs strings.Builder
	for i := range files {
		fmt.Fprintf(&concatInputs, "[v%d]", i)
		if anyHasAudio {
			fmt.Fprintf(&concatInputs, "[a%d]", i)
		}
	}

	audioStreams := 0
	if anyHasAudio {
		audioStreams = 1
	}

	concatFilter := fmt.Sprintf("%sconcat=n=%d:v=1:a=%d",
		concatInputs.String(), len(files), audioStreams)

	var mapArgs []string
	if anyHasAudio {
		concatFilter += "[outv][outa]"
		mapArgs = []string{"-map", "[outv]", "-map", "[outa]"}
	} else {
		concatFilter += "[outv]"
		mapArgs = []string{"-map", "[outv]"}
	}

	fullFilter := strings.Join(filterParts, ";") + ";" + concatFilter

	return &ConcatResult{
		Inputs:      inputs,
		FilterGraph: fullFilter,
		MapArgs:     mapArgs,
		NeedsAudio:  anyHasAudio,
	}
}

// BuildStreamCopyArgs returns ffmpeg args for simple stream copy concat.
// This is used when all files have matching resolution and codec.
func BuildStreamCopyArgs(concatFile, output string) []string {
	return []string{
		"-f", "concat",
		"-safe", "0",
		"-i", concatFile,
		"-c", "copy",
		"-avoid_negative_ts", "make_zero", // fix B-frame DTS underflow at segment joins
		"-movflags", "+faststart",
		"-y", output,
	}
}

// NeedsReencode checks if the file set requires re-encoding for concat.
// Returns true and a reason if re-encoding is needed.
func NeedsReencode(files []*model.VideoFile) (bool, string) {
	if len(files) < 2 {
		return false, ""
	}

	firstRes := fmt.Sprintf("%dx%d", files[0].Width, files[0].Height)
	firstCodec := files[0].Codec

	for _, f := range files {
		if f.IsVFR {
			return true, "Variable frame rate content detected"
		}
	}

	for _, f := range files[1:] {
		res := fmt.Sprintf("%dx%d", f.Width, f.Height)
		if res != firstRes {
			return true, "Resolution mismatch detected"
		}
		if f.Codec != firstCodec {
			return true, fmt.Sprintf("Mixed codecs detected (%s + %s)", firstCodec, f.Codec)
		}
	}

	return false, ""
}
