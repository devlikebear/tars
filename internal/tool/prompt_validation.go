package tool

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

var naturalPromptLetterPattern = regexp.MustCompile(`[A-Za-z가-힣]`)

var naturalPromptBlockedTokens = []string{
	"&&",
	"||",
	"|",
	";",
	"$(",
	"`",
	"sudo ",
	"rm ",
	"bash ",
	"zsh ",
	"sh ",
	"chmod ",
	"chown ",
	"curl ",
	"wget ",
}

func validateNaturalTaskPrompt(raw string) error {
	prompt := strings.TrimSpace(raw)
	if prompt == "" {
		return fmt.Errorf("prompt is required")
	}
	if utf8.RuneCountInString(prompt) < 4 {
		return fmt.Errorf("prompt는 자연어 할일 문장이어야 합니다")
	}
	if !naturalPromptLetterPattern.MatchString(prompt) {
		return fmt.Errorf("prompt는 자연어 할일 문장이어야 합니다")
	}
	lower := strings.ToLower(prompt)
	for _, token := range naturalPromptBlockedTokens {
		if strings.Contains(lower, token) {
			return fmt.Errorf("prompt는 자연어 할일 문장이어야 합니다")
		}
	}
	return nil
}
