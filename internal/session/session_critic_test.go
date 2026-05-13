package session

import (
	"testing"
	"time"
)

func TestNormalizeCritic_Defaults(t *testing.T) {
	c := NormalizeCritic(&SessionCritic{Enabled: true})
	if c == nil {
		t.Fatal("expected non-nil")
	}
	if c.MaxIterations != DefaultCriticMaxIterations {
		t.Fatalf("MaxIterations = %d, want %d", c.MaxIterations, DefaultCriticMaxIterations)
	}
	if c.Status != SessionCriticStatusIdle {
		t.Fatalf("Status = %q, want %q", c.Status, SessionCriticStatusIdle)
	}
}

func TestNormalizeCritic_ClampsAboveMax(t *testing.T) {
	c := NormalizeCritic(&SessionCritic{Enabled: true, MaxIterations: 99})
	if c.MaxIterations != MaxCriticMaxIterations {
		t.Fatalf("MaxIterations = %d, want clamp to %d", c.MaxIterations, MaxCriticMaxIterations)
	}
}

func TestNormalizeCritic_TruncatesFeedback(t *testing.T) {
	long := make([]byte, MaxCriticFeedbackLen+500)
	for i := range long {
		long[i] = 'a'
	}
	c := NormalizeCritic(&SessionCritic{Enabled: true, LastFeedback: string(long)})
	if len(c.LastFeedback) != MaxCriticFeedbackLen {
		t.Fatalf("LastFeedback len = %d, want %d", len(c.LastFeedback), MaxCriticFeedbackLen)
	}
}

func TestNormalizeCritic_NilPassthrough(t *testing.T) {
	if c := NormalizeCritic(nil); c != nil {
		t.Fatalf("expected nil, got %+v", c)
	}
}

func TestSessionCritic_IsEnabled(t *testing.T) {
	var nilCritic *SessionCritic
	if nilCritic.IsEnabled() {
		t.Fatal("nil critic should not be enabled")
	}
	if (&SessionCritic{Enabled: false}).IsEnabled() {
		t.Fatal("disabled critic should not be enabled")
	}
	if !(&SessionCritic{Enabled: true}).IsEnabled() {
		t.Fatal("enabled critic should be enabled")
	}
}

func TestSessionCritic_EffectiveMaxIterations(t *testing.T) {
	var nilCritic *SessionCritic
	if got := nilCritic.EffectiveMaxIterations(); got != DefaultCriticMaxIterations {
		t.Fatalf("nil → %d, want default %d", got, DefaultCriticMaxIterations)
	}
	if got := (&SessionCritic{MaxIterations: 0}).EffectiveMaxIterations(); got != DefaultCriticMaxIterations {
		t.Fatalf("zero → %d, want default %d", got, DefaultCriticMaxIterations)
	}
	if got := (&SessionCritic{MaxIterations: 999}).EffectiveMaxIterations(); got != MaxCriticMaxIterations {
		t.Fatalf("oversized → %d, want clamp %d", got, MaxCriticMaxIterations)
	}
	if got := (&SessionCritic{MaxIterations: 2}).EffectiveMaxIterations(); got != 2 {
		t.Fatalf("explicit 2 → %d, want 2", got)
	}
}

func TestCriticEqual(t *testing.T) {
	var nilCritic *SessionCritic
	if !criticEqual(nilCritic, nilCritic) {
		t.Fatal("nil == nil should be true")
	}
	a := &SessionCritic{Enabled: true, MaxIterations: 3}
	if criticEqual(nilCritic, a) {
		t.Fatal("nil != non-nil should be false")
	}
	b := &SessionCritic{Enabled: true, MaxIterations: 3}
	if !criticEqual(a, b) {
		t.Fatal("equal critics should compare equal")
	}
	c := &SessionCritic{Enabled: true, MaxIterations: 4}
	if criticEqual(a, c) {
		t.Fatal("different MaxIterations should compare unequal")
	}
	d := &SessionCritic{Enabled: true, MaxIterations: 3, LastTrigger: "plan_proposed"}
	if criticEqual(a, d) {
		t.Fatal("different LastTrigger should compare unequal")
	}
	now := time.Now().UTC()
	later := now.Add(time.Second)
	withT1 := &SessionCritic{Enabled: true, MaxIterations: 3, UpdatedAt: &now}
	withT1Copy := &SessionCritic{Enabled: true, MaxIterations: 3, UpdatedAt: &now}
	withT2 := &SessionCritic{Enabled: true, MaxIterations: 3, UpdatedAt: &later}
	if !criticEqual(withT1, withT1Copy) {
		t.Fatal("same UpdatedAt should compare equal")
	}
	if criticEqual(withT1, withT2) {
		t.Fatal("different UpdatedAt should compare unequal")
	}
	if criticEqual(withT1, a) {
		t.Fatal("UpdatedAt presence mismatch should compare unequal")
	}
}

func TestStoreSetCritic_AllowsAnyKind(t *testing.T) {
	store := NewStore(t.TempDir())
	worker, err := store.EnsureWorker("p1")
	if err != nil {
		t.Fatalf("ensure worker: %v", err)
	}
	updated, err := store.SetCritic(worker.ID, &SessionCritic{Enabled: true, MaxIterations: 2})
	if err != nil {
		t.Fatalf("worker SetCritic should succeed, got: %v", err)
	}
	if !updated.Critic.IsEnabled() || updated.Critic.MaxIterations != 2 {
		t.Fatalf("worker critic not persisted: %+v", updated.Critic)
	}

	created, err := store.Create("regular")
	if err != nil {
		t.Fatalf("create regular: %v", err)
	}
	if _, err := store.SetCritic(created.ID, &SessionCritic{Enabled: true}); err != nil {
		t.Fatalf("regular SetCritic should succeed: %v", err)
	}
}

