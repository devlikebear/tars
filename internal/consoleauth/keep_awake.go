package consoleauth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/devlikebear/tars/internal/atomicwrite"
)

type keepAwakeFile struct {
	Enabled bool `json:"enabled"`
}

// KeepAwake reports whether the desktop's owner asked to keep this computer
// awake while it is plugged in and a phone is paired
// (docs/decisions/mobile-remote.md). Off unless set.
func (s *Store) KeepAwake() (bool, error) {
	raw, err := os.ReadFile(s.keepAwakePath())
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read keep awake: %w", err)
	}
	if strings.TrimSpace(string(raw)) == "" {
		return false, nil
	}
	var file keepAwakeFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return false, fmt.Errorf("decode keep awake: %w", err)
	}
	return file.Enabled, nil
}

// SetKeepAwake stores the owner's choice.
func (s *Store) SetKeepAwake(enabled bool) error {
	raw, err := json.MarshalIndent(keepAwakeFile{Enabled: enabled}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode keep awake: %w", err)
	}
	return atomicwrite.Write(s.keepAwakePath(), append(raw, '\n'))
}

func (s *Store) keepAwakePath() string {
	return filepath.Join(s.authDir(), "keep-awake.json")
}
