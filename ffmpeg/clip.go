package ffmpeg

import (
	"fmt"
	"strconv"
	"strings"
)

// BuildClipArgs returns ffmpeg args to extract a time range from a video using
// stream copy (no re-encoding). Output timestamps start at 0 seconds.
// start and end are in seconds.
func BuildClipArgs(input, output string, start, end float64) []string {
	return []string{
		"-ss", FormatTimestamp(start),
		"-i", input,
		"-to", FormatTimestamp(end - start),
		"-c", "copy",
		"-avoid_negative_ts", "make_zero",
		"-movflags", "+faststart",
		"-y", output,
	}
}

// FormatTimestamp converts seconds to HH:MM:SS.mmm or MM:SS.mmm for use in
// ffmpeg arguments.
func FormatTimestamp(seconds float64) string {
	h := int(seconds) / 3600
	m := (int(seconds) % 3600) / 60
	s := seconds - float64(h*3600+m*60)
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%06.3f", h, m, s)
	}
	return fmt.Sprintf("%d:%06.3f", m, s)
}

// ParseTimestamp parses a human-readable timestamp string into seconds.
// Accepted formats: "90", "1:30", "1:30.5", "1:30:00", "01:30:00.500"
func ParseTimestamp(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty timestamp")
	}

	parts := strings.Split(s, ":")
	switch len(parts) {
	case 1:
		// Plain seconds
		v, err := strconv.ParseFloat(parts[0], 64)
		if err != nil {
			return 0, fmt.Errorf("invalid timestamp %q", s)
		}
		return v, nil

	case 2:
		// MM:SS or MM:SS.mmm
		mins, err := strconv.ParseFloat(parts[0], 64)
		if err != nil {
			return 0, fmt.Errorf("invalid timestamp %q: bad minutes", s)
		}
		sec, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			return 0, fmt.Errorf("invalid timestamp %q: bad seconds", s)
		}
		return mins*60 + sec, nil

	case 3:
		// HH:MM:SS or HH:MM:SS.mmm
		hrs, err := strconv.ParseFloat(parts[0], 64)
		if err != nil {
			return 0, fmt.Errorf("invalid timestamp %q: bad hours", s)
		}
		mins, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			return 0, fmt.Errorf("invalid timestamp %q: bad minutes", s)
		}
		sec, err := strconv.ParseFloat(parts[2], 64)
		if err != nil {
			return 0, fmt.Errorf("invalid timestamp %q: bad seconds", s)
		}
		return hrs*3600 + mins*60 + sec, nil

	default:
		return 0, fmt.Errorf("invalid timestamp %q", s)
	}
}
