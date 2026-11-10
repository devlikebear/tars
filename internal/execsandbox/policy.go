// Package execsandbox runs a shell command inside an operating-system
// sandbox: Seatbelt (sandbox-exec) on macOS, bubblewrap (bwrap) on Linux.
//
// A Policy says what one command may touch: the subtrees it may write, the
// ones that stay read-only inside them, the paths it may neither read nor
// write, and whether it may use the network. Resolve builds a Policy from the
// session's folders and the host (temp dir, git dirs, build caches, credential
// folders); SeatbeltProfile and BubblewrapArgs turn one into the backend's
// input and are pure, so they are tested as tables on every platform.
//
// The sandbox is the boundary for an unattended exec call. It is not a
// replacement for the exec tool's command validation, and it does not decide
// approvals.
package execsandbox

import (
	"context"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Kind records what a policy path is on disk when the policy was resolved.
// Seatbelt rules match paths whether or not they exist; bubblewrap can only
// mount over something that does, so it skips Missing paths.
type Kind int

const (
	Missing Kind = iota
	Dir
	File
)

// Path is one absolute, symlink-resolved path of a Policy.
type Path struct {
	Path string
	Kind Kind
}

// Policy is what one sandboxed command may touch. Everything not listed is
// readable and not writable.
type Policy struct {
	// Writable subtrees: the session folders, temp dirs, git dirs and build
	// caches.
	Writable []Path
	// ReadOnly subtrees stay unwritable even inside a Writable one: git hooks
	// and config, and the folders that hold settings TARS or a CLI provider
	// would later run unsandboxed.
	ReadOnly []Path
	// Hidden paths can be neither read nor written: credentials.
	Hidden []Path
	// Network allows outbound connections beyond loopback.
	Network bool
}

// Options are the inputs to Resolve.
type Options struct {
	// Dirs are the session's folders. Each is writable, and the git
	// repository each belongs to has its common dir made writable (a worktree
	// commits into it) with hooks and config kept read-only.
	Dirs []string
	// Writable, ReadOnly and Hidden add to the defaults.
	Writable []string
	ReadOnly []string
	Hidden   []string
	Network  bool
}

// homeCredentialPaths are hidden under the user's home directory. They hold
// keys and tokens a shell command has no business reading — and that a
// command tricked into exfiltrating would reach first.
var homeCredentialPaths = []string{
	".ssh",
	".aws",
	".azure",
	".gnupg",
	".kube",
	".docker",
	".netrc",
	".git-credentials",
	".config/gh",
	".config/gcloud",
	".config/op",
}

// darwinCredentialPaths are hidden under home on macOS only.
var darwinCredentialPaths = []string{
	"Library/Keychains",
}

// protectedDirEntries stay read-only inside every session folder: settings
// that TARS (.tars) or the Claude Code CLI (.claude, .mcp.json) read and act
// on outside the sandbox — hooks, MCP servers, permission modes.
var protectedDirEntries = []string{".tars", ".claude", ".mcp.json"}

// host is everything Resolve reads from the machine, swappable in tests.
type host struct {
	goos      string
	getenv    func(string) string
	homeDir   func() (string, error)
	cacheDir  func() (string, error)
	tempDir   func() string
	goEnv     func() (gocache, gomodcache string)
	gitCommon func(dir string) (string, bool)
	lstat     func(string) (os.FileInfo, error)
	realpath  func(string) (string, error)
}

func defaultHost() host {
	return host{
		goos:      runtime.GOOS,
		getenv:    os.Getenv,
		homeDir:   os.UserHomeDir,
		cacheDir:  os.UserCacheDir,
		tempDir:   os.TempDir,
		goEnv:     goEnvCaches,
		gitCommon: gitCommonDir,
		lstat:     os.Lstat,
		realpath:  filepath.EvalSymlinks,
	}
}

// Resolve builds the Policy for one command from the session folders and
// the host.
func Resolve(o Options) Policy {
	return defaultHost().resolve(o)
}

func (h host) resolve(o Options) Policy {
	var b policyBuilder
	b.h = h

	home, _ := h.homeDir()
	for _, dir := range o.Dirs {
		b.add(&b.writable, dir)
	}
	for _, dir := range o.Writable {
		b.add(&b.writable, expandHome(dir, home))
	}
	b.add(&b.writable, h.tempDir())
	if h.goos != "windows" {
		b.add(&b.writable, "/tmp")
	}
	b.addCaches(home)

	for _, raw := range o.Dirs {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		dir, err := filepath.Abs(strings.TrimSpace(raw))
		if err != nil {
			continue
		}
		for _, entry := range protectedDirEntries {
			b.add(&b.readOnly, filepath.Join(dir, entry))
		}
		if common, ok := h.gitCommon(dir); ok {
			b.add(&b.writable, common)
			b.add(&b.readOnly, filepath.Join(common, "hooks"))
			b.add(&b.readOnly, filepath.Join(common, "config"))
		}
	}
	for _, p := range o.ReadOnly {
		b.add(&b.readOnly, expandHome(p, home))
	}

	if home != "" {
		for _, rel := range homeCredentialPaths {
			b.add(&b.hidden, filepath.Join(home, rel))
		}
		if h.goos == "darwin" {
			for _, rel := range darwinCredentialPaths {
				b.add(&b.hidden, filepath.Join(home, rel))
			}
		}
	}
	// The ssh agent signs with the user's keys without reading them.
	if sock := strings.TrimSpace(h.getenv("SSH_AUTH_SOCK")); sock != "" {
		b.add(&b.hidden, sock)
	}
	for _, p := range o.Hidden {
		b.add(&b.hidden, expandHome(p, home))
	}

	return Policy{Writable: b.writable, ReadOnly: b.readOnly, Hidden: b.hidden, Network: o.Network}
}

// addCaches makes the Go and npm build caches writable: without them every
// go build or npm install inside the sandbox fails.
func (b *policyBuilder) addCaches(home string) {
	h := b.h
	gocache, gomodcache := "", ""
	if h.goEnv != nil {
		gocache, gomodcache = h.goEnv()
	}
	if gocache == "" {
		gocache = h.getenv("GOCACHE")
	}
	if gocache == "" {
		if dir, err := h.cacheDir(); err == nil && dir != "" {
			gocache = filepath.Join(dir, "go-build")
		}
	}
	if gomodcache == "" {
		gomodcache = h.getenv("GOMODCACHE")
	}
	if gomodcache == "" {
		gopath := strings.TrimSpace(strings.Split(h.getenv("GOPATH"), string(os.PathListSeparator))[0])
		if gopath == "" && home != "" {
			gopath = filepath.Join(home, "go")
		}
		if gopath != "" {
			gomodcache = filepath.Join(gopath, "pkg", "mod")
		}
	}
	npmCache := h.getenv("npm_config_cache")
	if npmCache == "" && home != "" {
		npmCache = filepath.Join(home, ".npm")
	}
	for _, dir := range []string{gocache, gomodcache, npmCache} {
		// GOCACHE=off and other non-paths are not directories to allow.
		if isRootedPath(dir) {
			b.add(&b.writable, dir)
		}
	}
}

// isRootedPath reports whether p names a directory from the root rather than
// a word such as "off". A path rooted at "/" counts even on Windows, where
// filepath.IsAbs wants a drive letter: the sandbox itself runs only on macOS
// and Linux, and Resolve's tests describe those hosts on every platform.
func isRootedPath(p string) bool {
	return filepath.IsAbs(p) || path.IsAbs(filepath.ToSlash(p))
}

type policyBuilder struct {
	h        host
	writable []Path
	readOnly []Path
	hidden   []Path
}

func (b *policyBuilder) add(list *[]Path, raw string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return
	}
	abs, err := filepath.Abs(raw)
	if err != nil {
		return
	}
	resolved := b.resolveLinks(abs)
	for _, existing := range *list {
		if existing.Path == resolved {
			return
		}
	}
	*list = append(*list, Path{Path: resolved, Kind: b.kind(resolved)})
}

