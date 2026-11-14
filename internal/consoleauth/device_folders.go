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

// MaxDeviceFolders bounds the list a desktop's owner registers for paired
// phones (docs/decisions/mobile-remote.md §4).
const MaxDeviceFolders = 32

type deviceFoldersFile struct {
	Folders []string `json:"folders"`
}

// DeviceFolders returns the folders a paired device may start a focus
// pipeline in. A device never sends a free path: it picks from this list,
// which only the desktop's admin writes.
func (s *Store) DeviceFolders() ([]string, error) {
	raw, err := os.ReadFile(s.deviceFoldersPath())
	if errors.Is(err, os.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read device folders: %w", err)
	}
	if strings.TrimSpace(string(raw)) == "" {
		return []string{}, nil
	}
	var file deviceFoldersFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("decode device folders: %w", err)
	}
	if file.Folders == nil {
		return []string{}, nil
	}
	return file.Folders, nil
}

// SetDeviceFolders replaces the list. Every entry must be an absolute path;
// entries are cleaned and duplicates dropped, in the given order. It returns
// the list as stored. Whether a folder exists is the caller's check: the
// store only keeps the policy.
func (s *Store) SetDeviceFolders(folders []string) ([]string, error) {
	out := make([]string, 0, len(folders))
	seen := map[string]struct{}{}
	for _, folder := range folders {
		normalized, err := NormalizeDeviceFolder(folder)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	if len(out) > MaxDeviceFolders {
		return nil, fmt.Errorf("at most %d folders", MaxDeviceFolders)
	}
	raw, err := json.MarshalIndent(deviceFoldersFile{Folders: out}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode device folders: %w", err)
	}
	if err := atomicwrite.Write(s.deviceFoldersPath(), append(raw, '\n')); err != nil {
		return nil, err
	}
	return out, nil
}

// DeviceFolderAllowed reports whether path, cleaned, is one of the
// registered folders.
func (s *Store) DeviceFolderAllowed(path string) (bool, error) {
	normalized, err := NormalizeDeviceFolder(path)
	if err != nil {
		return false, nil
	}
	folders, err := s.DeviceFolders()
	if err != nil {
		return false, err
	}
	for _, folder := range folders {
		if folder == normalized {
			return true, nil
		}
	}
	return false, nil
}

// NormalizeDeviceFolder trims and cleans an absolute path; anything else is
// an error.
func NormalizeDeviceFolder(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("folder path is required")
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("folder %q must be an absolute path", path)
	}
	return filepath.Clean(path), nil
}

func (s *Store) deviceFoldersPath() string {
	return filepath.Join(s.authDir(), "device-folders.json")
}
