package focuspipeline

import "strings"

// Scripted browser checks (docs/decisions/focus-mode.md §5.5): a web UI
// change is proven by a verify command that drives a real browser and
// reports through its exit code — a script, not a model. The pipeline adds
// nothing new to run it: the check is one more verify command, so the
// transitions, goal mode's lock on the verify list and the failure card are
// the ones every verify command already has.
//
// What the pipeline adds is the teaching, and only where it can be used:
// Pipeline.BrowserCheck is how a shell in the working folder runs the
// scripted-browser CLI (agent-browser), found when the pipeline was created
// (focusprobe.BrowserCheckCLI). Empty — the CLI is not on this desk —
// changes nothing: the guidance never mentions it.

// BrowserCheckCLIName is the scripted-browser CLI's name on PATH, and what
// a check script falls back to when AGENT_BROWSER is not set.
const BrowserCheckCLIName = "agent-browser"

// browserCheckEnv is the variable a check script reads the CLI from, so
// one script runs where the CLI is on PATH (CI, another desk) and where it
// is not.
const browserCheckEnv = "AGENT_BROWSER"

const browserCheckPlanHead = " Scripted browser checks: if this work changes what a web page shows or does, prove it in \"verify\" " +
	"with a check that drives a real browser and exits non-zero when the page is wrong — not by reading the code, and not " +
	"with a check that would also pass without the change. When the repository already has Playwright (a playwright.config " +
	"file), add or extend a spec there and list the command that runs it. Otherwise make one task that writes a shell script " +
	"of `agent-browser` commands (a CLI that drives its own Chrome, no model; "

const browserCheckPlanTail = "The server runs each verify command by itself in the working folder and starts nothing for it, so the " +
	"script does everything: build this folder's change, start the app in the background on a port of its own with its output " +
	"redirected, give every CLI command a browser session of its own (`--session <name>`; the unnamed one is shared with " +
	"everything else on this desk), stop the app and close that session in a `trap … EXIT`, wait until the app answers, then open the page, act " +
	"on it, and compare what the page shows with what is expected. " + browserCheckReportFormat +
	" Skip all of this when the work does not touch a web UI."

// browserCheckPlanInstructions is added to the plan stage of a pipeline
// whose desk has the scripted-browser CLI and whose work may be a web page
// (touchesWebUI: a template that ends in a pull request, or one marked
// WebUI; a writing or research template never hears of it).
func (p Pipeline) browserCheckPlanInstructions() string {
	cli := strings.TrimSpace(p.BrowserCheck)
	if cli == "" || !p.touchesWebUI() {
		return ""
	}
	var b strings.Builder
	b.WriteString(browserCheckPlanHead)
	b.WriteString("`" + cli + " skills get core` prints its usage) and list that script as a verify command. ")
	b.WriteString("The script calls the CLI as `\"${" + browserCheckEnv + ":-" + BrowserCheckCLIName + "}\"`")
	if cli != BrowserCheckCLIName {
		// Not on the PATH the server's shell has: the verify command names
		// it, the script stays free of this desk's paths.
		b.WriteString(", and because the CLI is not on PATH here the verify command sets it: `" +
			browserCheckEnv + "='" + cli + "' sh <script>`")
	}
	b.WriteString(". ")
	b.WriteString(browserCheckPlanTail)
	return b.String()
}

// browserCheckBuildNote is the build stage's short reminder: the plan
// stage's paragraph is not repeated, but the agent that writes the script
// may not be the conversation that planned it (a resumed CLI session, a
// different execution tier).
func (p Pipeline) browserCheckBuildNote() string {
	cli := strings.TrimSpace(p.BrowserCheck)
	if cli == "" || !p.touchesWebUI() {
		return ""
	}
	return " If a task is a scripted browser check: `" + cli + " skills get core` prints the CLI's usage; the script starts " +
		"and stops its own app and its own browser session (`--session <name>` on every command, closed in a `trap … EXIT`). " + browserCheckReportFormat +
		" Make a failing check pass by fixing the page, never by loosening the check."
}

// checkFailedMarker opens a check's own account of its failure.
const checkFailedMarker = "CHECK FAILED"

// browserCheckReportFormat teaches the four lines ParseCheckReport reads.
const browserCheckReportFormat = "On a mismatch it prints these four lines, each on one line, and exits 1: `" + checkFailedMarker +
	"`, `screen: <the page's name and address>`, `expected: <what should be there>`, `seen: <what the page showed>` — " +
	"in words someone who cannot read code understands, because the failure card shows them as they are."

// checkReportLines is how many lines after the marker are read for the
// fields; checkReportRunes caps one field.
const (
	checkReportLines = 6
	checkReportRunes = 160
)

// CheckReport is a failed check told in its own words: which screen, what
// was expected, what was seen. The check script prints it (the format
// above) and Go copies it — no model reads the output or words the card.
type CheckReport struct {
	Screen   string `json:"screen,omitempty"`
	Expected string `json:"expected"`
	Seen     string `json:"seen"`
}

// ParseCheckReport reads the last CHECK FAILED block of a verify command's
// output. It answers nil unless the block says both what was expected and
// what was seen; the screen may be missing. Any verify command may print
// the block — a Playwright spec as well as an agent-browser script.
func ParseCheckReport(output string) *CheckReport {
	lines := strings.Split(output, "\n")
	at := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == checkFailedMarker {
			at = i
		}
	}
	if at < 0 {
		return nil
	}
	var r CheckReport
	for _, line := range lines[at+1 : min(len(lines), at+1+checkReportLines)] {
		name, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		value = clipRunes(strings.TrimSpace(value), checkReportRunes)
		switch field := strings.ToLower(strings.TrimSpace(name)); {
		case field == "screen" && r.Screen == "":
			r.Screen = value
		case field == "expected" && r.Expected == "":
			r.Expected = value
		case field == "seen" && r.Seen == "":
			r.Seen = value
		}
	}
	if r.Expected == "" || r.Seen == "" {
		return nil
	}
	return &r
}

// title is the report as one plain sentence, the failure card's title.
func (r CheckReport) title() string {
	where := ""
	if r.Screen != "" {
		where = " on " + r.Screen
	}
	return "Check failed" + where + ": expected " + r.Expected + ", but saw " + r.Seen
}

func clipRunes(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit]) + "…"
}
