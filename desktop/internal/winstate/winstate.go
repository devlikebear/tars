// Package winstate remembers where the console window was, so it reopens at
// the same place and size.
package winstate

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Rect is a window or screen area in screen coordinates.
type Rect struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// State is what is saved between runs.
type State struct {
	Bounds    Rect `json:"bounds"`
	Maximised bool `json:"maximised"`
}

// MinWidth and MinHeight are the smallest window the console lays out in.
const (
	MinWidth  = 720
	MinHeight = 480
)

// DefaultPath is <user config dir>/tars-desktop/window.json.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "tars-desktop", "window.json"), nil
}

// Load reads the saved state. A missing or unreadable file reports ok=false
// so the window opens at its default place.
func Load(path string) (State, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return State{}, false
	}
	var st State
	if json.Unmarshal(raw, &st) != nil || st.Bounds.Width <= 0 || st.Bounds.Height <= 0 {
		return State{}, false
	}
	return st, true
}

// Save writes the state, creating the directory. The write goes to a
// temporary file first so a crash never leaves half a file.
func Save(path string, st State) error {
	if path == "" {
		return errors.New("winstate: empty path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Fit returns bounds the window can be shown at on the current screens:
// at least the minimum size, no larger than the screen it lands on, and
// with its top-left area on some screen. A window saved on a monitor that
// is now unplugged moves to the first screen. ok is false when there are no
// screens to fit to.
func Fit(saved Rect, screens []Rect) (Rect, bool) {
	if len(screens) == 0 {
		return Rect{}, false
	}
	target := screens[0]
	for _, s := range screens {
		// The title bar (top-left 100x40) must be reachable to drag it.
		if overlaps(Rect{X: saved.X, Y: saved.Y, Width: 100, Height: 40}, s) {
			target = s
			break
		}
	}
	out := saved
	out.Width = clamp(out.Width, min(MinWidth, target.Width), target.Width)
	out.Height = clamp(out.Height, min(MinHeight, target.Height), target.Height)
	out.X = clamp(out.X, target.X, target.X+target.Width-out.Width)
	out.Y = clamp(out.Y, target.Y, target.Y+target.Height-out.Height)
	return out, true
}

func overlaps(a, b Rect) bool {
	return a.X < b.X+b.Width && b.X < a.X+a.Width && a.Y < b.Y+b.Height && b.Y < a.Y+a.Height
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
