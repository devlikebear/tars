package tarsclient

import (
	"fmt"
	"strings"
)

func cmdTelegram(c commandContext) (bool, string, error) {
	if c.fields[0] != "/telegram" {
		return false, c.session, nil
	}
	if len(c.fields) == 1 || strings.EqualFold(strings.TrimSpace(c.fields[1]), "pairings") {
		info, err := c.runtime.telegramPairings(c.ctx)
		if err != nil {
			return true, c.session, err
		}
		fmt.Fprintf(c.stdout, "SYSTEM > telegram dm_policy=%s polling_enabled=%t pending=%d allowed=%d\n",
			strings.TrimSpace(info.DMPolicy),
			info.PollingEnabled,
			len(info.Pending),
			len(info.Allowed),
		)
		for _, item := range info.Pending {
			name := strings.TrimSpace(item.Username)
			if name == "" {
				name = "-"
			}
			fmt.Fprintf(c.stdout, "- pending code=%s user_id=%d chat_id=%s username=%s expires_at=%s\n",
				item.Code,
				item.UserID,
				item.ChatID,
				name,
				item.ExpiresAt,
			)
		}
		for _, item := range info.Allowed {
			name := strings.TrimSpace(item.Username)
			if name == "" {
				name = "-"
			}
			fmt.Fprintf(c.stdout, "- allowed user_id=%d chat_id=%s username=%s approved_at=%s\n",
				item.UserID,
				item.ChatID,
				name,
				item.ApprovedAt,
			)
		}
		return true, c.session, nil
	}
	if len(c.fields) >= 4 && strings.EqualFold(strings.TrimSpace(c.fields[1]), "pairing") && strings.EqualFold(strings.TrimSpace(c.fields[2]), "approve") {
		code := strings.TrimSpace(c.fields[3])
		if code == "" {
			return true, c.session, fmt.Errorf("usage: /telegram pairing approve {code}")
		}
		approved, err := c.runtime.approveTelegramPairing(c.ctx, code)
		if err != nil {
			return true, c.session, err
		}
		name := strings.TrimSpace(approved.Username)
		if name == "" {
			name = "-"
		}
		fmt.Fprintf(c.stdout, "SYSTEM > approved telegram pairing user_id=%d chat_id=%s username=%s\n",
			approved.UserID,
			approved.ChatID,
			name,
		)
		return true, c.session, nil
	}
	return true, c.session, fmt.Errorf("usage: /telegram {pairings|pairing approve {code}}")
}
