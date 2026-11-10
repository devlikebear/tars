package execsandbox

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeHost describes a machine as plain data. Paths go through filepath.Abs
// so the same table holds on Windows.
type fakeHost struct {
	goos  string
	env   map[string]string
	home  string
	cache string
	temp  string
	// links maps a symlinked directory to its target.
	links map[string]string
	// dirs and files exist; everything else is Missing.
	dirs  []string
	files []string
	// repos maps a session folder to its git common dir.
	repos map[string]string
	goEnv [2]string
}

func abs(t *testing.T, p string) string {
	t.Helper()
	a, err := filepath.Abs(filepath.FromSlash(p))
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func (f fakeHost) host(t *testing.T) host {
	links := map[string]string{}
	for from, to := range f.links {
		links[abs(t, from)] = abs(t, to)
	}
	kinds := map[string]Kind{}
	for _, d := range f.dirs {
		kinds[abs(t, d)] = Dir
	}
	for _, p := range f.files {
		kinds[abs(t, p)] = File
	}
	resolve := func(p string) string {
		for from, to := range links {
			if p == from || strings.HasPrefix(p, from+string(filepath.Separator)) {
				return to + strings.TrimPrefix(p, from)
			}
		}
		return p
	}
	return host{
		goos:     f.goos,
		getenv:   func(k string) string { return f.env[k] },
		homeDir:  func() (string, error) { return f.home, nil },
		cacheDir: func() (string, error) { return f.cache, nil },
		tempDir:  func() string { return f.temp },
		goEnv:    func() (string, string) { return f.goEnv[0], f.goEnv[1] },
		gitCommon: func(dir string) (string, bool) {
			common, ok := f.repos[dir]
			return common, ok
		},
		lstat: func(p string) (os.FileInfo, error) {
			kind, ok := kinds[p]
			if !ok {
				return nil, fs.ErrNotExist
			}
			return fakeInfo{dir: kind == Dir}, nil
		},
		realpath: func(p string) (string, error) {
			real := resolve(p)
			if _, ok := kinds[real]; !ok {
				return "", errors.New("no such file")
			}
			return real, nil
		},
	}
}

type fakeInfo struct{ dir bool }

func (i fakeInfo) Name() string       { return "" }
func (i fakeInfo) Size() int64        { return 0 }
func (i fakeInfo) Mode() fs.FileMode  { return 0 }
func (i fakeInfo) ModTime() time.Time { return time.Time{} }
func (i fakeInfo) IsDir() bool        { return i.dir }
func (i fakeInfo) Sys() any           { return nil }

func paths(list []Path) []string {
	out := make([]string, len(list))
	for i, p := range list {
		out[i] = p.Path
	}
	return out
}

func has(list []Path, want string) (Path, bool) {
	for _, p := range list {
		if p.Path == want {
			return p, true
		}
	}
	return Path{}, false
}

func TestResolveDefaults(t *testing.T) {
	f := fakeHost{
		goos:  "darwin",
		env:   map[string]string{"SSH_AUTH_SOCK": "/var/run/agent.sock"},
		home:  "/home/mac",
		cache: "/home/mac/Library/Caches",
		temp:  "/var/folders/xy/T",
		links: map[string]string{"/var": "/private/var", "/tmp": "/private/tmp"},
		dirs: []string{
			"/home/mac/repo-wt", "/private/var/folders/xy/T", "/private/tmp",
			"/home/mac/repo/.git", "/home/mac/repo/.git/hooks", "/home/mac/.ssh",
			"/home/mac/Library/Caches/go-build",
		},
		files: []string{"/home/mac/.netrc", "/private/var/run/agent.sock", "/home/mac/repo/.git/config"},
		repos: map[string]string{abs(t, "/home/mac/repo-wt"): abs(t, "/home/mac/repo/.git")},
	}
	p := f.host(t).resolve(Options{Dirs: []string{"/home/mac/repo-wt"}})

	for _, want := range []string{
		"/home/mac/repo-wt",
		"/private/var/folders/xy/T", // TMPDIR, symlink resolved
		"/private/tmp",
		"/home/mac/repo/.git", // a worktree commits into the main checkout's .git
		"/home/mac/Library/Caches/go-build",
		"/home/mac/go/pkg/mod",
		"/home/mac/.npm",
	} {
		if _, ok := has(p.Writable, abs(t, want)); !ok {
			t.Errorf("writable lacks %s: %v", want, paths(p.Writable))
		}
	}
	for _, want := range []string{
		"/home/mac/repo/.git/hooks", "/home/mac/repo/.git/config",
		"/home/mac/repo-wt/.tars", "/home/mac/repo-wt/.claude", "/home/mac/repo-wt/.mcp.json",
	} {
		if _, ok := has(p.ReadOnly, abs(t, want)); !ok {
			t.Errorf("read-only lacks %s: %v", want, paths(p.ReadOnly))
		}
	}
	for _, want := range []string{
		"/home/mac/.ssh", "/home/mac/.aws", "/home/mac/.config/gh", "/home/mac/.netrc",
		"/home/mac/Library/Keychains",
		"/private/var/run/agent.sock", // the ssh agent, symlink resolved
	} {
		if _, ok := has(p.Hidden, abs(t, want)); !ok {
			t.Errorf("hidden lacks %s: %v", want, paths(p.Hidden))
		}
	}

	kinds := map[string]Kind{"/home/mac/.ssh": Dir, "/home/mac/.netrc": File, "/home/mac/.aws": Missing}
	for path, want := range kinds {
		if got, _ := has(p.Hidden, abs(t, path)); got.Kind != want {
			t.Errorf("%s kind = %v, want %v", path, got.Kind, want)
		}
	}
	if p.Network {
		t.Error("network must stay off unless asked for")
	}
}

func TestResolveOptionsAndHostVariants(t *testing.T) {
	f := fakeHost{
		goos:  "linux",
		env:   map[string]string{"GOCACHE": "/env/gocache", "GOMODCACHE": "/env/gomod", "npm_config_cache": "/env/npm"},
		home:  "/home/u",
		cache: "/home/u/.cache",
		temp:  "/tmp",
	}
	p := f.host(t).resolve(Options{
		Dirs:     []string{"/work", "/work", " "},
		Writable: []string{"~/.cache/pip", "", "/opt/out"},
		ReadOnly: []string{"/opt/out/keep"},
		Hidden:   []string{"~/.secrets", "~"},
		Network:  true,
	})
	if !p.Network {
		t.Error("network option lost")
	}
	if got := strings.Count(strings.Join(paths(p.Writable), "\n"), abs(t, "/work")+"\n"); got > 1 {
		t.Errorf("/work listed %d times: %v", got, paths(p.Writable))
	}
	for _, want := range []string{"/home/u/.cache/pip", "/opt/out", "/env/gocache", "/env/gomod", "/env/npm", "/tmp"} {
		if _, ok := has(p.Writable, abs(t, want)); !ok {
			t.Errorf("writable lacks %s: %v", want, paths(p.Writable))
		}
	}
	if _, ok := has(p.ReadOnly, abs(t, "/opt/out/keep")); !ok {
		t.Errorf("read-only lacks /opt/out/keep: %v", paths(p.ReadOnly))
	}
	for _, want := range []string{"/home/u/.secrets", "/home/u"} {
		if _, ok := has(p.Hidden, abs(t, want)); !ok {
			t.Errorf("hidden lacks %s: %v", want, paths(p.Hidden))
		}
	}
	if _, ok := has(p.Hidden, abs(t, "/home/u/Library/Keychains")); ok {
		t.Error("the macOS keychain path is for darwin only")
	}

	// go env wins over the environment; GOCACHE=off is not a directory.
	f.goEnv = [2]string{"off", "/goenv/mod"}
	f.env = map[string]string{"GOPATH": "/gp1" + string(os.PathListSeparator) + "/gp2"}
	p = f.host(t).resolve(Options{})
	if _, ok := has(p.Writable, abs(t, "/goenv/mod")); !ok {
		t.Errorf("go env GOMODCACHE ignored: %v", paths(p.Writable))
	}
	for _, p := range paths(p.Writable) {
		if strings.HasSuffix(p, "off") {
			t.Errorf("GOCACHE=off became a path: %s", p)
		}
	}

	// GOPATH's first entry holds the module cache when nothing else says.
	f.goEnv = [2]string{}
	p = f.host(t).resolve(Options{})
	if _, ok := has(p.Writable, abs(t, "/gp1/pkg/mod")); !ok {
		t.Errorf("GOPATH module cache missing: %v", paths(p.Writable))
	}

	// No home directory: no home-relative paths at all.
	f.home = ""
	f.goos = "windows"
	f.temp = "/wintemp"
	p = f.host(t).resolve(Options{Hidden: []string{"~/x"}})
	if len(p.Hidden) != 1 {
		t.Errorf("hidden = %v, want only the option", paths(p.Hidden))
	}
	if _, ok := has(p.Writable, abs(t, "/tmp")); ok {
		t.Error("/tmp is not added on windows")
	}
}

func TestResolveUsesTheRealHost(t *testing.T) {
	dir := t.TempDir()
	p := Resolve(Options{Dirs: []string{dir}})
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := has(p.Writable, real); !ok || got.Kind != Dir {
		t.Fatalf("writable lacks %s as a dir: %+v", real, p.Writable)
	}
}
