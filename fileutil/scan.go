package fileutil

import (
	"os"
	"path/filepath"
	"strings"
)

// VideoExtensions is the set of recognized video file extensions.
var VideoExtensions = map[string]bool{
	".mov":  true,
	".mp4":  true,
	".avi":  true,
	".mkv":  true,
	".webm": true,
	".m4v":  true,
}

// ScanDir returns all video files in a directory (non-recursive).
// If ext is non-empty, only files matching that extension are returned.
func ScanDir(dir string, ext string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		fileExt := strings.ToLower(filepath.Ext(name))

		if ext != "" {
			// Filter by specific extension
			target := ext
			if !strings.HasPrefix(target, ".") {
				target = "." + target
			}
			if strings.EqualFold(fileExt, target) {
				files = append(files, filepath.Join(dir, name))
			}
		} else {
			// Match all video extensions
			if VideoExtensions[fileExt] {
				files = append(files, filepath.Join(dir, name))
			}
		}
	}

	return files, nil
}

// IsVideoFile checks if a path has a recognized video extension.
func IsVideoFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return VideoExtensions[ext]
}

// MoveToProcessedFolder moves video files to a "processed" subfolder in their source directory.
// Returns error if any file cannot be moved.
func MoveToProcessedFolder(sourceDir string, filePaths []string) error {
	if len(filePaths) == 0 {
		return nil
	}

	// Create processed folder in source directory
	processedDir := filepath.Join(sourceDir, "processed")
	if err := os.MkdirAll(processedDir, 0755); err != nil {
		return err
	}

	// Move each file to the processed folder
	for _, srcPath := range filePaths {
		fileName := filepath.Base(srcPath)
		dstPath := filepath.Join(processedDir, fileName)

		// Handle potential filename conflicts
		if _, err := os.Stat(dstPath); err == nil {
			// File exists, append timestamp
			ext := filepath.Ext(fileName)
			nameWithoutExt := strings.TrimSuffix(fileName, ext)
			dstPath = filepath.Join(processedDir, nameWithoutExt+"_"+filepath.Base(srcPath)+ext)
		}

		if err := os.Rename(srcPath, dstPath); err != nil {
			return err
		}
	}

	return nil
}
