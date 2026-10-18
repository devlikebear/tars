// Package casebridge is CASE's Bridge (case-companion phase 2): the facts
// CASE answers "what is going on in TARS?" from, and the cards it may point
// at in its answer.
//
// The server gathers a Snapshot in Go from the stores it already has
// (focus pipelines, chat activity, the ops queue, cron, usage, pulse) and
// Render turns it into the text CASE reads. Nothing here asks a model: the
// snapshot is computed, and the cards a model names are kept only when
// they point at something in that same snapshot (ValidCards). There is no
// LLM tool behind it on purpose — CLI providers cannot call TARS tools, and
// a registered tool would ride on every other session's prompt too.
package casebridge

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Snapshot is TARS's state at one moment, as CASE is allowed to see it.
type Snapshot struct {
	At        time.Time
	Pipelines []Pipeline
	Running   []Turn
	Approvals []Approval
	Cron      []CronFailure
	Usage     Usage
	Pulse     []Signal
}

// Pipeline is one focus pipeline.
type Pipeline struct {
	SessionID  string
	Title      string
	Stage      string
	Gate       string
	NeedsInput bool
	GoalMode   bool
	Finished   bool
	UpdatedAt  time.Time
}

// Turn is one chat turn running in a visible session.
type Turn struct {
	SessionID string
	Title     string
	StartedAt time.Time
}

// Approval is a tool call waiting on the user: asked in a chat turn
// (answered in that session) or queued by an unattended run (answered on
// the Ops page).
type Approval struct {
	SessionID string
	Title     string
	Tool      string
	Queued    bool
	Source    string
	At        time.Time
}

// CronFailure is a cron job whose last run failed.
type CronFailure struct {
	JobID string
	Name  string
	Error string
	At    time.Time
}

// Usage is the spend so far. Known is false when no tracker answered.
type Usage struct {
	Known    bool
	TodayUSD float64
	WeekUSD  float64
}

// Signal is one pulse watchdog finding from its latest tick.
type Signal struct {
	Kind     string
	Severity string
	Summary  string
}

// Budget of the rendered text: CASE reads it on every question, so it stays
// a summary — the top few of each kind, never the whole store.
const (
	MaxBytes      = 4000
	maxPerSection = 5
	maxTitleBytes = 80
	maxErrorBytes = 140
)

// Render is the text CASE answers from. Ids appear in brackets
// ([focus:<id>], [session:<id>], [cron:<id>]) so its answer can name them
// in a <case-cards> block. Sections with nothing to say are left out, and
// an empty snapshot says so in one line.
func Render(s Snapshot) string {
	var b strings.Builder
	fmt.Fprintf(&b, "TARS status at %s.\n", s.At.Format("2006-01-02 15:04 MST"))

	waiting := waitingLines(s)
	running := runningLines(s)
	failures := failureLines(s)
	pulse := pulseLines(s)
	section(&b, "Waiting on the user", waiting)
	section(&b, "Running", running)
	section(&b, "Recent failures", failures)
	section(&b, "Watchdog (pulse)", pulse)
	if s.Usage.Known {
		fmt.Fprintf(&b, "Usage: today $%.2f, this week $%.2f [usage]\n", s.Usage.TodayUSD, s.Usage.WeekUSD)
	}
	if len(waiting)+len(running)+len(failures)+len(pulse) == 0 {
		b.WriteString("Nothing is waiting, running or failing right now.\n")
	}
	return clip(b.String(), MaxBytes)
}

func section(b *strings.Builder, title string, lines []string) {
	if len(lines) == 0 {
		return
	}
	fmt.Fprintf(b, "%s:\n", title)
	shown := lines
	if len(shown) > maxPerSection {
		shown = shown[:maxPerSection]
	}
	for _, l := range shown {
		b.WriteString("- ")
		b.WriteString(l)
		b.WriteByte('\n')
	}
	if more := len(lines) - len(shown); more > 0 {
		fmt.Fprintf(b, "- and %d more\n", more)
	}
}

