package style

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

var (
	// Colors
	Purple    = lipgloss.Color("99")
	Pink      = lipgloss.Color("205")
	Green     = lipgloss.Color("42")
	Yellow    = lipgloss.Color("220")
	Red       = lipgloss.Color("196")
	Dim       = lipgloss.Color("241")
	White     = lipgloss.Color("255")
	DarkGray  = lipgloss.Color("236")

	// Text styles
	Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(Purple)

	Header = lipgloss.NewStyle().
		Bold(true).
		Foreground(Pink).
		MarginBottom(1)

	Divider = lipgloss.NewStyle().
		Foreground(Dim)

	Selected = lipgloss.NewStyle().
		Foreground(Green).
		Bold(true)

	Error = lipgloss.NewStyle().
		Foreground(Red).
		Bold(true)

	DimText = lipgloss.NewStyle().
		Foreground(Dim)

	Success = lipgloss.NewStyle().
		Foreground(Green)

	Warning = lipgloss.NewStyle().
		Foreground(Yellow)

	// Layout
	Box = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Purple).
		Padding(1, 2)

	// Step indicator
	StepBadge = lipgloss.NewStyle().
		Background(Purple).
		Foreground(White).
		Bold(true).
		Padding(0, 1)

	// Help bar
	HelpKey = lipgloss.NewStyle().
		Foreground(Pink).
		Bold(true)

	HelpText = lipgloss.NewStyle().
		Foreground(Dim)

	// File picker
	DateHeader = lipgloss.NewStyle().
		Foreground(Yellow).
		Bold(true)

	FileItem = lipgloss.NewStyle().
		Foreground(White)

	FileItemSelected = lipgloss.NewStyle().
		Foreground(Green).
		Bold(true)

	MetadataLabel = lipgloss.NewStyle().
		Foreground(Pink).
		Width(10)

	MetadataValue = lipgloss.NewStyle().
		Foreground(White)

	// Progress
	ProgressDone = lipgloss.NewStyle().
		Foreground(Green)
)

// DividerLine returns a horizontal divider of the given width.
func DividerLine(width int) string {
	line := ""
	for i := 0; i < width; i++ {
		line += "─"
	}
	return Divider.Render(line)
}

// StepHeader returns a formatted step header like "STEP 1  Choose Directory".
func StepHeader(step int, title string) string {
	badge := StepBadge.Render(fmt.Sprintf(" STEP %d ", step))
	return badge + " " + Header.Render(title)
}
