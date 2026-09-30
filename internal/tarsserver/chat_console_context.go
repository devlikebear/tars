package tarsserver

import "strings"

const (
	maxConsoleContextBytes = 2000

	consoleContextOpen  = "<console-context>"
	consoleContextClose = "</console-context>"
)

// appendConsoleContext puts the console's guidance for this turn (such as
// the companion handoff's tone and console area) after the user's message as
// one tagged block. Like review notes, this is how it reaches every provider
// and why the transcript keeps it; the console drops the block from the
// user's bubble and the session title, so they show only what the user
// typed. A slash command keeps its arguments clean, so it gets no block.
func appendConsoleContext(message, context string) string {
	context = strings.TrimSpace(strings.ReplaceAll(context, consoleContextClose, ""))
	if context == "" || strings.HasPrefix(strings.TrimSpace(message), "/") {
		return message
	}
	context = strings.TrimRight(clipBytes(context, maxConsoleContextBytes), "\n")
	return strings.TrimRight(message, "\n") + "\n\n" + consoleContextOpen + "\n" + context + "\n" + consoleContextClose
}
