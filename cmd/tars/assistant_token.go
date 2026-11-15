package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/devlikebear/tars/internal/atomicwrite"
)

// The assistant LaunchAgent used to carry the API token as a program
// argument ("--api-token <token>"): visible to anyone running ps, and written
// into a 0644 plist under ~/Library/LaunchAgents. install-launchagent now
// writes the token to a file only its owner can read and puts the file's path
// in the plist; `assistant start` reads it with --api-token-file. This is how
// the desktop shell keeps its tokens (tars-desktop/config.json, 0600).

// assistantUserConfigDir is os.UserConfigDir, replaceable in tests.
var assistantUserConfigDir = os.UserConfigDir

// defaultAssistantTokenPath is <user config dir>/tars-assistant/api-token.
func defaultAssistantTokenPath() (string, error) {
	dir, err := assistantUserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find the user config directory for the assistant token: %w", err)
	}
	return filepath.Join(dir, "tars-assistant", "api-token"), nil
}

// writeAssistantToken stores token at path: the directory 0700, the file
// 0600, replaced atomically so a reader never sees half a token.
func writeAssistantToken(path, token string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create assistant token directory: %w", err)
	}
	if err := atomicwrite.Write(path, []byte(token+"\n")); err != nil {
		return fmt.Errorf("write assistant token: %w", err)
	}
	// atomicwrite creates 0600 already; this keeps the guarantee if that
	// default ever changes.
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("restrict assistant token file: %w", err)
	}
	return nil
}

// resolveAssistantAPIToken picks the token `assistant start` runs with:
// --api-token or TARS_API_TOKEN first, then --api-token-file.
func resolveAssistantAPIToken(opts assistantOptions) (string, error) {
	if token := strings.TrimSpace(opts.apiToken); token != "" {
		return token, nil
	}
	if path := strings.TrimSpace(opts.apiTokenFile); path != "" {
		return readAssistantToken(path)
	}
	return "", nil
}

// readAssistantToken reads a token written by writeAssistantToken. A file
// named on the command line that is missing or empty is an error: the
// assistant would otherwise start unauthenticated and fail on every request.
func readAssistantToken(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read assistant token file: %w", err)
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", errors.New("assistant token file is empty: " + path)
	}
	return token, nil
}