func TestInheritCriticConfig(t *testing.T) {
	if c := InheritCriticConfig(nil); c != nil {
		t.Fatalf("nil source should yield nil, got %+v", c)
	}
	if c := InheritCriticConfig(&SessionCritic{Enabled: false, MaxIterations: 5}); c != nil {
		t.Fatalf("disabled source should yield nil, got %+v", c)
	}
	src := &SessionCritic{
		Enabled:             true,
		MaxIterations:       4,
		CurrentIteration:    3,
		Status:              SessionCriticStatusReviewing,
		LastFeedback:        "- gap",
		LastTrigger:         "plan_proposed",
		LastReviewedPlanSig: "sig-1",
	}
	got := InheritCriticConfig(src)
	if got == nil {
		t.Fatal("expected non-nil inherited critic")
	}
	if !got.Enabled || got.MaxIterations != 4 {
		t.Fatalf("config fields not copied: %+v", got)
	}
	if got.CurrentIteration != 0 || got.LastFeedback != "" ||
		got.LastTrigger != "" || got.LastReviewedPlanSig != "" {
		t.Fatalf("runtime state should be reset: %+v", got)
	}
	if got.Status != SessionCriticStatusIdle {
		t.Fatalf("Status should reset to idle, got %q", got.Status)
	}
}

func TestEnsureWorker_InheritsMainCritic(t *testing.T) {
	store := NewStore(t.TempDir())
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main: %v", err)
	}
	if _, err := store.SetCritic(main.ID, &SessionCritic{Enabled: true, MaxIterations: 4}); err != nil {
		t.Fatalf("set main critic: %v", err)
	}
	worker, err := store.EnsureWorker("proj-1")
	if err != nil {
		t.Fatalf("ensure worker: %v", err)
	}
	if !worker.Critic.IsEnabled() {
		t.Fatalf("worker should inherit enabled critic, got %+v", worker.Critic)
	}
	if worker.Critic.MaxIterations != 4 {
		t.Fatalf("worker MaxIterations = %d, want 4", worker.Critic.MaxIterations)
	}
	if worker.Critic.Status != SessionCriticStatusIdle {
		t.Fatalf("worker critic status = %q, want idle", worker.Critic.Status)
	}
}

func TestEnsureWorker_NoInheritWhenMainDisabled(t *testing.T) {
	store := NewStore(t.TempDir())
	if _, err := store.EnsureMain(); err != nil {
		t.Fatalf("ensure main: %v", err)
	}
	worker, err := store.EnsureWorker("proj-1")
	if err != nil {
		t.Fatalf("ensure worker: %v", err)
	}
	if worker.Critic != nil {
		t.Fatalf("expected nil critic when main has none, got %+v", worker.Critic)
	}
}

func TestStoreSetCritic_PersistsAndClears(t *testing.T) {
	store := NewStore(t.TempDir())
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main: %v", err)
	}
	sess, err := store.SetCritic(main.ID, &SessionCritic{Enabled: true, MaxIterations: 2})
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if sess.Critic == nil || sess.Critic.MaxIterations != 2 {
		t.Fatalf("unexpected critic after set: %+v", sess.Critic)
	}
	if sess.Critic.UpdatedAt == nil {
		t.Fatal("UpdatedAt not stamped on SetCritic")
	}
	cleared, err := store.SetCritic(main.ID, nil)
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if cleared.Critic != nil {
		t.Fatalf("expected nil critic after clear, got %+v", cleared.Critic)
	}
}

func TestStoreUpdateCriticProgress(t *testing.T) {
	store := NewStore(t.TempDir())
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main: %v", err)
	}
	if _, err := store.SetCritic(main.ID, &SessionCritic{Enabled: true, MaxIterations: 3}); err != nil {
		t.Fatalf("set: %v", err)
	}
	updated, err := store.UpdateCriticProgress(main.ID, func(c *SessionCritic) *SessionCritic {
		c.CurrentIteration = 2
		c.Status = SessionCriticStatusReviewing
		c.LastFeedback = "- add tests"
		c.LastTrigger = "plan_proposed"
		c.LastReviewedPlanSig = "sig-123"
		return c
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Critic.CurrentIteration != 2 {
		t.Fatalf("CurrentIteration = %d, want 2", updated.Critic.CurrentIteration)
	}
	if updated.Critic.Status != SessionCriticStatusReviewing {
		t.Fatalf("Status = %q", updated.Critic.Status)
	}
	if updated.Critic.LastReviewedPlanSig != "sig-123" {
		t.Fatalf("LastReviewedPlanSig = %q", updated.Critic.LastReviewedPlanSig)
	}
}

func TestStoreUpdateCriticProgress_NoOpWhenNoCritic(t *testing.T) {
	store := NewStore(t.TempDir())
	main, err := store.EnsureMain()
	if err != nil {
		t.Fatalf("ensure main: %v", err)
	}
	sess, err := store.UpdateCriticProgress(main.ID, func(c *SessionCritic) *SessionCritic {
		t.Fatal("mutator should not be invoked")
		return c
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if sess.Critic != nil {
		t.Fatalf("expected critic nil, got %+v", sess.Critic)
	}
}
