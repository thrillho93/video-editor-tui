package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"vid-tui/ffmpeg"
	"vid-tui/fileutil"
	"vid-tui/model"
	"vid-tui/style"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// progressModel handles the encoding execution with progress bar.
type progressModel struct {
	config     *model.WizardConfig
	hwAccel    ffmpeg.HWAccel
	width      int
	bar        progress.Model
	spinner    spinner.Model
	progress      float64
	startTime     time.Time
	bestRemaining time.Duration
	done          bool
	err        error
	cancel     context.CancelFunc
	status     string
	progressCh <-chan float64
	errCh      <-chan error
}

func newProgressModel(cfg *model.WizardConfig, hw ffmpeg.HWAccel, width int) progressModel {
	bar := progress.New(progress.WithDefaultGradient())
	bar.Width = min(width-10, 60)

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(style.Purple)

	return progressModel{
		config:  cfg,
		hwAccel: hw,
		width:   width,
		bar:     bar,
		spinner: s,
		status:  "Starting...",
	}
}

func (m progressModel) start() tea.Cmd {
	return func() tea.Msg {
		return startExecutionMsg{}
	}
}

type startExecutionMsg struct{}

type ffmpegStartedMsg struct {
	cancel     context.CancelFunc
	progressCh <-chan float64
	errCh      <-chan error
}

func (mm mainModel) updateProgress(msg tea.Msg) (mainModel, tea.Cmd) {
	switch msg := msg.(type) {
	case startExecutionMsg:
		mm.progress.startTime = time.Now()
		mm.progress.status = "Preparing..."
		return mm, mm.progress.execute()

	case ffmpegStartedMsg:
		mm.progress.cancel = msg.cancel
		mm.progress.progressCh = msg.progressCh
		mm.progress.errCh = msg.errCh
		mm.progress.status = "Encoding..."
		return mm, waitForNextProgress(msg.progressCh, msg.errCh)

	case progressMsg:
		mm.progress.progress = float64(msg)
		return mm, waitForNextProgress(mm.progress.progressCh, mm.progress.errCh)

	case executeCompleteMsg:
		mm.progress.done = true
		if msg.err != nil {
			mm.progress.err = msg.err
			mm.err = msg.err
			mm.step = stepDone
			return mm, nil
		}

		// On success: move source files to processed folder (except for rename/clip/dry-run)
		shouldMoveFiles := !mm.config.DryRun &&
			mm.config.Action != model.ActionRename &&
			mm.config.Action != model.ActionClip

		if shouldMoveFiles && len(mm.config.Files) > 0 {
			var filePaths []string
			for _, f := range mm.config.Files {
				filePaths = append(filePaths, f.Path)
			}
			if err := fileutil.MoveToProcessedFolder(mm.config.Directory, filePaths); err != nil {
				mm.err = fmt.Errorf("processing succeeded but failed to move source files: %w", err)
				mm.step = stepDone
				return mm, nil
			}
		}

		// Return to directory picker to start over
		mm.dirPicker = newDirPicker()
		mm.step = stepDirPicker
		return mm, mm.dirPicker.Init()

	case spinner.TickMsg:
		var cmd tea.Cmd
		mm.progress.spinner, cmd = mm.progress.spinner.Update(msg)
		return mm, cmd

	case progress.FrameMsg:
		progressModel, cmd := mm.progress.bar.Update(msg)
		mm.progress.bar = progressModel.(progress.Model)
		return mm, cmd

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" && mm.progress.cancel != nil {
			mm.progress.cancel()
			mm.progress.status = "Cancelling..."
		}
	}

	return mm, nil
}

// waitForNextProgress returns a tea.Cmd that blocks until the next progress update or completion.
func waitForNextProgress(progressCh <-chan float64, errCh <-chan error) tea.Cmd {
	return func() tea.Msg {
		select {
		case p, ok := <-progressCh:
			if !ok {
				// progress channel closed; wait for final error/nil
				err := <-errCh
				return executeCompleteMsg{err: err}
			}
			return progressMsg(p)
		case err, ok := <-errCh:
			if !ok {
				return executeCompleteMsg{}
			}
			return executeCompleteMsg{err: err}
		}
	}
}

