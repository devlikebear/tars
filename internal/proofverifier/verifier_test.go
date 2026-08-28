package proofverifier

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/workscheduler"
	"github.com/devlikebear/tars/internal/workstore"
)

func TestEngineRunsDeterministicCommandAndDetectsChangedSubject(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "result.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o600); err != nil {
		t.Fatalf("write verification subject: %v", err)
	}
	engine, err := New(Options{ID: "verifier-1", RootDir: root, Timeout: commandTestBudget})
	if err != nil {
		t.Fatalf("new proof verifier: %v", err)
	}
	requirement := workstore.ProofRequirement{
		Kind: "test", Verifier: engine.Name(), Command: "test -f result.txt", Paths: []string{"result.txt"},
	}
	result, err := engine.Verify(context.Background(), workscheduler.VerificationRequest{
		Requirement: requirement,
		Result:      workscheduler.ExecutionResult{Succeeded: true, OutputJSON: json.RawMessage(`{"ok":true}`)},
	})
	if err != nil {
		t.Fatalf("verify deterministic command: %v", err)
	}
	if result.Status != workstore.ProofStatusPassed || result.SubjectDigest == "" || result.Rationale == "" || len(result.ArtifactDigestsJSON) == 0 || result.UsedLLM {
		t.Fatalf("deterministic result = %+v", result)
	}
	before := result.SubjectDigest
	if err := os.WriteFile(path, []byte("after\n"), 0o600); err != nil {
		t.Fatalf("change verification subject: %v", err)
	}
	after, _, err := engine.SubjectDigest(context.Background(), requirement)
	if err != nil {
		t.Fatalf("digest changed subject: %v", err)
	}
	if after == before {
		t.Fatalf("subject digest did not change: %s", after)
	}
}

func TestEngineRecordsDeterministicCommandFailure(t *testing.T) {
	t.Parallel()

	engine, err := New(Options{ID: "verifier-1", RootDir: t.TempDir(), Timeout: commandTestBudget})
	if err != nil {
		t.Fatalf("new proof verifier: %v", err)
	}
	result, err := engine.Verify(context.Background(), workscheduler.VerificationRequest{
		Requirement: workstore.ProofRequirement{Kind: "test", Verifier: engine.Name(), Command: "exit 7"},
	})
	if err != nil {
		t.Fatalf("verify failing command: %v", err)
	}
	if result.Status != workstore.ProofStatusFailed || result.Rationale != "command exited with code 7" {
		t.Fatalf("failed command result = %+v", result)
	}
}

func TestEngineUsesBudgetedLLMOnlyWithoutDeterministicVerifier(t *testing.T) {
	t.Parallel()

	judge := &fakeJudge{result: JudgeResult{
		Status: workstore.ProofStatusPassed, Rationale: "semantic criteria satisfied",
		Model: "judge-model", Tokens: 120, CostUSD: 0.03,
	}}
	engine, err := New(Options{ID: "verifier-1", RootDir: t.TempDir(), LLMJudge: judge})
	if err != nil {
		t.Fatalf("new proof verifier: %v", err)
	}
	request := workscheduler.VerificationRequest{
		Requirement: workstore.ProofRequirement{Kind: "semantic", Verifier: engine.Name()},
		Result:      workscheduler.ExecutionResult{Succeeded: true, OutputJSON: json.RawMessage(`{"answer":"done"}`)},
	}
	result, err := engine.Verify(context.Background(), request)
	if err != nil {
		t.Fatalf("verify without LLM policy: %v", err)
	}
	if result.Status != workstore.ProofStatusPending || judge.calls != 0 {
		t.Fatalf("unapproved LLM result=%+v calls=%d", result, judge.calls)
	}

	request.Execution.Claim.Schedule.Policy.Proof = workstore.StepProofPolicy{
		Required: true, AllowLLMFallback: true, MaxLLMTokens: 100, MaxLLMCostUSD: 0.02,
	}
	result, err = engine.Verify(context.Background(), request)
	if err != nil {
		t.Fatalf("verify over-budget LLM result: %v", err)
	}
	if result.Status != workstore.ProofStatusPending || !result.UsedLLM || judge.calls != 1 {
		t.Fatalf("over-budget LLM result=%+v calls=%d", result, judge.calls)
	}

	request.Requirement.Command = "true"
	result, err = engine.Verify(context.Background(), request)
	if err != nil {
		t.Fatalf("verify deterministic priority: %v", err)
	}
	if result.Status != workstore.ProofStatusPassed || result.UsedLLM || judge.calls != 1 {
		t.Fatalf("deterministic priority result=%+v calls=%d", result, judge.calls)
	}
}

func TestEngineVerifiesHTTPSAndRejectsPrivateTargets(t *testing.T) {
	t.Parallel()

	body := "commit-a"
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"ETag": []string{`"proof"`}},
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    request,
		}, nil
	})}
	publicLookup := func(_ context.Context, _ string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
	}
	engine, err := New(Options{
		ID: "verifier-1", RootDir: t.TempDir(), HTTPClient: client, LookupIP: publicLookup,
	})
	if err != nil {
		t.Fatalf("new URL verifier: %v", err)
	}
	requirement := workstore.ProofRequirement{Kind: "pr", Verifier: engine.Name(), URL: "https://proof.example/pr/42"}
	verified, err := engine.Verify(context.Background(), workscheduler.VerificationRequest{Requirement: requirement})
	if err != nil || verified.Status != workstore.ProofStatusPassed {
		t.Fatalf("verify public HTTPS result=%+v err=%v", verified, err)
	}
	body = "commit-b"
	changed, _, err := engine.SubjectDigest(context.Background(), requirement)
	if err != nil || changed == verified.SubjectDigest {
		t.Fatalf("changed URL digest=%s original=%s err=%v", changed, verified.SubjectDigest, err)
	}

	privateEngine, err := New(Options{
		ID: "verifier-2", RootDir: t.TempDir(), HTTPClient: client,
		LookupIP: func(_ context.Context, _ string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
		},
	})
	if err != nil {
		t.Fatalf("new private-target verifier: %v", err)
	}
	rejected, err := privateEngine.Verify(context.Background(), workscheduler.VerificationRequest{Requirement: requirement})
	if err != nil || rejected.Status != workstore.ProofStatusFailed || !strings.Contains(rejected.Rationale, "non-public") {
		t.Fatalf("private target result=%+v err=%v", rejected, err)
	}
}

type fakeJudge struct {
	result JudgeResult
	err    error
	calls  int
}

func (judge *fakeJudge) Judge(_ context.Context, _ JudgeRequest) (JudgeResult, error) {
	judge.calls++
	return judge.result, judge.err
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (roundTrip roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}
