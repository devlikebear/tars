package tarsserver

import (
	"fmt"
	"strconv"
	"strings"
)

func (h *telegramInboundHandler) resolveSession(userID int64, username string) (string, error) {
	if userID <= 0 {
		return "", fmt.Errorf("user id is required")
	}
	if normalizeTelegramSessionScope(h.sessionScope) == "main" {
		mainSessionID := strings.TrimSpace(h.mainSessionID)
		if mainSessionID != "" {
			if _, err := h.store.Get(mainSessionID); err != nil {
				return "", err
			}
			return mainSessionID, nil
		}
	}
	if h.pairings != nil {
		if sessionID := strings.TrimSpace(h.pairings.sessionID(userID)); sessionID != "" {
			if _, err := h.store.Get(sessionID); err == nil {
				return sessionID, nil
			}
		}
	}

	title := strings.TrimSpace(username)
	if title == "" {
		title = strconv.FormatInt(userID, 10)
	}
	created, err := h.store.Create("telegram:" + title)
	if err != nil {
		return "", err
	}
	if h.pairings != nil {
		if err := h.pairings.bindSession(userID, created.ID); err != nil {
			h.logger.Debug().Err(err).Int64("user_id", userID).Msg("bind telegram session failed")
		}
	}
	return created.ID, nil
}

func (h *telegramInboundHandler) currentSessionID(userID int64) string {
	if normalizeTelegramSessionScope(h.sessionScope) == "main" {
		return strings.TrimSpace(h.mainSessionID)
	}
	if h.pairings == nil || userID <= 0 {
		return ""
	}
	return strings.TrimSpace(h.pairings.sessionID(userID))
}
