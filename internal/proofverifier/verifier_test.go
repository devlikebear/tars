package proofverifier

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/workscheduler"
	"github.com/devlikebear/tars/internal/workstore"
)

func TestEngineRunsDeterministicCommand(t *testing.T) {
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

func TestEngineCommandWithoutPathsRecordsTreeDigestNotEveryFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for index := 0; index < 40; index++ {
		name := filepath.Join(root, "pkg", "file"+strconv.Itoa(index)+".go")
		if err := os.MkdirAll(filepath.Dir(name), 0o700); err != nil {
			t.Fatalf("create workspace directory: %v", err)
		}
		if err := os.WriteFile(name, []byte("package pkg\n"), 0o600); err != nil {
			t.Fatalf("write workspace file: %v", err)
		}
	}
	engine, err := New(Options{ID: "verifier-1", RootDir: root, Timeout: commandTestBudget})
	if err != nil {
		t.Fatalf("new proof verifier: %v", err)
	}
	verify := func() workscheduler.VerificationResult {
		t.Helper()
		result, err := engine.Verify(context.Background(), workscheduler.VerificationRequest{
			Requirement: workstore.ProofRequirement{Kind: "test", Verifier: engine.Name(), Command: "true"},
		})
		if err != nil {
			t.Fatalf("verify workspace command: %v", err)
		}
		return result
	}
	result := verify()
	var recorded []treeDigest
	if err := json.Unmarshal(result.ArtifactDigestsJSON, &recorded); err != nil {
		t.Fatalf("decode artifact digests %s: %v", result.ArtifactDigestsJSON, err)
	}
	if len(recorded) != 1 || recorded[0].Path != "." || recorded[0].Files != 40 || recorded[0].Digest != result.SubjectDigest || recorded[0].SizeBytes != 40*int64(len("package pkg\n")) {
		t.Fatalf("workspace snapshot recorded %s, want one tree digest of 40 files matching subject %s", result.ArtifactDigestsJSON, result.SubjectDigest)
	}

	// The tree digest still commits to every file.
	if err := os.WriteFile(filepath.Join(root, "pkg", "file0.go"), []byte("package changed\n"), 0o600); err != nil {
		t.Fatalf("change workspace file: %v", err)
	}
	if changed := verify(); changed.SubjectDigest == result.SubjectDigest {
		t.Fatalf("subject digest %s did not change with a workspace file", changed.SubjectDigest)
	}
}

func TestEngineListsDeclaredPathsUpToTheLimit(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for _, dir := range []struct {
		name  string
		files int
	}{{"small", 3}, {"large", maxListedArtifacts + 1}} {
		if err := os.MkdirAll(filepath.Join(root, dir.name), 0o700); err != nil {
			t.Fatalf("create %s: %v", dir.name, err)
		}
		for index := 0; index < dir.files; index++ {
			if err := os.WriteFile(filepath.Join(root, dir.name, "f"+strconv.Itoa(index)), []byte("x"), 0o600); err != nil {
				t.Fatalf("write %s file: %v", dir.name, err)
			}
		}
	}
	engine, err := New(Options{ID: "verifier-1", RootDir: root, Timeout: commandTestBudget})
	if err != nil {
		t.Fatalf("new proof verifier: %v", err)
	}
	small, err := engine.snapshotPaths(context.Background(), []string{"small"})
	if err != nil {
		t.Fatalf("snapshot declared paths: %v", err)
	}
	var listed []fileDigest
	if err := json.Unmarshal(small.ArtifactDigestsJSON, &listed); err != nil || len(listed) != 3 || listed[0].Path != "small/f0" {
		t.Fatalf("declared paths recorded %s (err %v), want each of 3 files", small.ArtifactDigestsJSON, err)
	}
	large, err := engine.snapshotPaths(context.Background(), []string{"large"})
	if err != nil {
		t.Fatalf("snapshot large declared path: %v", err)
	}
	var tree []treeDigest
	if err := json.Unmarshal(large.ArtifactDigestsJSON, &tree); err != nil || len(tree) != 1 || tree[0].Files != maxListedArtifacts+1 || tree[0].Digest != large.SubjectDigest {
		t.Fatalf("large declared path recorded %d bytes (err %v), want one tree digest", len(large.ArtifactDigestsJSON), err)
	}
	if large.Count != maxListedArtifacts+1 || len(large.ByPath) != large.Count {
		t.Fatalf("large snapshot kept %d of %d digests for expected-digest checks", len(large.ByPath), large.Count)
	}
}
