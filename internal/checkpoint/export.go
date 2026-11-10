package checkpoint

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Snapshot sides a turn recorded (ExportTurn).
const (
	SideStart = "start"
	SideEnd   = "end"
)

// ExportTurn writes the work tree a turn's checkpoint recorded — its start
// or end snapshot (SideStart, SideEnd) — into dest, an existing folder.
// Verify promotion (focus) replays a proposed check on the two sides of the
// turn that fixed a defect.
//
// git writes the files itself: the snapshot's tree is read into an index of
// the export's own (never the shadow's) and checked out into dest, so git's
// own path checks apply — no entry lands outside dest, and nothing is
// written through a symbolic link. A symbolic link whose target is absolute
// or leaves dest is then removed (pruneEscapingLinks): a later write
// through it would reach outside. Nothing in the shadow repository or the
// session's own folder changes.
func (s *Store) ExportTurn(ctx context.Context, sessionID, turnID, side, dest string) error {
	if err := validID("session", sessionID); err != nil {
		return err
	}
	if err := validID("turn", turnID); err != nil {
		return err
	}
	e, err := s.turnEntry(sessionID, turnID)
	if err != nil {
		return err
	}
	commit := e.Start
	if side == SideEnd {
		commit = e.End
	} else if side != SideStart {
		return fmt.Errorf("%w: side %q", ErrInvalid, side)
	}
	sh, err := s.openShadow(e.Shadow)
	if err != nil {
		return err
	}
	root, err := filepath.Abs(dest)
	if err != nil {
		return err
	}
	scratch, err := os.MkdirTemp("", "tars-checkpoint-export-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(scratch) }()
	index := filepath.Join(scratch, "index")
	for _, args := range [][]string{{"read-tree", commit}, {"checkout-index", "--all", "--force"}} {
		call := gitCall{gitDir: sh.gitDir, workTree: root, dir: root, indexFile: index, args: args}
		if _, _, err := s.git.run(ctx, call); err != nil {
			return err
		}
	}
	return pruneEscapingLinks(root)
}

// pruneEscapingLinks removes every symbolic link under root whose target is
// absolute or, resolved from the link's folder, lies outside root.
func pruneEscapingLinks(root string) error {
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&fs.ModeSymlink == 0 {
			return nil
		}
		target, err := os.Readlink(p)
		if err != nil {
			return err
		}
		if !linkStaysInside(root, p, target) {
			return os.Remove(p)
		}
		return nil
	})
}

// linkStaysInside reports whether a link at link with target resolves
// (lexically) inside root.
func linkStaysInside(root, link, target string) bool {
	// A leading separator is rooted even where IsAbs says no (\etc on
	// Windows).
	if target == "" || filepath.IsAbs(target) || filepath.VolumeName(target) != "" ||
		strings.HasPrefix(target, "/") || strings.HasPrefix(target, string(filepath.Separator)) {
		return false
	}
	rel, err := filepath.Rel(root, filepath.Join(filepath.Dir(link), target))
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && filepath.IsLocal(rel)
}
