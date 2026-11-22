package easyproject

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Versions (vibe-coder-identity.md §4, phase 3): every finished run saves
// the project as a version (SaveVersion), so the folder's history is the
// list the person picks from. Going back never rewrites that history: it
// saves the old version's files as a new version, so the version left
// behind can be gone back to as well.

// VersionKind says where a version came from.
type VersionKind string

const (
	// VersionStart is the project as the scaffold left it (InitGit).
	VersionStart VersionKind = "start"
	// VersionSaved is the work of a finished run, or any other save.
	VersionSaved VersionKind = "saved"
	// VersionRestored is a going back; Title names the version restored.
	VersionRestored VersionKind = "restored"
)

// Version is one saved version, as the result screen lists it.
type Version struct {
	Commit string      `json:"commit"`
	Title  string      `json:"title"`
	Kind   VersionKind `json:"kind"`
	At     time.Time   `json:"at"`
	// Current marks the version the folder holds now: the newest.
	Current bool `json:"current,omitempty"`
}

// Messages TARS writes and reads back.
const (
	startMessage   = "Start the project"
	restorePrefix  = "Go back to: "
	unsavedMessage = "Unsaved work before going back"
)

// MaxVersions caps the list.
const MaxVersions = 50

// ErrVersionNotFound means the commit is not a version of this project.
var ErrVersionNotFound = errors.New("no such version")

var commitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

// fieldSep separates git log fields; it never appears in a message line.
const fieldSep = "\x1f"

// Versions lists the project's versions, newest first, at most MaxVersions.
func Versions(ctx context.Context, dir string) ([]Version, error) {
	out, err := git(ctx, dir, "log", "-n", strconv.Itoa(MaxVersions), "--format=%H"+fieldSep+"%cI"+fieldSep+"%P"+fieldSep+"%s")
	if err != nil {
		return nil, err
	}
	var versions []Version
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(line, fieldSep, 4)
		if len(parts) != 4 {
			continue
		}
		at, _ := time.Parse(time.RFC3339, parts[1])
		versions = append(versions, versionOf(parts[0], at, parts[2] == "", parts[3]))
	}
	if len(versions) > 0 {
		versions[0].Current = true
	}
	return versions, nil
}

func versionOf(commit string, at time.Time, root bool, subject string) Version {
	v := Version{Commit: commit, Title: subject, Kind: VersionSaved, At: at.UTC()}
	switch {
	case root && subject == startMessage:
		v.Kind = VersionStart
	case strings.HasPrefix(subject, restorePrefix):
		v.Kind = VersionRestored
		v.Title = strings.TrimPrefix(subject, restorePrefix)
	}
	return v
}

// RestoreVersion makes the folder hold commit's files again and saves that
// as a new version, answering it. Work not yet saved is saved first, so
// nothing the folder held is lost. Going back to the version the folder
// already holds saves nothing and answers it. Files git ignores (installed
// packages, local data) are left as they are.
func RestoreVersion(ctx context.Context, dir, commit string) (Version, error) {
	if !commitPattern.MatchString(commit) {
		return Version{}, ErrVersionNotFound
	}
	// Only a version of this project: one in the history of what it holds.
	if _, err := git(ctx, dir, "merge-base", "--is-ancestor", commit, "HEAD"); err != nil {
		return Version{}, ErrVersionNotFound
	}
	target, err := git(ctx, dir, "log", "-1", "--format=%s", commit)
	if err != nil {
		return Version{}, err
	}
	title := strings.TrimPrefix(target, restorePrefix)
	if _, err := SaveVersion(ctx, dir, unsavedMessage); err != nil {
		return Version{}, err
	}
	head, err := git(ctx, dir, "rev-parse", "HEAD")
	if err != nil {
		return Version{}, err
	}
	if head != commit {
		// The index and the files become commit's; files it lacks go.
		if _, err := git(ctx, dir, "read-tree", "-u", "--reset", commit); err != nil {
			return Version{}, err
		}
		if _, err := git(ctx, dir, append(tarsIdentity, "commit", "-q", "--allow-empty", "-m", restorePrefix+title)...); err != nil {
			return Version{}, err
		}
	}
	versions, err := Versions(ctx, dir)
	if err != nil {
		return Version{}, err
	}
	if len(versions) == 0 {
		return Version{}, ErrVersionNotFound
	}
	return versions[0], nil
}