func waitingLines(s Snapshot) []string {
	approvals := append([]Approval(nil), s.Approvals...)
	sort.SliceStable(approvals, func(i, j int) bool { return approvals[i].At.Before(approvals[j].At) })
	var out []string
	for _, a := range approvals {
		if a.Queued {
			source := ""
			if a.Source != "" {
				source = " from " + a.Source
			}
			out = append(out, fmt.Sprintf("unattended run%s waits for approval of %s in %s, %s [approvals]",
				source, label(a.Tool, "a tool"), quoted(a.Title), ago(s.At, a.At)))
			continue
		}
		out = append(out, fmt.Sprintf("approval of %s in %s, %s [session:%s]",
			label(a.Tool, "a tool"), quoted(a.Title), ago(s.At, a.At), a.SessionID))
	}
	for _, p := range sortedPipelines(s.Pipelines) {
		if p.Finished || !p.NeedsInput {
			continue
		}
		gate := ""
		if p.Gate != "" {
			gate = " at gate " + p.Gate
		}
		out = append(out, fmt.Sprintf("focus pipeline %s waits%s (stage %s) since %s [focus:%s]",
			quoted(p.Title), gate, label(p.Stage, "?"), ago(s.At, p.UpdatedAt), p.SessionID))
	}
	return out
}

func runningLines(s Snapshot) []string {
	turns := append([]Turn(nil), s.Running...)
	sort.SliceStable(turns, func(i, j int) bool { return turns[i].StartedAt.Before(turns[j].StartedAt) })
	var out []string
	for _, t := range turns {
		out = append(out, fmt.Sprintf("a turn in %s, running for %s [session:%s]", quoted(t.Title), since(s.At, t.StartedAt), t.SessionID))
	}
	for _, p := range sortedPipelines(s.Pipelines) {
		if p.Finished || p.NeedsInput {
			continue
		}
		goal := ""
		if p.GoalMode {
			goal = ", goal mode"
		}
		out = append(out, fmt.Sprintf("focus pipeline %s at stage %s%s [focus:%s]", quoted(p.Title), label(p.Stage, "?"), goal, p.SessionID))
	}
	return out
}

func failureLines(s Snapshot) []string {
	failures := append([]CronFailure(nil), s.Cron...)
	sort.SliceStable(failures, func(i, j int) bool { return failures[i].At.After(failures[j].At) })
	var out []string
	for _, f := range failures {
		out = append(out, fmt.Sprintf("cron job %s failed %s: %s [cron:%s]", quoted(f.Name), ago(s.At, f.At), clip(oneLine(f.Error), maxErrorBytes), f.JobID))
	}
	return out
}

func pulseLines(s Snapshot) []string {
	var out []string
	for _, sig := range s.Pulse {
		out = append(out, fmt.Sprintf("%s %s: %s [pulse]", label(sig.Severity, "info"), label(sig.Kind, "signal"), clip(oneLine(sig.Summary), maxErrorBytes)))
	}
	return out
}

// sortedPipelines puts the most recently changed first.
func sortedPipelines(in []Pipeline) []Pipeline {
	out := append([]Pipeline(nil), in...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out
}

func quoted(title string) string {
	title = clip(oneLine(title), maxTitleBytes)
	if title == "" {
		return "an untitled session"
	}
	return `"` + strings.ReplaceAll(title, `"`, "'") + `"`
}

func label(v, fallback string) string {
	v = clip(oneLine(v), maxTitleBytes)
	if v == "" {
		return fallback
	}
	return v
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// ago is a rough "N minutes ago" for a past moment; zero times say so.
func ago(now, at time.Time) string {
	if at.IsZero() {
		return "at an unknown time"
	}
	return span(now.Sub(at)) + " ago"
}

func since(now, at time.Time) string {
	if at.IsZero() {
		return "an unknown time"
	}
	return span(now.Sub(at))
}

func span(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "under a minute"
	case d < time.Hour:
		return plural(int(d/time.Minute), "minute")
	case d < 48*time.Hour:
		return plural(int(d/time.Hour), "hour")
	default:
		return plural(int(d/(24*time.Hour)), "day")
	}
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}

// clip cuts s to at most n bytes on a rune boundary, marking the cut.
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n - len("…")
	for cut > 0 && (s[cut]&0xC0) == 0x80 {
		cut--
	}
	return s[:cut] + "…"
}
