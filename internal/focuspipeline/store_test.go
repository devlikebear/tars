package focuspipeline

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if _, ok, err := s.Get("s1"); err != nil || ok {
		t.Fatalf("empty get: ok=%v err=%v", ok, err)
	}
	p := planned(t)
	p.SessionID = "s1"
	if err := s.Save(p); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "s1.pipeline.json")); err != nil {
		t.Fatalf("file: %v", err)
	}
	got, ok, err := s.Get("s1")
	if err != nil || !ok {
		t.Fatalf("Get: ok=%v err=%v", ok, err)
	}
	if got.OpenGate != GatePlan || len(got.Cards) != 1 || got.Plan == nil || string(got.Cards[0].Payload) != string(p.Cards[0].Payload) {
		t.Fatalf("got = %+v", got)
	}
	if err := s.Delete("s1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok, _ := s.Get("s1"); ok {
		t.Fatal("pipeline survived delete")
	}
	if err := s.Delete("s1"); err != nil {
		t.Fatalf("second Delete: %v", err)
	}
}

func TestStoreRejectsPathIDs(t *testing.T) {
	s := NewStore(t.TempDir())
	for _, id := range []string{"", "..", "../x", `a\b`, "a/b"} {
		if _, _, err := s.Get(id); !errors.Is(err, ErrInvalidSessionID) {
			t.Errorf("Get(%q) err = %v", id, err)
		}
		if err := s.Save(Pipeline{SessionID: id}); !errors.Is(err, ErrInvalidSessionID) {
			t.Errorf("Save(%q) err = %v", id, err)
		}
	}
}

func TestStoreList(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if got, err := NewStore(filepath.Join(dir, "missing")).List(); err != nil || len(got) != 0 {
		t.Fatalf("missing dir: %v %v", got, err)
	}
	older := New("a", "first", t0)
	newer := New("b", "second", t0.Add(time.Hour))
	for _, p := range []Pipeline{older, newer} {
		if err := s.Save(p); err != nil {
			t.Fatal(err)
		}
	}
	// Unrelated and broken files are ignored.
	if err := os.WriteFile(filepath.Join(dir, "a.jsonl"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bad.pipeline.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].SessionID != "b" || got[1].SessionID != "a" {
		t.Fatalf("list = %+v", got)
	}
}

func TestStoreUpdate(t *testing.T) {
	s := NewStore(t.TempDir())
	if _, ok, err := s.Update("none", func(p Pipeline) (Pipeline, error) { t.Fatal("fn called"); return p, nil }); ok || err != nil {
		t.Fatalf("missing: ok=%v err=%v", ok, err)
	}
	if err := s.Save(New("s1", "g", t0)); err != nil {
		t.Fatal(err)
	}
	boom := errors.New("boom")
	if _, _, err := s.Update("s1", func(p Pipeline) (Pipeline, error) { p.Goal = "changed"; return p, boom }); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if got, _, _ := s.Get("s1"); got.Goal != "g" {
		t.Fatal("failed update was saved")
	}

	// Concurrent updates through separate Store values never lose a card.
	var wg sync.WaitGroup
	const n = 20
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			other := NewStore(s.dir)
			_, _, err := other.Update("s1", func(p Pipeline) (Pipeline, error) {
				p.addCard(CardNotice, 0, "x", nil, t0)
				return p, nil
			})
			if err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	got, _, _ := s.Get("s1")
	if len(got.Cards) != n {
		t.Fatalf("cards = %d, want %d", len(got.Cards), n)
	}
}
