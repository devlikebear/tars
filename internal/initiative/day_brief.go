package initiative

import (
	"fmt"
	"strings"
	"time"
	"unicode"
)

// Morning brief and evening recap (tars#1001 brief). Both are worked out
// in Go from facts the server already has and written in fixed frames,
// English and Korean, like the arrival brief (brief.go): no model writes,
// picks or summarizes a line, so no user text goes anywhere for them.
//
// The morning brief has no intent of its own. It rides on the day's first
// arrival utterance — the greet, or the follow_up that replaces it — which
// #1269 already limits to once a local day, and goes out in CASE's bubble
// next to the arrival brief. The evening recap is its own intent
// (IntentEveningRecap): a written message, once a day, inside a window
// before quiet hours.

// WorkItem is one piece of work a brief names: a focus pipeline, a session
// goal or a cron job. Href is a console path built by the server.
type WorkItem struct {
	Title string `json:"title,omitempty"`
	// Stage is where an open item stands (a focus stage label), empty when
	// there is nothing to add.
	Stage string `json:"stage,omitempty"`
	Href  string `json:"href,omitempty"`
}

// CronRun is one cron job that runs during the rest of today: its first
// remaining run, or its interval for an every:/@every job.
type CronRun struct {
	Name  string
	At    time.Time
	Every time.Duration
}

// DayLog counts one day's daily log by kind of line: chat turns, the
// distinct sessions they came from, and cron runs. Only counts — the log's
// words stay in the log.
type DayLog struct {
	ChatTurns int
	Sessions  int
	CronRuns  int
}

// MorningFacts is what a morning brief reads.
type MorningFacts struct {
	Cron      []CronRun
	Yesterday DayLog
	Open      []WorkItem
}

// RecapFacts is what an evening recap reads: work finished today, what
// failed today, and what is still open.
type RecapFacts struct {
	Done   []WorkItem
	Failed []WorkItem
	Open   []WorkItem
}

// Empty reports whether a recap would have nothing to say.
func (f *RecapFacts) Empty() bool {
	return f == nil || len(f.Done)+len(f.Failed)+len(f.Open) == 0
}

// Day brief kinds, next to the arrival brief's (brief.go).
const (
	BriefSchedule  = "schedule"
	BriefYesterday = "yesterday"
	BriefOpen      = "open"
	BriefDone      = "done"
)

// maxDayBriefNames is how many items one line names before "and N more".
const maxDayBriefNames = 3

