package tarsserver

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/devlikebear/tars/internal/session"
)

func (h *telegramCommandHandler) cmdNew(line string) (string, string, error) {
	if h.store == nil {
		return "", "", fmt.Errorf("session store is not configured")
	}
	title := strings.TrimSpace(strings.TrimPrefix(line, "/new"))
	if title == "" {
		title = "chat"
	}
	created, err := h.store.Create(title)
	if err != nil {
		return "", "", err
	}
	return created.ID, fmt.Sprintf("SYSTEM > created session %s (%s)", created.ID, created.Title), nil
}

func (h *telegramCommandHandler) cmdResume(fields []string) (string, string, error) {
	if h.store == nil {
		return "", "", fmt.Errorf("session store is not configured")
	}
	if len(fields) < 2 || strings.TrimSpace(fields[1]) == "" {
		return "", "SYSTEM > usage: /resume {id|number|latest|main}", nil
	}
	arg := strings.TrimSpace(fields[1])
	if strings.EqualFold(arg, "main") {
		mainSessionID := strings.TrimSpace(h.mainSession)
		if mainSessionID == "" {
			return "", "", fmt.Errorf("main session is not configured")
		}
		if _, err := h.store.Get(mainSessionID); err != nil {
			return "", "", err
		}
		if h.sessionScope == "main" {
			return "", fmt.Sprintf("SYSTEM > using main session=%s", mainSessionID), nil
		}
		return mainSessionID, fmt.Sprintf("SYSTEM > resumed session=%s", mainSessionID), nil
	}
	if h.sessionScope == "main" {
		return "", blockInMainSessionMessage(), nil
	}
	if strings.EqualFold(arg, "latest") {
		latest, err := h.store.Latest()
		if err != nil {
			return "", "", err
		}
		return latest.ID, fmt.Sprintf("SYSTEM > resumed session=%s", latest.ID), nil
	}
	if idx, err := strconv.Atoi(arg); err == nil {
		sessions, err := listSessionsOrdered(h.store)
		if err != nil {
			return "", "", err
		}
		if idx <= 0 || idx > len(sessions) {
			return "", "", fmt.Errorf("resume target out of range: %d", idx)
		}
		next := strings.TrimSpace(sessions[idx-1].ID)
		return next, fmt.Sprintf("SYSTEM > resumed session=%s", next), nil
	}
	if _, err := h.store.Get(arg); err != nil {
		return "", "", err
	}
	next := strings.TrimSpace(arg)
	return next, fmt.Sprintf("SYSTEM > resumed session=%s", next), nil
}

func listSessionsOrdered(store *session.Store) ([]session.Session, error) {
	sessions, err := store.List()
	if err != nil {
		return nil, err
	}
	sort.Slice(sessions, func(i, j int) bool {
		if sessions[i].UpdatedAt.Equal(sessions[j].UpdatedAt) {
			return sessions[i].CreatedAt.After(sessions[j].CreatedAt)
		}
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})
	return sessions, nil
}
