package fileutil

import (
	"testing"
	"time"

	"vid-tui/model"
)

func TestSortByCreationTime(t *testing.T) {
	now := time.Now()
	files := []*model.VideoFile{
		{Path: "/c.mp4", CreationTime: now.Add(2 * time.Hour)},
		{Path: "/a.mp4", CreationTime: now},
		{Path: "/b.mp4", CreationTime: now.Add(1 * time.Hour)},
	}

	SortByCreationTime(files)

	if files[0].Path != "/a.mp4" || files[1].Path != "/b.mp4" || files[2].Path != "/c.mp4" {
		t.Errorf("sort order wrong: %s, %s, %s", files[0].Path, files[1].Path, files[2].Path)
	}
}

func TestSortByCreationTime_NoTimestamp(t *testing.T) {
	now := time.Now()
	files := []*model.VideoFile{
		{Path: "/z.mp4"}, // no timestamp → sorts last
		{Path: "/a.mp4", CreationTime: now},
	}

	SortByCreationTime(files)

	if files[0].Path != "/a.mp4" {
		t.Errorf("file with timestamp should sort first, got %s", files[0].Path)
	}
	if files[1].Path != "/z.mp4" {
		t.Errorf("file without timestamp should sort last, got %s", files[1].Path)
	}
}
