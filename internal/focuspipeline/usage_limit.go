package focuspipeline

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// A spent usage allowance (vibe-coder-identity.md §4, phase 4): a turn the
// provider refused because the subscription's limit is used up. Nothing is
// wrong with the work, and nothing the team does helps before the limit
// lifts, hours later. The facts below let easy mode say so in plain words
// and when it comes back, and let goal mode stop instead of retrying.

// resetsWords finds Claude Code's "resets 2:40pm (Asia/Seoul)" and keeps
// what follows the word up to the end of that line.
var resetsWords = regexp.MustCompile(`(?i)\bresets?\s+(?:at\s+)?([^\n\r]+)`)

// resetsEpoch finds the older Claude Code wording, "Claude AI usage limit
// reached|1731600000", where the number is when the limit lifts.
var resetsEpoch = regexp.MustCompile(`(?i)limit reached\|(\d{9,11})`)

// maxResetsRunes bounds the provider's words kept for when the limit lifts:
// a time and a zone, never the rest of a long error.
const maxResetsRunes = 48

// UsageLimitResets reads when a spent usage limit lifts from the error
// text: the provider's own words (Claude Code's "2:40pm (Asia/Seoul)"), or
// a time when the provider gave one as a number. Both are empty when the
// text does not say.
func UsageLimitResets(errText string) (string, *time.Time) {
	if m := resetsEpoch.FindStringSubmatch(errText); m != nil {
		if sec, err := strconv.ParseInt(m[1], 10, 64); err == nil {
			at := time.Unix(sec, 0).UTC()
			return "", &at
		}
	}
	m := resetsWords.FindStringSubmatch(errText)
	if m == nil {
		return "", nil
	}
	words := strings.TrimSpace(m[1])
	// Quotes and the JSON around a message the error carries are not part
	// of the time.
	if i := strings.IndexAny(words, "\"}"); i >= 0 {
		words = words[:i]
	}
	words = strings.TrimRight(strings.TrimSpace(words), ".,;:·")
	if r := []rune(words); len(r) > maxResetsRunes {
		words = strings.TrimSpace(string(r[:maxResetsRunes]))
	}
	return words, nil
}

// UsageLimitStop is the open blocked gate's fact when the pipeline stopped
// because the provider's usage limit refused the turn it sent, and nil
// otherwise. A fact recorded before the usage limit was read is read from
// its error text.
func (p Pipeline) UsageLimitStop() *BlockedFact {
	if p.OpenGate != GateBlocked {
		return nil
	}
	fact := p.openBlockedFact()
	if fact.Reason != BlockedTurnFailed {
		return nil
	}
	if !fact.UsageLimit {
		if !UsageLimitError(fact.Error) {
			return nil
		}
		fact.UsageLimit = true
		fact.Resets, fact.ResetsAt = UsageLimitResets(fact.Error)
	}
	return &fact
}