// resolveLinks returns abs with symlinks resolved, also for a path that does
// not exist yet: Seatbelt matches the real path (/private/var, not /var), so
// a rule written against the link would never match.
func (b *policyBuilder) resolveLinks(abs string) string {
	if real, err := b.h.realpath(abs); err == nil {
		return real
	}
	parent := filepath.Dir(abs)
	if parent == abs {
		return abs
	}
	return filepath.Join(b.resolveLinks(parent), filepath.Base(abs))
}

func (b *policyBuilder) kind(path string) Kind {
	info, err := b.h.lstat(path)
	if err != nil {
		return Missing
	}
	if info.IsDir() {
		return Dir
	}
	return File
}

func expandHome(p, home string) string {
	p = strings.TrimSpace(p)
	if home == "" {
		return p
	}
	if p == "~" {
		return home
	}
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(home, p[2:])
	}
	return p
}

const gitLookupTimeout = 2 * time.Second

// gitCommonDirs caches the repositories already found. Only hits are kept: a
// folder can become a repository later, and a miss costs one git call.
var gitCommonDirs sync.Map

// gitCommonDir returns the git common dir of the repository dir belongs to:
// the main checkout's .git for a worktree, dir's own .git otherwise.
func gitCommonDir(dir string) (string, bool) {
	if cached, ok := gitCommonDirs.Load(dir); ok {
		return cached.(string), true
	}
	ctx, cancel := context.WithTimeout(context.Background(), gitLookupTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--git-common-dir").Output()
	if err != nil {
		return "", false
	}
	common := strings.TrimSpace(string(out))
	if common == "" {
		return "", false
	}
	if !filepath.IsAbs(common) {
		common = filepath.Join(dir, common)
	}
	gitCommonDirs.Store(dir, common)
	return common, true
}

// goEnvCaches asks the go command once where its caches are, which also
// honors settings written with go env -w. An empty result falls back to the
// environment and Go's defaults.
var goEnvCaches = sync.OnceValues(func() (string, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "go", "env", "GOCACHE", "GOMODCACHE").Output()
	if err != nil {
		return "", ""
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != 2 {
		return "", ""
	}
	return strings.TrimSpace(lines[0]), strings.TrimSpace(lines[1])
})
