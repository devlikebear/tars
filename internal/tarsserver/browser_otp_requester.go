package tarsserver

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/tarsncase/internal/approval"
	"github.com/devlikebear/tarsncase/internal/browser"
)

type browserTelegramOTPRequester struct {
	sender   telegramSender
	pairings *telegramPairingStore
	manager  *approval.OTPManager
}

func newBrowserTelegramOTPRequester(
	sender telegramSender,
	pairings *telegramPairingStore,
	manager *approval.OTPManager,
) browser.OTPRequester {
	if sender == nil || pairings == nil || manager == nil {
		return nil
	}
	return &browserTelegramOTPRequester{
		sender:   sender,
		pairings: pairings,
		manager:  manager,
	}
}

func (r *browserTelegramOTPRequester) RequestOTP(ctx context.Context, siteID string, timeout time.Duration) (string, error) {
	if r == nil || r.sender == nil || r.pairings == nil || r.manager == nil {
		return "", fmt.Errorf("otp requester is not configured")
	}
	chatID, err := r.pairings.resolveDefaultChatID()
	if err != nil {
		return "", err
	}
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return "", fmt.Errorf("otp requires paired telegram chat")
	}
	if timeout <= 0 {
		timeout = 300 * time.Second
	}
	notice := fmt.Sprintf("OTP required for site=%s. Reply in this chat with the OTP code within %ds.", strings.TrimSpace(siteID), int(timeout.Seconds()))
	if _, err := r.sender.Send(ctx, telegramSendRequest{
		ChatID: chatID,
		Text:   notice,
	}); err != nil {
		return "", err
	}
	return r.manager.Request(ctx, chatID, timeout)
}
