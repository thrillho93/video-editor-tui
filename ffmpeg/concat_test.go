package ffmpeg

import (
	"strings"
	"testing"

	"vid-tui/model"
)

func TestBuildConcatFilter_Basic(t *testing.T) {
	files := []*model.VideoFile{
		{Path: "/a.mp4", Width: 1920, Height: 1080, HasAudio: true, Codec: "h264"},
		{Path: "/b.mp4", Width: 1920, Height: 1080, HasAudio: true, Codec: "h264"},
	}
	cfg := &model.WizardConfig{FileRotations: map[string]int{}}

	result := BuildConcatFilter(files, cfg)

	if len(result.Inputs) != 2 {
		t.Errorf("expected 2 inputs, got %d", len(result.Inputs))
	}

	// Should contain concat filter
	if !strings.Contains(result.FilterGraph, "concat=n=2:v=1:a=1") {
		t.Errorf("filter graph missing concat: %s", result.FilterGraph)
	}

	// Should have audio mapping
	if !result.NeedsAudio {
		t.Error("expected NeedsAudio=true")
	}
}

func TestBuildConcatFilter_MixedRes(t *testing.T) {
	files := []*model.VideoFile{
		{Path: "/a.mp4", Width: 1920, Height: 1080, HasAudio: true},
		{Path: "/b.mp4", Width: 1280, Height: 720, HasAudio: true},
	}
	cfg := &model.WizardConfig{FileRotations: map[string]int{}}

	result := BuildConcatFilter(files, cfg)

	// Should contain scale+pad filter for mixed resolutions
	if !strings.Contains(result.FilterGraph, "scale=1920:1080") {
		t.Errorf("filter graph missing scale filter: %s", result.FilterGraph)
	}
}

func TestBuildConcatFilter_NoAudio(t *testing.T) {
	files := []*model.VideoFile{
		{Path: "/a.mp4", Width: 1920, Height: 1080, HasAudio: false},
		{Path: "/b.mp4", Width: 1920, Height: 1080, HasAudio: false},
	}
	cfg := &model.WizardConfig{FileRotations: map[string]int{}}

	result := BuildConcatFilter(files, cfg)

	if result.NeedsAudio {
		t.Error("expected NeedsAudio=false for files without audio")
	}
	if !strings.Contains(result.FilterGraph, "a=0") {
		t.Errorf("expected a=0 in filter: %s", result.FilterGraph)
	}
}

func TestBuildConcatFilter_WithRotation(t *testing.T) {
	files := []*model.VideoFile{
		{Path: "/a.mp4", Width: 1920, Height: 1080, HasAudio: true},
	}
	cfg := &model.WizardConfig{
		Rotation:      90,
		FileRotations: map[string]int{},
	}

	result := BuildConcatFilter(files, cfg)

	if !strings.Contains(result.FilterGraph, "transpose=1") {
		t.Errorf("filter graph missing rotation filter: %s", result.FilterGraph)
	}
}

func TestBuildConcatFilter_MixedAudio(t *testing.T) {
	files := []*model.VideoFile{
		{Path: "/a.mp4", Width: 1920, Height: 1080, HasAudio: true},
		{Path: "/b.mp4", Width: 1920, Height: 1080, HasAudio: false},
	}
	cfg := &model.WizardConfig{FileRotations: map[string]int{}}

	result := BuildConcatFilter(files, cfg)

	// Should generate anullsrc for the file without audio
	if !strings.Contains(result.FilterGraph, "anullsrc") {
		t.Errorf("filter graph missing anullsrc: %s", result.FilterGraph)
	}
	if !result.NeedsAudio {
		t.Error("expected NeedsAudio=true when at least one file has audio")
	}
}

func TestBuildNormalizeArgs_AudioHandling(t *testing.T) {
	tests := []struct {
		name          string
		hasAudio      bool
		wantAudio     bool
		wantAnullsrc  bool
		wantAudioArgs bool
		wantShortest  bool
	}{
		{
			name:          "file has audio, want audio",
			hasAudio:      true,
			wantAudio:     true,
			wantAnullsrc:  false,
			wantAudioArgs: true,
			wantShortest:  false,
		},
		{
			name:          "file has no audio, want audio (silent fill)",
			hasAudio:      false,
			wantAudio:     true,
			wantAnullsrc:  true,
			wantAudioArgs: true,
			wantShortest:  true,
		},
		{
			name:          "file has no audio, no audio wanted",
			hasAudio:      false,
			wantAudio:     false,
			wantAnullsrc:  false,
			wantAudioArgs: false,
			wantShortest:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := BuildNormalizeArgs("/in.mp4", "/out.mp4", 0, 1920, 1080, HWAccelSoftware, tt.hasAudio, tt.wantAudio)
			joined := strings.Join(args, " ")

			if tt.wantAnullsrc && !strings.Contains(joined, "anullsrc") {
				t.Error("expected anullsrc in args")
			}
			if !tt.wantAnullsrc && strings.Contains(joined, "anullsrc") {
				t.Error("unexpected anullsrc in args")
			}
			if tt.wantAudioArgs && !strings.Contains(joined, "-c:a") {
				t.Error("expected -c:a in args")
			}
			if !tt.wantAudioArgs && strings.Contains(joined, "-c:a") {
				t.Error("unexpected -c:a in args")
			}
			if tt.wantShortest && !strings.Contains(joined, "-shortest") {
				t.Error("expected -shortest in args")
			}
			if !tt.wantShortest && strings.Contains(joined, "-shortest") {
				t.Error("unexpected -shortest in args")
			}
			if !strings.Contains(joined, "-avoid_negative_ts make_zero") {
				t.Error("expected -avoid_negative_ts make_zero in args")
			}
		})
	}
}

func TestNeedsReencode(t *testing.T) {
	tests := []struct {
		name  string
		files []*model.VideoFile
		want  bool
	}{
		{
			"same everything",
			[]*model.VideoFile{
				{Width: 1920, Height: 1080, Codec: "h264"},
				{Width: 1920, Height: 1080, Codec: "h264"},
			},
			false,
		},
		{
			"different resolution",
			[]*model.VideoFile{
				{Width: 1920, Height: 1080, Codec: "h264"},
				{Width: 1280, Height: 720, Codec: "h264"},
			},
			true,
		},
		{
			"different codec",
			[]*model.VideoFile{
				{Width: 1920, Height: 1080, Codec: "h264"},
				{Width: 1920, Height: 1080, Codec: "hevc"},
			},
			true,
		},
		{
			"single file",
			[]*model.VideoFile{
				{Width: 1920, Height: 1080, Codec: "h264"},
			},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := NeedsReencode(tt.files)
			if got != tt.want {
				t.Errorf("NeedsReencode() = %v, want %v", got, tt.want)
			}
		})
	}
}
