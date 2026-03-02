package main

import (
	"path/filepath"

	"vid-tui/ffmpeg"
	"vid-tui/model"
	"vid-tui/style"

	tea "github.com/charmbracelet/bubbletea"
)

// previewModel handles ffplay preview with TUI suspend/resume.
// returnTo controls which step to go back to after preview.
type previewModel struct {
	config   *model.WizardConfig
	file     *model.VideoFile
	rotation int
	returnTo step
	done     bool
}

type previewDoneMsg struct{}

func newPreviewModel(cfg *model.WizardConfig, returnTo step) previewModel {
	file := cfg.Files[0]
	rotation := cfg.Rotation
	if cfg.PerVideoRotate {
		rotation = cfg.FileRotations[file.Path]
	}
	return previewModel{
		config:   cfg,
		file:     file,
		rotation: rotation,
		returnTo: returnTo,
	}
}

// newPreviewModelForFile creates a preview for a specific file and rotation,
// returning to the given step when done.
func newPreviewModelForFile(cfg *model.WizardConfig, file *model.VideoFile, rotation int, returnTo step) previewModel {
	return previewModel{
		config:   cfg,
		file:     file,
		rotation: rotation,
		returnTo: returnTo,
	}
}

func (m previewModel) start() tea.Cmd {
	cmd := ffmpeg.PreviewCmd(m.file.Path, m.rotation)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return previewDoneMsg{}
	})
}

func (mm mainModel) updatePreview(msg tea.Msg) (mainModel, tea.Cmd) {
	switch msg.(type) {
	case previewDoneMsg:
		mm.preview.done = true
		return mm, nil
	}

	if _, ok := msg.(tea.KeyMsg); ok && mm.preview.done {
		kmsg := msg.(tea.KeyMsg)
		switch kmsg.String() {
		case "enter", "y":
			mm.step = mm.preview.returnTo
			return mm, nil
		case "r":
			// Replay with same rotation
			mm.preview.done = false
			return mm, mm.preview.start()
		case "esc", "b":
			mm.step = mm.preview.returnTo
			return mm, nil
		}
	}

	return mm, nil
}

func (m previewModel) View() string {
	if !m.done {
		return style.DimText.Render("  Playing preview...")
	}

	name := filepath.Base(m.file.Path)
	s := style.StepHeader(0, "Preview") + "\n\n"
	s += "  " + style.DimText.Render(name) + "  " + style.Selected.Render(rotationLabel(m.rotation)) + "\n\n"
	s += "  " + style.HelpKey.Render("Enter") + style.HelpText.Render(" looks good, continue  ")
	s += style.HelpKey.Render("r") + style.HelpText.Render(" replay  ")
	s += style.HelpKey.Render("Esc") + style.HelpText.Render(" back")
	return s
}
