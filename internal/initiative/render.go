package initiative

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	maxMessageRunes = 200
	maxProfileRunes = 600
	maxTextMessages = 5
	textWindow      = 2 * time.Hour
)

// RenderState builds the English state a System One reads. User text goes in
// only when includeText is true (a loopback backend). The returned key hashes
// the local date and the text, so a cached reading survives clock ticks but
// not a new day; it is empty when there is no text to read.
func RenderState(cfg Config, obs Observation, includeText bool) (string, string) {
	now := obs.Now.In(cfg.Location)
	lines := []string{"now: " + now.Format("Mon 2006-01-02 15:04")}
	if obs.ConsoleConnectedAt.IsZero() {
		lines = append(lines, "console: not connected")
	} else {
		lines = append(lines, "console: connected "+ago(obs.Now.Sub(obs.ConsoleConnectedAt)))
	}
	if n := len(obs.RecentUser); n > 0 {
		lines = append(lines, "last_user_message: "+ago(obs.Now.Sub(obs.RecentUser[n-1].At)))
	} else {
		lines = append(lines, "last_user_message: none")
	}
	if !includeText {
		return strings.Join(lines, "\n"), ""
	}

	var textParts []string
	var recent []UserMessage
	for _, m := range obs.RecentUser {
		if obs.Now.Sub(m.At) <= textWindow && strings.TrimSpace(m.Text) != "" {
			recent = append(recent, m)
		}
	}
	if len(recent) > maxTextMessages {
		recent = recent[len(recent)-maxTextMessages:]
	}
	if len(recent) > 0 {
		lines = append(lines, "recent_user_messages (newest last):")
		for _, m := range recent {
			text := truncateRunes(strings.TrimSpace(m.Text), maxMessageRunes)
			lines = append(lines, fmt.Sprintf("- %s (%s)", strconv.Quote(text), ago(obs.Now.Sub(m.At))))
			textParts = append(textParts, text)
		}
	}
	if profile := truncateRunes(strings.TrimSpace(obs.Profile), maxProfileRunes); profile != "" {
		lines = append(lines, "profile:", profile)
		textParts = append(textParts, profile)
	}
	if len(textParts) == 0 {
		return strings.Join(lines, "\n"), ""
	}
	sum := sha256.Sum256([]byte(now.Format("2006-01-02") + "\x00" + strings.Join(textParts, "\x00")))
	return strings.Join(lines, "\n"), hex.EncodeToString(sum[:8])
}

func ago(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
