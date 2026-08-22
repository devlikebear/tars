package fileuri

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestRoundTripPreservesPath is the property every caller depends on: a path
// turned into a URI and back must name the same file.
func TestRoundTripPreservesPath(t *testing.T) {
	want := filepath.Join(t.TempDir(), "nested", "artifact.txt")

	uri := New(want)
	got, err := Path(uri)
	if err != nil {
		t.Fatalf("Path(%q): %v", uri, err)
	}
	if got != want {
		t.Fatalf("round trip = %q, want %q", got, want)
	}
}

// TestNewHasNoHostAndForwardSlashes pins the shape that was wrong on Windows,
// where url.URL{Scheme: "file", Path: path} put the drive letter in the host
// and percent-encoded every separator.
func TestNewHasNoHostAndForwardSlashes(t *testing.T) {
	uri := New(filepath.Join(t.TempDir(), "artifact.txt"))
	if !strings.HasPrefix(uri, "file:///") {
		t.Fatalf("uri = %q, want a file:/// prefix", uri)
	}
	if strings.Contains(uri, "%5C") || strings.Contains(uri, `\`) {
		t.Fatalf("uri = %q, want forward slashes only", uri)
	}
}

func TestNewResolvesRelativePaths(t *testing.T) {
	t.Chdir(t.TempDir())

	uri := New("artifact.txt")
	path, err := Path(uri)
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if !filepath.IsAbs(path) {
		t.Fatalf("path = %q, want an absolute path", path)
	}
	if filepath.Base(path) != "artifact.txt" {
		t.Fatalf("path = %q, want it to end at the requested file", path)
	}
}

func TestPathRejectsUnusableURIs(t *testing.T) {
	cases := map[string]string{
		"other scheme":  "https://example.test/artifact.txt",
		"host present":  "file://example.test/artifact.txt",
		"no path":       "file://",
		"not a uri":     "::not a uri::",
		"malformed old": `file://C:%5Cdir%5Cartifact.txt`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if got, err := Path(raw); err == nil {
				t.Fatalf("Path(%q) = %q, want an error", raw, got)
			}
		})
	}
}
