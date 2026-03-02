package ffmpeg

import "testing"

func TestRotationFilter(t *testing.T) {
	tests := []struct {
		degrees int
		want    string
	}{
		{0, ""},
		{90, "transpose=1"},
		{180, "hflip,vflip"},
		{270, "transpose=2"},
		{45, ""},
	}

	for _, tt := range tests {
		got := RotationFilter(tt.degrees)
		if got != tt.want {
			t.Errorf("RotationFilter(%d) = %q, want %q", tt.degrees, got, tt.want)
		}
	}
}

func TestEffectiveDimensions(t *testing.T) {
	tests := []struct {
		w, h, deg  int
		wantW, wantH int
	}{
		{1920, 1080, 0, 1920, 1080},
		{1920, 1080, 90, 1080, 1920},
		{1920, 1080, 180, 1920, 1080},
		{1920, 1080, 270, 1080, 1920},
	}

	for _, tt := range tests {
		gotW, gotH := EffectiveDimensions(tt.w, tt.h, tt.deg)
		if gotW != tt.wantW || gotH != tt.wantH {
			t.Errorf("EffectiveDimensions(%d, %d, %d) = (%d, %d), want (%d, %d)",
				tt.w, tt.h, tt.deg, gotW, gotH, tt.wantW, tt.wantH)
		}
	}
}

func TestScalePadFilter(t *testing.T) {
	got := ScalePadFilter(1920, 1080)
	want := "scale=1920:1080:force_original_aspect_ratio=decrease,pad=1920:1080:(ow-iw)/2:(oh-ih)/2:black,setsar=1"
	if got != want {
		t.Errorf("ScalePadFilter(1920, 1080) = %q, want %q", got, want)
	}
}
