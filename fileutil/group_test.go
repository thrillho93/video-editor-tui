package fileutil

import (
	"testing"
	"time"

	"vid-tui/model"
)

func TestGroupByDate(t *testing.T) {
	loc := time.Local
	files := []*model.VideoFile{
		{Path: "/a.mp4", CreationTime: time.Date(2024, 2, 15, 14, 31, 0, 0, loc)},
		{Path: "/b.mp4", CreationTime: time.Date(2024, 2, 15, 14, 35, 0, 0, loc)},
		{Path: "/c.mp4", CreationTime: time.Date(2024, 2, 15, 15, 5, 0, 0, loc)},
		{Path: "/d.mp4", CreationTime: time.Date(2024, 2, 16, 10, 0, 0, 0, loc)},
	}

	groups := GroupByDate(files)

	if len(groups) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(groups))
	}

	// First group: 14:30 bucket (minutes 31 and 35 both round to 30)
	if len(groups[0].Files) != 2 {
		t.Errorf("group 0: expected 2 files, got %d", len(groups[0].Files))
	}
	if groups[0].Label != "2024-02-15 14:30" {
		t.Errorf("group 0 label = %q, want %q", groups[0].Label, "2024-02-15 14:30")
	}

	// Second group: 15:00 bucket
	if len(groups[1].Files) != 1 {
		t.Errorf("group 1: expected 1 file, got %d", len(groups[1].Files))
	}

	// Third group: different day
	if len(groups[2].Files) != 1 {
		t.Errorf("group 2: expected 1 file, got %d", len(groups[2].Files))
	}
}

func TestGroupByDate_NoMetadata(t *testing.T) {
	files := []*model.VideoFile{
		{Path: "/a.mp4"},
		{Path: "/b.mp4"},
	}

	groups := GroupByDate(files)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if groups[0].Label != "(no metadata)" {
		t.Errorf("label = %q, want %q", groups[0].Label, "(no metadata)")
	}
}

func TestGroupByDate_Empty(t *testing.T) {
	groups := GroupByDate(nil)
	if groups != nil {
		t.Errorf("expected nil, got %v", groups)
	}
}
