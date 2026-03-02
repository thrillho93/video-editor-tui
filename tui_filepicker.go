package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"vid-tui/fileutil"
	"vid-tui/model"
	"vid-tui/style"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// listItem is either a date header or a video file in the picker.
type listItem struct {
	isHeader bool
	label    string // date header label
	file     *model.VideoFile
	selected bool
	groupIdx int // which date group this belongs to
}

// thumbnailCache stores generated chafa output keyed by file path.
type thumbnailCache struct {
	mu      sync.Mutex
	dir     string            // temp directory for thumbnail images
	results map[string]string // path → chafa text output
	pending map[string]bool   // paths currently being generated
}

func newThumbnailCache() *thumbnailCache {
	dir, _ := os.MkdirTemp("", "vid-thumbs-")
	return &thumbnailCache{
		dir:     dir,
		results: make(map[string]string),
		pending: make(map[string]bool),
	}
}

func (tc *thumbnailCache) cleanup() {
	if tc.dir != "" {
		os.RemoveAll(tc.dir)
	}
}

// get returns cached thumbnail text, or empty string if not yet available.
func (tc *thumbnailCache) get(path string) string {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	return tc.results[path]
}

// isPending returns true if a thumbnail is currently being generated.
func (tc *thumbnailCache) isPending(path string) bool {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	return tc.pending[path]
}

// store saves generated thumbnail text.
func (tc *thumbnailCache) store(path, text string) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.results[path] = text
	delete(tc.pending, path)
}

// markPending marks a path as being generated.
func (tc *thumbnailCache) markPending(path string) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.pending[path] = true
}

// thumbnailMsg is sent when an async thumbnail generation completes.
type thumbnailMsg struct {
	path string
	text string
}

// filePickerModel is a custom grouped multi-select list with metadata preview.
type filePickerModel struct {
	items    []listItem
	groups   []fileutil.DateGroup
	cursor   int
	offset   int // scroll offset
	width    int
	height   int
	thumbs   *thumbnailCache
}

func newFilePicker(groups []fileutil.DateGroup, width, height int) filePickerModel {
	var items []listItem
	for gi, g := range groups {
		items = append(items, listItem{
			isHeader: true,
			label:    g.Label,
			groupIdx: gi,
		})
		for _, f := range g.Files {
			items = append(items, listItem{
				file:     f,
				groupIdx: gi,
			})
		}
	}

	return filePickerModel{
		items:  items,
		groups: groups,
		width:  width,
		height: height,
		thumbs: newThumbnailCache(),
	}
}

func (m filePickerModel) Init() tea.Cmd {
	return m.requestThumbnail()
}

// requestThumbnail starts async thumbnail generation for the current cursor item.
func (m filePickerModel) requestThumbnail() tea.Cmd {
	if m.cursor < 0 || m.cursor >= len(m.items) {
		return nil
	}
	item := m.items[m.cursor]
	if item.isHeader || item.file == nil {
		return nil
	}
	path := item.file.Path

	// Already cached or pending
	if m.thumbs.get(path) != "" || m.thumbs.isPending(path) {
		return nil
	}

	m.thumbs.markPending(path)
	thumbDir := m.thumbs.dir

	// Calculate thumbnail dimensions to fill available preview space
	listWidth := m.width*55/100 - 2
	if listWidth < 40 {
		listWidth = 40
	}
	previewWidth := m.width - listWidth - 10 // match content area: View's padding-box (−6) minus horizontal padding (−4)
	if previewWidth < 16 {
		previewWidth = 16
	}
	// Must match the thumbAreaHeight calculation in View()
	const metadataLines = 9
	thumbH := m.visibleLines() - metadataLines - 4 // -4 for border/padding/gap
	if thumbH < 4 {
		thumbH = 4
	}

	return func() tea.Msg {
		text := generateThumbnail(path, thumbDir, previewWidth, thumbH)
		return thumbnailMsg{path: path, text: text}
	}
}

