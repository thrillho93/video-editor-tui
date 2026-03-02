package fileutil

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"vid-tui/model"
)

// localTempDir returns the best directory for large temporary files.
// Prefers /var/tmp (real disk, survives reboots) over /tmp which on many
// Linux systems is a tmpfs backed by RAM — a problem for large video files.
func localTempDir() string {
	// /var/tmp is on real disk on most Linux systems
	if info, err := os.Stat("/var/tmp"); err == nil && info.IsDir() {
		var st syscall.Statfs_t
		if syscall.Statfs("/var/tmp", &st) == nil {
			// 0x01021994 = tmpfs magic; avoid tmpfs for large files
			if int64(st.Type) != 0x01021994 {
				return "/var/tmp"
			}
		}
	}
	return ""
}

// IsNetworkPath reports whether path resides on a network filesystem.
// It checks the filesystem magic number via statfs.
func IsNetworkPath(path string) bool {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		// Try parent directory (e.g. if the file doesn't exist yet)
		if err2 := syscall.Statfs(filepath.Dir(path), &st); err2 != nil {
			return false
		}
	}
	// Common network filesystem magic numbers on Linux
	switch int64(st.Type) {
	case 0x6969,      // NFS
		0xFF534D42,   // CIFS/SMB
		0xFE534D42,   // SMB2
		0x65735546,   // FUSE (sshfs, etc.)
		0x517B:       // SMB_SUPER_MAGIC
		return true
	}
	return false
}

// AnyNetworkFiles reports whether any file in the slice resides on a network filesystem.
func AnyNetworkFiles(files []*model.VideoFile) bool {
	for _, f := range files {
		if IsNetworkPath(f.Path) {
			return true
		}
	}
	return false
}

// PrepareLocalFiles copies input files from network shares to a temporary directory.
// Returns:
//   - localFiles: VideoFile slice with paths pointing to /tmp copies
//   - localOutput: the output path to use (inside /tmp)
//   - cleanup: call after ffmpeg; moves the output to its final destination, removes tmpDir
//
// If no files are on a network path, originals are returned unchanged with a no-op cleanup.
func PrepareLocalFiles(files []*model.VideoFile, outputPath string) (
	localFiles []*model.VideoFile,
	localOutput string,
	cleanup func() error,
	err error,
) {
	if !AnyNetworkFiles(files) {
		return files, outputPath, func() error { return nil }, nil
	}

	tmpDir, err := os.MkdirTemp(localTempDir(), "vid-local-*")
	if err != nil {
		return nil, "", nil, fmt.Errorf("create temp dir: %w", err)
	}

	localFiles = make([]*model.VideoFile, len(files))
	for i, f := range files {
		local := filepath.Join(tmpDir, filepath.Base(f.Path))
		if copyErr := copyFile(f.Path, local); copyErr != nil {
			os.RemoveAll(tmpDir)
			return nil, "", nil, fmt.Errorf("copy %s: %w", filepath.Base(f.Path), copyErr)
		}
		vf := *f
		vf.Path = local
		localFiles[i] = &vf
	}

	localOutput = filepath.Join(tmpDir, filepath.Base(outputPath))
	destOutput := outputPath

	cleanup = func() error {
		defer os.RemoveAll(tmpDir)
		if _, statErr := os.Stat(localOutput); os.IsNotExist(statErr) {
			return nil // no output produced (e.g. dry run)
		}
		return moveFile(localOutput, destOutput)
	}

	return localFiles, localOutput, cleanup, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func moveFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	// Try rename first; falls back to copy+delete across filesystems
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if err := copyFile(src, dst); err != nil {
		return err
	}
	return os.Remove(src)
}
