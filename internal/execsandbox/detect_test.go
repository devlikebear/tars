package execsandbox

import (
	"errors"
	"strings"
	"testing"
)

func TestDetect(t *testing.T) {
	onPath := func(found ...string) func(string) (string, error) {
		return func(name string) (string, error) {
			for _, f := range found {
				if f == name {
					if strings.HasPrefix(name, "/") {
						return name, nil
					}
					return "/usr/bin/" + name, nil
				}
			}
			return "", errors.New("not found")
		}
	}
	cases := []struct {
		name     string
		goos     string
		lookPath func(string) (string, error)
		want     Backend
		wantErr  string
	}{
		{"macOS on PATH", "darwin", onPath("sandbox-exec"), Backend{Name: Seatbelt, Path: "/usr/bin/sandbox-exec"}, ""},
		{"macOS off PATH falls back to /usr/bin", "darwin", onPath("/usr/bin/sandbox-exec"), Backend{Name: Seatbelt, Path: "/usr/bin/sandbox-exec"}, ""},
		{"macOS without sandbox-exec", "darwin", onPath(), Backend{}, "sandbox-exec not found"},
		{"Linux with bwrap", "linux", onPath("bwrap"), Backend{Name: Bubblewrap, Path: "/usr/bin/bwrap"}, ""},
		{"Linux without bwrap", "linux", onPath(), Backend{}, "bwrap not found"},
		{"Windows is unsupported", "windows", onPath("bwrap", "sandbox-exec"), Backend{}, "supported on windows"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := detect(tc.goos, tc.lookPath)
			if tc.wantErr != "" {
				if err == nil || !errors.Is(err, ErrUnavailable) || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want ErrUnavailable with %q", err, tc.wantErr)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("detect = %+v, %v; want %+v", got, err, tc.want)
			}
		})
	}
}