// generateThumbnail creates a thumbnail image and converts to terminal art.
// Uses text-only output (block+braille characters) which bubbletea can properly
// redraw and clear. Graphics protocols (kitty/sixel) can't be used because
// bubbletea redraws every frame and can't clear pixel-based output.
func generateThumbnail(videoPath, tmpDir string, thumbWidth, thumbHeight int) string {
	thumbPath := filepath.Join(tmpDir, filepath.Base(videoPath)+".jpg")

	// Try ffmpegthumbnailer first (fast)
	err := exec.Command("ffmpegthumbnailer", "-i", videoPath, "-o", thumbPath, "-s", "0", "-q", "10").Run()
	if err != nil {
		// Fallback: ffmpeg with high-quality thumbnail extraction
		err = exec.Command("ffmpeg", "-v", "error", "-i", videoPath,
			"-vf", "thumbnail,scale=640:-1", "-frames:v", "1", "-q:v", "2", "-y", thumbPath).Run()
		if err != nil {
			return ""
		}
	}

	sizeArg := fmt.Sprintf("%dx%d", thumbWidth, thumbHeight)

	// Use block+braille characters for maximum text-mode resolution.
	// Blocks handle solid fills well; braille gives 2x4 subpixel detail per cell.
	out, err := exec.Command("chafa",
		"--format=symbols",
		"--symbols=block+braille",
		"--size="+sizeArg,
		"--dither=diffusion",
		"--color-space=din99d",
		"--work=9",
		"--animate=off",
		thumbPath,
	).Output()
	if err != nil {
		return ""
	}

	return strings.TrimRight(string(out), "\n\r ")
}

func (mm mainModel) updateFilePicker(msg tea.Msg) (mainModel, tea.Cmd) {
	switch msg := msg.(type) {
	case thumbnailMsg:
		mm.filePicker.thumbs.store(msg.path, msg.text)
		return mm, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			mm.filePicker.moveCursor(-1)
			return mm, mm.filePicker.requestThumbnail()
		case "down", "j":
			mm.filePicker.moveCursor(1)
			return mm, mm.filePicker.requestThumbnail()
		case "tab", " ":
			mm.filePicker.toggleCurrent()
		case "a":
			mm.filePicker.selectAll()
		case "A":
			mm.filePicker.deselectAll()
		case "enter":
			selected := mm.filePicker.selectedFiles()
			if len(selected) == 0 {
				return mm, nil
			}
			mm.config.Files = selected
			mm.filePicker.thumbs.cleanup()

			// Initialize config form
			mm.configForm = newConfigModel(selected, mm.width)
			mm.step = stepConfig
			return mm, mm.configForm.Init()

		case "esc", "b":
			mm.filePicker.thumbs.cleanup()
			// Go back to directory picker
			mm.dirPicker = newDirPicker()
			mm.step = stepDirPicker
			return mm, mm.dirPicker.Init()
		}
	}
	return mm, nil
}

func (m *filePickerModel) moveCursor(delta int) {
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.items) {
		m.cursor = len(m.items) - 1
	}

	// Adjust scroll offset
	visible := m.visibleLines()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+visible {
		m.offset = m.cursor - visible + 1
	}
}

func (m *filePickerModel) visibleLines() int {
	v := m.height - 12 // header, help, etc.
	if v < 5 {
		v = 20
	}
	return v
}

func (m *filePickerModel) toggleCurrent() {
	item := &m.items[m.cursor]
	if item.isHeader {
		// Toggle entire group
		groupIdx := item.groupIdx
		// Check if all in group are selected
		allSelected := true
		for i := range m.items {
			if !m.items[i].isHeader && m.items[i].groupIdx == groupIdx {
				if !m.items[i].selected {
					allSelected = false
					break
				}
			}
		}
		// Toggle: if all selected, deselect all; otherwise select all
		newState := !allSelected
		for i := range m.items {
			if !m.items[i].isHeader && m.items[i].groupIdx == groupIdx {
				m.items[i].selected = newState
			}
		}
	} else {
		item.selected = !item.selected
	}
}

func (m *filePickerModel) selectAll() {
	for i := range m.items {
		if !m.items[i].isHeader {
			m.items[i].selected = true
		}
	}
}

func (m *filePickerModel) deselectAll() {
	for i := range m.items {
		m.items[i].selected = false
	}
}

func (m *filePickerModel) selectedFiles() []*model.VideoFile {
	var files []*model.VideoFile
	for _, item := range m.items {
		if !item.isHeader && item.selected {
			files = append(files, item.file)
		}
	}
	return files
}

