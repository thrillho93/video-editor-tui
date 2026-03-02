package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"vid-tui/ffmpeg"
	"vid-tui/fileutil"
	"vid-tui/model"
)

type cliRunner struct {
	output   string
	ext      string
	rotation int
	preview  bool
	reencode bool
	rename   bool
	dryrun   bool
	args     []string
}

func (c *cliRunner) run() error {
	// Build file list
	var paths []string
	for _, arg := range c.args {
		info, err := os.Stat(arg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: '%s' not found, skipping\n", arg)
			continue
		}
		if info.IsDir() {
			found, err := fileutil.ScanDir(arg, c.ext)
			if err != nil {
				return fmt.Errorf("scan %s: %w", arg, err)
			}
			paths = append(paths, found...)
		} else {
			paths = append(paths, arg)
		}
	}

	if len(paths) == 0 {
		return fmt.Errorf("no video files found")
	}

	// Probe all files
	fmt.Printf("Found %d file(s), reading metadata...\n", len(paths))
	var files []*model.VideoFile
	for _, p := range paths {
		vf, err := ffmpeg.ProbeFile(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to probe %s: %v\n", p, err)
			continue
		}
		files = append(files, vf)
	}

	if len(files) == 0 {
		return fmt.Errorf("no valid video files found")
	}

	// Sort by creation time
	fileutil.SortByCreationTime(files)

	// Detect HW acceleration
	hw := ffmpeg.DetectHWAccel()
	fmt.Printf("Encoder: %s\n\n", hw)

	// Print chronological order
	fmt.Println("Files in chronological order:")
	fmt.Println("-----------------------------")
	for i, f := range files {
		ts := "(no metadata)"
		if !f.CreationTime.IsZero() {
			ts = f.CreationTime.Format("2006-01-02T15:04:05")
		}
		fmt.Printf("  %d. [%s] %s\n", i+1, ts, filepath.Base(f.Path))
	}
	fmt.Println()

	// Handle rename
	if c.rename {
		return c.doRename(files)
	}

	// Preview rotation if requested
	if c.preview && c.rotation != 0 {
		fmt.Printf("Previewing %d rotation on: %s\n", c.rotation, filepath.Base(files[0].Path))
		cmd := ffmpeg.PreviewCmd(files[0].Path, c.rotation)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		_ = cmd.Run()
	}

	// Single file rotation
	if len(files) == 1 && c.rotation != 0 {
		return c.rotateSingle(files[0], hw)
	}

	// Multi-file concatenation
	return c.concat(files, hw)
}

func (c *cliRunner) doRename(files []*model.VideoFile) error {
	pairs := fileutil.BuildRenamePairs(files)
	if c.dryrun {
		for _, p := range pairs {
			fmt.Printf("  %s -> %s\n", filepath.Base(p.Src), filepath.Base(p.Dst))
		}
		fmt.Println("(Dry run - no files renamed)")
		return nil
	}
	if err := fileutil.ExecuteRename(pairs); err != nil {
		return err
	}
	for _, p := range pairs {
		fmt.Printf("  %s -> %s\n", filepath.Base(p.Src), filepath.Base(p.Dst))
	}
	fmt.Println("Done.")
	return nil
}

func (c *cliRunner) rotateSingle(vf *model.VideoFile, hw ffmpeg.HWAccel) error {
	if c.dryrun {
		fmt.Printf("Would rotate %s by %d -> %s\n", filepath.Base(vf.Path), c.rotation, c.output)
		return nil
	}

	localFiles, localOutput, cleanup, err := fileutil.PrepareLocalFiles([]*model.VideoFile{vf}, c.output)
	if err != nil {
		return fmt.Errorf("prepare local files: %w", err)
	}

	ctime := ffmpeg.GetCreationTime(vf.Path)
	fmt.Printf("Rotating %s by %d -> %s\n", filepath.Base(vf.Path), c.rotation, c.output)

	args := ffmpeg.BuildRotateArgs(localFiles[0].Path, localOutput, c.rotation, hw, ctime)
	if err := ffmpeg.RunSimple(args); err != nil {
		return err
	}

	if err := cleanup(); err != nil {
		return fmt.Errorf("finalize output: %w", err)
	}

	if err := ffmpeg.VerifyAndRepair(c.output, vf.HasAudio); err != nil {
		return err
	}

	fmt.Printf("\nDone: %s\n", c.output)
	return nil
}

