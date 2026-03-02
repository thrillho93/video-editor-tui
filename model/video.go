package model

import (
	"fmt"
	"time"
)

// VideoFile holds metadata for a single video file.
type VideoFile struct {
	Path         string
	CreationTime time.Time
	Duration     float64 // seconds
	Width        int
	Height       int
	Codec        string
	HasAudio     bool
	IsVFR        bool  // true when r_frame_rate differs from avg_frame_rate by >2%
	Size         int64 // bytes
}

// DurationString returns a human-readable duration like "2m05s".
func (v *VideoFile) DurationString() string {
	d := time.Duration(v.Duration * float64(time.Second))
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm%02ds", m, s)
}

// SizeString returns a human-readable file size.
func (v *VideoFile) SizeString() string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case v.Size >= GB:
		return fmt.Sprintf("%.1fG", float64(v.Size)/float64(GB))
	case v.Size >= MB:
		return fmt.Sprintf("%.1fM", float64(v.Size)/float64(MB))
	case v.Size >= KB:
		return fmt.Sprintf("%.1fK", float64(v.Size)/float64(KB))
	default:
		return fmt.Sprintf("%dB", v.Size)
	}
}

// Resolution returns "WxH" string.
func (v *VideoFile) Resolution() string {
	return fmt.Sprintf("%dx%d", v.Width, v.Height)
}
