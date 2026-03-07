package tarsserver

import (
	"fmt"
	"strings"
)

func (h *telegramInboundHandler) applyPolicy(userID int64, chatID, username string) (bool, string, string) {
	policy := normalizeTelegramDMPolicy(h.dmPolicy)
	switch policy {
	case "disabled":
		return false, "telegram direct messages are disabled.", policy
	case "open":
		return true, "", policy
	case "allowlist":
		if h.pairings == nil || !h.pairings.isAllowed(userID) {
			return false, "telegram access is restricted. ask the bot owner to allow your account.", policy
		}
		return true, "", policy
	default:
		if h.pairings == nil {
			return false, "telegram pairing is not configured.", policy
		}
		if h.pairings.isAllowed(userID) {
			return true, "", policy
		}
		entry, _, err := h.pairings.issue(telegramPairingIdentity{
			UserID:   userID,
			ChatID:   chatID,
			Username: username,
		}, telegramPairingTTL)
		if err != nil {
			h.logger.Error().Err(err).Int64("user_id", userID).Str("chat_id", chatID).Msg("telegram pairing issue failed")
			return false, "failed to issue pairing code. please retry.", policy
		}
		message := fmt.Sprintf(
			"Pairing code: %s\nAsk the bot owner to approve with: /telegram pairing approve %s",
			entry.Code,
			entry.Code,
		)
		return false, message, policy
	}
}

func normalizeTelegramDMPolicy(raw string) string {
	policy := strings.TrimSpace(strings.ToLower(raw))
	switch policy {
	case "pairing", "allowlist", "open", "disabled":
		return policy
	default:
		return "pairing"
	}
}

func normalizeTelegramSessionScope(raw string) string {
	scope := strings.TrimSpace(strings.ToLower(raw))
	switch scope {
	case "main", "per-user":
		return scope
	default:
		return "main"
	}
}
