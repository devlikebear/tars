package session

import internal "github.com/devlikebear/tars/internal/session"

// ErrSessionNotFound is returned when a session ID does not resolve to an
// existing session.
var ErrSessionNotFound = internal.ErrSessionNotFound

// ErrCwdNotEligible is returned by SetCurrentDir when the supplied directory
// is not under a configured work_dirs entry.
var ErrCwdNotEligible = internal.ErrCwdNotEligible

// ErrSessionKindUnsupported is returned by goal mutations when the session
// kind does not support goals.
var ErrSessionKindUnsupported = internal.ErrSessionKindUnsupported

type Store = internal.Store
type Session = internal.Session
type Message = internal.Message
type HistorySnapshot = internal.HistorySnapshot

// NewStore returns a Store rooted at dir (transcripts live under dir/sessions).
func NewStore(dir string) *Store { return internal.NewStore(dir) }

// AppendMessage appends one message as a JSON line to the transcript at path.
func AppendMessage(path string, msg Message) error { return internal.AppendMessage(path, msg) }

// ReadMessages reads all messages from the transcript at path (empty if absent).
func ReadMessages(path string) ([]Message, error) { return internal.ReadMessages(path) }

// RewriteMessages replaces the transcript contents with msgs.
func RewriteMessages(path string, msgs []Message) error { return internal.RewriteMessages(path, msgs) }

// LoadHistory returns the most recent messages fitting within maxTokens, oldest-first.
func LoadHistory(path string, maxTokens int) ([]Message, error) {
	return internal.LoadHistory(path, maxTokens)
}

// LoadHistorySnapshot is LoadHistory plus token/compaction metadata.
func LoadHistorySnapshot(path string, maxTokens int) (HistorySnapshot, error) {
	return internal.LoadHistorySnapshot(path, maxTokens)
}

// EstimateMessageTokenCost returns the token estimate used for history budgeting.
func EstimateMessageTokenCost(msg Message) int { return internal.EstimateMessageTokenCost(msg) }
