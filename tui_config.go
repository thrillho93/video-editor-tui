package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"vid-tui/model"
	"vid-tui/style"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
)

// configFields holds form-bound values on the heap so huh's Value() pointers
// and WithHideFunc closures remain valid after configModel is copied by value.
type configFields struct {
	actionStr    string
	outputFormat string
	outputName   string
	rotationMode string // "same" or "per_video"
	rotationStr  string
	encodingStr  string
	dryRunStr    string
	fileCount    int
	sourceExt    string // common source extension (e.g. "mov"), or "mp4" if mixed
}

// configModel embeds a huh.Form for the configuration wizard steps.
type configModel struct {
	form   *huh.Form
	fields *configFields // heap-allocated, shared with form closures
	width  int
}

// commonExt returns the lowercase extension shared by all files (without the
// leading dot), or "mp4" if files have mixed or unrecognised extensions.
func commonExt(files []*model.VideoFile) string {
	if len(files) == 0 {
		return "mp4"
	}
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(files[0].Path), "."))
	for _, f := range files[1:] {
		e := strings.ToLower(strings.TrimPrefix(filepath.Ext(f.Path), "."))
		if e != ext {
			return "mp4"
		}
	}
	if ext == "" {
		return "mp4"
	}
	return ext
}

func newConfigModel(files []*model.VideoFile, width int) configModel {
	srcExt := commonExt(files)

	// Default outputFormat: use srcExt when it matches a named format option,
	// otherwise fall back to "original" (keep source extension as-is).
	defaultFormat := srcExt
	switch srcExt {
	case "mp4", "mov", "mkv", "webm", "avi":
		// known format option — use directly
	default:
		defaultFormat = "original"
	}

	f := &configFields{
		actionStr:    "stitch",
		outputFormat: defaultFormat,
		outputName:   "output." + srcExt,
		rotationMode: "same",
		rotationStr:  "90",
		encodingStr:  "copy",
		dryRunStr:    "no",
		fileCount:    len(files),
		sourceExt:    srcExt,
	}
	if len(files) < 2 {
		f.actionStr = "rotate"
	}

	m := configModel{
		fields: f,
		width:  width,
	}
	m.form = buildConfigForm(f)
	return m
}

func (m configModel) Init() tea.Cmd {
	return m.form.Init()
}

func (mm mainModel) updateConfig(msg tea.Msg) (mainModel, tea.Cmd) {
	// Check for back navigation
	if kmsg, ok := msg.(tea.KeyMsg); ok {
		if kmsg.String() == "esc" {
			mm.filePicker = newFilePicker(mm.scanner.groups, mm.width, mm.height)
			mm.step = stepFilePicker
			return mm, mm.filePicker.Init()
		}
	}

	form, cmd := mm.configForm.form.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		mm.configForm.form = f
	}

	if mm.configForm.form.State == huh.StateCompleted {
		mm.applyConfigValues()

		// Clip action: go to clip picker step
		if mm.config.Action == model.ActionClip {
			mm.clipper = newClipperModel(mm.config)
			mm.step = stepClipPicker
			return mm, nil
		}

		// If per-video rotation was selected, go to per-video rotation step
		if mm.configForm.fields.rotationMode == "per_video" && len(mm.config.Files) >= 2 {
			mm.config.PerVideoRotate = true
			mm.perVideoRotation = newPerVideoRotationModel(mm.config, mm.height)
			mm.step = stepPerVideoRotation
			return mm, nil
		}

		mm.summary = newSummaryModel(mm.config)
		mm.step = stepSummary
		return mm, nil
	}

	return mm, cmd
}

func (m configModel) View() string {
	header := style.StepHeader(4, "Configure") + "\n\n"
	return header + m.form.View()
}

