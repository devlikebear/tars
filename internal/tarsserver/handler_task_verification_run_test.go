package tarsserver

import (
	"context"
	"net/http"
	"strings"
	"testing"

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
	results, ok, err := runTaskVerificationCommands(context.Background(), store, id, "", []string{noisy}, 0, focusFailureExcerpt)
	if err != nil || ok || len(results) != 1 {
		t.Fatalf("results = %+v ok = %v err = %v", results, ok, err)
	}
	if !strings.Contains(results[0].Output, "--- FAIL: TestGreet") {
		t.Fatalf("the failure fell out of the excerpt:\n%.300s", results[0].Output)
	}
}
