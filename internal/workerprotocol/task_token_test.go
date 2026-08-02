package workerprotocol

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestTaskTokenIsShortLivedScopedAndPubliclyVerifiable(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 2, 15, 0, 0, 0, time.UTC)
	seed := make([]byte, ed25519.SeedSize)
	for index := range seed {
		seed[index] = byte(index + 1)
	}
	issuer, err := NewTaskTokenIssuer(TaskTokenIssuerOptions{
		PrivateKey: ed25519.NewKeyFromSeed(seed), MaxTTL: 5 * time.Minute,
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("new task token issuer: %v", err)
	}
	binding := TaskTokenBinding{
		WorkspaceID: "workspace-a", WorkID: "work-a", StepID: "step-a", AttemptID: "attempt-a",
		PlacementID: "placement-a", WorkerID: "worker-a",
	}
	token, claims, err := issuer.Issue(binding, []TaskScope{TaskScopeCollect, TaskScopeExecute, TaskScopeCheckpoint, TaskScopeExecute}, 2*time.Minute)
	if err != nil {
		t.Fatalf("issue task token: %v", err)
	}
	if !strings.HasPrefix(token, taskTokenPrefix+".") || claims.TokenID == "" || claims.KeyID == "" {
		t.Fatalf("issued token=%q claims=%+v", token, claims)
	}
	if claims.IssuedAtUnix != now.Unix() || claims.ExpiresAtUnix != now.Add(2*time.Minute).Unix() {
		t.Fatalf("unexpected token times: %+v", claims)
	}
	if len(claims.Scopes) != 3 || claims.Scopes[0] != TaskScopeCheckpoint || claims.Scopes[1] != TaskScopeCollect || claims.Scopes[2] != TaskScopeExecute {
		t.Fatalf("normalized scopes=%v", claims.Scopes)
	}

	verified, err := issuer.PublicVerifier().Verify(token, binding, TaskScopeExecute, TaskScopeCheckpoint)
	if err != nil {
		t.Fatalf("verify task token: %v", err)
	}
	if verified.TokenID != claims.TokenID || verified.KeyID != issuer.KeyID() {
		t.Fatalf("verified claims=%+v issued=%+v", verified, claims)
	}
	rawVerifier, err := json.Marshal(issuer.PublicVerifier())
	if err != nil {
		t.Fatalf("marshal verifier: %v", err)
	}
	if strings.Contains(string(rawVerifier), string(seed)) || strings.Contains(string(rawVerifier), token) {
		t.Fatalf("public verifier exposed private material: %s", rawVerifier)
	}
}

func TestTaskTokenRejectsScopeBindingExpiryAndTampering(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 2, 15, 0, 0, 0, time.UTC)
	seed := make([]byte, ed25519.SeedSize)
	seed[0] = 42
	issuer, err := NewTaskTokenIssuer(TaskTokenIssuerOptions{
		PrivateKey: ed25519.NewKeyFromSeed(seed), MaxTTL: 5 * time.Minute,
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("new task token issuer: %v", err)
	}
	binding := TaskTokenBinding{
		WorkspaceID: "workspace-a", WorkID: "work-a", StepID: "step-a", AttemptID: "attempt-a",
		PlacementID: "placement-a", WorkerID: "worker-a",
	}
	token, _, err := issuer.Issue(binding, []TaskScope{TaskScopeExecute}, time.Minute)
	if err != nil {
		t.Fatalf("issue task token: %v", err)
	}
	verifier := issuer.PublicVerifier()

	wrongWorker := binding
	wrongWorker.WorkerID = "worker-b"
	if _, err := verifier.Verify(token, wrongWorker, TaskScopeExecute); !errors.Is(err, ErrTaskTokenBinding) {
		t.Fatalf("wrong-worker error=%v want ErrTaskTokenBinding", err)
	}
	if _, err := verifier.Verify(token, binding, TaskScopeCollect); !errors.Is(err, ErrTaskTokenScope) {
		t.Fatalf("missing-scope error=%v want ErrTaskTokenScope", err)
	}
	replacement := byte('A')
	if token[len(token)-1] == replacement {
		replacement = 'B'
	}
	tampered := token[:len(token)-1] + string(replacement)
	if _, err := verifier.Verify(tampered, binding, TaskScopeExecute); !errors.Is(err, ErrTaskTokenSignature) {
		t.Fatalf("tampered error=%v want ErrTaskTokenSignature", err)
	}
	verifier.now = func() time.Time { return now.Add(2 * time.Minute) }
	if _, err := verifier.Verify(token, binding, TaskScopeExecute); !errors.Is(err, ErrTaskTokenExpired) {
		t.Fatalf("expired error=%v want ErrTaskTokenExpired", err)
	}
	if _, _, err := issuer.Issue(binding, []TaskScope{TaskScopeExecute}, 6*time.Minute); !errors.Is(err, ErrTaskTokenTTL) {
		t.Fatalf("oversized TTL error=%v want ErrTaskTokenTTL", err)
	}
}
