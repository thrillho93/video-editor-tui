package ffmpeg

import "fmt"

// RotationFilter returns the ffmpeg video filter string for the given rotation.
// Supported values: 90, 180, 270. Returns empty string for 0 or invalid values.
func RotationFilter(degrees int) string {
	switch degrees {
	case 90:
		return "transpose=1"
	case 180:
		return "hflip,vflip"
	case 270:
		return "transpose=2"
	default:
		return ""
	}
}

// EffectiveDimensions returns width and height after rotation.
// 90 and 270 degree rotations swap width and height.
func EffectiveDimensions(w, h, degrees int) (int, int) {
	switch degrees {
	case 90, 270:
		return h, w
	default:
		return w, h
	}
}

// ScalePadFilter returns a filter that scales and pads video to target dimensions.
func ScalePadFilter(targetW, targetH int) string {
	return fmt.Sprintf(
		"scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:black,setsar=1",
		targetW, targetH, targetW, targetH,
	)
}