// MorningBrief is the morning brief's lines: the rest of today's cron
// runs, yesterday's daily log in counts, and the work still open — one line
// each, in that order, a line left out when it has nothing to say. locale
// picks the words ("ko…" Korean, anything else English); loc is the zone
// run times are shown in.
func MorningBrief(f MorningFacts, loc *time.Location, locale string) []BriefItem {
	ko := koreanLocale(locale)
	if loc == nil {
		loc = time.Local
	}
	var out []BriefItem
	if len(f.Cron) > 0 {
		names := make([]string, 0, len(f.Cron))
		for _, run := range f.Cron {
			name := briefTitle(run.Name, ko)
			switch {
			case run.Every > 0 && ko:
				names = append(names, fmt.Sprintf(`"%s" %s마다`, name, shortDuration(run.Every)))
			case run.Every > 0:
				names = append(names, fmt.Sprintf(`"%s" every %s`, name, shortDuration(run.Every)))
			case ko:
				names = append(names, fmt.Sprintf(`"%s" %s`, name, run.At.In(loc).Format("15:04")))
			default:
				names = append(names, fmt.Sprintf(`"%s" at %s`, name, run.At.In(loc).Format("15:04")))
			}
		}
		out = append(out, BriefItem{Kind: BriefSchedule, Severity: BriefInfo, Href: "/console/cron",
			Text: briefText(ko, "Cron today: %s", "오늘 cron: %s", joinNames(names, ko))})
	}
	if line := dayLogLine(f.Yesterday, ko); line != "" {
		out = append(out, BriefItem{Kind: BriefYesterday, Severity: BriefInfo, Href: "/console/memory", Text: line})
	}
	if len(f.Open) > 0 {
		out = append(out, BriefItem{Kind: BriefOpen, Severity: BriefInfo, Href: workHref(f.Open),
			Text: briefText(ko, "In progress: %s", "진행 중: %s", workNames(f.Open, ko))})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// EveningRecap is the evening recap's lines: what finished today, what
// failed today (block), and what is still open. Nil when there is nothing.
func EveningRecap(f RecapFacts, locale string) []BriefItem {
	ko := koreanLocale(locale)
	var out []BriefItem
	if len(f.Done) > 0 {
		out = append(out, BriefItem{Kind: BriefDone, Severity: BriefInfo, Href: workHref(f.Done),
			Text: briefText(ko, "Done today: %s", "오늘 끝낸 일: %s", workNames(f.Done, ko))})
	}
	if len(f.Failed) > 0 {
		out = append(out, BriefItem{Kind: BriefFailed, Severity: BriefBlock, Href: workHref(f.Failed),
			Text: briefText(ko, "Failed today: %s", "오늘 실패: %s", workNames(f.Failed, ko))})
	}
	if len(f.Open) > 0 {
		out = append(out, BriefItem{Kind: BriefOpen, Severity: BriefInfo, Href: workHref(f.Open),
			Text: briefText(ko, "Still open: %s", "남은 일: %s", workNames(f.Open, ko))})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// EveningRecapSpeech is the recap as the message written to the main
// session: a fixed first line, then the recap's lines. Empty when there are
// no lines.
func EveningRecapSpeech(items []BriefItem, ko bool) string {
	if len(items) == 0 {
		return ""
	}
	lines := make([]string, 0, len(items)+1)
	if ko {
		lines = append(lines, "오늘 하루 정리예요.")
	} else {
		lines = append(lines, "Here's today's wrap-up.")
	}
	for _, item := range items {
		lines = append(lines, item.Text)
	}
	return strings.Join(lines, "\n")
}

// WithoutSaid drops the open items another line in the same bubble already
// names (an arrival brief line links to the same screen), so one pipeline is
// not reported twice.
func WithoutSaid(open []WorkItem, said []BriefItem) []WorkItem {
	if len(open) == 0 || len(said) == 0 {
		return open
	}
	seen := map[string]bool{}
	for _, item := range said {
		seen[item.Href] = true
	}
	out := make([]WorkItem, 0, len(open))
	for _, w := range open {
		if w.Href != "" && seen[w.Href] {
			continue
		}
		out = append(out, w)
	}
	return out
}

// PrefersKorean picks the language of a written recap: Korean when the
// user's recent messages (or, with none, their profile) are written in
// Hangul. Read in Go only, for this one boolean.
func PrefersKorean(recent []UserMessage, profile string) bool {
	for i := len(recent) - 1; i >= 0; i-- {
		if text := strings.TrimSpace(recent[i].Text); text != "" {
			return hasHangul(text)
		}
	}
	return hasHangul(profile)
}

// DefaultRecapWindow is the hour before quiet hours start ("23:00-07:00"
// gives "22:00-23:00"), or 21:00-22:00 when quiet hours do not parse.
func DefaultRecapWindow(quietHours string) string {
	start, _, ok := parseWindow(quietHours)
	if !ok {
		return "21:00-22:00"
	}
	from := (start - 60 + 24*60) % (24 * 60)
	return fmt.Sprintf("%02d:%02d-%02d:%02d", from/60, from%60, start/60, start%60)
}

func dayLogLine(d DayLog, ko bool) string {
	var parts []string
	if d.ChatTurns > 0 {
		switch {
		case ko && d.Sessions > 0:
			parts = append(parts, fmt.Sprintf("세션 %d곳에서 대화 %d번", d.Sessions, d.ChatTurns))
		case ko:
			parts = append(parts, fmt.Sprintf("대화 %d번", d.ChatTurns))
		case d.Sessions > 0:
			parts = append(parts, fmt.Sprintf("%s in %s", plural(d.ChatTurns, "chat turn"), plural(d.Sessions, "session")))
		default:
			parts = append(parts, plural(d.ChatTurns, "chat turn"))
		}
	}
	if d.CronRuns > 0 {
		if ko {
			parts = append(parts, fmt.Sprintf("cron 실행 %d번", d.CronRuns))
		} else {
			parts = append(parts, plural(d.CronRuns, "cron run"))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return briefText(ko, "Yesterday: %s", "어제: %s", strings.Join(parts, ", "))
}

func workNames(items []WorkItem, ko bool) string {
	names := make([]string, 0, len(items))
	for _, w := range items {
		name := fmt.Sprintf(`"%s"`, briefTitle(w.Title, ko))
		if stage := clipRunes(strings.TrimSpace(w.Stage), maxBriefTitleRunes); stage != "" {
			name += " (" + stage + ")"
		}
		names = append(names, name)
	}
	return joinNames(names, ko)
}

func joinNames(names []string, ko bool) string {
	if len(names) <= maxDayBriefNames {
		return strings.Join(names, ", ")
	}
	rest := len(names) - maxDayBriefNames
	head := strings.Join(names[:maxDayBriefNames], ", ")
	if ko {
		return fmt.Sprintf("%s 외 %d건", head, rest)
	}
	return fmt.Sprintf("%s and %d more", head, rest)
}

// workHref links a line to its one item, or to the console's board when it
// names several.
func workHref(items []WorkItem) string {
	if len(items) == 1 && items[0].Href != "" {
		return items[0].Href
	}
	for _, w := range items[1:] {
		if w.Href != items[0].Href {
			return "/console"
		}
	}
	if items[0].Href != "" {
		return items[0].Href
	}
	return "/console"
}

func shortDuration(d time.Duration) string {
	d = d.Round(time.Minute)
	if d < time.Minute {
		return "1m"
	}
	h, m := int(d/time.Hour), int(d%time.Hour/time.Minute)
	switch {
	case h > 0 && m > 0:
		return fmt.Sprintf("%dh%dm", h, m)
	case h > 0:
		return fmt.Sprintf("%dh", h)
	default:
		return fmt.Sprintf("%dm", m)
	}
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}

func koreanLocale(locale string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(locale)), "ko")
}

func hasHangul(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool { return unicode.Is(unicode.Hangul, r) }) >= 0
}
