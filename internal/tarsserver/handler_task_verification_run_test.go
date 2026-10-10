package tarsserver

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/focusprobe"
	"github.com/devlikebear/tars/internal/session"
)

func verificationSession(t *testing.T, commands ...string) (*session.Store, string) {
	t.Helper()
	store := session.NewStore(t.TempDir())
	sess, err := store.EnsureMain()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveTasks(sess.ID, session.SessionTasks{
		Contract: &session.TaskContract{Goal: "g", Status: session.ContractStatusApproved, VerificationCommands: commands},
		Tasks:    []session.Task{{ID: "1", Title: "t1", Status: "pending"}, {ID: "2", Title: "t2", Status: "pending"}},
	}); err != nil {
		t.Fatal(err)
	}
	return store, sess.ID
}

func TestRunTaskVerification(t *testing.T) {
	const pass, fail = "printf verification-ok", "go env -definitely-not-a-real-flag"
	t.Run("all pass", func(t *testing.T) {
		store, id := verificationSession(t, pass)
		results, ok, err := runTaskVerification(context.Background(), store, id, "", 0)
		if err != nil || !ok || len(results) != 1 || results[0].Status != "passed" || results[0].EvidenceID == "" {
			t.Fatalf("results = %+v ok = %v err = %v", results, ok, err)
		}
		st, _ := store.GetTasks(id)
		if len(st.Tasks[0].Evidence) != 1 || st.Tasks[0].Evidence[0].Command != pass {
			t.Fatalf("evidence = %+v", st.Tasks[0].Evidence)
		}
	})
	t.Run("one fails", func(t *testing.T) {
		store, id := verificationSession(t, pass, fail)
		results, ok, err := runTaskVerification(context.Background(), store, id, "2", 0)
		if err != nil || ok || len(results) != 2 {
			t.Fatalf("results = %+v ok = %v err = %v", results, ok, err)
		}
		if results[1].ExitCode == 0 || results[1].Status == "passed" || strings.TrimSpace(results[1].Output) == "" {
			t.Fatalf("failed result = %+v", results[1])
		}
		st, _ := store.GetTasks(id)
		if len(st.Tasks[0].Evidence) != 0 || len(st.Tasks[1].Evidence) != 2 {
			t.Fatalf("evidence must land on the requested task: %+v", st.Tasks)
		}
	})
	t.Run("explicit commands run only those", func(t *testing.T) {
		store, id := verificationSession(t, pass, fail)
		results, ok, err := runTaskVerificationCommands(context.Background(), store, id, "", []string{pass}, 0, nil)
		if err != nil || !ok || len(results) != 1 || results[0].Command != pass {
			t.Fatalf("results = %+v ok = %v err = %v", results, ok, err)
		}
	})
	t.Run("errors carry their status", func(t *testing.T) {
		store, id := verificationSession(t, pass)
		if _, _, err := runTaskVerificationCommands(context.Background(), store, id, "", []string{" "}, 0, nil); verificationErrorStatus(err) != http.StatusBadRequest {
			t.Fatalf("blank commands: err = %v", err)
		}
		if _, _, err := runTaskVerification(context.Background(), store, "missing", "", 0); verificationErrorStatus(err) != http.StatusNotFound {
			t.Fatalf("missing session: err = %v", err)
		}
		if _, _, err := runTaskVerification(context.Background(), store, id, "9", 0); verificationErrorStatus(err) != http.StatusBadRequest {
			t.Fatalf("unknown task: err = %v", err)
		}
		unapproved, uid := verificationSession(t, pass)
		_ = unapproved.SaveTasks(uid, session.SessionTasks{Contract: &session.TaskContract{Status: "draft", VerificationCommands: []string{pass}}, Tasks: []session.Task{{ID: "1"}}})
		if _, _, err := runTaskVerificationCommands(context.Background(), unapproved, uid, "", []string{pass}, 0, nil); verificationErrorStatus(err) != http.StatusBadRequest {
			t.Fatalf("unapproved contract: err = %v", err)
		}
	})
}

func TestFocusVerificationExcerptKeepsTheFailure(t *testing.T) {
	const noisy = `for i in $(seq 1 400); do echo "ok  pkg/a$i 0.1s"; done; echo "--- FAIL: TestGreet"; for i in $(seq 1 400); do echo "ok  pkg/z$i 0.1s"; done; exit 1`
	store, id := verificationSession(t, noisy)
	results, ok, err := runTaskVerificationCommands(context.Background(), store, id, "", []string{noisy}, 0, focusprobe.FailureExcerpt)
	if err != nil || ok || len(results) != 1 {
		t.Fatalf("results = %+v ok = %v err = %v", results, ok, err)
	}
	if !strings.Contains(results[0].Output, "--- FAIL: TestGreet") {
		t.Fatalf("the failure fell out of the excerpt:\n%.300s", results[0].Output)
	}
	// The shared default (the HTTP handler) keeps the head as before.
	results, _, _ = runTaskVerification(context.Background(), store, id, "", 0)
	if strings.Contains(results[0].Output, "--- FAIL: TestGreet") {
		t.Fatal("the default excerpt changed")
	}
}

// runTaskVerification runs the approved contract commands the way the HTTP
// handler does, without a line emitter or a command subset.
func runTaskVerification(ctx context.Context, store *session.Store, sessionID, taskID string, timeout time.Duration) ([]taskVerificationResult, bool, error) {
	run, err := verifySessionTask(ctx, store, sessionID, taskID, nil, timeout, nil)
	if err != nil {
		return nil, false, err
	}
	return run.results, run.allPassed, nil
}
