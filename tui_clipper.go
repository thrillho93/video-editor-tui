package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"vid-tui/ffmpeg"
	"vid-tui/model"
	"vid-tui/style"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
)

// clipFormFields holds the heap-allocated values bound to the huh form.
type clipFormFields struct {
	startStr string
	endStr   string
	outName  string
}

// clipperModel manages the clip selection step.
type clipperModel struct {
	config      *model.WizardConfig
	clips       []model.ClipRange
	cursor      int

	// Form state (manually adding a clip)
	addingClip bool
	form       *huh.Form
	formFields *clipFormFields

	nextClipNum int
}

// clipMarksReadMsg is sent after mpv exits, carrying any clips the user marked.
type clipMarksReadMsg struct {
	clips []model.ClipRange
	err   error
}

// clipPreviewDoneMsg is sent after a plain ffplay preview exits.
type clipPreviewDoneMsg struct{}

func newClipperModel(cfg *model.WizardConfig) clipperModel {
	return clipperModel{
		config:      cfg,
		nextClipNum: 1,
	}
}

// --- mpv marker session ---

// mpvSession implements tea.ExecCommand so we can print a keybinding guide
// to the terminal before mpv opens its window.
type mpvSession struct {
	cmd    *exec.Cmd
	stdout io.Writer
}

func (s *mpvSession) SetStdin(r io.Reader)  { s.cmd.Stdin = r }
func (s *mpvSession) SetStdout(w io.Writer) { s.stdout = w; s.cmd.Stdout = w }
func (s *mpvSession) SetStderr(w io.Writer) { s.cmd.Stderr = w }

func (s *mpvSession) Run() error {
	fmt.Fprint(s.stdout, mpvGuide)
	return s.cmd.Run()
}

const mpvGuide = `
  vid – clip marker

  i        mark start of clip
  o        mark end of clip  (auto-saves)

  Space    pause / resume
  ← →      seek 5 seconds
  , .      step one frame
  q        quit and return to vid

  Repeat i / o for each clip you want.
  All clips appear in the list on return.

`

// luaMarkerScript returns a Lua script that binds i/o in mpv to write
// in/out timestamps to marksFile.
func luaMarkerScript(marksFile string) string {
	return `local in_point = nil

mp.add_forced_key_binding("i", "vid-mark-in", function()
    in_point = mp.get_property_number("time-pos")
    local m = math.floor(in_point / 60)
    local s = in_point % 60
    mp.osd_message(string.format("[ IN ]  %d:%05.2f", m, s), 2)
end)

mp.add_forced_key_binding("o", "vid-mark-out", function()
    if in_point == nil then
        mp.osd_message("[ ! ]  Press i first to set the in-point", 2)
        return
    end
    local out_point = mp.get_property_number("time-pos")
    if out_point <= in_point then
        mp.osd_message("[ ! ]  Out-point must be after in-point", 2)
        return
    end
    local f = io.open("` + marksFile + `", "a")
    if f then
        f:write(string.format("%.3f,%.3f\n", in_point, out_point))
        f:close()
    end
    local im = math.floor(in_point / 60)
    local is_ = in_point % 60
    local om = math.floor(out_point / 60)
    local os_ = out_point % 60
    mp.osd_message(string.format("[ OUT ]  %d:%05.2f -> %d:%05.2f  clip saved", im, is_, om, os_), 3)
    in_point = nil
end)
`
}

// buildMpvMarkerSession writes a Lua script to a temp dir and returns an
// mpvSession ready to pass to tea.Exec.
func buildMpvMarkerSession(file *model.VideoFile) (*mpvSession, string, string, error) {
	tmpDir, err := os.MkdirTemp("", "vid-marks-*")
	if err != nil {
		return nil, "", "", fmt.Errorf("create temp dir: %w", err)
	}

	marksFile := filepath.Join(tmpDir, "marks.txt")
	luaFile := filepath.Join(tmpDir, "marker.lua")

	if err := os.WriteFile(luaFile, []byte(luaMarkerScript(marksFile)), 0644); err != nil {
		os.RemoveAll(tmpDir)
		return nil, "", "", fmt.Errorf("write lua script: %w", err)
	}

	cmd := exec.Command("mpv",
		"--really-quiet",
		"--hwdec=auto",
		"--script="+luaFile,
		"--osd-font-size=42",
		"--osd-duration=2500",
		file.Path,
	)
	return &mpvSession{cmd: cmd}, marksFile, tmpDir, nil
}

