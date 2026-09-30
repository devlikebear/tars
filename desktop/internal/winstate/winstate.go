// Package winstate remembers where the shell's windows were, so they reopen
// at the same place and size: the console window, and the chat windows that
// were open when the shell quit.
package winstate

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// Rect is a window or screen area in screen coordinates.
type Rect struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// Valid reports whether r has an area; a zero Rect means "never saved".
func (r Rect) Valid() bool { return r.Width > 0 && r.Height > 0 }

// Window is one window's place.
type Window struct {
	Bounds    Rect `json:"bounds,omitzero"`
	Maximised bool `json:"maximised,omitempty"`
}

// SessionWindow is a chat open in a window of its own.
type SessionWindow struct {
	ID string `json:"id"`
	Window
}

// State is what is saved between runs. The console window's place sits at
// the top level, as the first version of the file had it.
type State struct {
	Window
	// Sessions are the chat windows open at the last save, oldest first.
	Sessions []SessionWindow `json:"sessions,omitempty"`
}

// MinWidth and MinHeight are the smallest window the console lays out in.
const (
	MinWidth  = 720
	MinHeight = 480
)

// MaxSessions caps the chat windows the shell keeps open. A tars:// link can
// come from any page; a page opening links in a loop must not bury the
// desktop in windows.
const MaxSessions = 8

// maxSide bounds a coordinate or size when there is no screen to fit to:
// larger than any real desktop, small enough that nothing overflows.
const maxSide = 16384

// DefaultPath is <user config dir>/tars-desktop/window.json.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "tars-desktop", "window.json"), nil
}

// Load reads the saved state. A missing or unreadable file reports ok=false
// so the windows open at their default place. Entries that make no sense (a
// console window without an area, a chat without a usable id, the same chat
// twice, more than MaxSessions chats) are dropped, not fatal.
func Load(path string, validID func(string) bool) (State, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return State{}, false
	}
	var st State
	if json.Unmarshal(raw, &st) != nil {
		return State{}, false
	}
	if !st.Bounds.Valid() {
		st.Window = Window{}
	}
	seen := map[string]bool{}
	kept := st.Sessions[:0]
	for _, sw := range st.Sessions {
		if len(kept) == MaxSessions || seen[sw.ID] || validID == nil || !validID(sw.ID) {
			continue
		}
		seen[sw.ID] = true
		if !sw.Bounds.Valid() {
			sw.Window = Window{}
		}
		kept = append(kept, sw)
	}
	st.Sessions = kept
	if len(st.Sessions) == 0 {
		st.Sessions = nil
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

// Store holds the state in memory and writes the file on every change, so
// the console window and the chat windows, which save at different times,
// never overwrite each other's part. An empty path keeps it in memory only.
type Store struct {
	path string

	mu sync.Mutex
	st State
}

// Open loads the store at path; validID accepts a chat id (see Load).
func Open(path string, validID func(string) bool) *Store {
	s := &Store{path: path}
	if path != "" {
		s.st, _ = Load(path, validID)
	}
	return s
}

// Main is the console window's saved place; ok is false before it was ever
// saved.
func (s *Store) Main() (Window, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.st.Window, s.st.Bounds.Valid()
}

// SetMain records the console window's place.
func (s *Store) SetMain(w Window) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.st.Window = merge(s.st.Window, w)
	return s.saveLocked()
}

// Sessions lists the chat windows to reopen, oldest first.
func (s *Store) Sessions() []SessionWindow {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]SessionWindow(nil), s.st.Sessions...)
}

// Session is chat id's saved window; ok is false when it has none.
func (s *Store) Session(id string) (SessionWindow, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if i := s.indexLocked(id); i >= 0 {
		return s.st.Sessions[i], true
	}
	return SessionWindow{}, false
}

// AddSession records that chat id has a window open. It reports false when
// MaxSessions chats already have one; adding an open chat again is a no-op.
func (s *Store) AddSession(id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.indexLocked(id) >= 0 {
		return true, nil
	}
	if len(s.st.Sessions) >= MaxSessions {
		return false, nil
	}
	s.st.Sessions = append(s.st.Sessions, SessionWindow{ID: id})
	return true, s.saveLocked()
}

// SetSession records where chat id's window is. A chat without a window
// (closed meanwhile) is left alone.
func (s *Store) SetSession(id string, w Window) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.indexLocked(id)
	if i < 0 {
		return nil
	}
	s.st.Sessions[i].Window = merge(s.st.Sessions[i].Window, w)
	return s.saveLocked()
}

// RemoveSession forgets chat id's window: it was closed, so it does not
// reopen on the next start.
func (s *Store) RemoveSession(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.indexLocked(id)
	if i < 0 {
		return nil
	}
	s.st.Sessions = append(s.st.Sessions[:i], s.st.Sessions[i+1:]...)
	if len(s.st.Sessions) == 0 {
		s.st.Sessions = nil
	}
	return s.saveLocked()
}

func (s *Store) indexLocked(id string) int {
	for i, sw := range s.st.Sessions {
		if sw.ID == id {
			return i
		}
	}
	return -1
}

func (s *Store) saveLocked() error {
	if s.path == "" {
		return nil
	}
	return Save(s.path, s.st)
}

// merge is the place to save when a window reports now: a maximised window
// keeps the size it had before it was maximised, so un-maximising after a
// restart goes back to it.
func merge(old, now Window) Window {
	if now.Maximised && old.Bounds.Valid() {
		now.Bounds = old.Bounds
	}
	return now
}

// Fit returns bounds the window can be shown at on the current screens: at
// least the minimum size, no larger than the screen it lands on, and with
// its top-left area on that screen. ok is false when saved has no area or
// lies off every screen (a monitor since unplugged): the window then opens
// at its default place. With no screen information at all, saved is only
// clamped to sane values, since there is nothing to check it against.
func Fit(saved Rect, screens []Rect) (Rect, bool) {
	if !saved.Valid() {
		return Rect{}, false
	}
	if len(screens) == 0 {
		return Rect{
			X:      clamp(saved.X, -maxSide, maxSide),
			Y:      clamp(saved.Y, -maxSide, maxSide),
			Width:  clamp(saved.Width, MinWidth, maxSide),
			Height: clamp(saved.Height, MinHeight, maxSide),
		}, true
	}
	var target Rect
	found := false
	for _, s := range screens {
		// The title bar (top-left 100x40) must be reachable to drag it.
		if overlaps(Rect{X: saved.X, Y: saved.Y, Width: 100, Height: 40}, s) {
			target, found = s, true
			break
		}
	}
	if !found {
		return Rect{}, false
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
