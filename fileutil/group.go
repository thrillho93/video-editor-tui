package fileutil

import (
	"fmt"
	"time"

	"vid-tui/model"
)

// DateGroup represents a group of videos taken around the same time.
type DateGroup struct {
	Label string          // e.g., "2024-02-15 14:30"
	Files []*model.VideoFile
}

// GroupByDate groups sorted video files into 10-minute time windows.
// Files without creation_time metadata are grouped under "(no metadata)".
func GroupByDate(files []*model.VideoFile) []DateGroup {
	if len(files) == 0 {
		return nil
	}

	var groups []DateGroup
	var current *DateGroup

	for _, f := range files {
		label := bucketLabel(f.CreationTime)
		if current == nil || current.Label != label {
			if current != nil {
				groups = append(groups, *current)
			}
			current = &DateGroup{Label: label}
		}
		current.Files = append(current.Files, f)
	}
	if current != nil {
		groups = append(groups, *current)
	}

	return groups
}

// bucketLabel returns the 10-minute bucket label for a timestamp.
func bucketLabel(t time.Time) string {
	if t.IsZero() {
		return "(no metadata)"
	}
	// Round minutes down to nearest 10
	local := t.Local()
	roundedMin := (local.Minute() / 10) * 10
	return fmt.Sprintf("%s %02d:%02d",
		local.Format("2006-01-02"),
		local.Hour(),
		roundedMin,
	)
}
