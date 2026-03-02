package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"vid-tui/model"
	"vid-tui/style"

	tea "github.com/charmbracelet/bubbletea"
)

// summaryModel displays the configuration summary and waits for confirmation.
type summaryModel struct {
	config *model.WizardConfig
}

func newSummaryModel(cfg *model.WizardConfig) summaryModel {
	return summaryModel{config: cfg}
}

func (mm mainModel) updateSummary(msg tea.Msg) (mainModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter", "y":
			// Proceed to execution
			mm.hwAccel = detectHWAccelCached()
			mm.progress = newProgressModel(mm.config, mm.hwAccel, mm.width)
			mm.step = stepExecuting
			return mm, mm.progress.start()

		case "esc", "b":
			// Go back to config
			mm.configForm = newConfigModel(mm.config.Files, mm.width)
			mm.step = stepConfig
			return mm, mm.configForm.Init()

		case "n":
			// Cancel
			mm.quitting = true
			return mm, tea.Quit

		case "p":
			// Preview — returns to summary when done
			if mm.config.Action.NeedsRotation() && len(mm.config.Files) > 0 {
				mm.preview = newPreviewModel(mm.config, stepSummary)
				mm.step = stepPreview
				return mm, mm.preview.start()
			}
		}
	}
	return mm, nil
}

func (m summaryModel) View() string {
	cfg := m.config

	s := style.StepHeader(5, "Summary") + "\n\n"

	var lines []string

	// Files
	lines = append(lines, style.Header.Render(fmt.Sprintf("Files: %d selected", len(cfg.Files))))
	for _, f := range cfg.Files {
		name := filepath.Base(f.Path)
		if cfg.PerVideoRotate {
			rot := cfg.FileRotations[f.Path]
			if rot != 0 {
				name += fmt.Sprintf(" [%d°]", rot)
			}
		}
		lines = append(lines, "  - "+name)
	}
	lines = append(lines, "")

	// Action
	lines = append(lines, fmt.Sprintf("Action:    %s", style.Selected.Render(cfg.Action.String())))

	// Rotation
	if cfg.Rotation != 0 {
		lines = append(lines, fmt.Sprintf("Rotation:  %d° (all files)", cfg.Rotation))
	} else if cfg.PerVideoRotate {
		lines = append(lines, "Rotation:  Individual per file")
	}

	// Format
	if cfg.OutputFormat != "" && cfg.OutputFormat != "original" {
		lines = append(lines, fmt.Sprintf("Format:    %s", cfg.OutputFormat))
	}

	// Output
	switch {
	case cfg.Action == model.ActionClip:
		lines = append(lines, fmt.Sprintf("Clips:     %d to extract", len(cfg.ClipRanges)))
		for i, clip := range cfg.ClipRanges {
			lines = append(lines, fmt.Sprintf("  %d. %s → %s  →  %s",
				i+1,
				clipDisplayTime(clip.Start),
				clipDisplayTime(clip.End),
				clip.OutName))
		}
	case cfg.Action == model.ActionRename:
		lines = append(lines, "Output:    Files renamed to 1.EXT, 2.EXT, ...")
	case cfg.Action == model.ActionRotateOnly && len(cfg.Files) >= 2:
		lines = append(lines, fmt.Sprintf("Output:    Each file → <name>_rotated.%s", cfg.OutputFormat))
	case cfg.Action == model.ActionReencode && len(cfg.Files) >= 2:
		lines = append(lines, fmt.Sprintf("Output:    Each file → <name>_reencoded.%s", cfg.OutputFormat))
	default:
		lines = append(lines, fmt.Sprintf("Output:    %s", cfg.OutputFilename))
	}

	// Reencode
	if cfg.Reencode {
		lines = append(lines, "Encoding:  Re-encode")
	}

	// Dry run
	if cfg.DryRun {
		lines = append(lines, style.Warning.Render("Dry run:   Yes (no files will be created)"))
	}

	content := strings.Join(lines, "\n")
	boxed := style.Box.Render(content)

	s += boxed + "\n\n"

	// Help
	help := style.HelpKey.Render("Enter") + style.HelpText.Render(" proceed  ")
	if cfg.Action.NeedsRotation() {
		help += style.HelpKey.Render("p") + style.HelpText.Render(" preview  ")
	}
	help += style.HelpKey.Render("Esc") + style.HelpText.Render(" back  ")
	help += style.HelpKey.Render("n") + style.HelpText.Render(" cancel")
	s += "  " + help

	return s
}
