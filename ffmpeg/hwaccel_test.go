package ffmpeg

import "testing"

func TestEncoderParams(t *testing.T) {
	tests := []struct {
		hw   HWAccel
		want []string
	}{
		{HWAccelNVENC, []string{"-c:v", "h264_nvenc", "-preset", "p7", "-cq", "15"}},
		{HWAccelQSV, []string{"-c:v", "h264_qsv", "-preset", "slow", "-global_quality", "18"}},
		{HWAccelVideoToolbox, []string{"-c:v", "h264_videotoolbox", "-q:v", "65"}},
		{HWAccelSoftware, []string{"-c:v", "libx264", "-crf", "18", "-preset", "slow"}},
	}

	for _, tt := range tests {
		got := EncoderParams(tt.hw)
		if len(got) != len(tt.want) {
			t.Errorf("EncoderParams(%s) len = %d, want %d", tt.hw, len(got), len(tt.want))
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("EncoderParams(%s)[%d] = %q, want %q", tt.hw, i, got[i], tt.want[i])
			}
		}
	}
}

func TestAudioParams(t *testing.T) {
	got := AudioParams()
	if len(got) != 4 || got[0] != "-c:a" || got[1] != "aac" || got[2] != "-b:a" || got[3] != "320k" {
		t.Errorf("AudioParams() = %v, want [-c:a aac -b:a 320k]", got)
	}
}

func TestHWAccelString(t *testing.T) {
	tests := []struct {
		hw   HWAccel
		want string
	}{
		{HWAccelNVENC, "NVIDIA NVENC"},
		{HWAccelQSV, "Intel QuickSync"},
		{HWAccelVideoToolbox, "VideoToolbox"},
		{HWAccelSoftware, "Software (libx264)"},
	}

	for _, tt := range tests {
		if got := tt.hw.String(); got != tt.want {
			t.Errorf("%q.String() = %q, want %q", tt.hw, got, tt.want)
		}
	}
}
