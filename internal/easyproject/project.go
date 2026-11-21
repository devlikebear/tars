// Package easyproject makes the project an easy-mode idea is built in
// (docs/decisions/vibe-coder-identity.md, phase 2): a folder of its own,
// the web app stack set up in it, and version control started, before the
// focus pipeline that builds the idea is opened. The person who asked never
// picks a folder, a stack or a git setting.
//
// It runs processes (npm, git) and knows nothing of chat or sessions: the
// pipeline is opened through Creator.Open, which the server supplies.
package easyproject

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

// Status is where a project's creation is.
type Status string

const (
	StatusCreating Status = "creating"
	StatusReady    Status = "ready"
	StatusFailed   Status = "failed"
)

// Error codes a failed project carries. They are fixed words, never a
// tool's output: the console turns them into plain sentences.
const (
	ErrorNodeMissing    = "node_missing"
	ErrorScaffoldFailed = "scaffold_failed"
	ErrorGitFailed      = "git_failed"
	ErrorPipelineFailed = "pipeline_failed"
	ErrorInterrupted    = "interrupted"
)

// Project is one easy-mode project.
type Project struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Idea   string `json:"idea"`
	Folder string `json:"folder"`
	Status Status `json:"status"`
	// ErrorCode is set when Status is failed.
	ErrorCode string `json:"error_code,omitempty"`
	// SessionID is the focus pipeline's session once the project is ready.
	SessionID string    `json:"session_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// MaxIdeaRunes caps the idea: it becomes the pipeline's goal.
const MaxIdeaRunes = 2000

// ErrIdeaRequired and ErrIdeaTooLong reject an idea before anything is made.
var (
	ErrIdeaRequired = errors.New("an idea is required")
	ErrIdeaTooLong  = fmt.Errorf("an idea is at most %d characters", MaxIdeaRunes)
)

// CleanIdea trims the idea and checks its length.
func CleanIdea(idea string) (string, error) {
	idea = strings.TrimSpace(idea)
	if idea == "" {
		return "", ErrIdeaRequired
	}
	if len([]rune(idea)) > MaxIdeaRunes {
		return "", ErrIdeaTooLong
	}
	return idea, nil
}

// nameRunes caps a project's display name, the idea's first line.
const nameRunes = 60

// NameOf is the project's display name: the idea's first line, clipped.
func NameOf(idea string) string {
	line := strings.TrimSpace(strings.SplitN(strings.TrimSpace(idea), "\n", 2)[0])
	if runes := []rune(line); len(runes) > nameRunes {
		return strings.TrimSpace(string(runes[:nameRunes-1])) + "…"
	}
	return line
}

// slugRunes caps a folder name made from the idea.
const slugRunes = 40

// Slug is a folder name made from the idea: lower-case ASCII letters and
// digits joined by single hyphens. An idea with none (a Korean sentence,
// say) gives "project"; the folder is still unique (NewFolder).
func Slug(idea string) string {
	var b strings.Builder
	hyphen := false
	for _, r := range strings.ToLower(NameOf(idea)) {
		switch {
		case r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			if hyphen && b.Len() > 0 {
				b.WriteByte('-')
			}
			hyphen = false
			b.WriteRune(r)
		default:
			hyphen = true
		}
		if b.Len() >= slugRunes {
			break
		}
	}
	slug := strings.Trim(b.String(), "-")
	if len(slug) > slugRunes {
		slug = strings.Trim(slug[:slugRunes], "-")
	}
	if slug == "" {
		return "project"
	}
	return slug
}

// BaseDirEnv overrides where projects are made (tests, e2e).
const BaseDirEnv = "TARS_EASY_PROJECTS_DIR"

// DefaultBaseDirName is the folder in the user's home that holds the
// projects: visible in Finder, easy to open in another tool.
const DefaultBaseDirName = "TARS Projects"

// BaseDir is where projects are made: BaseDirEnv, else ~/TARS Projects.
func BaseDir() (string, error) {
	if dir := strings.TrimSpace(os.Getenv(BaseDirEnv)); dir != "" {
		return filepath.Abs(dir)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, DefaultBaseDirName), nil
}

// maxFolderTries bounds the -2, -3, … suffixes NewFolder tries.
const maxFolderTries = 100

// NewFolder makes a new, empty folder for the idea under base: base/slug,
// else base/slug-2 and so on. os.Mkdir makes the choice atomic, so two
// projects never share a folder and an existing folder is never reused.
func NewFolder(base, idea string) (string, error) {
	if err := os.MkdirAll(base, 0o755); err != nil {
		return "", err
	}
	slug := Slug(idea)
	for i := 1; i <= maxFolderTries; i++ {
		name := slug
		if i > 1 {
			name = fmt.Sprintf("%s-%d", slug, i)
		}
		dir := filepath.Join(base, name)
		err := os.Mkdir(dir, 0o755)
		if err == nil {
			return dir, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return "", err
		}
	}
	return "", fmt.Errorf("no free folder name for %q under %s", slug, base)
}

// newID is a project's id: 16 hex characters.
func newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%016x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// validID reports whether id has the shape newID makes, before it is used
// to look a project up.
func validID(id string) bool {
	if len(id) != 16 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

// Opener opens the focus pipeline that builds the idea in folder, starting
// its first turn, and returns the pipeline's session id.
type Opener func(ctx context.Context, folder, idea string) (string, error)
