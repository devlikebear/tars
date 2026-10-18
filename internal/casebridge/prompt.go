package casebridge

import (
	"fmt"
	"strings"
)

// Instructions is the console context of every Bridge question: who CASE
// is, what it answers from, and the card block format. It rides on the
// question (saved in CASE's own hidden session), while the status itself
// rides on the turn context and is never saved — a snapshot is only true
// for the turn it was taken for.
const Instructions = `You are CASE, TARS's on-call companion robot, answering a question in the Bridge panel.
Answer from the <tars-status> block in this turn's context: it is the live state of TARS, computed by the server for this question. Do not use tools to look things up.
Lead with the answer in one or two short sentences, in the language of the question. Use the exact numbers and names from the status. Suggest at most one next step. No exclamation marks, no emoji.
You cannot change anything. Never offer to approve, run, retry, cancel or delete; point to the screen where the user can do it.
If the status does not answer the question, say so plainly.
When the answer is about specific items, end with a <case-cards> block: a JSON array of at most 4 objects {"kind","id","title","sub","severity"}. kind is focus, session or cron with the id in brackets from the status, or approvals, usage or pulse with no id. severity is "block" for something waiting on the user or failed, otherwise "info". Example:
<case-cards>[{"kind":"focus","id":"abc123","title":"Fix login","sub":"waits at G2","severity":"block"}]</case-cards>`

// Scope of a question.
const (
	ScopeAll    = "all"
	ScopeScreen = "screen"
)

// View is where the user asked from: the console route and, when it shows
// one, the session or pipeline on screen.
type View struct {
	Route     string `json:"route"`
	SessionID string `json:"session_id,omitempty"`
	Title     string `json:"title,omitempty"`
}

// QuestionContext is the console context of one question: the standing
// instructions plus where the user is looking and the asked scope.
func QuestionContext(view View, scope string) string {
	var b strings.Builder
	b.WriteString(Instructions)
	b.WriteString("\n\n")
	route := label(view.Route, "board")
	id := strings.TrimSpace(view.SessionID)
	switch {
	case id != "" && strings.TrimSpace(view.Title) != "":
		fmt.Fprintf(&b, "The user is looking at the %s screen, on %s [%s].\n", route, quoted(view.Title), id)
	case id != "":
		fmt.Fprintf(&b, "The user is looking at the %s screen, on [%s].\n", route, id)
	default:
		fmt.Fprintf(&b, "The user is looking at the %s screen.\n", route)
	}
	if scope == ScopeScreen && strings.TrimSpace(view.SessionID) != "" {
		b.WriteString("Answer only about what this screen shows; leave the rest of TARS out unless it blocks it.\n")
	} else {
		b.WriteString("Answer about TARS as a whole.\n")
	}
	return b.String()
}

// TurnContext wraps a rendered status for the turn context.
func TurnContext(status string) string {
	if strings.TrimSpace(status) == "" {
		return ""
	}
	return "<tars-status>\n" + strings.TrimSpace(status) + "\n</tars-status>"
}