func (c *cliRunner) concat(files []*model.VideoFile, hw ffmpeg.HWAccel) error {
	if c.dryrun {
		fmt.Println("(Dry run - no output created)")
		return nil
	}

	cfg := model.NewWizardConfig()
	cfg.Files = files
	cfg.Rotation = c.rotation
	cfg.Reencode = c.reencode

	needReencode := c.reencode || c.rotation != 0
	if !needReencode {
		need, reason := ffmpeg.NeedsReencode(files)
		if need {
			fmt.Printf("Warning: %s, re-encoding required.\n", reason)
			needReencode = true
		}
	}

	anyHasAudio := false
	for _, f := range files {
		if f.HasAudio {
			anyHasAudio = true
			break
		}
	}

	localFiles, localOutput, cleanup, err := fileutil.PrepareLocalFiles(files, c.output)
	if err != nil {
		return fmt.Errorf("prepare local files: %w", err)
	}

	fmt.Printf("Concatenating to: %s\n", c.output)

	if needReencode {
		if c.rotation != 0 {
			fmt.Printf("(Rotating %d)\n", c.rotation)
		}
		fmt.Println("(Re-encoding)")

		// Normalize files one at a time to avoid decoding all inputs
		// simultaneously via filter_complex, which can consume 20-30 GB RAM
		// for multi-file 4K encodes.
		tmpDir, err := os.MkdirTemp("", "vid-norm-*")
		if err != nil {
			return fmt.Errorf("create temp dir: %w", err)
		}
		defer os.RemoveAll(tmpDir)

		targetW, targetH := ffmpeg.FirstEffectiveDimensions(localFiles, cfg)

		var normPaths []string
		for i, f := range localFiles {
			rot := cfg.Rotation
			if cfg.PerVideoRotate {
				rot = cfg.FileRotations[f.Path]
			}
			normOut := filepath.Join(tmpDir, fmt.Sprintf("norm_%03d.mp4", i))
			fmt.Printf("\r  Normalizing %d/%d: %s", i+1, len(localFiles), filepath.Base(f.Path))

			args := ffmpeg.BuildNormalizeArgs(f.Path, normOut, rot, targetW, targetH, hw, f.HasAudio, anyHasAudio)
			if err := ffmpeg.RunSimple(args); err != nil {
				return fmt.Errorf("normalize %s: %w", filepath.Base(f.Path), err)
			}
			normPaths = append(normPaths, normOut)
		}
		fmt.Println()

		// Stream copy the normalized files together
		listFile, err := os.CreateTemp(tmpDir, "concat-*.txt")
		if err != nil {
			return err
		}
		for _, p := range normPaths {
			escaped := strings.ReplaceAll(p, "'", "'\\''")
			fmt.Fprintf(listFile, "file '%s'\n", escaped)
		}
		listFile.Close()

		args := ffmpeg.BuildStreamCopyArgs(listFile.Name(), localOutput)
		if err := ffmpeg.RunSimple(args); err != nil {
			return err
		}
	} else {
		// Stream copy via concat demuxer
		tmpFile, err := os.CreateTemp("", "vid-concat-*.txt")
		if err != nil {
			return err
		}
		defer os.Remove(tmpFile.Name())

		for _, f := range localFiles {
			escaped := strings.ReplaceAll(f.Path, "'", "'\\''")
			fmt.Fprintf(tmpFile, "file '%s'\n", escaped)
		}
		tmpFile.Close()

		args := ffmpeg.BuildStreamCopyArgs(tmpFile.Name(), localOutput)
		if err := ffmpeg.RunSimple(args); err != nil {
			return err
		}
	}

	if err := cleanup(); err != nil {
		return fmt.Errorf("finalize output: %w", err)
	}

	if err := ffmpeg.VerifyAndRepair(c.output, anyHasAudio); err != nil {
		return err
	}

	fmt.Printf("\nDone: %s\n", c.output)
	return nil
}
