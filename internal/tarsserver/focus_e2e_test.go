package tarsserver

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/computeruse"
	"github.com/devlikebear/tars/internal/focuspipeline"
	"github.com/devlikebear/tars/internal/jev"
)

// fakeCuaDriver is the minimal computeruse.Driver a "done on the first
// decision" run needs: one window, one empty (non-degraded) snapshot.
type fakeCuaDriver struct {
	resolvedApp string
}

func (f *fakeCuaDriver) Ping(context.Context) error { return nil }
func (f *fakeCuaDriver) ResolveWindow(_ context.Context, app string) (computeruse.Window, error) {
	f.resolvedApp = app
	return computeruse.Window{}, nil
}
func (f *fakeCuaDriver) Snapshot(context.Context, computeruse.Window, computeruse.SnapshotOpts) (computeruse.Snapshot, error) {
	return computeruse.Snapshot{}, nil
}
func (f *fakeCuaDriver) Click(context.Context, computeruse.Window, string) (computeruse.Effect, error) {
	return computeruse.EffectConfirmed, nil
}
func (f *fakeCuaDriver) SetValue(context.Context, computeruse.Window, string, string) (computeruse.Effect, error) {
	return computeruse.EffectConfirmed, nil
}
func (f *fakeCuaDriver) TypeText(context.Context, computeruse.Window, string, string) (computeruse.Effect, error) {
	return computeruse.EffectConfirmed, nil
}
func (f *fakeCuaDriver) PressKey(context.Context, computeruse.Window, string) (computeruse.Effect, error) {
	return computeruse.EffectConfirmed, nil
}
func (f *fakeCuaDriver) Scroll(context.Context, computeruse.Window, string) (computeruse.Effect, error) {
	return computeruse.EffectConfirmed, nil
}

// fakeDoneBackend decides the goal is already achieved, first step.
type fakeDoneBackend struct{}

func (fakeDoneBackend) Decide(context.Context, string, map[string]jev.Question) (computeruse.Decision, computeruse.DecisionUsage, error) {
	return computeruse.Decision{Op: computeruse.OpDone}, computeruse.DecisionUsage{}, nil
}

func TestSplitE2EGoal(t *testing.T) {
	tests := []struct {
		item     string
		wantApp  string
		wantGoal string
	}{
		{"open the calculator and compute 2+2", "", "open the calculator and compute 2+2"},
		{"@Calculator compute 2+2", "Calculator", "compute 2+2"},
		{"  @Calculator   compute 2+2  ", "Calculator", "compute 2+2"},
		{"@Calculator", "", "@Calculator"},
		{"@", "", "@"},
		{"", "", ""},
	}
	for _, tt := range tests {
		app, goal := splitE2EGoal(tt.item)
		if app != tt.wantApp || goal != tt.wantGoal {
			t.Errorf("splitE2EGoal(%q) = (%q, %q), want (%q, %q)", tt.item, app, goal, tt.wantApp, tt.wantGoal)
		}
	}
}

func TestE2EVerificationResultMapsStatuses(t *testing.T) {
	tests := []struct {
		name         string
		status       computeruse.Status
		wantPassed   bool
		wantSkipped  bool
		wantExitCode int
	}{
		{"done passes", computeruse.StatusDone, true, false, 0},
		{"stuck fails", computeruse.StatusStuck, false, false, -1},
		{"max steps fails", computeruse.StatusMaxSteps, false, false, -1},
		{"needs confirmation fails", computeruse.StatusNeedsConfirmation, false, false, -1},
		{"error fails", computeruse.StatusError, false, false, -1},
		{"cancelled fails", computeruse.StatusCancelled, false, false, -1},
		{"unavailable is skipped, not failed", computeruse.StatusUnavailable, true, true, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := computeruse.Result{Status: tt.status, Reason: "why", Hint: "try this", LastScreen: []string{"line one"}}
			got := e2eVerificationResult("@Calculator do the thing", res)
			if got.Command != "@Calculator do the thing" || got.Passed != tt.wantPassed || got.Skipped != tt.wantSkipped || got.ExitCode != tt.wantExitCode {
				t.Fatalf("result = %+v", got)
			}
			if !strings.Contains(got.Excerpt, "why") || !strings.Contains(got.Excerpt, "try this") || !strings.Contains(got.Excerpt, "line one") {
				t.Fatalf("excerpt = %q", got.Excerpt)
			}
			// A failed e2e check must never show the console's "→ 0" (a
			// successful shell exit) next to it (f2): ExitCode -1 is not
			// the string "0", so FocusCard.svelte's `f.exit` branch never
			// renders a misleading success-looking suffix.
			if !tt.wantPassed && got.ExitCode == 0 {
				t.Fatalf("a failed e2e result must not carry exit code 0: %+v", got)
			}
		})
	}
}

func TestFocusVerifyCommandsSplitsVerifyAndE2E(t *testing.T) {
	now := time.Now()
	plan := &focuspipeline.Plan{Verify: []string{"make test", " "}, E2E: []string{"@Notes write a note", ""}}
	building := focuspipeline.New("s1", "g", now)
	building.Plan = plan
	if verify, e2e := focusVerifyCommands(building); len(verify) != 1 || verify[0] != "make test" || e2e != nil {
		t.Fatalf("build stage: verify=%v e2e=%v", verify, e2e)
	}
	reviewing := building
	reviewing.Current = focuspipeline.StageReview
	verify, e2e := focusVerifyCommands(reviewing)
	if len(verify) != 1 || verify[0] != "make test" || len(e2e) != 1 || e2e[0] != "@Notes write a note" {
		t.Fatalf("review stage: verify=%v e2e=%v", verify, e2e)
	}
	if verify, e2e := focusVerifyCommands(focuspipeline.New("s2", "g", now)); verify != nil || e2e != nil {
		t.Fatalf("no plan: verify=%v e2e=%v", verify, e2e)
	}
}

func TestNewFocusE2ERunnerRunsThroughTheEngine(t *testing.T) {
	driver := &fakeCuaDriver{}
	engine := computeruse.NewEngineWithBackend(driver, fakeDoneBackend{}, computeruse.DefaultConfig())
	run := newFocusE2ERunner(engine)
	result, err := run(context.Background(), "s1", "@Notes write a note")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !result.Passed || result.Skipped {
		t.Fatalf("result = %+v", result)
	}
	if driver.resolvedApp != "Notes" {
		t.Fatalf("app = %q, want Notes (parsed from the @App prefix)", driver.resolvedApp)
	}
}
