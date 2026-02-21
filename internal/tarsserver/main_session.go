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

	latest, err := store.Latest()
	if err == nil {
		if id := strings.TrimSpace(latest.ID); id != "" {
			return id, nil
		}
	}
	if err != nil && !strings.Contains(strings.ToLower(strings.TrimSpace(err.Error())), "session not found") {
		return "", err
	}

	created, err := store.Create("main")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(created.ID), nil
}
