# vid

A terminal video tool for rotation, concatenation, and clip extraction. Run it
with no arguments for an interactive TUI, or pass files/flags for quick CLI use.

## Features

- **Rotate** videos 90, 180, or 270 degrees with optional preview before committing
- **Concatenate** multiple videos sorted by creation time (stream copy or re-encode)
- **Extract clips** — open a video in mpv, press `i`/`o` to mark start/end, clips auto-populate
- **Per-video rotation** when combining clips recorded in different orientations
- **Hardware acceleration** — auto-detects VAAPI, NVENC, VideoToolbox, etc.
- **Batch rename** files to `1.MOV`, `2.MOV`, … by creation time
- **Network-safe** — automatically copies files from network shares to local storage before processing
- **Dry run** mode to preview file order and operations without writing anything

## Requirements

| Tool | Purpose |
|------|---------|
| `ffmpeg` | Video encoding |
| `ffprobe` | Video metadata |
| `ffplay` | Rotation preview |
| `mpv` | Interactive clip marking |
| `chafa` | *(optional)* Thumbnail previews in file picker |

```
# Arch Linux
sudo pacman -S ffmpeg mpv

# Ubuntu/Debian
sudo apt install ffmpeg mpv

# macOS
brew install ffmpeg mpv
```

## Installation

```bash
git clone https://github.com/thrillho93/vid-tui.git
cd vid-tui
make        # builds ./vid
```

Or install directly to `~/bin`:

```bash
make install
```

## Usage

```
vid                        # interactive TUI (recommended)
vid -i                     # interactive TUI (explicit flag)
vid [OPTIONS] <input>...   # CLI mode
```

### Options

| Flag | Description |
|------|-------------|
| `-i, --interactive` | Launch interactive TUI |
| `-o, --output FILE` | Output filename (default: `combined.mp4`) |
| `-e, --extension EXT` | File extension filter (default: `MOV`) |
| `-R, --rotate DEG` | Rotate 90, 180, or 270 degrees clockwise |
| `-P, --preview` | Preview rotation with ffplay before processing |
| `-r, --reencode` | Force re-encode instead of stream copy |
| `-n, --rename` | Rename files by creation time (`1.MOV`, `2.MOV`, …) |
| `-d, --dryrun` | Show order and operations without writing output |
| `-h, --help` | Show help |

### Examples

```bash
# Interactive mode (best way to use it)
vid

# Rotate a single clip
vid -R 90 -o rotated.mp4 clip.MOV

# Preview rotation first, then process
vid -R 90 -P clip.MOV

# Combine all MOV files in a directory by creation time
vid ~/Videos/*.MOV

# Combine everything in a folder
vid -o merged.mp4 ~/Downloads/footage

# Preview file order without creating output
vid -d ~/Videos

# Rename files to 1.MOV, 2.MOV, ... by creation time
vid -n ~/Videos
```

## Building

```bash
go build -o vid .
```

Requires Go 1.21+.
