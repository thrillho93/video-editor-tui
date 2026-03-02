package ffmpeg

import (
	"os/exec"
	"runtime"
	"strings"
)

// HWAccel represents the detected hardware acceleration type.
type HWAccel string

const (
	HWAccelNVENC         HWAccel = "nvenc"
	HWAccelQSV           HWAccel = "qsv"
	HWAccelVideoToolbox  HWAccel = "videotoolbox"
	HWAccelSoftware      HWAccel = "software"
)

func (h HWAccel) String() string {
	switch h {
	case HWAccelNVENC:
		return "NVIDIA NVENC"
	case HWAccelQSV:
		return "Intel QuickSync"
	case HWAccelVideoToolbox:
		return "VideoToolbox"
	default:
		return "Software (libx264)"
	}
}

// DetectHWAccel checks for available hardware encoders.
func DetectHWAccel() HWAccel {
	encoders := getAvailableEncoders()

	// Check NVIDIA NVENC
	if hasNVIDIA() && strings.Contains(encoders, "h264_nvenc") && probeNVENC() {
		return HWAccelNVENC
	}

	// Check Intel QuickSync — probe an actual encode to confirm the hardware
	// and drivers are functional. hasDRI() alone is insufficient: /dev/dri
	// exists for any GPU (AMD, NVIDIA, etc.), but h264_qsv requires Intel
	// hardware + oneVPL/libmfx. Without a probe, a missing or incompatible
	// QSV setup causes ffmpeg to exit with AVERROR(EINVAL) = code 234.
	if strings.Contains(encoders, "h264_qsv") && probeQSV() {
		return HWAccelQSV
	}

	// Check macOS VideoToolbox
	if runtime.GOOS == "darwin" && strings.Contains(encoders, "h264_videotoolbox") {
		return HWAccelVideoToolbox
	}

	return HWAccelSoftware
}

// EncoderParams returns the ffmpeg encoder arguments for the given HW accel type.
func EncoderParams(hw HWAccel) []string {
	switch hw {
	case HWAccelNVENC:
		return []string{"-c:v", "h264_nvenc", "-preset", "p7", "-cq", "15"}
	case HWAccelQSV:
		return []string{"-c:v", "h264_qsv", "-preset", "slow", "-global_quality", "18"}
	case HWAccelVideoToolbox:
		return []string{"-c:v", "h264_videotoolbox", "-q:v", "65"}
	default:
		return []string{"-c:v", "libx264", "-crf", "18", "-preset", "slow"}
	}
}

// AudioParams returns the standard audio encoding parameters.
func AudioParams() []string {
	return []string{"-c:a", "aac", "-b:a", "320k"}
}

func getAvailableEncoders() string {
	out, err := exec.Command("ffmpeg", "-hide_banner", "-encoders").Output()
	if err != nil {
		return ""
	}
	return string(out)
}

func hasNVIDIA() bool {
	cmd := exec.Command("nvidia-smi")
	return cmd.Run() == nil
}

// probeNVENC verifies h264_nvenc actually works with the preset and options
// this tool uses. NVENC has minimum frame-size requirements that vary by
// preset (p7 needs at least ~145×17); 320×240 is safely above all limits.
func probeNVENC() bool {
	cmd := exec.Command("ffmpeg", "-hide_banner",
		"-threads", ffmpegThreads(),
		"-f", "lavfi", "-i", "color=c=black:s=320x240:d=0.1:r=30",
		"-vf", "scale=320:240,setsar=1",
		"-c:v", "h264_nvenc", "-preset", "p7", "-cq", "15",
		"-fps_mode", "cfr",
		"-pix_fmt", "yuv420p",
		"-f", "null", "-",
	)
	return cmd.Run() == nil
}

// probeQSV does a minimal test encode to confirm h264_qsv is actually usable
// with the parameters this tool will pass. It returns false if the encoder
// can't be initialized (no Intel hardware, missing oneVPL/libmfx, invalid
// preset, etc.), allowing graceful fallback to software encoding.
func probeQSV() bool {
	// Mirror the exact args RunWithProgress prepends, plus a representative
	// filter chain and pix_fmt, so the probe catches any argument that would
	// cause AVERROR(EINVAL) (exit 234) in the real encode.
	cmd := exec.Command("ffmpeg", "-hide_banner",
		"-threads", ffmpegThreads(),
		"-f", "lavfi", "-i", "color=c=black:s=64x64:d=0.1",
		"-vf", "scale=64:64,setsar=1",
		"-c:v", "h264_qsv", "-preset", "slow", "-global_quality", "18",
		"-fps_mode", "cfr",
		"-pix_fmt", "yuv420p",
		"-f", "null", "-",
	)
	return cmd.Run() == nil
}
