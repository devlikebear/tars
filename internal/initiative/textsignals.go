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

// textQuestionsFor returns atomic questions over the user's own words. The
// P0 spike (tars#998) showed small models answer these well and fail at the
// composite "should I speak now". special_day names today's date in the
// question: Kev cannot match "birthday: Sep 29" against the date line in the
// state, but answers "does the profile list a birthday on September 29".
func textQuestionsFor(today time.Time) map[string]jev.Question {
	return map[string]jev.Question{
		"quiet_requested": {Type: "noul", Instructions: "Did the user ask not to be messaged or disturbed?"},
		"user_strained":   {Type: "noul", Instructions: "Does the user seem tired, stressed, frustrated, or unwell?"},
		"special_day": {Type: "noul", Instructions: fmt.Sprintf(
			"Does the profile list a birthday or anniversary that falls on %s?", today.Format("January 2"))},
	}
}

type textReader struct {
	client     SystemOne
	thresholds Thresholds
	lastKey    string
	last       TextSignals
}

func newTextReader(client SystemOne, th Thresholds) *textReader {
	return &textReader{client: client, thresholds: th}
}

// Read asks the System One unless there is no text (empty key), no client,
// or the text is unchanged since the last successful read. Errors are not
// cached, so the next tick asks again.
func (r *textReader) Read(ctx context.Context, key, state string, questions map[string]jev.Question) (TextSignals, error) {
	if key == "" || r.client == nil {
		return TextSignals{Source: "skipped"}, nil
	}
	if key == r.lastKey {
		cached := r.last
		cached.Source = "cache"
		return cached, nil
	}
	resp, err := r.client.Ask(ctx, state, questions)
	if err != nil {
		return TextSignals{Source: "error"}, err
	}
	probs := make(map[string]float64, len(questions))
	for name := range questions {
		if a, ok := resp.Answers[name]; ok && a.Noul != nil {
			probs[name] = *a.Noul
		}
	}
	out := TextSignals{
		QuietRequested: probs["quiet_requested"] >= r.thresholds.QuietRequested,
		UserStrained:   probs["user_strained"] >= r.thresholds.UserStrained,
		SpecialDay:     probs["special_day"] >= r.thresholds.SpecialDay,
		Probabilities:  probs,
		Source:         "systemone",
	}
	r.lastKey, r.last = key, out
	return out, nil
}
