package initiative

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// Arrival brief (case-companion phase 3): when a greet goes out, CASE's
// bubble carries up to three lines on what changed while the user was
// away. Brief is the whole judgment and it is pure — two snapshots in, the
// lines out. No model writes or picks a line: the words are fixed frames
// filled with titles the server already has, and each line links to the
// screen it is about, built here from ids (the same links CASE's Bridge
// cards use, internal/casebridge).

// StatusSnapshot is what TARS looked like at one moment, reduced to what
// Brief compares. Pipelines are keyed by their session id, cron jobs by job
// id.
type StatusSnapshot struct {
	At        time.Time                `json:"at"`
	Pipelines map[string]PipelinePoint `json:"pipelines,omitempty"`
	Cron      map[string]CronPoint     `json:"cron,omitempty"`
	// ApprovalSessions are the sessions with a chat turn waiting for a tool
	// approval, one entry per waiting approval; QueuedApprovals counts the
	// unattended runs' calls waiting in the ops queue.
	ApprovalSessions []string `json:"approval_sessions,omitempty"`
	QueuedApprovals  int      `json:"queued_approvals,omitempty"`
}

// PipelinePoint is one focus pipeline in a snapshot.
type PipelinePoint struct {
	Title    string `json:"title,omitempty"`
	Stage    string `json:"stage,omitempty"`
	Gate     string `json:"gate,omitempty"`
	Waiting  bool   `json:"waiting,omitempty"`
	Finished bool   `json:"finished,omitempty"`
}

// CronPoint is one cron job in a snapshot: whether its last run failed.
type CronPoint struct {
	Name   string `json:"name,omitempty"`
	Failed bool   `json:"failed,omitempty"`
}

// Brief item kinds and severities.
const (
	BriefWaiting   = "waiting"
	BriefFailed    = "failed"
	BriefFinished  = "finished"
	BriefRecovered = "recovered"
	BriefAdvanced  = "advanced"

	BriefBlock = "block"
	BriefInfo  = "info"
)

// MaxBriefItems is how many lines one brief keeps.
const MaxBriefItems = 3

const maxBriefTitleRunes = 60

// BriefItem is one line of an arrival brief.
type BriefItem struct {
	Kind     string `json:"kind"`
	Severity string `json:"severity"`
	Text     string `json:"text"`
	Href     string `json:"href"`
}

// briefOrder ranks kinds within a severity: what needs the user first.
var briefOrder = map[string]int{BriefWaiting: 0, BriefFailed: 1, BriefFinished: 2, BriefRecovered: 3, BriefAdvanced: 4}

type briefCandidate struct {
	BriefItem
	sortKey string
}

