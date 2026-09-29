package initiative

import (
	"context"

	"github.com/devlikebear/tars/internal/jev"
)

// SystemOne is the part of *jev.Client the runtime uses.
type SystemOne interface {
	Ask(ctx context.Context, state string, questions map[string]jev.Question) (jev.Response, error)
}

// textQuestions are atomic questions over the user's own words. The P0 spike
// (tars#998) showed small models answer these well and fail at the composite
// "should I speak now".
var textQuestions = map[string]jev.Question{
	"quiet_requested": {Type: "noul", Instructions: "Did the user ask not to be messaged or disturbed?"},
	"user_strained":   {Type: "noul", Instructions: "Does the user seem tired, stressed, frustrated, or unwell?"},
	"special_day":     {Type: "noul", Instructions: "Is today a special day for the user, such as a birthday or an anniversary?"},
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
func (r *textReader) Read(ctx context.Context, key, state string) (TextSignals, error) {
	if key == "" || r.client == nil {
		return TextSignals{Source: "skipped"}, nil
	}
	if key == r.lastKey {
		cached := r.last
		cached.Source = "cache"
		return cached, nil
	}
	resp, err := r.client.Ask(ctx, state, textQuestions)
	if err != nil {
		return TextSignals{Source: "error"}, err
	}
	probs := make(map[string]float64, len(textQuestions))
	for name := range textQuestions {
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
