package initiative

import (
	"context"
	"errors"
	"strings"
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
	r := newJevTextReader(fake, Config{}.WithDefaults().Thresholds)
	got, err := r.Read(context.Background(), "k1", "state", textQuestionsFor(at(9, 0)))
	if err != nil || !got.QuietRequested || got.UserStrained || !got.SpecialDay || got.Source != "systemone" {
		t.Fatalf("got %+v err=%v", got, err)
	}
	if got.Probabilities["user_strained"] != 0.59 {
		t.Fatalf("probabilities = %v", got.Probabilities)
	}
	again, _ := r.Read(context.Background(), "k1", "state moved", textQuestionsFor(at(9, 0)))
	if fake.calls != 1 || again.Source != "cache" || !again.QuietRequested {
		t.Fatalf("expected cache hit, calls=%d got=%+v", fake.calls, again)
	}
	if _, _ = r.Read(context.Background(), "k2", "new text", textQuestionsFor(at(9, 0))); fake.calls != 2 {
		t.Fatalf("new key must re-ask, calls=%d", fake.calls)
	}
}

func TestTextReaderSkipsWithoutKeyOrClient(t *testing.T) {
	fake := &fakeSystemOne{}
	if got, _ := newJevTextReader(fake, Thresholds{}).Read(context.Background(), "", "s", textQuestionsFor(at(9, 0))); got.Source != "skipped" || fake.calls != 0 {
		t.Fatalf("got %+v", got)
	}
	if got, _ := newJevTextReader(nil, Thresholds{}).Read(context.Background(), "k", "s", textQuestionsFor(at(9, 0))); got.Source != "skipped" {
		t.Fatalf("got %+v", got)
	}
}

func TestTextReaderErrorIsNotCached(t *testing.T) {
	fake := &fakeSystemOne{err: errors.New("down")}
	r := newJevTextReader(fake, Config{}.WithDefaults().Thresholds)
	got, err := r.Read(context.Background(), "k", "s", textQuestionsFor(at(9, 0)))
	if err == nil || got.Source != "error" || got.QuietRequested {
		t.Fatalf("got %+v err=%v", got, err)
	}
	fake.err = nil
	if _, err := r.Read(context.Background(), "k", "s", textQuestionsFor(at(9, 0))); err != nil || fake.calls != 2 {
		t.Fatalf("expected a retry after an error, calls=%d err=%v", fake.calls, err)
	}
}

func TestTextReaderMissingAnswerIsFalse(t *testing.T) {
	r := newJevTextReader(&fakeSystemOne{probs: map[string]float64{}}, Config{}.WithDefaults().Thresholds)
	got, err := r.Read(context.Background(), "k", "s", textQuestionsFor(at(9, 0)))
	if err != nil || got.QuietRequested || got.UserStrained || got.SpecialDay {
		t.Fatalf("got %+v err=%v", got, err)
	}
}

func TestTextQuestionsNameTodayForSpecialDay(t *testing.T) {
	// Kev cannot match "birthday: Sep 29" against the date in the state
	// (p≈0.36), but answers "does the profile list a birthday on September
	// 29" cleanly (AUC 1.0), so the date goes into the question itself.
	qs := textQuestionsFor(at(9, 0))
	special := qs["special_day"].Instructions
	if !strings.Contains(special, "September 29") || !strings.Contains(special, "profile") {
		t.Fatalf("special_day instructions = %q", special)
	}
	if len(qs) != 3 || qs["quiet_requested"].Type != "noul" || qs["user_strained"].Type != "noul" {
		t.Fatalf("questions = %+v", qs)
	}
}

func TestTextReaderSendsTheGivenQuestions(t *testing.T) {
	var asked map[string]jev.Question
	fake := &recordingSystemOne{onAsk: func(qs map[string]jev.Question) { asked = qs }}
	r := newJevTextReader(fake, Config{}.WithDefaults().Thresholds)
	if _, err := r.Read(context.Background(), "k", "s", textQuestionsFor(at(9, 0))); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(asked["special_day"].Instructions, "September 29") {
		t.Fatalf("asked = %+v", asked)
	}
}

type recordingSystemOne struct{ onAsk func(map[string]jev.Question) }

func (r *recordingSystemOne) Ask(_ context.Context, _ string, qs map[string]jev.Question) (jev.Response, error) {
	r.onAsk(qs)
	return jev.Response{}, nil
}