// Brief lists what changed between prev (the last moment the user was at
// the console) and now, blocking items first, at most MaxBriefItems. A zero
// prev (no snapshot yet) or no change returns nil: the greet then goes out
// without a summary. locale picks the words ("ko…" Korean, anything else
// English); the items and their order never depend on it.
//
//   - waiting (block): a pipeline that now waits on the user and did not
//     before, or waits at another gate or stage than before
//   - failed (block): a cron job whose last run now fails and did not
//   - waiting (block): more tool approvals waiting than before, as one line
//   - finished (info): a pipeline that finished
//   - recovered (info): a cron job that failed before and now does not
//   - advanced (info): a running pipeline that moved to another stage
//
// A pipeline or job missing from either side says nothing (a deleted
// pipeline did not "finish", a job that aged out did not "recover"), except
// that a new pipeline already waiting, or a new job already failing, is
// news.
func Brief(prev, now StatusSnapshot, locale string) []BriefItem {
	if prev.At.IsZero() {
		return nil
	}
	ko := strings.HasPrefix(strings.ToLower(strings.TrimSpace(locale)), "ko")
	var out []briefCandidate
	add := func(kind, severity, text, href, key string) {
		out = append(out, briefCandidate{BriefItem: BriefItem{Kind: kind, Severity: severity, Text: text, Href: href}, sortKey: key})
	}

	for _, id := range sortedKeys(now.Pipelines) {
		n := now.Pipelines[id]
		p, had := prev.Pipelines[id]
		href := "/console/focus/" + id
		title := briefTitle(n.Title, ko)
		switch {
		case n.Finished:
			if had && !p.Finished {
				add(BriefFinished, BriefInfo, briefText(ko, `"%s" finished`, `"%s" — 끝났어요`, title), href, n.Title+id)
			}
		case n.Waiting:
			if !had || !p.Waiting || p.Gate != n.Gate || p.Stage != n.Stage {
				if gate := briefGate(n.Gate, ko); gate != "" {
					add(BriefWaiting, BriefBlock, briefText(ko, `"%s" is waiting at %s`, `"%s" — %s에서 기다려요`, title, gate), href, n.Title+id)
				} else {
					add(BriefWaiting, BriefBlock, briefText(ko, `"%s" needs your input`, `"%s" — 입력을 기다려요`, title), href, n.Title+id)
				}
			}
		case had && !p.Finished && p.Stage != n.Stage && strings.TrimSpace(n.Stage) != "":
			add(BriefAdvanced, BriefInfo, briefText(ko, `"%s" moved to %s`, `"%s" — %s 단계로 넘어갔어요`, title, clipRunes(n.Stage, maxBriefTitleRunes)), href, n.Title+id)
		}
	}

	for _, id := range sortedKeys(now.Cron) {
		n := now.Cron[id]
		p, had := prev.Cron[id]
		name := briefTitle(n.Name, ko)
		switch {
		case n.Failed && (!had || !p.Failed):
			add(BriefFailed, BriefBlock, briefText(ko, `Cron job "%s" failed`, `cron 작업 "%s" 실패`, name), "/console/cron", n.Name+id)
		case !n.Failed && had && p.Failed:
			add(BriefRecovered, BriefInfo, briefText(ko, `Cron job "%s" works again`, `cron 작업 "%s" 다시 정상`, name), "/console/cron", n.Name+id)
		}
	}

	if total := len(now.ApprovalSessions) + now.QueuedApprovals; total > len(prev.ApprovalSessions)+prev.QueuedApprovals {
		href := "/console/ops"
		if now.QueuedApprovals == 0 {
			href = "/console/chat/" + now.ApprovalSessions[0]
		}
		var text string
		switch {
		case ko:
			text = fmt.Sprintf("승인 %d건이 기다려요", total)
		case total == 1:
			text = "1 approval is waiting for you"
		default:
			text = fmt.Sprintf("%d approvals are waiting for you", total)
		}
		add(BriefWaiting, BriefBlock, text, href, "")
	}

	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Severity != b.Severity {
			return a.Severity == BriefBlock
		}
		if briefOrder[a.Kind] != briefOrder[b.Kind] {
			return briefOrder[a.Kind] < briefOrder[b.Kind]
		}
		return a.sortKey < b.sortKey
	})
	if len(out) > MaxBriefItems {
		out = out[:MaxBriefItems]
	}
	items := make([]BriefItem, 0, len(out))
	for _, c := range out {
		items = append(items, c.BriefItem)
	}
	if len(items) == 0 {
		return nil
	}
	return items
}

func briefText(ko bool, en, kor string, args ...any) string {
	if ko {
		return fmt.Sprintf(kor, args...)
	}
	return fmt.Sprintf(en, args...)
}

// briefGate names a focus gate the way the console's pipeline screen does
// (G1 plan, G2 triage, G3 PR, G4 merge); empty for no gate.
func briefGate(gate string, ko bool) string {
	en := map[string]string{"plan": "G1 (plan)", "triage": "G2 (review)", "pr": "G3 (PR)", "merge": "G4 (merge)", "blocked": "a blocked step"}
	kor := map[string]string{"plan": "G1(계획)", "triage": "G2(리뷰)", "pr": "G3(PR)", "merge": "G4(머지)", "blocked": "막힌 단계"}
	if ko {
		return kor[gate]
	}
	return en[gate]
}

func briefTitle(title string, ko bool) string {
	title = clipRunes(strings.ReplaceAll(strings.Join(strings.Fields(title), " "), `"`, "'"), maxBriefTitleRunes)
	if title == "" {
		if ko {
			return "제목 없음"
		}
		return "untitled"
	}
	return title
}

func clipRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
