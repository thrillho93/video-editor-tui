package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"vid-tui/ffmpeg"
	"vid-tui/model"
	"vid-tui/style"

	tea "github.com/charmbracelet/bubbletea"
)

// perVideoRotationModel lets users set rotation for each file individually,
// with inline preview-and-adjust support per file.
type perVideoRotationModel struct {
	config    *model.WizardConfig
	cursor    int   // which file we're on
	rotations []int // rotation per file (0, 90, 180, 270)
	offset    int   // scroll offset for viewport
	height    int   // terminal height
}

// perVideoPreviewDoneMsg is sent when ffplay exits during per-video preview.
type perVideoPreviewDoneMsg struct{}

func newPerVideoRotationModel(cfg *model.WizardConfig, height int) perVideoRotationModel {
	rots := make([]int, len(cfg.Files))
	for i, f := range cfg.Files {
		if r, ok := cfg.FileRotations[f.Path]; ok {
			rots[i] = r
		}
	}
	return perVideoRotationModel{
		config:    cfg,
		rotations: rots,
		height:    height,
	}
}

func (mm mainModel) updatePerVideoRotation(msg tea.Msg) (mainModel, tea.Cmd) {
	m := &mm.perVideoRotation

	switch msg := msg.(type) {
	case perVideoPreviewDoneMsg:
		// Preview finished — stay on this screen so user can adjust or accept
		return mm, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.adjustScrollOffset()
			}
		case "down", "j":
			if m.cursor < len(m.config.Files)-1 {
				m.cursor++
				m.adjustScrollOffset()
			}
		case "1":
			m.rotations[m.cursor] = 90
		case "2":
			m.rotations[m.cursor] = 180
		case "3":
			m.rotations[m.cursor] = 270
		case "0":
			m.rotations[m.cursor] = 0
		case "left", "h":
			m.rotations[m.cursor] = prevRotation(m.rotations[m.cursor])
		case "right", "l", "tab", " ":
			m.rotations[m.cursor] = nextRotation(m.rotations[m.cursor])
		case "p":
			// Preview current file with its current rotation
			f := m.config.Files[m.cursor]
			rot := m.rotations[m.cursor]
			cmd := ffmpeg.PreviewCmd(f.Path, rot)
			return mm, tea.ExecProcess(cmd, func(err error) tea.Msg {
				return perVideoPreviewDoneMsg{}
			})
		case "enter":
			// Apply rotations to config and go to summary
			for i, f := range m.config.Files {
				m.config.FileRotations[f.Path] = m.rotations[i]
			}
			m.config.PerVideoRotate = true
			mm.summary = newSummaryModel(mm.config)
			mm.step = stepSummary
			return mm, nil
		case "esc", "b":
			mm.configForm = newConfigModel(mm.config.Files, mm.width)
			mm.step = stepConfig
			return mm, mm.configForm.Init()
		}
	}

	return mm, nil
}

func (m perVideoRotationModel) View() string {
	s := style.StepHeader(5, "Set Rotation Per Video") + "\n\n"

	// Calculate visible range
	visible := m.visibleLines()
	end := m.offset + visible
	if end > len(m.config.Files) {
		end = len(m.config.Files)
	}

	// Only render visible items
	for i := m.offset; i < end; i++ {
		f := m.config.Files[i]
		name := filepath.Base(f.Path)
		rot := m.rotations[i]

		isCursor := i == m.cursor
		prefix := "  "
		if isCursor {
			prefix = "> "
		}

		// Inline rotation selector: [  0 ]  90   180   270
		var opts []string
		for _, deg := range []int{0, 90, 180, 270} {
			label := rotationShort(deg)
			if deg == rot {
				opts = append(opts, style.Selected.Render("["+label+"]"))
			} else {
				opts = append(opts, style.DimText.Render(" "+label+" "))
			}
		}

		nameStr := style.FileItem.Render(name)
		if isCursor {
			nameStr = style.FileItemSelected.Render(name)
		}

		line := fmt.Sprintf("%s%s  %s  %s",
			prefix,
			nameStr,
			strings.Join(opts, " "),
			style.DimText.Render(rotationLabel(rot)),
		)
		s += line + "\n"
	}

	s += "\n" + style.DimText.Render(
		"  "+style.HelpKey.Render("j/k")+" navigate  "+
			style.HelpKey.Render("←/→")+" cycle  "+
			style.HelpKey.Render("0/1/2/3")+" set  "+
			style.HelpKey.Render("p")+" preview  "+
			style.HelpKey.Render("Enter")+" confirm  "+
			style.HelpKey.Render("Esc")+" back")

	return s
}

func nextRotation(current int) int {
	switch current {
	case 0:
		return 90
	case 90:
		return 180
	case 180:
		return 270
	default:
		return 0
	}
}

func prevRotation(current int) int {
	switch current {
	case 0:
		return 270
	case 90:
		return 0
	case 180:
		return 90
	default:
		return 180
	}
}

func rotationLabel(deg int) string {
	switch deg {
	case 90:
		return "90° CW"
	case 180:
		return "180°"
	case 270:
		return "270° CW"
	default:
		return "No rotation"
	}
}

func rotationShort(deg int) string {
	switch deg {
	case 90:
		return " 90"
	case 180:
		return "180"
	case 270:
		return "270"
	default:
		return "  0"
	}
}

// visibleLines calculates how many file lines can fit on screen
func (m *perVideoRotationModel) visibleLines() int {
	// Account for: header (3 lines), help text (2 lines), and padding
	v := m.height - 10
	if v < 5 {
		v = 20
	}
	return v
}

// adjustScrollOffset updates the scroll offset to keep cursor visible
func (m *perVideoRotationModel) adjustScrollOffset() {
	visible := m.visibleLines()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+visible {
		m.offset = m.cursor - visible + 1
	}
}
