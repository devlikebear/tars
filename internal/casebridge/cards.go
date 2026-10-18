package casebridge

import (
	"encoding/json"
	"regexp"
	"strings"
)

// Card is one live link under CASE's answer. The model proposes Kind, ID,
// Title, Sub and Severity in a <case-cards> block; Href is always built
// here, never taken from the model.
type Card struct {
	Kind     string `json:"kind"`
	ID       string `json:"id,omitempty"`
	Title    string `json:"title"`
	Sub      string `json:"sub,omitempty"`
	Severity string `json:"severity"`
	Href     string `json:"href"`
}

// Card kinds. focus, session and cron name one thing by id; approvals,
// usage and pulse are whole screens.
const (
	KindFocus     = "focus"
	KindSession   = "session"
	KindCron      = "cron"
	KindApprovals = "approvals"
	KindUsage     = "usage"
	KindPulse     = "pulse"
)

// MaxCards is how many cards one answer keeps.
const MaxCards = 4

const (
	maxCardTitleBytes = 80
	maxCardSubBytes   = 120
)

var cardsBlock = regexp.MustCompile(`(?s)\s*<case-cards>(.*?)</case-cards>\s*`)

// SplitCards takes the <case-cards> block out of an answer: the text
// without it, and the cards it proposed (none when the block is missing or
// not a JSON array of objects). Only the first block counts; every block
// is removed from the text.
func SplitCards(answer string) (string, []Card) {
	var proposed []Card
	if m := cardsBlock.FindStringSubmatch(answer); m != nil {
		var raw []Card
		if err := json.Unmarshal([]byte(strings.TrimSpace(m[1])), &raw); err == nil {
			proposed = raw
		}
	}
	text := strings.TrimSpace(cardsBlock.ReplaceAllString(answer, "\n"))
	return text, proposed
}

// ValidCards keeps the proposed cards that point at something in the
// snapshot CASE answered from, builds each one's console link, cleans the
// model's text, drops duplicates and keeps at most MaxCards.
func ValidCards(proposed []Card, s Snapshot) []Card {
	known := knownIDs(s)
	seen := map[string]bool{}
	out := []Card{}
	for _, c := range proposed {
		kind := strings.ToLower(strings.TrimSpace(c.Kind))
		id := strings.TrimSpace(c.ID)
		href, ok := cardHref(kind, id, known, s)
		if !ok {
			continue
		}
		key := kind + ":" + id
		if seen[key] {
			continue
		}
		seen[key] = true
		severity := "info"
		if strings.EqualFold(strings.TrimSpace(c.Severity), "block") {
			severity = "block"
		}
		title := clip(oneLine(c.Title), maxCardTitleBytes)
		if title == "" {
			title = known[key]
		}
		out = append(out, Card{
			Kind:     kind,
			ID:       id,
			Title:    title,
			Sub:      clip(oneLine(c.Sub), maxCardSubBytes),
			Severity: severity,
			Href:     href,
		})
		if len(out) == MaxCards {
			break
		}
	}
	return out
}

// knownIDs maps "kind:id" to a fallback title for everything the snapshot
// names by id.
func knownIDs(s Snapshot) map[string]string {
	known := map[string]string{}
	for _, p := range s.Pipelines {
		known[KindFocus+":"+p.SessionID] = p.Title
		known[KindSession+":"+p.SessionID] = p.Title
	}
	for _, t := range s.Running {
		known[KindSession+":"+t.SessionID] = t.Title
	}
	for _, a := range s.Approvals {
		if !a.Queued {
			known[KindSession+":"+a.SessionID] = a.Title
		}
	}
	for _, f := range s.Cron {
		known[KindCron+":"+f.JobID] = f.Name
	}
	return known
}

func cardHref(kind, id string, known map[string]string, s Snapshot) (string, bool) {
	switch kind {
	case KindFocus, KindSession, KindCron:
		if id == "" {
			return "", false
		}
		if _, ok := known[kind+":"+id]; !ok {
			return "", false
		}
		switch kind {
		case KindFocus:
			return "/console/focus/" + id, true
		case KindSession:
			return "/console/chat/" + id, true
		default:
			return "/console/cron", true
		}
	case KindApprovals:
		for _, a := range s.Approvals {
			if a.Queued {
				return "/console/ops", true
			}
		}
		return "", false
	case KindUsage:
		return "/console/analytics", s.Usage.Known
	case KindPulse:
		return "/console/pulse", len(s.Pulse) > 0
	}
	return "", false
}
