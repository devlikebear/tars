package easyproject

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ErrNodeMissing means the stack cannot be set up on this computer: npm
// and npx are not installed where the server can find them.
var ErrNodeMissing = errors.New("node and npm are not installed")

// Scaffolder sets the web app stack up in an empty folder.
type Scaffolder interface {
	// Ready answers ErrNodeMissing when the scaffolder cannot run here,
	// before any folder is made.
	Ready() error
	// Scaffold sets the stack up in dir, which exists and is empty.
	Scaffold(ctx context.Context, dir string) error
}

// CreateCloudflareVersion pins Cloudflare's scaffolder: every project
// starts from the same files, so the preview and the checks do not move
// under a new release. It is the version measured on 2026-11-21
// (vibe-coder-identity.md §7); move it on purpose, after trying it.
const CreateCloudflareVersion = "2.75.0"

// createCloudflareArgs are the scaffolder's answers, so it never asks:
// React with TypeScript on Workers, nothing deployed, no git (InitGit
// starts it), no AGENTS.md, no browser opened.
var createCloudflareArgs = []string{
	"--framework=react", "--platform=workers", "--variant=react-ts", "--lang=ts",
	"--no-deploy", "--no-git", "--no-agents", "--no-open", "--no-auto-update", "-y",
}

// stackPackages are the rest of the fixed stack, added after the
// scaffolder: Hono for server code, Tailwind for styling. Major versions
// are pinned.
var stackPackages = []string{"hono@^4", "tailwindcss@^4", "@tailwindcss/vite@^4"}

// stepTimeout bounds one npm step; a cold install took 25 s when measured.
const stepTimeout = 5 * time.Minute

// knownNodeDirs are where npm is installed when the server's PATH (launchd's,
// for a service) does not list it.
var knownNodeDirs = []string{"/opt/homebrew/bin", "/usr/local/bin"}

// NPMScaffolder runs Cloudflare's scaffolder through npx, then installs the
// stack's other packages with npm.
type NPMScaffolder struct{}

// findNPM returns the folder holding npm and npx.
func findNPM() (string, error) {
	if path, err := exec.LookPath("npm"); err == nil {
		dir := filepath.Dir(path)
		if isFile(filepath.Join(dir, "npx")) {
			return dir, nil
		}
	}
	for _, dir := range knownNodeDirs {
		if isFile(filepath.Join(dir, "npm")) && isFile(filepath.Join(dir, "npx")) {
			return dir, nil
		}
	}
	return "", ErrNodeMissing
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// Ready reports whether npm and npx can be found.
func (NPMScaffolder) Ready() error {
	_, err := findNPM()
	return err
}

// Scaffold runs the pinned scaffolder for dir (from its parent, naming the
// folder) and installs the stack's other packages in it.
//
// The scaffolder takes only lower-case ASCII letters, digits and dashes for
// a name, while a project's folder keeps the idea's own words (a Korean
// idea gives a Hangul folder, Slug). Such a folder is scaffolded under an
// ASCII name (ScaffoldName) in a staging folder inside it, and what the
// scaffolder made is moved up into the folder.
func (NPMScaffolder) Scaffold(ctx context.Context, dir string) error {
	bin, err := findNPM()
	if err != nil {
		return err
	}
	name, from := filepath.Base(dir), filepath.Dir(dir)
	staging := ""
	if !validScaffoldName(name) {
		staging = filepath.Join(dir, scaffoldStaging)
		if err := os.Mkdir(staging, 0o755); err != nil {
			return fmt.Errorf("scaffold: %w", err)
		}
		name, from = ScaffoldName(filepath.Base(dir)), staging
	}
	create := append([]string{"--yes", "create-cloudflare@" + CreateCloudflareVersion, name}, createCloudflareArgs...)
	if err := runStep(ctx, from, bin, filepath.Join(bin, "npx"), create...); err != nil {
		return fmt.Errorf("scaffold: %w", err)
	}
	if staging != "" {
		if err := moveInto(filepath.Join(staging, name), dir); err != nil {
			return fmt.Errorf("scaffold: %w", err)
		}
		if err := os.RemoveAll(staging); err != nil {
			return fmt.Errorf("scaffold: %w", err)
		}
	}
	if !isFile(filepath.Join(dir, "package.json")) {
		return errors.New("scaffold: no package.json was written")
	}
	install := append([]string{"install", "--no-audit", "--no-fund"}, stackPackages...)
	if err := runStep(ctx, dir, bin, filepath.Join(bin, "npm"), install...); err != nil {
		return fmt.Errorf("install the stack's packages: %w", err)
	}
	return nil
}

// scaffoldStaging is the folder inside a project's folder where a project
// whose folder name the scaffolder refuses is scaffolded first.
const scaffoldStaging = ".tars-scaffold"

// scaffoldNameMax is the scaffolder's own limit on a name's length.
const scaffoldNameMax = 58

// validScaffoldName reports whether the scaffolder takes name as it is:
// lower-case ASCII letters, digits and dashes, not starting or ending with
// a dash, shorter than its limit.
func validScaffoldName(name string) bool {
	if name == "" || len(name) > scaffoldNameMax || name[0] == '-' || name[len(name)-1] == '-' {
		return false
	}
	for _, r := range name {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return false
		}
	}
	return true
}

