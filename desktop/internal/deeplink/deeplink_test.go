package deeplink

import (
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func absDir() string {
	if runtime.GOOS == "windows" {
		return `C:\work\repo`
	}
	return "/work/repo"
}

func TestParse(t *testing.T) {
	dir := absDir()
	ok := map[string]Link{
		"tars://":                       {Kind: Open, Path: "/console"},
		"TARS://open":                   {Kind: Open, Path: "/console"},
		"tars:open":                     {Kind: Open, Path: "/console"},
		"tars://session/abc-123":        {Kind: Open, Path: "/console/chat/abc-123"},
		"tars://chat/abc_1.2/":          {Kind: Open, Path: "/console/chat/abc_1.2"},
		"tars:session/abc":              {Kind: Open, Path: "/console/chat/abc"},
		"tars://session/abc?window=new": {Kind: OpenWindow, Path: "/console/chat/abc", SessionID: "abc"},
		"tars:chat/a.b?window=new":      {Kind: OpenWindow, Path: "/console/chat/a.b", SessionID: "a.b"},
		"tars://session/abc?other=1":    {Kind: Open, Path: "/console/chat/abc"},
		"tars://new?cwd=" + url.QueryEscape(dir+"/../repo"): {Kind: NewChat, Dir: filepath.Clean(dir)},
		"tars://new?isolate=1&cwd=" + url.QueryEscape(dir):  {Kind: NewChat, Dir: filepath.Clean(dir), Isolate: true},
		"tars://new?isolate=0&cwd=" + url.QueryEscape(dir):  {Kind: NewChat, Dir: filepath.Clean(dir)},
	}
	for raw, want := range ok {
		got, err := Parse(raw)
		if err != nil || got != want {
			t.Errorf("Parse(%q) = %+v, %v; want %+v", raw, got, err, want)
		}
	}
	bad := []string{
		"https://example.com",
		"tars://session",
		"tars://session/a/b",
		"tars://session/..%2F..%2Fv1",
		"tars://session/a%20x",
		"tars://session/..",
		"tars://session/abc?window=tab",
		"tars://session/..?window=new",
		"tars://session/a/b?window=new",
		"tars://open/extra",
		"tars://new",
		"tars://new?cwd=relative/dir",
		"tars://new?isolate=yes&cwd=" + url.QueryEscape(dir),
		"tars://new/x?cwd=" + url.QueryEscape(dir),
		"tars://approve/r1",
		"tars://%zz",
	}
	for _, raw := range bad {
		if got, err := Parse(raw); err == nil {
			t.Errorf("Parse(%q) accepted: %+v", raw, got)
		}
	}
}

func TestValidSessionID(t *testing.T) {
	for _, id := range []string{"abc", "a.b", "A_1-2", "..a"} {
		if !ValidSessionID(id) {
			t.Errorf("%q must be valid", id)
		}
	}
	for _, id := range []string{"", ".", "..", "a/b", "a b", "a%2F", string(make([]byte, 129))} {
		if ValidSessionID(id) {
			t.Errorf("%q must be rejected", id)
		}
	}
}

func TestFromArgs(t *testing.T) {
	if got, ok := FromArgs([]string{"--flag", " TARS://session/x "}); !ok || got != "TARS://session/x" {
		t.Fatalf("FromArgs = %q, %v", got, ok)
	}
	if _, ok := FromArgs([]string{"--hidden", "https://x"}); ok {
		t.Fatal("no link among the args")
	}
}

func TestInGitRepository(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "repo", "sub", "dir")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if InGitRepository(nested) {
		t.Fatal("no .git anywhere yet")
	}
	// A worktree or submodule has a .git file, not a folder.
	if err := os.WriteFile(filepath.Join(root, "repo", ".git"), []byte("gitdir: elsewhere\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !InGitRepository(nested) || !InGitRepository(filepath.Join(root, "repo")) {
		t.Fatal("folders under the .git entry are in the repository")
	}
	if InGitRepository(root) {
		t.Fatal("the parent is not")
	}
}