// execute starts the operation and returns either ffmpegStartedMsg (for encoding actions)
// or executeCompleteMsg (for instant actions like list/rename).
func (m *progressModel) execute() tea.Cmd {
	cfg := m.config
	hw := m.hwAccel

	return func() tea.Msg {
		switch cfg.Action {
		case model.ActionRename:
			return executeCompleteMsg{err: executeRename(cfg)}

		case model.ActionRotateOnly:
			cancel, progressCh, errCh, err := startRotateWithProgress(cfg, hw)
			if err != nil {
				return executeCompleteMsg{err: err}
			}
			return ffmpegStartedMsg{cancel: cancel, progressCh: progressCh, errCh: errCh}

		case model.ActionReencode:
			cancel, progressCh, errCh, err := startReencodeWithProgress(cfg, hw)
			if err != nil {
				return executeCompleteMsg{err: err}
			}
			return ffmpegStartedMsg{cancel: cancel, progressCh: progressCh, errCh: errCh}

		case model.ActionStitch, model.ActionRotateStitch:
			cancel, progressCh, errCh, err := startConcatWithProgress(cfg, hw)
			if err != nil {
				return executeCompleteMsg{err: err}
			}
			return ffmpegStartedMsg{cancel: cancel, progressCh: progressCh, errCh: errCh}

		case model.ActionClip:
			cancel, progressCh, errCh, err := startClipWithProgress(cfg)
			if err != nil {
				return executeCompleteMsg{err: err}
			}
			return ffmpegStartedMsg{cancel: cancel, progressCh: progressCh, errCh: errCh}
		}

		return executeCompleteMsg{}
	}
}

// sendProgress is a non-blocking send that drops old unread values.
func sendProgress(ch chan float64, p float64) {
	select {
	case ch <- p:
	default:
		select {
		case <-ch:
		default:
		}
		select {
		case ch <- p:
		default:
		}
	}
}

// closedProgressCh returns an already-closed float64 channel (used for dry-run / instant completion).
func closedProgressCh() <-chan float64 {
	ch := make(chan float64)
	close(ch)
	return ch
}

// closedErrCh returns an already-closed error channel with no value (signals success).
func closedErrCh() <-chan error {
	ch := make(chan error)
	close(ch)
	return ch
}

func noopCancel() {}

// startRotateWithProgress starts rotation for one or more files with live progress.
// If source files are on a network share, they are copied to /tmp first; the output
// is moved back to the source directory after encoding completes.
func startRotateWithProgress(cfg *model.WizardConfig, hw ffmpeg.HWAccel) (context.CancelFunc, <-chan float64, <-chan error, error) {
	if cfg.DryRun {
		return noopCancel, closedProgressCh(), closedErrCh(), nil
	}

	ctx, cancel := context.WithCancel(context.Background())

	type rotateJob struct {
		args     []string
		duration float64
		cleanup  func() error
		output   string
		hasAudio bool
	}

	var jobs []rotateJob
	totalDur := 0.0

	for _, f := range cfg.Files {
		rot := cfg.Rotation
		if cfg.PerVideoRotate {
			rot = cfg.FileRotations[f.Path]
		}

		base := strings.TrimSuffix(f.Path, filepath.Ext(f.Path))
		ext := cfg.OutputFormat
		if ext == "original" || ext == "" {
			ext = strings.TrimPrefix(filepath.Ext(f.Path), ".")
		}

		var dstOutput string
		if len(cfg.Files) == 1 && cfg.OutputFilename != "" {
			dstOutput = cfg.OutputFilename
		} else {
			dstOutput = base + "_rotated." + ext
		}

		localFiles, localOutput, cleanup, err := fileutil.PrepareLocalFiles([]*model.VideoFile{f}, dstOutput)
		if err != nil {
			cancel()
			return nil, nil, nil, err
		}

		ctime := ffmpeg.GetCreationTime(f.Path)
		args := ffmpeg.BuildRotateArgs(localFiles[0].Path, localOutput, rot, hw, ctime)

		jobs = append(jobs, rotateJob{
			args:     args,
			duration: f.Duration,
			cleanup:  cleanup,
			output:   dstOutput,
			hasAudio: f.HasAudio,
		})
		totalDur += f.Duration
	}

	progressCh := make(chan float64, 1)
	errCh := make(chan error, 1)

	go func() {
		defer close(progressCh)
		defer close(errCh)

		elapsed := 0.0
		for _, job := range jobs {
			pCh, eCh := ffmpeg.RunWithProgress(ctx, job.args, job.duration)
			for p := range pCh {
				if totalDur > 0 {
					sendProgress(progressCh, (elapsed+p*job.duration)/totalDur)
				}
			}
			if err := <-eCh; err != nil {
				errCh <- err
				return
			}
			if err := job.cleanup(); err != nil {
				errCh <- err
				return
			}
			if err := ffmpeg.VerifyAndRepair(job.output, job.hasAudio); err != nil {
				errCh <- err
				return
			}
			elapsed += job.duration
		}
	}()

	return cancel, progressCh, errCh, nil
}