// readClipMarks parses the marks file written by the Lua script.
func readClipMarks(marksFile, tmpDir string, file *model.VideoFile, startNum int) clipMarksReadMsg {
	defer os.RemoveAll(tmpDir)

	data, err := os.ReadFile(marksFile)
	if err != nil {
		if os.IsNotExist(err) {
			return clipMarksReadMsg{}
		}
		return clipMarksReadMsg{err: err}
	}

	base := strings.TrimSuffix(filepath.Base(file.Path), filepath.Ext(file.Path))
	ext := strings.TrimPrefix(filepath.Ext(file.Path), ".")
	if ext == "" {
		ext = "mp4"
	}

	var clips []model.ClipRange
	for i, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ",", 2)
		if len(parts) != 2 {
			continue
		}
		start, err1 := strconv.ParseFloat(parts[0], 64)
		end, err2 := strconv.ParseFloat(parts[1], 64)
		if err1 != nil || err2 != nil {
			continue
		}
		clips = append(clips, model.ClipRange{
			Start:   start,
			End:     end,
			OutName: fmt.Sprintf("%s_clip_%03d.%s", base, startNum+i, ext),
		})
	}

	return clipMarksReadMsg{clips: clips}
}

// --- manual add form ---

func (m *clipperModel) defaultClipName() string {
	if len(m.config.Files) == 0 {
		return fmt.Sprintf("clip_%03d.mp4", m.nextClipNum)
	}
	f := m.config.Files[0]
	base := strings.TrimSuffix(filepath.Base(f.Path), filepath.Ext(f.Path))
	ext := strings.TrimPrefix(filepath.Ext(f.Path), ".")
	if ext == "" {
		ext = "mp4"
	}
	return fmt.Sprintf("%s_clip_%03d.%s", base, m.nextClipNum, ext)
}

func (m *clipperModel) buildAddForm() {
	f := &clipFormFields{
		outName: m.defaultClipName(),
	}
	m.formFields = f

	dur := 0.0
	if len(m.config.Files) > 0 {
		dur = m.config.Files[0].Duration
	}

	m.form = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Start Time").
				Description("e.g.  0:30  or  1:05.5  or  90 (seconds)").
				Value(&f.startStr).
				Validate(func(s string) error {
					t, err := ffmpeg.ParseTimestamp(s)
					if err != nil {
						return fmt.Errorf("use MM:SS, HH:MM:SS, or plain seconds")
					}
					if t < 0 {
						return fmt.Errorf("must be >= 0")
					}
					if dur > 0 && t >= dur {
						return fmt.Errorf("exceeds file duration (%.1fs)", dur)
					}
					return nil
				}),
			huh.NewInput().
				Title("End Time").
				Description("e.g.  1:30  or  2:00.0  or  180 (seconds)").
				Value(&f.endStr).
				Validate(func(s string) error {
					start, _ := ffmpeg.ParseTimestamp(f.startStr)
					end, err := ffmpeg.ParseTimestamp(s)
					if err != nil {
						return fmt.Errorf("use MM:SS, HH:MM:SS, or plain seconds")
					}
					if end <= start {
						return fmt.Errorf("must be after start time")
					}
					if dur > 0 && end > dur {
						return fmt.Errorf("exceeds file duration (%.1fs)", dur)
					}
					return nil
				}),
			huh.NewInput().
				Title("Output Name").
				Description("Leave blank to use the default auto-name").
				Value(&f.outName).
				Placeholder(m.defaultClipName()),
		),
	).WithShowHelp(true)

	m.addingClip = true
}

func (m *clipperModel) finishAddForm() {
	f := m.formFields
	start, _ := ffmpeg.ParseTimestamp(f.startStr)
	end, _ := ffmpeg.ParseTimestamp(f.endStr)

	outName := strings.TrimSpace(f.outName)
	if outName == "" {
		outName = m.defaultClipName()
	}

	m.clips = append(m.clips, model.ClipRange{
		Start:   start,
		End:     end,
		OutName: outName,
	})
	m.nextClipNum++
	m.cursor = len(m.clips) - 1
	m.addingClip = false
	m.form = nil
	m.formFields = nil
}

// --- update ---

