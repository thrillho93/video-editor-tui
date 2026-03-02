package fileutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"vid-tui/model"
)

// RenamePair describes a source → destination rename.
type RenamePair struct {
	Src string
	Dst string
}

// BuildRenamePairs creates sequential rename pairs from sorted files.
// Files are renamed to 1.EXT, 2.EXT, etc., preserving their original extension.
func BuildRenamePairs(files []*model.VideoFile) []RenamePair {
	pairs := make([]RenamePair, len(files))
	for i, f := range files {
		dir := filepath.Dir(f.Path)
		ext := strings.ToUpper(filepath.Ext(f.Path))
		if ext == "" {
			ext = ".MOV"
		}
		pairs[i] = RenamePair{
			Src: f.Path,
			Dst: filepath.Join(dir, fmt.Sprintf("%d%s", i+1, ext)),
		}
	}
	return pairs
}

// ExecuteRename performs the rename using a temp-name intermediate step
// to avoid collisions (e.g., renaming 2.MOV→1.MOV when 1.MOV already exists).
func ExecuteRename(pairs []RenamePair) error {
	// First pass: rename to temp names
	type tmpPair struct {
		tmp string
		dst string
	}
	var tmps []tmpPair

	for _, p := range pairs {
		tmp := p.Src + ".vid_rename_tmp"
		if err := os.Rename(p.Src, tmp); err != nil {
			return fmt.Errorf("rename %s → tmp: %w", filepath.Base(p.Src), err)
		}
		tmps = append(tmps, tmpPair{tmp: tmp, dst: p.Dst})
	}

	// Second pass: rename from temp to final
	for _, t := range tmps {
		if err := os.Rename(t.tmp, t.dst); err != nil {
			return fmt.Errorf("rename tmp → %s: %w", filepath.Base(t.dst), err)
		}
	}

	return nil
}
