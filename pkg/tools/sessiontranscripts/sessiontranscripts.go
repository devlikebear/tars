// Package sessiontranscripts adapts a TARS session store to the
// tools.TranscriptSource the memory search tool reads past conversations
// through.
//
// It is a separate package on purpose. pkg/tools promises to stay free of
// TARS' session runtime so that an application embedding a single tool does
// not link the transcript store; this package is where the two meet, and only
// an importer that wants conversation search pays for it.
package sessiontranscripts

import (
	"github.com/devlikebear/tars/pkg/session"
	"github.com/devlikebear/tars/pkg/tools"
)

// New returns a TranscriptSource over store. A nil store yields a source with
// nothing to search rather than a nil interface, so callers can pass the
// result straight through without a nil check of their own.
func New(store *session.Store) tools.TranscriptSource {
	return storeSource{store: store}
}

type storeSource struct {
	store *session.Store
}

func (s storeSource) ListTranscripts() ([]tools.TranscriptRef, error) {
	if s.store == nil {
		return nil, nil
	}
	sessions, err := s.store.List()
	if err != nil {
		return nil, err
	}
	refs := make([]tools.TranscriptRef, 0, len(sessions))
	for _, item := range sessions {
		refs = append(refs, tools.TranscriptRef{ID: item.ID, UpdatedAt: item.UpdatedAt})
	}
	return refs, nil
}

func (s storeSource) ReadTranscript(id string) ([]tools.TranscriptMessage, error) {
	if s.store == nil {
		return nil, nil
	}
	msgs, err := session.ReadMessages(s.store.TranscriptPath(id))
	if err != nil {
		return nil, err
	}
	out := make([]tools.TranscriptMessage, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, tools.TranscriptMessage{Role: m.Role, Content: m.Content, Timestamp: m.Timestamp})
	}
	return out, nil
}