func (mm mainModel) updateClipPicker(msg tea.Msg) (mainModel, tea.Cmd) {
	m := &mm.clipper

	// ESC: cancel form or go back to config
	if kmsg, ok := msg.(tea.KeyMsg); ok && kmsg.String() == "esc" {
		if m.addingClip {
			m.addingClip = false
			m.form = nil
			m.formFields = nil
			return mm, nil
		}
		mm.configForm = newConfigModel(mm.config.Files, mm.width)
		mm.step = stepConfig
		return mm, mm.configForm.Init()
	}

	// clips returned after mpv exits
	if msg, ok := msg.(clipMarksReadMsg); ok {
		if len(msg.clips) > 0 {
			m.clips = append(m.clips, msg.clips...)
			m.nextClipNum += len(msg.clips)
			m.cursor = len(m.clips) - 1
		}
		return mm, nil
	}

	if _, ok := msg.(clipPreviewDoneMsg); ok {
		return mm, nil
	}

	// form is active
	if m.addingClip && m.form != nil {
		form, cmd := m.form.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			m.form = f
		}
		switch m.form.State {
		case huh.StateCompleted:
			m.finishAddForm()
			return mm, nil
		case huh.StateAborted:
			m.addingClip = false
			m.form = nil
			m.formFields = nil
			return mm, nil
		}
		return mm, cmd
	}

	// list navigation
	if kmsg, ok := msg.(tea.KeyMsg); ok {
		switch kmsg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.clips)-1 {
				m.cursor++
			}

		case "p":
			if len(m.config.Files) == 0 {
				return mm, nil
			}
			file := m.config.Files[0]
			if ffmpeg.MpvAvailable() {
				session, marksFile, tmpDir, err := buildMpvMarkerSession(file)
				if err == nil {
					startNum := m.nextClipNum
					return mm, tea.Exec(session, func(err error) tea.Msg {
						return readClipMarks(marksFile, tmpDir, file, startNum)
					})
				}
			}
			// fallback: plain ffplay
			return mm, tea.ExecProcess(
				exec.Command("ffplay", "-autoexit", "-loglevel", "error", file.Path),
				func(err error) tea.Msg { return clipPreviewDoneMsg{} },
			)

		case "n", "a":
			m.buildAddForm()
			return mm, m.form.Init()

		case "d", "x":
			if len(m.clips) > 0 && m.cursor < len(m.clips) {
				m.clips = append(m.clips[:m.cursor], m.clips[m.cursor+1:]...)
				if m.cursor >= len(m.clips) && m.cursor > 0 {
					m.cursor--
				}
			}

		case "enter":
			if len(m.clips) > 0 {
				mm.config.ClipRanges = m.clips
				mm.summary = newSummaryModel(mm.config)
				mm.step = stepSummary
				return mm, nil
			}
		}
	}

	return mm, nil
}

// --- view ---

func (m clipperModel) View() string {
	if m.addingClip && m.form != nil {
		s := style.StepHeader(5, "Add Clip Manually") + "\n\n"
		s += m.form.View()
		return s
	}

	s := style.StepHeader(5, "Clip Selection") + "\n\n"

	// File info
	if len(m.config.Files) > 0 {
		f := m.config.Files[0]
		s += "  " + style.Header.Render(filepath.Base(f.Path)) + "  " +
			style.DimText.Render(f.DurationString()) + "\n\n"
	}

	// Workflow instructions
	if ffmpeg.MpvAvailable() {
		s += "  " + style.Selected.Render("How to mark clips:") + "\n"
		s += "  " + style.DimText.Render("  1.  Press p — the video opens in mpv") + "\n"
		s += "  " + style.DimText.Render("  2.  Seek to where a clip should start, press i") + "\n"
		s += "  " + style.DimText.Render("  3.  Seek to where it should end, press o — clip saved") + "\n"
		s += "  " + style.DimText.Render("  4.  Repeat i / o for more clips, then press q to return") + "\n"
		s += "  " + style.DimText.Render("  Clips appear in the list below automatically.") + "\n\n"
	} else {
		s += "  " + style.Warning.Render("mpv not found — manual entry only") + "\n"
		s += "  " + style.DimText.Render("  Press n to enter start/end times by hand.") + "\n\n"
	}

	// Clips list
	if len(m.clips) == 0 {
		s += "  " + style.DimText.Render("No clips yet.") + "\n"
	} else {
		s += "  " + style.Selected.Render(fmt.Sprintf("%d clip(s) ready to extract:", len(m.clips))) + "\n\n"
		for i, clip := range m.clips {
			prefix := "    "
			nameStyle := style.FileItem
			if i == m.cursor {
				prefix = "  > "
				nameStyle = style.FileItemSelected
			}
			line := fmt.Sprintf("%s%s  %s  to  %s  →  %s",
				prefix,
				style.DimText.Render(fmt.Sprintf("%d.", i+1)),
				style.DimText.Render(clipDisplayTime(clip.Start)),
				style.DimText.Render(clipDisplayTime(clip.End)),
				nameStyle.Render(clip.OutName),
			)
			s += line + "\n"
		}
	}

	s += "\n"

	// Help bar
	help := "  "
	if ffmpeg.MpvAvailable() {
		help += style.HelpKey.Render("p") + style.HelpText.Render(" preview & mark  ")
	}
	help += style.HelpKey.Render("n") + style.HelpText.Render(" add manually  ")
	if len(m.clips) > 0 {
		if len(m.clips) > 1 {
			help += style.HelpKey.Render("↑/↓") + style.HelpText.Render(" navigate  ")
		}
		help += style.HelpKey.Render("d") + style.HelpText.Render(" remove  ")
		help += style.HelpKey.Render("Enter") + style.HelpText.Render(" extract clips  ")
	}
	help += style.HelpKey.Render("Esc") + style.HelpText.Render(" back")
	s += help

	return s
}

// clipDisplayTime formats seconds as a compact human-readable timestamp.
func clipDisplayTime(seconds float64) string {
	h := int(seconds) / 3600
	m := (int(seconds) % 3600) / 60
	s := int(seconds) % 60
	frac := seconds - float64(int(seconds))

	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	if frac > 0.05 {
		return fmt.Sprintf("%d:%02d.%d", m, s, int(frac*10))
	}
	return fmt.Sprintf("%d:%02d", m, s)
}