// ScaffoldName is the ASCII name a project is scaffolded under (and what
// its app calls itself) when its folder name has other letters: the
// folder's ASCII words, else "app-" and a short hash of the folder name,
// so two Korean projects do not share one.
func ScaffoldName(folder string) string {
	words := strings.FieldsFunc(strings.ToLower(folder), func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	})
	name := strings.Join(words, "-")
	if len(name) > slugRunes {
		name = strings.Trim(name[:slugRunes], "-")
	}
	if name != "" {
		return name
	}
	sum := sha256.Sum256([]byte(folder))
	return "app-" + hex.EncodeToString(sum[:4])
}

// moveInto moves every entry of from into to, which must not hold an
// entry of the same name.
func moveInto(from, to string) error {
	entries, err := os.ReadDir(from)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !filepath.IsLocal(e.Name()) || filepath.Base(e.Name()) != e.Name() {
			return fmt.Errorf("%w: %q", ErrFolderName, e.Name())
		}
		if err := os.Rename(filepath.Join(from, e.Name()), filepath.Join(to, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// outputTail is how much of a failed step's output its error keeps, for
// the server log.
const outputTail = 2000

// runStep runs one command in dir with nodeBin first on PATH (npx runs node
// scripts that look node up there), no stdin, and a timeout.
func runStep(ctx context.Context, dir, nodeBin, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, stepTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...) // #nosec G204 -- fixed program and arguments
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PATH="+nodeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		tail := out.String()
		if len(tail) > outputTail {
			tail = tail[len(tail)-outputTail:]
		}
		return fmt.Errorf("%s: %w: %s", filepath.Base(name), err, strings.TrimSpace(tail))
	}
	return nil
}

// ScaffoldPathEnv names a program to run instead of npm (e2e tests): it is
// called with the project folder as its only argument.
const ScaffoldPathEnv = "TARS_EASY_SCAFFOLD_PATH"

// ScriptScaffolder runs one program with the folder as its argument.
type ScriptScaffolder struct{ Path string }

// Ready reports whether the program exists.
func (s ScriptScaffolder) Ready() error {
	if !isFile(s.Path) {
		return ErrNodeMissing
	}
	return nil
}

// Scaffold runs the program for dir.
func (s ScriptScaffolder) Scaffold(ctx context.Context, dir string) error {
	ctx, cancel := context.WithTimeout(ctx, stepTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.Path, dir) // #nosec G204 -- the program is the server operator's own setting
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("scaffold script: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ScaffolderFromEnv is ScriptScaffolder when ScaffoldPathEnv is set, else
// NPMScaffolder.
func ScaffolderFromEnv() Scaffolder {
	if path := strings.TrimSpace(os.Getenv(ScaffoldPathEnv)); path != "" {
		return ScriptScaffolder{Path: path}
	}
	return NPMScaffolder{}
}

// tarsIdentity is the author of the versions TARS saves, given per
// command, so a computer with no git identity still works and the user's
// own settings are never written.
var tarsIdentity = []string{"-c", "user.name=TARS", "-c", "user.email=tars@localhost", "-c", "commit.gpgSign=false"}

func git(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...) // #nosec G204 -- fixed program; arguments are the server's own
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		name := args[0]
		for i := 0; i+1 < len(args) && args[i] == "-c"; i += 2 {
			name = args[i+2]
		}
		return "", fmt.Errorf("git %s: %w: %s", name, err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// InitGit starts version control in dir and records the scaffold as the
// first version.
func InitGit(ctx context.Context, dir string) error {
	if _, err := git(ctx, dir, "init", "-q", "-b", "main"); err != nil {
		return err
	}
	if _, err := git(ctx, dir, "add", "-A"); err != nil {
		return err
	}
	_, err := git(ctx, dir, append(tarsIdentity, "commit", "-q", "--allow-empty", "-m", startMessage)...)
	return err
}

// SaveVersion records everything in dir as a new version with message and
// answers its commit. When nothing changed it saves nothing and answers the
// commit that was already there.
func SaveVersion(ctx context.Context, dir, message string) (string, error) {
	status, err := git(ctx, dir, "status", "--porcelain")
	if err != nil {
		return "", err
	}
	if status != "" {
		if _, err := git(ctx, dir, "add", "-A"); err != nil {
			return "", err
		}
		if _, err := git(ctx, dir, append(tarsIdentity, "commit", "-q", "-m", message)...); err != nil {
			return "", err
		}
	}
	return git(ctx, dir, "rev-parse", "HEAD")
}
