package main

import (
	"fmt"
	"os"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
	flag "github.com/spf13/pflag"
)

var version = "dev"

func main() {
	// CLI flags
	var (
		output   string
		ext      string
		rotation int
		preview  bool
		reencode bool
		rename   bool
		dryrun   bool
		interact bool
		showHelp bool
	)

	flag.StringVarP(&output, "output", "o", "combined.mp4", "Output filename")
	flag.StringVarP(&ext, "extension", "e", "MOV", "File extension filter")
	flag.IntVarP(&rotation, "rotate", "R", 0, "Rotate: 90, 180, or 270 degrees")
	flag.BoolVarP(&preview, "preview", "P", false, "Preview rotation with ffplay")
	flag.BoolVarP(&reencode, "reencode", "r", false, "Re-encode instead of stream copy")
	flag.BoolVarP(&rename, "rename", "n", false, "Rename files by creation order")
	flag.BoolVarP(&dryrun, "dryrun", "d", false, "Dry run (preview only)")
	flag.BoolVarP(&interact, "interactive", "i", false, "Interactive TUI mode")
	flag.BoolVarP(&showHelp, "help", "h", false, "Show help")

	flag.Parse()

	if showHelp {
		printUsage()
		os.Exit(0)
	}

	if !checkDeps() {
		os.Exit(1)
	}

	// Interactive mode: no args or -i flag
	if interact || flag.NArg() == 0 {
		runTUI()
		return
	}

	// Validate rotation
	if rotation != 0 && rotation != 90 && rotation != 180 && rotation != 270 {
		fmt.Fprintf(os.Stderr, "Error: Rotation must be 90, 180, or 270\n")
		os.Exit(1)
	}

	// CLI mode
	cli := &cliRunner{
		output:   output,
		ext:      ext,
		rotation: rotation,
		preview:  preview,
		reencode: reencode,
		rename:   rename,
		dryrun:   dryrun,
		args:     flag.Args(),
	}

	if err := cli.run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// checkDeps verifies that required external tools are available on PATH.
// Returns false (and prints a clear message) if anything is missing.
func checkDeps() bool {
	type dep struct {
		bin      string
		note     string
		required bool
	}

	deps := []dep{
		{"ffmpeg", "video encoding", true},
		{"ffprobe", "video metadata", true},
		{"ffplay", "rotation preview", true},
		{"mpv", "interactive clip marking", true},
	}

	var missing []string
	for _, d := range deps {
		if _, err := exec.LookPath(d.bin); err != nil {
			missing = append(missing, fmt.Sprintf("  %-14s  %s", d.bin, d.note))
		}
	}

	if len(missing) == 0 {
		return true
	}

	fmt.Fprintln(os.Stderr, "Error: the following required tools are not installed or not on PATH:")
	fmt.Fprintln(os.Stderr)
	for _, m := range missing {
		fmt.Fprintln(os.Stderr, m)
	}
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "On Arch Linux:  sudo pacman -S ffmpeg mpv")
	fmt.Fprintln(os.Stderr, "On Ubuntu/Deb:  sudo apt install ffmpeg mpv")
	fmt.Fprintln(os.Stderr, "On macOS:       brew install ffmpeg mpv")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "Run  vid --help  for full requirements.\n")
	return false
}

func runTUI() {
	m := newMainModel()
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	fmt.Print("\033[2J\033[H") // clear terminal on exit
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Printf(`Usage: vid [OPTIONS] <input>...
       vid                  (interactive mode - recommended!)
       vid -i               (interactive mode)

Video tool for rotation, concatenation, and clip extraction.

REQUIREMENTS:
  ffmpeg, ffprobe, ffplay   video processing and rotation preview
  mpv                       interactive clip marking (Clip action)
  chafa                     optional — thumbnail previews in file picker

FEATURES:
  - Rotate single videos (fix orientation: 90, 180, 270)
  - Combine multiple videos sorted by creation time
  - Extract clips: open in mpv, press i/o to mark start/end, clips auto-populate
  - Interactive preview to dial in the correct rotation
  - Batch rename by creation order

Arguments:
  <input>    Video file(s) or a directory containing videos

Options:
  -i, --interactive     Interactive TUI mode (also triggered with no arguments)
  -o, --output FILE     Output file (default: combined.mp4)
  -e, --extension EXT   File extension filter (default: MOV)
  -R, --rotate DEG      Rotate video: 90, 180, or 270 degrees clockwise
  -P, --preview         Preview rotation with ffplay before processing
  -r, --reencode        Re-encode instead of stream copy
  -n, --rename          Rename files by creation time (1.MOV, 2.MOV, ...)
  -d, --dryrun          Dry run - show order without creating output
  -h, --help            Show this help message

Examples:
  vid                                # interactive mode (best way!)
  vid -R 90 -o rotated.mp4 clip.MOV # rotate a single video 90
  vid -R 90 -P clip.MOV             # preview rotation, then process
  vid ~/Videos/*.MOV                 # combine multiple videos
  vid -o merged.mp4 ~/Downloads      # combine all videos in dir
  vid -d ~/Videos                    # preview file order only
  vid -n ~/Videos                    # rename files to 1.MOV, 2.MOV, ...
`)
}
