package initiative

import (
	"context"
	"errors"
	"testing"

	"github.com/devlikebear/tars/internal/jev"
)

type fakeSystemOne struct {
	calls int
	probs map[string]float64
	err   error
	state string
}

func (f *fakeSystemOne) Ask(_ context.Context, state string, qs map[string]jev.Question) (jev.Response, error) {
	f.calls++
	f.state = state
	if f.err != nil {
		return jev.Response{}, f.err
	}
	out := jev.Response{Answers: map[string]jev.Answer{}}
	for k := range qs {
		p := f.probs[k]
		out.Answers[k] = jev.Answer{Noul: &p}
	}
	return out, nil
}

func TestTextReaderAppliesThresholdsAndCaches(t *testing.T) {
	fake := &fakeSystemOne{probs: map[string]float64{"quiet_requested": 0.56, "user_strained": 0.59, "special_day": 0.9}}
	r := newTextReader(fake, Config{}.WithDefaults().Thresholds)
	got, err := r.Read(context.Background(), "k1", "state")
	if err != nil || !got.QuietRequested || got.UserStrained || !got.SpecialDay || got.Source != "systemone" {
		t.Fatalf("got %+v err=%v", got, err)
	}
	if got.Probabilities["user_strained"] != 0.59 {
		t.Fatalf("probabilities = %v", got.Probabilities)
	}
	again, _ := r.Read(context.Background(), "k1", "state moved")
	if fake.calls != 1 || again.Source != "cache" || !again.QuietRequested {
		t.Fatalf("expected cache hit, calls=%d got=%+v", fake.calls, again)
	}
	if _, _ = r.Read(context.Background(), "k2", "new text"); fake.calls != 2 {
		t.Fatalf("new key must re-ask, calls=%d", fake.calls)
	}
}

func TestTextReaderSkipsWithoutKeyOrClient(t *testing.T) {
	fake := &fakeSystemOne{}
	if got, _ := newTextReader(fake, Thresholds{}).Read(context.Background(), "", "s"); got.Source != "skipped" || fake.calls != 0 {
		t.Fatalf("got %+v", got)
	}
	if got, _ := newTextReader(nil, Thresholds{}).Read(context.Background(), "k", "s"); got.Source != "skipped" {
		t.Fatalf("got %+v", got)
	}
}

func TestTextReaderErrorIsNotCached(t *testing.T) {
	fake := &fakeSystemOne{err: errors.New("down")}
	r := newTextReader(fake, Config{}.WithDefaults().Thresholds)
	got, err := r.Read(context.Background(), "k", "s")
	if err == nil || got.Source != "error" || got.QuietRequested {
		t.Fatalf("got %+v err=%v", got, err)
	}
	fake.err = nil
	if _, err := r.Read(context.Background(), "k", "s"); err != nil || fake.calls != 2 {
		t.Fatalf("expected a retry after an error, calls=%d err=%v", fake.calls, err)
	}
}

func TestTextReaderMissingAnswerIsFalse(t *testing.T) {
	r := newTextReader(&fakeSystemOne{probs: map[string]float64{}}, Config{}.WithDefaults().Thresholds)
	got, err := r.Read(context.Background(), "k", "s")
	if err != nil || got.QuietRequested || got.UserStrained || got.SpecialDay {
		t.Fatalf("got %+v err=%v", got, err)
	}
}
