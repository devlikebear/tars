package consoleauth

import (
	"bufio"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/atomicwrite"
	"golang.org/x/crypto/argon2"
)

const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

const (
	defaultSessionIDBytes = 32
	argon2idMemoryKiB     = 64 * 1024
	argon2idIterations    = 1
	argon2idParallelism   = 4
	argon2idSaltBytes     = 16
	argon2idKeyBytes      = 32
)

type Store struct {
	workspaceDir string
	now          func() time.Time
	randReader   io.Reader
}

type Option func(*Store)

func WithNow(now func() time.Time) Option {
	return func(s *Store) {
		if now != nil {
			s.now = now
		}
	}
}

func WithRandomReader(reader io.Reader) Option {
	return func(s *Store) {
		if reader != nil {
			s.randReader = reader
		}
	}
}

type UserRecord struct {
	Hash      string    `json:"hash"`
	CreatedAt time.Time `json:"created_at"`
}

type usersFile struct {
	Admin *UserRecord `json:"admin,omitempty"`
	User  *UserRecord `json:"user,omitempty"`
}

type Session struct {
	ID            string     `json:"id"`
	Role          string     `json:"role"`
	CreatedAt     time.Time  `json:"created_at"`
	LastSeen      time.Time  `json:"last_seen"`
	ExpiresAt     time.Time  `json:"expires_at"`
	UserAgentHint string     `json:"user_agent_hint,omitempty"`
	RevokedAt     *time.Time `json:"revoked_at,omitempty"`
}

type PairingCode struct {
	Code      string     `json:"code"`
	Role      string     `json:"role"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt time.Time  `json:"expires_at"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
}

func NewStore(workspaceDir string, opts ...Option) *Store {
	s := &Store{
		workspaceDir: strings.TrimSpace(workspaceDir),
		now: func() time.Time {
			return time.Now().UTC()
		},
		randReader: rand.Reader,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	return s
}

func (s *Store) SetPassword(role, password string) error {
	role, err := normalizeRole(role)
	if err != nil {
		return err
	}
	if strings.TrimSpace(password) == "" {
		return errors.New("password is required")
	}
	users, err := s.loadUsers()
	if err != nil {
		return err
	}
	hash, err := hashPassword(password, s.randReader)
	if err != nil {
		return err
	}
	record := &UserRecord{Hash: hash, CreatedAt: s.now().UTC()}
	switch role {
	case RoleAdmin:
		users.Admin = record
	case RoleUser:
		users.User = record
	}
	if err := s.saveUsers(users); err != nil {
		return err
	}
	return s.RevokeRoleSessions(role)
}

func (s *Store) HasPassword(role string) (bool, error) {
	role, err := normalizeRole(role)
	if err != nil {
		return false, err
	}
	users, err := s.loadUsers()
	if err != nil {
		return false, err
	}
	return userRecordForRole(users, role) != nil, nil
}

func (s *Store) VerifyPassword(role, password string) (bool, error) {
	role, err := normalizeRole(role)
	if err != nil {
		return false, err
	}
	users, err := s.loadUsers()
	if err != nil {
		return false, err
	}
	record := userRecordForRole(users, role)
	if record == nil {
		return false, nil
	}
	return verifyPasswordHash(password, record.Hash)
}

func (s *Store) CreateSession(role, userAgentHint string, ttl time.Duration) (Session, error) {
	role, err := normalizeRole(role)
	if err != nil {
		return Session{}, err
	}
	if ttl <= 0 {
		return Session{}, errors.New("session ttl must be positive")
	}
	id, err := randomOpaqueID(s.randReader, defaultSessionIDBytes)
	if err != nil {
		return Session{}, err
	}
	now := s.now().UTC()
	session := Session{
		ID:            id,
		Role:          role,
		CreatedAt:     now,
		LastSeen:      now,
		ExpiresAt:     now.Add(ttl),
		UserAgentHint: strings.TrimSpace(userAgentHint),
	}
	sessions, err := s.loadSessions()
	if err != nil {
		return Session{}, err
	}
	sessions = append(sessions, session)
	if err := s.saveSessions(sessions); err != nil {
		return Session{}, err
	}
	return session, nil
}

func (s *Store) ValidateSession(id string) (Session, bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Session{}, false, nil
	}
	sessions, err := s.loadSessions()
	if err != nil {
		return Session{}, false, err
	}
	for i := len(sessions) - 1; i >= 0; i-- {
		session := sessions[i]
		if session.ID != id {
			continue
		}
		if session.RevokedAt != nil {
			return Session{}, false, nil
		}
		if !s.now().UTC().Before(session.ExpiresAt) {
			return Session{}, false, nil
		}
		return session, true, nil
	}
	return Session{}, false, nil
}

