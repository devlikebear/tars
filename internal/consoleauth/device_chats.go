package consoleauth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/devlikebear/tars/internal/atomicwrite"
)

// deviceChatsMu serializes the read-modify-write on device-chats.json
// (docs/decisions/mobile-remote.md §8: one chat session per paired device).
// Package-level, not a Store field, because what it guards against is two
// concurrent requests interleaving a read and a write of the same file, not
// fairness across *Store values — and a server process has exactly one
// workspace directory, so one mutex is correct even if callers build more
// than one *Store over it.
var deviceChatsMu sync.Mutex

// deviceChatsFile maps a paired device's id to the single chat session it
// owns.
type deviceChatsFile struct {
	Sessions map[string]string `json:"sessions"`
}

// DeviceChatSession returns the chat session id recorded for deviceID, if
// any. The caller (the device-chat handler) still has to check the session
// still exists and is reachable — this store only remembers the mapping.
func (s *Store) DeviceChatSession(deviceID string) (string, bool, error) {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return "", false, nil
	}
	deviceChatsMu.Lock()
	defer deviceChatsMu.Unlock()
	file, err := s.loadDeviceChats()
	if err != nil {
		return "", false, err
	}
	sessionID := strings.TrimSpace(file.Sessions[deviceID])
	if sessionID == "" {
		return "", false, nil
	}
	return sessionID, true, nil
}

// SetDeviceChatSession records sessionID as deviceID's chat session,
// replacing any session recorded before it (the caller found the previous
// one gone — deleted or past recovery — and made a new one).
func (s *Store) SetDeviceChatSession(deviceID, sessionID string) error {
	deviceID = strings.TrimSpace(deviceID)
	sessionID = strings.TrimSpace(sessionID)
	if deviceID == "" {
		return errors.New("device id is required")
	}
	if sessionID == "" {
		return errors.New("session id is required")
	}
	deviceChatsMu.Lock()
	defer deviceChatsMu.Unlock()
	file, err := s.loadDeviceChats()
	if err != nil {
		return err
	}
	if file.Sessions == nil {
		file.Sessions = map[string]string{}
	}
	file.Sessions[deviceID] = sessionID
	return s.saveDeviceChats(file)
}

func (s *Store) loadDeviceChats() (deviceChatsFile, error) {
	raw, err := os.ReadFile(s.deviceChatsPath())
	if errors.Is(err, os.ErrNotExist) {
		return deviceChatsFile{Sessions: map[string]string{}}, nil
	}
	if err != nil {
		return deviceChatsFile{}, fmt.Errorf("read device chats: %w", err)
	}
	if strings.TrimSpace(string(raw)) == "" {
		return deviceChatsFile{Sessions: map[string]string{}}, nil
	}
	var file deviceChatsFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return deviceChatsFile{}, fmt.Errorf("decode device chats: %w", err)
	}
	if file.Sessions == nil {
		file.Sessions = map[string]string{}
	}
	return file, nil
}

func (s *Store) saveDeviceChats(file deviceChatsFile) error {
	raw, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("encode device chats: %w", err)
	}
	return atomicwrite.Write(s.deviceChatsPath(), append(raw, '\n'))
}

func (s *Store) deviceChatsPath() string {
	return filepath.Join(s.authDir(), "device-chats.json")
}
