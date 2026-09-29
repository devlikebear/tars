package protocol

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDesktopEntry(t *testing.T) {
	entry := DesktopEntry(`/opt/My Apps/tars-desktop`)
	for _, want := range []string{
		"[Desktop Entry]\n",
		"Exec=\"/opt/My Apps/tars-desktop\" %u\n",
		"MimeType=x-scheme-handler/tars;\n",
	} {
		if !strings.Contains(entry, want) {
			t.Errorf("entry lacks %q:\n%s", want, entry)
		}
	}
}

func TestQuoteExecArg(t *testing.T) {
	cases := map[string]string{
		`/usr/bin/tars-desktop`: `"/usr/bin/tars-desktop"`,
		`/a "b"/c`:              `"/a \\"b\\"/c"`,
		"/a/$HOME/`x`":          "\"/a/\\\\$HOME/\\\\`x\\\\`\"",
		`/a\b`:                  `"/a\\\\b"`,
		`/100%/x`:               `"/100%%/x"`,
	}
	for in, want := range cases {
		if got := quoteExecArg(in); got != want {
			t.Errorf("quoteExecArg(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestWindowsCommand(t *testing.T) {
	if got := WindowsCommand(`C:\Program Files\TARS\tars-desktop.exe`); got != `"C:\Program Files\TARS\tars-desktop.exe" "%1"` {
		t.Fatal(got)
	}
}

func TestDataHome(t *testing.T) {
	env := func(v string) func(string) string { return func(string) string { return v } }
	if got := DataHome(env("/xdg"), "/home/u"); got != "/xdg" {
		t.Fatal(got)
	}
	if got := DataHome(env("relative"), "/home/u"); got != filepath.Join("/home/u", ".local", "share") {
		t.Fatal(got)
	}
}

func TestLinuxRegistrar(t *testing.T) {
	data := t.TempDir()
	var ran [][]string
	r := LinuxRegistrar{DataHome: data, Run: func(name string, args ...string) error {
		ran = append(ran, append([]string{name}, args...))
		return nil
	}}
	changed, err := r.Register("/opt/tars-desktop")
	if err != nil || !changed {
		t.Fatalf("first register = %v, %v", changed, err)
	}
	raw, err := os.ReadFile(filepath.Join(data, "applications", DesktopFileName))
	if err != nil || string(raw) != DesktopEntry("/opt/tars-desktop") {
		t.Fatalf("entry = %q, %v", raw, err)
	}
	if len(ran) != 1 || strings.Join(ran[0], " ") != "xdg-mime default tars-desktop.desktop x-scheme-handler/tars" {
		t.Fatalf("ran %v", ran)
	}
	if changed, err := r.Register("/opt/tars-desktop"); err != nil || changed || len(ran) != 1 {
		t.Fatalf("unchanged register = %v, %v, ran %d", changed, err, len(ran))
	}
	r.Run = func(string, ...string) error { return errors.New("boom") }
	if changed, err := r.Register("/moved/tars-desktop"); !changed || err == nil {
		t.Fatalf("a failing xdg-mime = %v, %v", changed, err)
	}
	r.Run = nil
	if _, err := r.Register("/again"); err != nil {
		t.Fatal(err)
	}
	if _, err := (LinuxRegistrar{}).Register("/x"); err == nil {
		t.Fatal("no data home must fail")
	}
	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := (LinuxRegistrar{DataHome: blocked}).Register("/x"); err == nil {
		t.Fatal("a file in the way must fail")
	}
}
