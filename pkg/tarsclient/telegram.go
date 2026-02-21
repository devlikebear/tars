package tarsclient

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

func (c *Client) TelegramPairings(ctx context.Context) (TelegramPairingsInfo, error) {
	var out TelegramPairingsInfo
	if _, err := c.doJSON(ctx, http.MethodGet, "/v1/channels/telegram/pairings", nil, true, &out); err != nil {
		return TelegramPairingsInfo{}, err
	}
	if out.Pending == nil {
		out.Pending = []TelegramPairingPending{}
	}
	if out.Allowed == nil {
		out.Allowed = []TelegramPairingAllowed{}
	}
	return out, nil
}

func (c *Client) ApproveTelegramPairing(ctx context.Context, code string) (TelegramPairingAllowed, error) {
	normalizedCode := strings.TrimSpace(code)
	if normalizedCode == "" {
		return TelegramPairingAllowed{}, fmt.Errorf("pairing code is required")
	}
	var payload struct {
		Approved TelegramPairingAllowed `json:"approved"`
	}
	req := map[string]any{
		"code": normalizedCode,
	}
	if _, err := c.doJSON(ctx, http.MethodPost, "/v1/channels/telegram/pairings/approve", req, true, &payload); err != nil {
		return TelegramPairingAllowed{}, err
	}
	return payload.Approved, nil
}