func (m filePickerModel) View() string {
	// Left pane: file list
	var listLines []string
	visible := m.visibleLines()
	end := m.offset + visible
	if end > len(m.items) {
		end = len(m.items)
	}

	for i := m.offset; i < end; i++ {
		item := m.items[i]
		isCursor := i == m.cursor
		var line string

		if item.isHeader {
			line = style.DateHeader.Render("── " + item.label + " ──")
			if isCursor {
				line = "> " + line
			} else {
				line = "  " + line
			}
		} else {
			name := filepath.Base(item.file.Path)
			check := "[ ]"
			nameStyle := style.FileItem
			if item.selected {
				check = "[x]"
				nameStyle = style.FileItemSelected
			}
			line = fmt.Sprintf("  %s %s  %s  %s",
				check,
				nameStyle.Render(name),
				style.DimText.Render(item.file.DurationString()),
				style.DimText.Render(item.file.SizeString()),
			)
			if isCursor {
				line = "> " + line[2:]
			}
		}
		listLines = append(listLines, line)
	}

	listPane := strings.Join(listLines, "\n")
	listWidth := m.width*55/100 - 2
	if listWidth < 40 {
		listWidth = 40
	}

	previewWidth := m.width - listWidth - 6
	if previewWidth < 20 {
		previewWidth = 20
	}

	// Fixed number of lines for metadata (always the same regardless of content)
	const metadataLines = 9 // filename + blank + 6 fields + blank

	// Right pane: always exactly `visible` lines tall.
	// Reserve fixed space for metadata and fill the rest with thumbnail or blanks.
	thumbAreaHeight := visible - metadataLines - 2 // -2 for padding/border
	if thumbAreaHeight < 4 {
		thumbAreaHeight = 4
	}

	var previewPane string
	if m.cursor >= 0 && m.cursor < len(m.items) && !m.items[m.cursor].isHeader && m.items[m.cursor].file != nil {
		f := m.items[m.cursor].file
		previewPane = renderMetadata(f)

		if thumb := m.thumbs.get(f.Path); thumb != "" {
			previewPane += "\n\n" + thumb
		} else if m.thumbs.isPending(f.Path) {
			previewPane += "\n\n" + style.DimText.Render("  Loading preview...")
		}
	}

	// Pad or trim preview to exactly `visible` lines so layout never shifts
	previewLines := strings.Split(previewPane, "\n")
	targetLines := visible - 4 // account for border + padding
	if targetLines < 1 {
		targetLines = visible
	}
	for len(previewLines) < targetLines {
		previewLines = append(previewLines, "")
	}
	if len(previewLines) > targetLines {
		previewLines = previewLines[:targetLines]
	}
	previewPane = strings.Join(previewLines, "\n")

	// Layout — both panes fixed height
	leftBox := lipgloss.NewStyle().Width(listWidth).Height(visible).Render(listPane)
	rightBox := lipgloss.NewStyle().
		Width(previewWidth).
		Height(visible).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(style.Dim).
		Padding(1, 2).
		Render(previewPane)

	content := lipgloss.JoinHorizontal(lipgloss.Top, leftBox, "  ", rightBox)

	// Count selected
	selCount := 0
	for _, item := range m.items {
		if !item.isHeader && item.selected {
			selCount++
		}
	}

	header := style.StepHeader(3, "Select Video Files") + "\n"
	header += style.DimText.Render(fmt.Sprintf("  %d selected", selCount)) + "\n\n"

	help := "\n" + style.DimText.Render(
		"  "+style.HelpKey.Render("j/k")+" move  "+
			style.HelpKey.Render("Tab/Space")+" toggle  "+
			style.HelpKey.Render("a/A")+" all/none  "+
			style.HelpKey.Render("Enter")+" confirm  "+
			style.HelpKey.Render("Esc")+" back")

	return header + content + help
}

func renderMetadata(f *model.VideoFile) string {
	var lines []string
	lines = append(lines, style.Header.Render(filepath.Base(f.Path)))
	lines = append(lines, "")

	addField := func(label, value string) {
		lines = append(lines, style.MetadataLabel.Render(label)+style.MetadataValue.Render(value))
	}

	addField("Size", f.SizeString())
	addField("Duration", f.DurationString())
	addField("Resolution", f.Resolution())
	addField("Codec", f.Codec)

	if !f.CreationTime.IsZero() {
		addField("Created", f.CreationTime.Local().Format("2006-01-02 15:04:05"))
	} else {
		addField("Created", "(no metadata)")
	}

	audio := "No"
	if f.HasAudio {
		audio = "Yes"
	}
	addField("Audio", audio)

	return strings.Join(lines, "\n")
}
