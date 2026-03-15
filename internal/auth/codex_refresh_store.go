package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

const (
	codexRefreshTokenStorageModeEnv  = "TARS_OPENAI_CODEX_REFRESH_TOKEN_STORAGE"
	codexRefreshTokenStorageAuto     = "auto"
	codexRefreshTokenStorageFile     = "file"
	codexRefreshTokenStorageKeychain = "keychain"
	codexRefreshTokenKeychainService = "github.com/devlikebear/tars/openai-codex/refresh-token"
)

type codexRefreshTokenStore interface {
	Name() string
	Load(path string) (string, error)
	Save(path string, token string) error
}

var currentCodexRefreshTokenStore = func() codexRefreshTokenStore {
	mode := normalizeCodexRefreshTokenStorageMode(os.Getenv(codexRefreshTokenStorageModeEnv))
	switch mode {
	case codexRefreshTokenStorageFile:
		return nil
	case codexRefreshTokenStorageKeychain:
		if runtime.GOOS == "darwin" {
			return newKeychainCodexRefreshTokenStore(defaultCodexSecurityRunner)
		}
		return nil
	default:
		if runtime.GOOS == "darwin" {
			return newKeychainCodexRefreshTokenStore(defaultCodexSecurityRunner)
		}
		return nil
	}
}

type codexSecurityRunner func(name string, args ...string) ([]byte, error)

type keychainCodexRefreshTokenStore struct {
	run codexSecurityRunner
}

func newKeychainCodexRefreshTokenStore(run codexSecurityRunner) codexRefreshTokenStore {
	if run == nil {
		run = defaultCodexSecurityRunner
	}
	return keychainCodexRefreshTokenStore{run: run}
}

func (s keychainCodexRefreshTokenStore) Name() string {
	return "macos-keychain"
}

func (s keychainCodexRefreshTokenStore) Load(path string) (string, error) {
	account := codexRefreshTokenStoreAccount(path)
	if account == "" {
		return "", nil
	}
	out, err := s.run("/usr/bin/security", "find-generic-password", "-a", account, "-s", codexRefreshTokenKeychainService, "-w")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (s keychainCodexRefreshTokenStore) Save(path string, token string) error {
	account := codexRefreshTokenStoreAccount(path)
	token = strings.TrimSpace(token)
	if account == "" || token == "" {
		return nil
	}
	_, err := s.run("/usr/bin/security", "add-generic-password", "-U", "-a", account, "-s", codexRefreshTokenKeychainService, "-w", token)
	return err
}

func defaultCodexSecurityRunner(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	return cmd.CombinedOutput()
}

func normalizeCodexRefreshTokenStorageMode(raw string) string {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "", codexRefreshTokenStorageAuto:
		return codexRefreshTokenStorageAuto
	case codexRefreshTokenStorageFile:
		return codexRefreshTokenStorageFile
	case codexRefreshTokenStorageKeychain:
		return codexRefreshTokenStorageKeychain
	default:
		return codexRefreshTokenStorageAuto
	}
}

func codexRefreshTokenStoreAccount(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(trimmed))
	return hex.EncodeToString(sum[:])
}
