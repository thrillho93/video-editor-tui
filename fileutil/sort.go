package fileutil

import (
	"sort"

	"vid-tui/model"
)

// SortByCreationTime sorts video files by their creation time (ascending).
func SortByCreationTime(files []*model.VideoFile) {
	sort.Slice(files, func(i, j int) bool {
		ti := files[i].CreationTime
		tj := files[j].CreationTime
		if ti.IsZero() && tj.IsZero() {
			return files[i].Path < files[j].Path
		}
		if ti.IsZero() {
			return false // files without timestamps sort last
		}
		if tj.IsZero() {
			return true
		}
		return ti.Before(tj)
	})
}
