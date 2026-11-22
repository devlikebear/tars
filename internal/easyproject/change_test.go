package easyproject

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestChangeRecordAndGoal(t *testing.T) {
	if _, err := CleanChange("   "); !errors.Is(err, ErrChangeRequired) {
		t.Fatalf("empty: %v", err)
	}
	if _, err := CleanChange(strings.Repeat("가", MaxIdeaRunes+1)); !errors.Is(err, ErrChangeTooLong) {
		t.Fatalf("long: %v", err)
	}
	got, err := CleanChange("  Make the button green \n")
	if err != nil || got != "Make the button green" {
		t.Fatalf("clean = %q, %v", got, err)
	}
	if g := ChangeGoal(got); !strings.HasSuffix(g, ": Make the button green") || !strings.Contains(g, "already in this folder") {
		t.Fatalf("goal = %q", g)
	}

	at := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	p := Project{ID: "0123456789abcdef", SessionID: "first"}
	one := p.WithChange("green", "second", at)
	two := one.WithChange("bigger", "third", at.Add(time.Hour))
	if p.SessionID != "first" || len(p.Changes) != 0 {
		t.Fatalf("WithChange changed its receiver: %+v", p)
	}
	if two.SessionID != "third" || len(two.Changes) != 2 || two.Changes[0].Request != "green" ||
		two.Changes[1].SessionID != "third" || !two.UpdatedAt.Equal(at.Add(time.Hour)) || len(one.Changes) != 1 {
		t.Fatalf("after two changes: %+v (one: %+v)", two, one)
	}
}
