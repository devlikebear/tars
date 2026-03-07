package tarsserver

import (
	"fmt"
	"strings"

	"github.com/devlikebear/tarsncase/internal/session"
)

func resolveMainSessionID(store *session.Store, configuredID string) (string, error) {
	if store == nil {
		return "", fmt.Errorf("session store is not configured")
	}
	sessionID := strings.TrimSpace(configuredID)
	if sessionID != "" {
		if _, err := store.Get(sessionID); err != nil {
			return "", fmt.Errorf("session_default_id %q not found: %w", sessionID, err)
		}
		return sessionID, nil
	}
	created, err := store.EnsureMain()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(created.ID), nil
}