func (s *Store) RevokeSession(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	sessions, err := s.loadSessions()
	if err != nil {
		return err
	}
	now := s.now().UTC()
	changed := false
	for i := range sessions {
		if sessions[i].ID == id && sessions[i].RevokedAt == nil {
			sessions[i].RevokedAt = &now
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return s.saveSessions(sessions)
}

func (s *Store) RevokeRoleSessions(role string) error {
	role, err := normalizeRole(role)
	if err != nil {
		return err
	}
	sessions, err := s.loadSessions()
	if err != nil {
		return err
	}
	if len(sessions) == 0 {
		return nil
	}
	now := s.now().UTC()
	changed := false
	for i := range sessions {
		if sessions[i].Role == role && sessions[i].RevokedAt == nil {
			sessions[i].RevokedAt = &now
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return s.saveSessions(sessions)
}

func (s *Store) CreatePairingCode(role string, ttl time.Duration) (PairingCode, error) {
	role, err := normalizeRole(role)
	if err != nil {
		return PairingCode{}, err
	}
	if ttl <= 0 {
		return PairingCode{}, errors.New("pairing code ttl must be positive")
	}
	codes, err := s.loadPairingCodes()
	if err != nil {
		return PairingCode{}, err
	}
	codeValue, err := s.uniquePairingCode(codes)
	if err != nil {
		return PairingCode{}, err
	}
	now := s.now().UTC()
	code := PairingCode{
		Code:      codeValue,
		Role:      role,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
	}
	codes = append(codes, code)
	if err := s.savePairingCodes(codes); err != nil {
		return PairingCode{}, err
	}
	return code, nil
}

func (s *Store) ConsumePairingCode(code string) (PairingCode, bool, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return PairingCode{}, false, nil
	}
	codes, err := s.loadPairingCodes()
	if err != nil {
		return PairingCode{}, false, err
	}
	now := s.now().UTC()
	for i := len(codes) - 1; i >= 0; i-- {
		candidate := codes[i]
		if candidate.Code != code || candidate.UsedAt != nil || !now.Before(candidate.ExpiresAt) {
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

func (s *Store) uniquePairingCode(existing []PairingCode) (string, error) {
	for attempts := 0; attempts < 10; attempts++ {
		value, err := randomDigits(s.randReader, 6)
		if err != nil {
			return "", err
		}
		if !pairingCodeActive(existing, value, s.now().UTC()) {
			return value, nil
		}
	}
	return "", errors.New("failed to generate unique pairing code")
}

func pairingCodeActive(codes []PairingCode, value string, now time.Time) bool {
	for _, code := range codes {
		if code.Code == value && code.UsedAt == nil && now.Before(code.ExpiresAt) {
			return true
		}
	}
	return false
}

func (s *Store) loadUsers() (usersFile, error) {
	var users usersFile
	raw, err := os.ReadFile(s.usersPath())
	if errors.Is(err, os.ErrNotExist) {
		return users, nil
	}
	if err != nil {
		return users, fmt.Errorf("read users file: %w", err)
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return users, nil
	}
	if err := json.Unmarshal(raw, &users); err != nil {
		return users, fmt.Errorf("decode users file: %w", err)
	}
	return users, nil
}

func (s *Store) saveUsers(users usersFile) error {
	raw, err := json.MarshalIndent(users, "", "  ")
	if err != nil {
		return fmt.Errorf("encode users file: %w", err)
	}
	raw = append(raw, '\n')
	if err := atomicwrite.Write(s.usersPath(), raw); err != nil {
		return err
	}
	return nil
}

func (s *Store) loadSessions() ([]Session, error) {
	var sessions []Session
	if err := readJSONL(s.sessionsPath(), func(raw []byte) error {
		var session Session
		if err := json.Unmarshal(raw, &session); err != nil {
			return err
		}
		sessions = append(sessions, session)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("load sessions: %w", err)
	}
	return sessions, nil
}

func (s *Store) saveSessions(sessions []Session) error {
	return writeJSONL(s.sessionsPath(), sessions)
}

func (s *Store) loadPairingCodes() ([]PairingCode, error) {
	var codes []PairingCode
	if err := readJSONL(s.pairingPath(), func(raw []byte) error {
		var code PairingCode
		if err := json.Unmarshal(raw, &code); err != nil {
			return err
		}
		codes = append(codes, code)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("load pairing codes: %w", err)
	}
	return codes, nil
}

func (s *Store) savePairingCodes(codes []PairingCode) error {
	return writeJSONL(s.pairingPath(), codes)
}

func (s *Store) authDir() string {
	return filepath.Join(s.workspaceDir, "auth")
}

func (s *Store) usersPath() string {
	return filepath.Join(s.authDir(), "users.json")
}

func (s *Store) sessionsPath() string {
	return filepath.Join(s.authDir(), "sessions.jsonl")
}

func (s *Store) pairingPath() string {
	return filepath.Join(s.authDir(), "pairing.jsonl")
}

func userRecordForRole(users usersFile, role string) *UserRecord {
	switch role {
	case RoleAdmin:
		return users.Admin
	case RoleUser:
		return users.User
	default:
		return nil
	}
}

func normalizeRole(role string) (string, error) {
	normalized := strings.TrimSpace(strings.ToLower(role))
	switch normalized {
	case RoleAdmin, RoleUser:
		return normalized, nil
	default:
		return "", fmt.Errorf("unsupported console auth role %q", role)
	}
}

func hashPassword(password string, reader io.Reader) (string, error) {
	salt, err := randomBytes(reader, argon2idSaltBytes)
	if err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, argon2idIterations, argon2idMemoryKiB, argon2idParallelism, argon2idKeyBytes)
	return fmt.Sprintf(
		"$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argon2idMemoryKiB,
		argon2idIterations,
		argon2idParallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

func verifyPasswordHash(password, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false, errors.New("unsupported password hash format")
	}
	var memory uint32
	var iterations uint32
	var parallelism uint32
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism); err != nil {
		return false, fmt.Errorf("parse argon2id params: %w", err)
	}
	if parallelism == 0 || parallelism > 255 {
		return false, errors.New("invalid argon2id parallelism")
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, fmt.Errorf("decode argon2id salt: %w", err)
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, fmt.Errorf("decode argon2id hash: %w", err)
	}
	actual := argon2.IDKey([]byte(password), salt, iterations, memory, uint8(parallelism), uint32(len(expected)))
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}

func randomOpaqueID(reader io.Reader, size int) (string, error) {
	raw, err := randomBytes(reader, size)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func randomBytes(reader io.Reader, size int) ([]byte, error) {
	if reader == nil {
		reader = rand.Reader
	}
	raw := make([]byte, size)
	if _, err := io.ReadFull(reader, raw); err != nil {
		return nil, fmt.Errorf("read random bytes: %w", err)
	}
	return raw, nil
}

func randomDigits(reader io.Reader, digits int) (string, error) {
	max := big.NewInt(1)
	ten := big.NewInt(10)
	for i := 0; i < digits; i++ {
		max.Mul(max, ten)
	}
	n, err := rand.Int(reader, max)
	if err != nil {
		return "", fmt.Errorf("generate random digits: %w", err)
	}
	return fmt.Sprintf("%0*d", digits, n.Int64()), nil
}

func readJSONL(path string, each func([]byte) error) error {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if err := each([]byte(line)); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func writeJSONL[T any](path string, items []T) error {
	var builder strings.Builder
	encoder := json.NewEncoder(&builder)
	for _, item := range items {
		if err := encoder.Encode(item); err != nil {
			return fmt.Errorf("encode jsonl item: %w", err)
		}
	}
	return atomicwrite.Write(path, []byte(builder.String()))
}
