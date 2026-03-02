package main

import (
	"os"
	"path/filepath"
	"strings"

	"vid-tui/style"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// dirPickerModel handles directory input with ~ expansion and validation.
type dirPickerModel struct {
	input    textinput.Model
	errMsg   string
}

func newDirPicker() dirPickerModel {
	ti := textinput.New()
	ti.Placeholder = ". (current directory)"
	ti.Focus()
	ti.CharLimit = 256
	ti.Width = 50

	return dirPickerModel{input: ti}
}

func (m dirPickerModel) Init() tea.Cmd {
	return textinput.Blink
}

// updateDirPicker handles messages for the directory picker step.
func (mm mainModel) updateDirPicker(msg tea.Msg) (mainModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			dir := mm.dirPicker.input.Value()
			if dir == "" {
				dir = "."
			}

			// Expand ~
			if strings.HasPrefix(dir, "~") {
				home, err := os.UserHomeDir()
				if err == nil {
					dir = filepath.Join(home, dir[1:])
				}
			}

			// Resolve to absolute path
			abs, err := filepath.Abs(dir)
			if err != nil {
				mm.dirPicker.errMsg = "Invalid path: " + err.Error()
				return mm, nil
			}

			// Validate it's a directory
			info, err := os.Stat(abs)
			if err != nil || !info.IsDir() {
				mm.dirPicker.errMsg = "Not a directory: " + abs
				return mm, nil
			}

			mm.config.Directory = abs
			mm.dirPicker.errMsg = ""
			mm.step = stepScanning
			return mm, startScan(abs)

		case "tab":
			// Tab completion
			mm.dirPicker = tabComplete(mm.dirPicker)
			return mm, nil
		}
	}

	var cmd tea.Cmd
	mm.dirPicker.input, cmd = mm.dirPicker.input.Update(msg)
	return mm, cmd
}

func (m dirPickerModel) View() string {
	s := style.StepHeader(1, "Choose Directory") + "\n\n"
	s += "  " + m.input.View() + "\n"

	if m.errMsg != "" {
		s += "\n  " + style.Error.Render(m.errMsg)
	}

	s += "\n" + style.DimText.Render("  Enter to confirm - Tab to complete - Ctrl+C to quit")
	return s
}

// tabComplete performs simple directory tab completion.
func tabComplete(m dirPickerModel) dirPickerModel {
	val := m.input.Value()
	if val == "" {
		return m
	}

	// Expand ~
	expanded := val
	if strings.HasPrefix(expanded, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			expanded = filepath.Join(home, expanded[1:])
		}
	}

	dir := filepath.Dir(expanded)
	prefix := filepath.Base(expanded)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return m
	}

	var matches []string
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), prefix) {
			matches = append(matches, filepath.Join(dir, e.Name()))
		}
	}

	if len(matches) == 1 {
		result := matches[0] + "/"
		// Compress back to ~ if possible
		home, err := os.UserHomeDir()
		if err == nil && strings.HasPrefix(result, home) {
			result = "~" + result[len(home):]
		}
		m.input.SetValue(result)
		m.input.CursorEnd()
	} else if len(matches) > 1 {
		// Find common prefix
		common := matches[0]
		for _, match := range matches[1:] {
			common = commonPrefix(common, match)
		}
		if len(common) > len(expanded) {
			result := common
			home, err := os.UserHomeDir()
			if err == nil && strings.HasPrefix(result, home) {
				result = "~" + result[len(home):]
			}
			m.input.SetValue(result)
			m.input.CursorEnd()
		}
	}

	return m
}

func commonPrefix(a, b string) string {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return a[:i]
		}
	}
	return a[:n]
}
