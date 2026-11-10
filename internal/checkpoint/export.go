package checkpoint

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
)

// Snapshot sides a turn recorded (ExportTurn).
const (
	SideStart = "start"
	SideEnd   = "end"
)

// exportMaxBytes caps what one export writes, so a snapshot that grew past
// what a check could need never fills the disk.
const exportMaxBytes = 2 << 30

// ExportTurn writes the work tree a turn's checkpoint recorded — its start
// or end snapshot (SideStart, SideEnd) — into dest, an existing folder.
// Only regular files, folders and symbolic links that stay inside dest are
// written; nothing in the shadow repository or the session's own folder
// changes. Verify promotion (focus) replays a proposed check on the two
// sides of the turn that fixed a defect.
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
	call := sh.objectCall("archive", "--format=tar", commit)
	cmd := exec.CommandContext(ctx, s.git.exe, call.args...)
	cmd.Dir = call.dir
	cmd.Env = gitEnv(call)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	extractErr := extractTar(out, dest)
	// Drain what is left so git is never stuck writing to a closed pipe.
	_, _ = io.Copy(io.Discard, out)
	waitErr := cmd.Wait()
	if extractErr != nil {
		return extractErr
	}
	if waitErr != nil {
		return &gitError{args: call.args, code: cmd.ProcessState.ExitCode(), stderr: trimStderr(stderr.String()), err: waitErr}
	}
	return nil
}

// extractTar writes a git archive into dest, refusing any entry that would
// land outside it.
func extractTar(r io.Reader, dest string) error {
	root, err := filepath.Abs(dest)
	if err != nil {
		return err
	}
	tr := tar.NewReader(r)
	var written int64
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("checkpoint: read archive: %w", err)
		}
		name := path.Clean(h.Name)
		if h.Typeflag == tar.TypeXGlobalHeader || name == "." {
			continue
		}
		if path.IsAbs(name) || name == ".." || strings.HasPrefix(name, "../") {
			return fmt.Errorf("%w: archive entry %q", ErrInvalid, h.Name)
		}
		target := filepath.Join(root, filepath.FromSlash(name))
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			written += h.Size
			if written > exportMaxBytes {
				return fmt.Errorf("checkpoint: snapshot larger than %d bytes", int64(exportMaxBytes))
			}
			if err := writeArchiveFile(tr, target, h.FileInfo().Mode().Perm()); err != nil {
				return err
			}
		case tar.TypeSymlink:
			// A link that leaves the tree is not recreated: it would let the
			// next write through it reach outside dest.
			link := filepath.FromSlash(h.Linkname)
			if filepath.IsAbs(link) {
				continue
			}
			resolved := filepath.Join(filepath.Dir(target), link)
			if rel, err := filepath.Rel(root, resolved); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				continue
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := os.Symlink(link, target); err != nil {
				return err
			}
		}
	}
}

func writeArchiveFile(r io.Reader, target string, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if perm&0o100 != 0 {
		perm = 0o755
	} else {
		perm = 0o644
	}
	f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
