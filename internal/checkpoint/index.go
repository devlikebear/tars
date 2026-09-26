package checkpoint

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/devlikebear/tars/internal/atomicwrite"
)

// Entry records one turn's checkpoint.
type Entry struct {
	TurnID string `json:"turn_id"`
	// Root is the work tree the turn was recorded against: the repository's
	// top level, or the session's folder when that is not in a repository.
	Root      string    `json:"root"`
	Shadow    string    `json:"shadow"`
	Start     string    `json:"start,omitempty"`
	End       string    `json:"end,omitempty"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at,omitzero"`
	// Preview is the start of the user's message, so a turn can be named
	// after compaction drops the message itself.
	Preview   string `json:"preview,omitempty"`
	Files     int    `json:"files"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	// Unknown lists paths left out of a snapshot: too large, unreadable, or
	// a nested repository. Diffs hide them and reverts never touch them.
	Unknown []string `json:"unknown,omitempty"`
	// Skipped says why no checkpoint was taken (see the Skip* reasons);
	// SkipDetail carries the specifics.
	Skipped    string `json:"skipped,omitempty"`
	SkipDetail string `json:"skip_detail,omitempty"`
}

type sessionIndex struct {
	Version   int     `json:"version"`
	SessionID string  `json:"session_id"`
	Turns     []Entry `json:"turns"`
}

const indexVersion = 1

// Session and turn IDs become file names and ref name components, so they
// are held to a conservative alphabet.
var safeID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

func validID(kind, id string) error {
	if !safeID.MatchString(id) || id == "." || id == ".." {
		return fmt.Errorf("checkpoint: invalid %s id %q", kind, id)
	}
	return nil
}

func (s *Store) indexPath(sessionID string) string {
	return filepath.Join(s.dir, "sessions", sessionID+".json")
}

func (s *Store) readIndex(sessionID string) (sessionIndex, error) {
	data, err := os.ReadFile(s.indexPath(sessionID))
	if errors.Is(err, fs.ErrNotExist) {
		return sessionIndex{Version: indexVersion, SessionID: sessionID}, nil
	}
	if err != nil {
		return sessionIndex{}, fmt.Errorf("checkpoint: read index: %w", err)
	}
	var idx sessionIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return sessionIndex{}, fmt.Errorf("checkpoint: parse index: %w", err)
	}
	idx.SessionID = sessionID
	return idx, nil
}

func (s *Store) writeIndex(idx sessionIndex) error {
	idx.Version = indexVersion
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	return atomicwrite.Write(s.indexPath(idx.SessionID), append(data, '\n'))
}
