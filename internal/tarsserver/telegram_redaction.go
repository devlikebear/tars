package tarsserver

import (
	"net/url"
	"regexp"
	"strings"
)

var (
	telegramBotPathPattern        = regexp.MustCompile(`/bot[^/\\\s]+/`)
	telegramBotEscapedPathPattern = regexp.MustCompile(`\\/bot[^\\/\s]+\\/`)
)

func sanitizeTelegramLogText(raw, botToken string) string {
	text := strings.TrimSpace(raw)
	if text == "" {
		return text
	}
	token := strings.TrimSpace(botToken)
	if token != "" {
		text = strings.ReplaceAll(text, token, "[REDACTED]")
		text = redactTelegramEncodedToken(text, token)
	}
	text = telegramBotEscapedPathPattern.ReplaceAllString(text, `\/bot[REDACTED]\/`)
	return telegramBotPathPattern.ReplaceAllString(text, "/bot[REDACTED]/")
}

func sanitizeTelegramError(err error, botToken string) string {
	if err == nil {
		return ""
	}
	return sanitizeTelegramLogText(err.Error(), botToken)
}

func redactTelegramEncodedToken(text, token string) string {
	encodedVariants := []string{
		url.QueryEscape(token),
		url.PathEscape(token),
	}
	for _, variant := range encodedVariants {
		if variant == "" {
			continue
		}
		re := regexp.MustCompile(`(?i)` + regexp.QuoteMeta(variant))
		text = re.ReplaceAllString(text, "[REDACTED]")
	}
	return text
}
