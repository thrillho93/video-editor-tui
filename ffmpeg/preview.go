package ffmpeg

import (
	"os/exec"
)

// PreviewCmd returns an exec.Cmd for a 10-second ffplay preview with optional rotation.
// The TUI will use tea.ExecProcess to suspend and run this.
func PreviewCmd(path string, rotation int) *exec.Cmd {
	args := []string{"-autoexit", "-t", "10", "-loglevel", "error"}

	if rf := RotationFilter(rotation); rf != "" {
		args = append(args, "-vf", rf)
	}

	args = append(args, path)
	return exec.Command("ffplay", args...)
}

// MpvAvailable reports whether mpv is installed and on PATH.
func MpvAvailable() bool {
	_, err := exec.LookPath("mpv")
	return err == nil
}
