package consoleauth

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

// RoleFocus is the role of a paired mobile device (docs/decisions/mobile-remote.md
// §4). It never logs in with a password and never gets a browser session: a
// device presents its token as a bearer, and the server lets it reach only the
// focus routes (serverauth's focus endpoint table).
const RoleFocus = "focus"

const (
	deviceTokenBytes  = 32
	deviceIDBytes     = 9
	maxDeviceNameRune = 64
)

// Device is one paired mobile device. The token itself is never stored: only
// its sha256, so a copy of the workspace does not hand out a working token.
type Device struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Role      string     `json:"role"`
	TokenHash string     `json:"token_hash"`
	CreatedAt time.Time  `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

// Active reports whether the device can still authenticate.
func (d Device) Active() bool { return d.RevokedAt == nil }

// ErrDeviceNotFound is returned when revoking an id the store does not have.
var ErrDeviceNotFound = errors.New("device not found")

// CreateDevice records a new device for role and returns it with its token.
// The token is returned once, here; the store keeps only its hash.
func (s *Store) CreateDevice(name, role string) (Device, string, error) {
	if strings.TrimSpace(strings.ToLower(role)) != RoleFocus {
		return Device{}, "", fmt.Errorf("unsupported device role %q", role)
	}
	name, err := normalizeDeviceName(name)
	if err != nil {
		return Device{}, "", err
	}
	token, err := randomOpaqueID(s.randReader, deviceTokenBytes)
	if err != nil {
		return Device{}, "", err
	}
	id, err := randomOpaqueID(s.randReader, deviceIDBytes)
	if err != nil {
		return Device{}, "", err
	}
	device := Device{
		ID:        id,
		Name:      name,
		Role:      RoleFocus,
		TokenHash: hashDeviceToken(token),
		CreatedAt: s.now().UTC(),
	}
	devices, err := s.loadDevices()
	if err != nil {
		return Device{}, "", err
	}
	devices = append(devices, device)
	if err := s.saveDevices(devices); err != nil {
		return Device{}, "", err
	}
	return device, token, nil
}

// ValidateDeviceToken returns the active device whose token is token.
func (s *Store) ValidateDeviceToken(token string) (Device, bool, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return Device{}, false, nil
	}
	devices, err := s.loadDevices()
	if err != nil {
		return Device{}, false, err
	}
	presented := []byte(hashDeviceToken(token))
	for _, device := range devices {
		if subtle.ConstantTimeCompare([]byte(device.TokenHash), presented) != 1 {
			continue
		}
		if !device.Active() {
			return Device{}, false, nil
		}
		return device, true, nil
	}
	return Device{}, false, nil
}

// ListDevices returns every device, revoked ones included, oldest first.
func (s *Store) ListDevices() ([]Device, error) {
	return s.loadDevices()
}

// RevokeDevice stops the device's token from authenticating. Revoking a
// device that is already revoked is not an error.
func (s *Store) RevokeDevice(id string) error {
	id = strings.TrimSpace(id)
	devices, err := s.loadDevices()
	if err != nil {
		return err
	}
	for i := range devices {
		if devices[i].ID != id {
			continue
		}
		if devices[i].RevokedAt != nil {
			return nil
		}
		now := s.now().UTC()
		devices[i].RevokedAt = &now
		return s.saveDevices(devices)
	}
	return ErrDeviceNotFound
}

// ConsumePairingCodeForRole consumes code only if it was issued for role, so a
// code meant for a browser is not burned by a device trying it (and the
// reverse).
func (s *Store) ConsumePairingCodeForRole(code, role string) (PairingCode, bool, error) {
	code = strings.TrimSpace(code)
	role = strings.TrimSpace(strings.ToLower(role))
	if code == "" || role == "" {
		return PairingCode{}, false, nil
	}
	codes, err := s.loadPairingCodes()
	if err != nil {
		return PairingCode{}, false, err
	}
	now := s.now().UTC()
	for i := len(codes) - 1; i >= 0; i-- {
		candidate := codes[i]
		if candidate.Code != code || candidate.Role != role || candidate.UsedAt != nil || !now.Before(candidate.ExpiresAt) {
			continue
		}
		codes[i].UsedAt = &now
		if err := s.savePairingCodes(codes); err != nil {
			return PairingCode{}, false, err
		}
		return codes[i], true, nil
	}
	return PairingCode{}, false, nil
}

func normalizeDeviceName(name string) (string, error) {
	name = strings.Join(strings.Fields(name), " ")
	if name == "" {
		return "", errors.New("device name is required")
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return "", errors.New("device name has control characters")
		}
	}
	if utf8.RuneCountInString(name) > maxDeviceNameRune {
		runes := []rune(name)
		name = strings.TrimSpace(string(runes[:maxDeviceNameRune]))
	}
	return name, nil
}

func hashDeviceToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *Store) loadDevices() ([]Device, error) {
	var devices []Device
	if err := readJSONL(s.devicesPath(), func(raw []byte) error {
		var device Device
		if err := json.Unmarshal(raw, &device); err != nil {
			return err
		}
		devices = append(devices, device)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("load devices: %w", err)
	}
	return devices, nil
}

func (s *Store) saveDevices(devices []Device) error {
	return writeJSONL(s.devicesPath(), devices)
}

func (s *Store) devicesPath() string {
	return filepath.Join(s.authDir(), "devices.jsonl")
}
