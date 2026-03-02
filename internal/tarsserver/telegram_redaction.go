package tarsserver

import (
	"regexp"
	"strings"
)

var telegramBotPathPattern = regexp.MustCompile(`/bot[^/\s]+/`)

func sanitizeTelegramLogText(raw, botToken string) string {
	text := strings.TrimSpace(raw)
	if text == "" {
		return text
	}
	token := strings.TrimSpace(botToken)
	if token != "" {
		text = strings.ReplaceAll(text, token, "[REDACTED]")
	}
	return telegramBotPathPattern.ReplaceAllString(text, "/bot[REDACTED]/")
}

func sanitizeTelegramError(err error, botToken string) string {
	if err == nil {
		return ""
	}
	return sanitizeTelegramLogText(err.Error(), botToken)
}
