package winstate

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func anyID(id string) bool { return id != "" && !strings.Contains(id, "/") }

func TestSaveLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "window.json")
	if _, ok := Load(path, anyID); ok {
		t.Fatal("a missing file must not load")
	}
	want := State{
		Window:   Window{Bounds: Rect{X: 10, Y: 20, Width: 1200, Height: 800}, Maximised: true},
		Sessions: []SessionWindow{{ID: "a", Window: Window{Bounds: Rect{1, 2, 900, 700}}}, {ID: "b"}},
	}
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	got, ok := Load(path, anyID)
	if !ok || !reflect.DeepEqual(got, want) {
		t.Fatalf("loaded %+v, %v", got, ok)
	}
	if info, err := os.Stat(path); err != nil || (info.Mode().Perm()&0o077 != 0 && os.PathSeparator == '/') {
		t.Fatalf("the file must be private: %v %v", info.Mode(), err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("the temporary file must be renamed away")
	}
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := Load(path, anyID); ok {
		t.Fatal("broken JSON must not load")
	}
	if err := Save("", want); err == nil {
		t.Fatal("an empty path must fail")
	}
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Save(filepath.Join(blocker, "window.json"), want); err == nil {
		t.Fatal("a file in the way of the directory must fail")
	}
}

func TestLoadFirstVersion(t *testing.T) {
	// The file before chat windows: only the console window's place.
	path := filepath.Join(t.TempDir(), "window.json")
	body := `{"bounds":{"x":5,"y":6,"width":1000,"height":700},"maximised":true}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	got, ok := Load(path, anyID)
	want := State{Window: Window{Bounds: Rect{5, 6, 1000, 700}, Maximised: true}}
	if !ok || !reflect.DeepEqual(got, want) {
		t.Fatalf("loaded %+v, %v", got, ok)
	}
}

func TestLoadDropsNonsense(t *testing.T) {
	path := filepath.Join(t.TempDir(), "window.json")
	var sessions []string
	for i := range MaxSessions + 2 {
		sessions = append(sessions, fmt.Sprintf(`{"id":"s%d"}`, i))
	}
	body := `{"bounds":{"width":0,"height":10},"maximised":true,"sessions":[` +
		`{"id":"../x"},{"id":""},{"id":"dup","bounds":{"width":-1,"height":5},"maximised":true},{"id":"dup"},` +
		strings.Join(sessions, ",") + `]}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	got, ok := Load(path, anyID)
	if !ok {
		t.Fatal("a file with bad entries still loads")
	}
	if got.Window != (Window{}) {
		t.Fatalf("a console window without an area is dropped: %+v", got.Window)
	}
	if len(got.Sessions) != MaxSessions || got.Sessions[0] != (SessionWindow{ID: "dup"}) || got.Sessions[1].ID != "s0" {
		t.Fatalf("sessions = %+v", got.Sessions)
	}
	if got, _ := Load(path, nil); got.Sessions != nil {
		t.Fatal("without an id check no chat is trusted")
	}
	if err := os.WriteFile(path, []byte(`{"sessions":[{"id":"../x"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, _ := Load(path, anyID); got.Sessions != nil {
		t.Fatalf("no valid chat leaves nil, got %+v", got.Sessions)
	}
}

func TestStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "window.json")
	s := Open(path, anyID)
	if _, ok := s.Main(); ok {
		t.Fatal("nothing saved yet")
	}

	normal := Window{Bounds: Rect{10, 10, 1000, 700}}
	if err := s.SetMain(normal); err != nil {
		t.Fatal(err)
	}
	// Maximised keeps the size from before, so un-maximising restores it.
	if err := s.SetMain(Window{Bounds: Rect{0, 0, 1920, 1080}, Maximised: true}); err != nil {
		t.Fatal(err)
	}
	if got, ok := s.Main(); !ok || got != (Window{Bounds: normal.Bounds, Maximised: true}) {
		t.Fatalf("main = %+v, %v", got, ok)
	}

	for _, id := range []string{"a", "b", "a"} {
		if ok, err := s.AddSession(id); !ok || err != nil {
			t.Fatalf("add %s: %v %v", id, ok, err)
		}
	}
	if raw, _ := os.ReadFile(path); strings.Count(string(raw), `"bounds"`) != 1 {
		t.Fatalf("a chat window not placed yet has no bounds in the file: %s", raw)
	}
	if err := s.SetSession("b", Window{Bounds: Rect{50, 60, 800, 600}}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSession("gone", normal); err != nil {
		t.Fatal("a chat without a window is ignored")
	}
	if sw, ok := s.Session("b"); !ok || sw.Bounds != (Rect{50, 60, 800, 600}) {
		t.Fatalf("session b = %+v, %v", sw, ok)
	}
	if _, ok := s.Session("gone"); ok {
		t.Fatal("no such chat window")
	}

	// A second store reads what the first wrote.
	again := Open(path, anyID)
	if got := again.Sessions(); len(got) != 2 || got[0].ID != "a" || got[1].Bounds.Width != 800 {
		t.Fatalf("reloaded sessions = %+v", got)
	}
	if got, _ := again.Main(); got.Bounds != normal.Bounds || !got.Maximised {
		t.Fatalf("reloaded main = %+v", got)
	}

	if err := s.RemoveSession("a"); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveSession("a"); err != nil {
		t.Fatal("removing twice is fine")
	}
	if err := s.RemoveSession("b"); err != nil {
		t.Fatal(err)
	}
	if got := Open(path, anyID).Sessions(); len(got) != 0 {
		t.Fatalf("closed windows must not reopen: %+v", got)
	}
	raw, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(raw), "sessions") {
		t.Fatalf("an empty list is left out of the file: %s", raw)
	}
}

func TestStoreCap(t *testing.T) {
	s := Open("", anyID)
	for i := range MaxSessions {
		if ok, err := s.AddSession(fmt.Sprint(i)); !ok || err != nil {
			t.Fatalf("add %d: %v %v", i, ok, err)
		}
	}
	if ok, _ := s.AddSession("one-more"); ok {
		t.Fatal("the cap must hold")
	}
	if ok, _ := s.AddSession("3"); !ok {
		t.Fatal("an open chat is not a new window")
	}
	if len(s.Sessions()) != MaxSessions {
		t.Fatal("in-memory store keeps the list")
	}
}

func TestStoreSaveError(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	s := Open(filepath.Join(blocker, "window.json"), anyID)
	if err := s.SetMain(Window{Bounds: Rect{0, 0, 800, 600}}); err == nil {
		t.Fatal("a failed write must be reported")
	}
	if _, err := s.AddSession("a"); err == nil {
		t.Fatal("a failed write must be reported")
	}
	if got, ok := s.Main(); !ok || got.Bounds.Width != 800 {
		t.Fatal("the memory copy is kept even when the file cannot be written")
	}
}

func TestDefaultPath(t *testing.T) {
	path, err := DefaultPath()
	if err != nil {
		t.Skipf("no user config dir: %v", err)
	}
	if !strings.HasSuffix(path, filepath.Join("tars-desktop", "window.json")) {
		t.Fatalf("path = %q", path)
	}
}

func TestFit(t *testing.T) {
	main := Rect{X: 0, Y: 0, Width: 1920, Height: 1080}
	left := Rect{X: -1280, Y: 0, Width: 1280, Height: 1024}
	small := Rect{X: 0, Y: 0, Width: 640, Height: 400}
	cases := []struct {
		name    string
		saved   Rect
		screens []Rect
		want    Rect
	}{
		{"fits as is", Rect{100, 100, 1200, 800}, []Rect{main}, Rect{100, 100, 1200, 800}},
		{"second monitor kept", Rect{-1200, 50, 1000, 700}, []Rect{main, left}, Rect{-1200, 50, 1000, 700}},
		{"too small grows", Rect{10, 10, 100, 100}, []Rect{main}, Rect{10, 10, MinWidth, MinHeight}},
		{"too large shrinks", Rect{0, 0, 4000, 3000}, []Rect{main}, Rect{0, 0, 1920, 1080}},
		{"hanging off the edge slides in", Rect{1800, 900, 800, 600}, []Rect{main}, Rect{1120, 480, 800, 600}},
		{"tiny screen", Rect{0, 0, 1000, 800}, []Rect{small}, Rect{0, 0, 640, 400}},
		{"no screens: kept when sane", Rect{-500, 20, 1000, 700}, nil, Rect{-500, 20, 1000, 700}},
		{"no screens: clamped", Rect{-99999, 99999, 100, 99999}, nil, Rect{-maxSide, maxSide, MinWidth, maxSide}},
	}
	for _, c := range cases {
		got, ok := Fit(c.saved, c.screens)
		if !ok || got != c.want {
			t.Errorf("%s: Fit = %+v, %v, want %+v", c.name, got, ok, c.want)
		}
	}
	// Off every screen (an unplugged monitor), or never saved: the default
	// place.
	if _, ok := Fit(Rect{-1200, 50, 1000, 700}, []Rect{main}); ok {
		t.Fatal("a window off every screen must fall back to the default place")
	}
	if _, ok := Fit(Rect{}, []Rect{main}); ok {
		t.Fatal("a window never saved has no place to fit")
	}
	if _, ok := Fit(Rect{}, nil); ok {
		t.Fatal("a window never saved has no place to clamp")
	}
}
