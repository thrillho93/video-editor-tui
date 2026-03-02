package model

// Action represents the operation to perform on video files.
type Action int

const (
	ActionStitch       Action = iota // Combine videos by creation time
	ActionRotateOnly                 // Rotate each video separately
	ActionRotateStitch               // Rotate then combine
	ActionReencode                   // Re-encode without rotating
	ActionClip                       // Extract time-range clips from a single video
	ActionRename                     // Sequential rename by creation order
)

func (a Action) String() string {
	switch a {
	case ActionStitch:
		return "Stitch"
	case ActionRotateOnly:
		return "Rotate"
	case ActionRotateStitch:
		return "Rotate + Stitch"
	case ActionReencode:
		return "Re-encode"
	case ActionClip:
		return "Clip"
	case ActionRename:
		return "Rename"
	default:
		return "Unknown"
	}
}

// NeedsRotation returns true if the action involves rotation.
func (a Action) NeedsRotation() bool {
	return a == ActionRotateOnly || a == ActionRotateStitch
}

// NeedsOutput returns true if the action produces an output file.
func (a Action) NeedsOutput() bool {
	return a != ActionRename
}

// NeedsEncoding returns true if the action involves encoding decisions.
func (a Action) NeedsEncoding() bool {
	return a == ActionStitch
}

// ClipRange defines a time range to extract from a video file.
type ClipRange struct {
	Start   float64 // seconds
	End     float64 // seconds
	OutName string  // output filename (basename only)
}

// WizardConfig holds all configuration collected during the TUI wizard.
type WizardConfig struct {
	Directory      string
	Files          []*VideoFile
	Action         Action
	OutputFormat   string         // mp4, mov, mkv, webm, avi, original
	OutputFilename string
	Rotation       int            // 0, 90, 180, 270 (global)
	FileRotations  map[string]int // per-file rotations (path → degrees)
	PerVideoRotate bool           // use individual rotations
	Reencode       bool           // re-encode vs stream copy
	DryRun         bool
	ClipRanges     []ClipRange // for ActionClip
}

// NewWizardConfig returns a config with sensible defaults.
func NewWizardConfig() *WizardConfig {
	return &WizardConfig{
		OutputFormat:   "mp4",
		OutputFilename: "output.mp4",
		FileRotations:  make(map[string]int),
	}
}
