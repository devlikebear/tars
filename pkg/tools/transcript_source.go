package tools

import "time"

// TranscriptSource lets the memory search tool read past conversations
// without this package owning a session store.
//
// The package doc promises that TARS' session wiring stays internal, but
// memory_search used to open internal/session directly to search old
// transcripts -- so every importer of pkg/tools, including one that only
// wanted web_fetch, linked the whole session runtime. linetta's dependency
// gate caught it: an application embedding these tools is not supposed to
// carry TARS' transcript store, and it certainly should not have to just to
// fetch a URL.
//
// Search over transcripts is now something a caller opts into by supplying a
// source. pkg/tools/sessiontranscripts adapts a *session.Store; an application
// with its own conversation storage supplies its own. A nil source means
// "no transcripts", and include_sessions then simply finds nothing.
type TranscriptSource interface {
	// ListTranscripts returns the conversations available to search. Order
	// does not matter; the tool sorts newest-first by UpdatedAt and searches
	// at most a fixed number of them.
	ListTranscripts() ([]TranscriptRef, error)
	// ReadTranscript returns the messages of one conversation, oldest first.
	ReadTranscript(id string) ([]TranscriptMessage, error)
}

// TranscriptRef identifies one searchable conversation.
type TranscriptRef struct {
	ID        string
	UpdatedAt time.Time
}

// TranscriptMessage is the slice of a conversation turn the search reads.
// Role "system" and "tool" turns are skipped by the tool; Timestamp may be
// zero, in which case the transcript's UpdatedAt stands in for it.
type TranscriptMessage struct {
	Role      string
	Content   string
	Timestamp time.Time
}