// startReencodeWithProgress starts re-encoding for one or more files with live progress.
// Network share files are copied to /tmp; outputs are moved back after encoding.
func startReencodeWithProgress(cfg *model.WizardConfig, hw ffmpeg.HWAccel) (context.CancelFunc, <-chan float64, <-chan error, error) {
	if cfg.DryRun {
		return noopCancel, closedProgressCh(), closedErrCh(), nil
	}

	ctx, cancel := context.WithCancel(context.Background())

	type reencodeJob struct {
		args     []string
		duration float64
		cleanup  func() error
		output   string
		hasAudio bool
	}

	var jobs []reencodeJob
	totalDur := 0.0

	for _, f := range cfg.Files {
		base := strings.TrimSuffix(f.Path, filepath.Ext(f.Path))
		ext := cfg.OutputFormat
		if ext == "original" || ext == "" {
			ext = strings.TrimPrefix(filepath.Ext(f.Path), ".")
		}

		var dstOutput string
		if len(cfg.Files) == 1 && cfg.OutputFilename != "" {
			dstOutput = cfg.OutputFilename
		} else {
			dstOutput = base + "_reencoded." + ext
		}

		localFiles, localOutput, cleanup, err := fileutil.PrepareLocalFiles([]*model.VideoFile{f}, dstOutput)
		if err != nil {
			cancel()
			return nil, nil, nil, err
		}

		ctime := ffmpeg.GetCreationTime(f.Path)
		args := ffmpeg.BuildReencodeArgs(localFiles[0].Path, localOutput, hw, ctime)

		jobs = append(jobs, reencodeJob{
			args:     args,
			duration: f.Duration,
			cleanup:  cleanup,
			output:   dstOutput,
			hasAudio: f.HasAudio,
		})
		totalDur += f.Duration
	}

	progressCh := make(chan float64, 1)
	errCh := make(chan error, 1)

	go func() {
		defer close(progressCh)
		defer close(errCh)

		elapsed := 0.0
		for _, job := range jobs {
			pCh, eCh := ffmpeg.RunWithProgress(ctx, job.args, job.duration)
			for p := range pCh {
				if totalDur > 0 {
					sendProgress(progressCh, (elapsed+p*job.duration)/totalDur)
				}
			}
			if err := <-eCh; err != nil {
				errCh <- err
				return
			}
			if err := job.cleanup(); err != nil {
				errCh <- err
				return
			}
			if err := ffmpeg.VerifyAndRepair(job.output, job.hasAudio); err != nil {
				errCh <- err
				return
			}
			elapsed += job.duration
		}
	}()

	return cancel, progressCh, errCh, nil
}

