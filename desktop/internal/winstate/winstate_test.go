package winstate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "window.json")
	if _, ok := Load(path); ok {
		t.Fatal("a missing file must not load")
	}
	want := State{Bounds: Rect{X: 10, Y: 20, Width: 1200, Height: 800}, Maximised: true}
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	got, ok := Load(path)
	if !ok || got != want {
		t.Fatalf("loaded %+v, %v", got, ok)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("the temporary file must be renamed away")
	}
	for _, body := range []string{"{", `{"bounds":{"width":0,"height":10}}`} {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, ok := Load(path); ok {
			t.Fatalf("%q must not load", body)
		}
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
		{"unplugged monitor moves to the first screen", Rect{-1200, 50, 1000, 700}, []Rect{main}, Rect{0, 50, 1000, 700}},
		{"too small grows", Rect{10, 10, 100, 100}, []Rect{main}, Rect{10, 10, MinWidth, MinHeight}},
		{"too large shrinks", Rect{0, 0, 4000, 3000}, []Rect{main}, Rect{0, 0, 1920, 1080}},
		{"hanging off the edge slides in", Rect{1800, 900, 800, 600}, []Rect{main}, Rect{1120, 480, 800, 600}},
		{"tiny screen", Rect{0, 0, 1000, 800}, []Rect{small}, Rect{0, 0, 640, 400}},
	}
	for _, c := range cases {
		got, ok := Fit(c.saved, c.screens)
		if !ok || got != c.want {
			t.Errorf("%s: Fit = %+v, want %+v", c.name, got, c.want)
		}
	}
	if _, ok := Fit(Rect{Width: 800, Height: 600}, nil); ok {
		t.Fatal("no screens, nothing to fit")
	}
}
