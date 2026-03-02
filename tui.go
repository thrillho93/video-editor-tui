package main

import (
	"fmt"
	"sync"

	"vid-tui/ffmpeg"
	"vid-tui/fileutil"
	"vid-tui/model"
	"vid-tui/style"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// step tracks the current wizard step.
type step int

const (
	stepDirPicker step = iota
	stepScanning
	stepFilePicker
	stepConfig
	stepClipPicker
	stepPerVideoRotation
	stepPreview
	stepSummary
	stepExecuting
	stepDone
)

// mainModel is the top-level bubbletea model that dispatches to step handlers.
type mainModel struct {
	step      step
	width     int
	height    int
	config    *model.WizardConfig
	hwAccel   ffmpeg.HWAccel
	err       error
	quitting  bool

	// Step sub-models
	dirPicker        dirPickerModel
	scanner          scannerModel
	filePicker       filePickerModel
	configForm       configModel
	clipper          clipperModel
	perVideoRotation perVideoRotationModel
	summary          summaryModel
	progress         progressModel
	preview          previewModel
}

// scannerModel handles the scanning phase with a spinner.
type scannerModel struct {
	spinner spinner.Model
	dir     string
	files   []*model.VideoFile
	groups  []fileutil.DateGroup
	done    bool
	err     error
}

// Messages
type (
	scanCompleteMsg struct {
		files  []*model.VideoFile
		groups []fileutil.DateGroup
		err    error
	}
	executeCompleteMsg struct {
		err error
	}
	progressMsg float64
)

func newMainModel() mainModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(style.Purple)

	return mainModel{
		step:   stepDirPicker,
		config: model.NewWizardConfig(),
		dirPicker: newDirPicker(),
		scanner: scannerModel{spinner: s},
	}
}

func (m mainModel) Init() tea.Cmd {
	return tea.Batch(
		m.dirPicker.Init(),
		m.scanner.spinner.Tick,
	)
}

func (m mainModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Propagate to sub-models that need it
		m.filePicker.width = msg.Width
		m.filePicker.height = msg.Height
		m.configForm.width = msg.Width
		m.progress.width = msg.Width
		m.perVideoRotation.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd

	switch m.step {
	case stepDirPicker:
		m, cmd = m.updateDirPicker(msg)
	case stepScanning:
		m, cmd = m.updateScanning(msg)
	case stepFilePicker:
		m, cmd = m.updateFilePicker(msg)
	case stepConfig:
		m, cmd = m.updateConfig(msg)
	case stepClipPicker:
		m, cmd = m.updateClipPicker(msg)
	case stepPerVideoRotation:
		m, cmd = m.updatePerVideoRotation(msg)
	case stepPreview:
		m, cmd = m.updatePreview(msg)
	case stepSummary:
		m, cmd = m.updateSummary(msg)
	case stepExecuting:
		m, cmd = m.updateProgress(msg)
	case stepDone:
		// Any key to quit
		if _, ok := msg.(tea.KeyMsg); ok {
			return m, tea.Quit
		}
	}

	return m, cmd
}

func (m mainModel) View() string {
	if m.quitting {
		return ""
	}

	// Title bar
	title := style.Title.Render("VID - Interactive Video Tool")
	subtitle := style.DimText.Render("Combine videos - Rotate videos - Batch rename")
	header := title + "\n" + subtitle + "\n" + style.DividerLine(min(m.width, 50)) + "\n\n"

	var body string
	switch m.step {
	case stepDirPicker:
		body = m.dirPicker.View()
	case stepScanning:
		body = m.viewScanning()
	case stepFilePicker:
		body = m.filePicker.View()
	case stepConfig:
		body = m.configForm.View()
	case stepClipPicker:
		body = m.clipper.View()
	case stepPerVideoRotation:
		body = m.perVideoRotation.View()
	case stepPreview:
		body = m.preview.View()
	case stepSummary:
		body = m.summary.View()
	case stepExecuting:
		body = m.progress.View()
	case stepDone:
		body = m.viewDone()
	}

	// Error display
	if m.err != nil {
		body += "\n\n" + style.Error.Render("Error: "+m.err.Error())
	}

	return header + body
}

// --- Scanning step ---

func (m mainModel) updateScanning(msg tea.Msg) (mainModel, tea.Cmd) {
	switch msg := msg.(type) {
	case scanCompleteMsg:
		if msg.err != nil {
			m.err = msg.err
			m.step = stepDirPicker
			return m, nil
		}
		m.scanner.files = msg.files
		m.scanner.groups = msg.groups
		m.scanner.done = true
		m.err = nil // clear any previous error

		// Initialize file picker with results
		m.filePicker = newFilePicker(msg.groups, m.width, m.height)
		m.step = stepFilePicker
		return m, m.filePicker.Init()

	default:
		var cmd tea.Cmd
		m.scanner.spinner, cmd = m.scanner.spinner.Update(msg)
		return m, cmd
	}
}

func (m mainModel) viewScanning() string {
	return style.StepHeader(2, "Scanning") + "\n\n" +
		m.scanner.spinner.View() + " Reading video metadata from " +
		style.DimText.Render(m.config.Directory) + "..."
}

// startScan creates a command that probes all files in parallel.
func startScan(dir string) tea.Cmd {
	return func() tea.Msg {
		paths, err := fileutil.ScanDir(dir, "")
		if err != nil {
			return scanCompleteMsg{err: err}
		}
		if len(paths) == 0 {
			return scanCompleteMsg{err: fmt.Errorf("no video files found in %s", dir)}
		}

		// Parallel probe with 8 workers
		files := make([]*model.VideoFile, len(paths))
		var wg sync.WaitGroup
		sem := make(chan struct{}, 8)
		var mu sync.Mutex
		var probeErr error

		for i, p := range paths {
			wg.Add(1)
			go func(idx int, path string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				vf, err := ffmpeg.ProbeFile(path)
				if err != nil {
					mu.Lock()
					if probeErr == nil {
						probeErr = fmt.Errorf("probe %s: %w", path, err)
					}
					mu.Unlock()
					return
				}
				files[idx] = vf
			}(i, p)
		}
		wg.Wait()

		// Filter out nil entries (failed probes)
		var valid []*model.VideoFile
		for _, f := range files {
			if f != nil {
				valid = append(valid, f)
			}
		}

		if len(valid) == 0 {
			return scanCompleteMsg{err: fmt.Errorf("no valid video files found")}
		}

		fileutil.SortByCreationTime(valid)
		groups := fileutil.GroupByDate(valid)

		return scanCompleteMsg{files: valid, groups: groups}
	}
}

// --- Done step ---

func (m mainModel) viewDone() string {
	if m.err != nil {
		return style.Error.Render("Failed: " + m.err.Error()) + "\n\n" +
			style.DimText.Render("Press any key to exit")
	}
	return style.Success.Render("Done!") + "\n\n" +
		style.DimText.Render("Press any key to exit")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