// startConcatWithProgress starts concatenation (stitch or rotate+stitch) with live progress.
// Network share files are copied to /tmp; the output is moved to the source directory after encoding.
func startConcatWithProgress(cfg *model.WizardConfig, hw ffmpeg.HWAccel) (context.CancelFunc, <-chan float64, <-chan error, error) {
	if cfg.DryRun {
		return noopCancel, closedProgressCh(), closedErrCh(), nil
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Resolve output path: if source is on a network share and the output is a bare
	// filename, place it in the source directory instead of the current working directory.
	outputPath := cfg.OutputFilename
	if fileutil.AnyNetworkFiles(cfg.Files) && !filepath.IsAbs(outputPath) {
		outputPath = filepath.Join(cfg.Directory, filepath.Base(outputPath))
	}

	localFiles, localOutput, cleanup, err := fileutil.PrepareLocalFiles(cfg.Files, outputPath)
	if err != nil {
		cancel()
		return nil, nil, nil, err
	}

	// Build a local copy of cfg with remapped paths and FileRotations keys.
	localCfg := *cfg
	localCfg.Files = localFiles
	if cfg.PerVideoRotate {
		localCfg.FileRotations = make(map[string]int)
		for i, f := range cfg.Files {
			if rot, ok := cfg.FileRotations[f.Path]; ok {
				localCfg.FileRotations[localFiles[i].Path] = rot
			}
		}
	}

	needReencode := cfg.Reencode || cfg.Rotation != 0 || cfg.PerVideoRotate
	if !needReencode {
		need, _ := ffmpeg.NeedsReencode(localFiles)
		needReencode = need
	}

	anyHasAudio := false
	for _, f := range localFiles {
		if f.HasAudio {
			anyHasAudio = true
			break
		}
	}

	totalDur := ffmpeg.TotalDuration(cfg.Files)
	progressCh := make(chan float64, 1)
	errCh := make(chan error, 1)

	if needReencode {
		targetW, targetH := ffmpeg.FirstEffectiveDimensions(localFiles, &localCfg)

		go func() {
			defer close(progressCh)
			defer close(errCh)

			tmpDir, err := os.MkdirTemp("", "vid-norm-*")
			if err != nil {
				errCh <- fmt.Errorf("create temp dir: %w", err)
				return
			}
			defer os.RemoveAll(tmpDir)

			// Phase 1: normalize each file one at a time (O(1) memory).
			var normPaths []string
			elapsed := 0.0
			for i, f := range localFiles {
				rot := localCfg.Rotation
				if localCfg.PerVideoRotate {
					rot = localCfg.FileRotations[f.Path]
				}
				normOut := filepath.Join(tmpDir, fmt.Sprintf("norm_%03d.mp4", i))
				args := ffmpeg.BuildNormalizeArgs(f.Path, normOut, rot, targetW, targetH, hw, f.HasAudio, anyHasAudio)

				pCh, eCh := ffmpeg.RunWithProgress(ctx, args, f.Duration)
				for p := range pCh {
					if totalDur > 0 {
						sendProgress(progressCh, (elapsed+p*f.Duration)/totalDur)
					}
				}
				if err := <-eCh; err != nil {
					errCh <- err
					return
				}
				elapsed += f.Duration
				normPaths = append(normPaths, normOut)
			}

			// Phase 2: stream-copy the normalized files (near-instant).
			listFile, err := os.CreateTemp(tmpDir, "concat-*.txt")
			if err != nil {
				errCh <- err
				return
			}
			for _, p := range normPaths {
				escaped := strings.ReplaceAll(p, "'", "'\\''")
				fmt.Fprintf(listFile, "file '%s'\n", escaped)
			}
			listFile.Close()

			args := ffmpeg.BuildStreamCopyArgs(listFile.Name(), localOutput)
			if err := ffmpeg.RunSimple(args); err != nil {
				errCh <- err
				return
			}
			if err := cleanup(); err != nil {
				errCh <- err
				return
			}
			if err := ffmpeg.VerifyAndRepair(outputPath, anyHasAudio); err != nil {
				errCh <- err
			}
		}()
	} else {
		// Stream copy via concat demuxer — near-instant, no meaningful progress
		go func() {
			defer close(progressCh)
			defer close(errCh)

			tmpFile, err := os.CreateTemp("", "vid-concat-*.txt")
			if err != nil {
				errCh <- err
				return
			}
			defer os.Remove(tmpFile.Name())

			for _, f := range localFiles {
				escaped := strings.ReplaceAll(f.Path, "'", "'\\''")
				fmt.Fprintf(tmpFile, "file '%s'\n", escaped)
			}
			tmpFile.Close()

			args := ffmpeg.BuildStreamCopyArgs(tmpFile.Name(), localOutput)
			if err := ffmpeg.RunSimple(args); err != nil {
				errCh <- err
				return
			}
			if err := cleanup(); err != nil {
				errCh <- err
				return
			}
			if err := ffmpeg.VerifyAndRepair(outputPath, anyHasAudio); err != nil {
				errCh <- err
			}
		}()
	}

	return cancel, progressCh, errCh, nil
}

// startClipWithProgress extracts each defined clip from the source file using
// stream copy. Clips are written to the same directory as the source file.
func startClipWithProgress(cfg *model.WizardConfig) (context.CancelFunc, <-chan float64, <-chan error, error) {
	if cfg.DryRun {
		return noopCancel, closedProgressCh(), closedErrCh(), nil
	}
	if len(cfg.Files) == 0 || len(cfg.ClipRanges) == 0 {
		return noopCancel, closedProgressCh(), closedErrCh(), nil
	}

	ctx, cancel := context.WithCancel(context.Background())

	src := cfg.Files[0]
	srcDir := filepath.Dir(src.Path)
	n := len(cfg.ClipRanges)

	progressCh := make(chan float64, 1)
	errCh := make(chan error, 1)

	go func() {
		defer close(progressCh)
		defer close(errCh)

		for i, clip := range cfg.ClipRanges {
			outPath := filepath.Join(srcDir, clip.OutName)
			clipDur := clip.End - clip.Start

			args := ffmpeg.BuildClipArgs(src.Path, outPath, clip.Start, clip.End)
			pCh, eCh := ffmpeg.RunWithProgress(ctx, args, clipDur)

			for p := range pCh {
				overall := (float64(i) + p) / float64(n)
				sendProgress(progressCh, overall)
			}
			if err := <-eCh; err != nil {
				errCh <- err
				return
			}
		}
	}()

	return cancel, progressCh, errCh, nil
}

// --- Non-encoding helpers (rename) ---

func executeRename(cfg *model.WizardConfig) error {
	pairs := fileutil.BuildRenamePairs(cfg.Files)
	if cfg.DryRun {
		return nil
	}
	return fileutil.ExecuteRename(pairs)
}

func detectHWAccelCached() ffmpeg.HWAccel {
	return ffmpeg.DetectHWAccel()
}

func (m progressModel) View() string {
	s := style.StepHeader(6, "Processing") + "\n\n"

	if m.done {
		if m.err != nil {
			s += style.Error.Render("  Failed: "+m.err.Error()) + "\n"
		} else {
			s += style.Success.Render("  Complete!") + "\n"
		}
	} else {
		s += "  " + m.spinner.View() + " " + m.status + "\n\n"
		s += "  " + m.bar.ViewAs(m.progress) + "\n\n"

		// Elapsed time
		if !m.startTime.IsZero() {
			elapsed := time.Since(m.startTime).Round(time.Second)
			s += "  " + style.DimText.Render(fmt.Sprintf("Elapsed: %s", elapsed))
			if m.progress > 0.01 {
				est := time.Duration(float64(elapsed) / m.progress)
				remaining := est - elapsed
				if remaining > 0 && (m.bestRemaining == 0 || remaining < m.bestRemaining) {
					m.bestRemaining = remaining
				}
				if m.bestRemaining > 0 {
					s += style.DimText.Render(fmt.Sprintf("  Remaining: ~%s", m.bestRemaining.Round(time.Second)))
				}
			}
			s += "\n"
		}
	}

	s += "\n  " + style.DimText.Render("Ctrl+C to cancel")
	return s
}
