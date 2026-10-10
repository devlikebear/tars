package initiative

import (
	"context"
	"fmt"
	"time"

	"github.com/devlikebear/tars/internal/jev"
)

// SystemOne is the part of *jev.Client the runtime uses.
type SystemOne interface {
	Ask(ctx context.Context, state string, questions map[string]jev.Question) (jev.Response, error)
}

// TextAnswer is the atomic text-signal booleans read from one backend call.
// Probabilities is set by the jev backend (a System One reports a
// probability per question); it is nil for the llm backend, which is never
// asked to invent a confidence number (tars#1219) — only a strict boolean.
type TextAnswer struct {
	QuietRequested bool
	UserStrained   bool
	SpecialDay     bool
	Probabilities  map[string]float64
}

// TextBackend reads the atomic question set over a rendered state string.
// jevTextBackend and LLMTextBackend (llmsignals.go) are the two
// implementations; textReader is agnostic to which one it holds.
type TextBackend interface {
	ReadText(ctx context.Context, state string, questions map[string]jev.Question) (TextAnswer, error)
}

// textQuestionsFor returns atomic questions over the user's own words. The
// P0 spike (tars#998) showed small models answer these well and fail at the
// composite "should I speak now". special_day names today's date in the
// question: Kev cannot match "birthday: Sep 29" against the date line in the
// state, but answers "does the profile list a birthday on September 29".
//
// Both backends share this wording: jev reads the Type field ("noul") to
// pick its wire answer shape, and the llm backend (which always answers a
// fixed set of three booleans) uses only the Instructions text.
func textQuestionsFor(today time.Time) map[string]jev.Question {
	return map[string]jev.Question{
		"quiet_requested": {Type: "noul", Instructions: "Did the user ask not to be messaged or disturbed?"},
		"user_strained":   {Type: "noul", Instructions: "Does the user seem tired, stressed, frustrated, or unwell?"},
		"special_day": {Type: "noul", Instructions: fmt.Sprintf(
			"Does the profile list a birthday or anniversary that falls on %s?", today.Format("January 2"))},
	}
}

// jevTextBackend adapts a System One client (probabilities over a fixed
// threshold) to TextBackend.
type jevTextBackend struct {
	client     SystemOne
	thresholds Thresholds
}

func (b jevTextBackend) ReadText(ctx context.Context, state string, questions map[string]jev.Question) (TextAnswer, error) {
	resp, err := b.client.Ask(ctx, state, questions)
	if err != nil {
		return TextAnswer{}, err
	}
	probs := make(map[string]float64, len(questions))
	for name := range questions {
		if a, ok := resp.Answers[name]; ok && a.Noul != nil {
			probs[name] = *a.Noul
		}
	}
	return TextAnswer{
		QuietRequested: probs["quiet_requested"] >= b.thresholds.QuietRequested,
		UserStrained:   probs["user_strained"] >= b.thresholds.UserStrained,
		SpecialDay:     probs["special_day"] >= b.thresholds.SpecialDay,
		Probabilities:  probs,
	}, nil
}

// textReader asks a TextBackend unless there is no text (empty key), no
// backend, or the text is unchanged since the last successful read. Errors
// are not cached, so the next tick asks again. source labels where a fresh
// reading came from ("systemone" or "llm"); "cache"/"skipped"/"error" are
// set by Read itself.
type textReader struct {
	backend TextBackend
	source  string
	lastKey string
	last    TextSignals
}

// newTextReader builds a reader over any TextBackend. backend may be nil,
// in which case Read always reports "skipped".
func newTextReader(backend TextBackend, source string) *textReader {
	return &textReader{backend: backend, source: source}
}

// newJevTextReader builds a reader backed by a System One client, applying
// the configured probability thresholds.
func newJevTextReader(client SystemOne, th Thresholds) *textReader {
	var backend TextBackend
	if client != nil {
		backend = jevTextBackend{client: client, thresholds: th}
	}
	return newTextReader(backend, "systemone")
}

func (r *textReader) Read(ctx context.Context, key, state string, questions map[string]jev.Question) (TextSignals, error) {
	if key == "" || r.backend == nil {
		return TextSignals{Source: "skipped"}, nil
	}
	if key == r.lastKey {
		cached := r.last
		cached.Source = "cache"
		return cached, nil
	}
	answer, err := r.backend.ReadText(ctx, state, questions)
	if err != nil {
		return TextSignals{Source: "error"}, err
	}
	out := TextSignals{
		QuietRequested: answer.QuietRequested,
		UserStrained:   answer.UserStrained,
		SpecialDay:     answer.SpecialDay,
		Probabilities:  answer.Probabilities,
		Source:         r.source,
	}
	r.lastKey, r.last = key, out
	return out, nil
}