func buildConfigForm(f *configFields) *huh.Form {
	// Build action options based on file count
	var actionOptions []huh.Option[string]
	if f.fileCount >= 2 {
		actionOptions = []huh.Option[string]{
			huh.NewOption("Stitch - Combine videos by creation time", "stitch"),
			huh.NewOption("Rotate - Rotate each video separately", "rotate"),
			huh.NewOption("Rotate + Stitch - Rotate then combine", "rotate_stitch"),
			huh.NewOption("Re-encode - Convert format without rotating", "reencode"),
			huh.NewOption("Rename - Sequential rename by creation order", "rename"),
		}
	} else {
		actionOptions = []huh.Option[string]{
			huh.NewOption("Rotate - Fix video orientation", "rotate"),
			huh.NewOption("Re-encode - Convert to different format", "reencode"),
			huh.NewOption("Clip - Preview and extract time-range clips", "clip"),
		}
	}

	formatOptions := []huh.Option[string]{
		huh.NewOption("mp4 (H.264 - universal)", "mp4"),
		huh.NewOption("mov (QuickTime - Apple)", "mov"),
		huh.NewOption("mkv (Matroska - open format)", "mkv"),
		huh.NewOption("webm (Web optimized)", "webm"),
		huh.NewOption("avi (Legacy format)", "avi"),
		huh.NewOption("Keep original extension", "original"),
	}

	rotationModeOptions := []huh.Option[string]{
		huh.NewOption("Same rotation for all videos", "same"),
		huh.NewOption("Different rotation for each video", "per_video"),
	}

	rotationOptions := []huh.Option[string]{
		huh.NewOption("90° clockwise (portrait → landscape)", "90"),
		huh.NewOption("180° (upside down)", "180"),
		huh.NewOption("270° clockwise (landscape → portrait)", "270"),
	}

	encodingOptions := []huh.Option[string]{
		huh.NewOption("Stream copy (fast, no quality loss)", "copy"),
		huh.NewOption("Re-encode (slower, fixes timestamp issues)", "reencode"),
	}

	dryRunOptions := []huh.Option[string]{
		huh.NewOption("No - Execute normally", "no"),
		huh.NewOption("Yes - Preview only, no output", "yes"),
	}

	needsRotation := func() bool {
		return f.actionStr == "rotate" || f.actionStr == "rotate_stitch"
	}

	groups := []*huh.Group{
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Action").
				Options(actionOptions...).
				Value(&f.actionStr),
		),
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Output Format").
				Options(formatOptions...).
				Value(&f.outputFormat),
		).WithHideFunc(func() bool {
			return f.actionStr == "clip" || f.actionStr == "rename"
		}),
		huh.NewGroup(
			huh.NewInput().
				Title("Output Filename").
				Value(&f.outputName).
				Placeholder("output.mp4"),
		).WithHideFunc(func() bool {
			return f.actionStr == "rename" ||
				f.actionStr == "clip" ||
				(f.actionStr == "rotate" && f.fileCount >= 2) ||
				(f.actionStr == "reencode" && f.fileCount >= 2)
		}),
		// Rotation mode: same for all vs per-video (only for multi-file rotation)
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Rotation Mode").
				Description("Choose how to apply rotation across files").
				Options(rotationModeOptions...).
				Value(&f.rotationMode),
		).WithHideFunc(func() bool {
			return !needsRotation() || f.fileCount < 2
		}),
		// Global rotation angle (shown for single file, or multi-file with "same" mode)
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Rotation").
				Options(rotationOptions...).
				Value(&f.rotationStr),
		).WithHideFunc(func() bool {
			if !needsRotation() {
				return true
			}
			if f.fileCount >= 2 && f.rotationMode == "per_video" {
				return true
			}
			return false
		}),
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Encoding Method").
				Description("Stream copy is fast but may not work with mixed formats").
				Options(encodingOptions...).
				Value(&f.encodingStr),
		).WithHideFunc(func() bool {
			return f.actionStr != "stitch"
		}),
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Dry Run?").
				Options(dryRunOptions...).
				Value(&f.dryRunStr),
		),
	}

	return huh.NewForm(groups...).WithShowHelp(true).WithShowErrors(true)
}

func (mm *mainModel) applyConfigValues() {
	cfg := mm.config
	f := mm.configForm.fields

	// Action
	switch f.actionStr {
	case "stitch":
		cfg.Action = model.ActionStitch
	case "rotate":
		cfg.Action = model.ActionRotateOnly
	case "rotate_stitch":
		cfg.Action = model.ActionRotateStitch
	case "reencode":
		cfg.Action = model.ActionReencode
	case "clip":
		cfg.Action = model.ActionClip
	case "rename":
		cfg.Action = model.ActionRename
	}

	// Format
	cfg.OutputFormat = f.outputFormat

	// Output filename
	if cfg.Action == model.ActionRename || cfg.Action == model.ActionClip {
		// no output filename: rename uses original names, clip uses per-clip names
	} else if cfg.Action == model.ActionRotateOnly && len(cfg.Files) >= 2 {
		// multi-file rotate: each gets _rotated suffix
	} else if cfg.Action == model.ActionReencode && len(cfg.Files) >= 2 {
		// multi-file reencode: each gets _reencoded suffix
	} else {
		name := f.outputName
		if name == "" {
			name = "output"
		}
		if f.outputFormat != "original" {
			ext := "." + f.outputFormat
			if !strings.HasSuffix(name, ext) {
				name = strings.TrimSuffix(name, "."+getExt(name)) + ext
			}
		}
		// Ensure the filename always has an extension so ffmpeg can determine
		// the container format. Without one ffmpeg exits with AVERROR(EINVAL)
		// = code 234. Fall back to the source extension when "Keep original"
		// is selected and the user typed a bare name.
		if getExt(name) == "" {
			name += "." + f.sourceExt
		}
		cfg.OutputFilename = name
	}

	// Rotation (global — only if action needs it and not per-video)
	if cfg.Action.NeedsRotation() && f.rotationMode != "per_video" {
		switch f.rotationStr {
		case "90":
			cfg.Rotation = 90
		case "180":
			cfg.Rotation = 180
		case "270":
			cfg.Rotation = 270
		default:
			cfg.Rotation = 0
		}
		cfg.PerVideoRotate = false
	} else if !cfg.Action.NeedsRotation() {
		cfg.Rotation = 0
		cfg.PerVideoRotate = false
	}

	// Encoding
	cfg.Reencode = f.encodingStr == "reencode"

	// Dry run
	cfg.DryRun = f.dryRunStr == "yes"
}

func getExt(name string) string {
	parts := strings.Split(name, ".")
	if len(parts) > 1 {
		return parts[len(parts)-1]
	}
	return ""
}

func formatActionSummary(action model.Action, fileCount int, format string) string {
	switch action {
	case model.ActionRotateOnly:
		if fileCount >= 2 {
			return fmt.Sprintf("Each file → <name>_rotated.%s", format)
		}
		return "Rotated output file"
	case model.ActionReencode:
		if fileCount >= 2 {
			return fmt.Sprintf("Each file → <name>_reencoded.%s", format)
		}
		return "Re-encoded output file"
	case model.ActionRename:
		return "Files renamed to 1.EXT, 2.EXT, ..."
	default:
		return ""
	}
}
