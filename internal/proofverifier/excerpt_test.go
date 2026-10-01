package proofverifier

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/workscheduler"
	"github.com/devlikebear/tars/internal/workstore"
)

type fixedRunner struct{ result CommandResult }

func (f fixedRunner) Run(context.Context, string, string, time.Duration) (CommandResult, error) {
	return f.result, nil
}

func excerptsOf(t *testing.T, opts Options) (string, string) {
	t.Helper()
	opts.ID, opts.RootDir = "v", t.TempDir()
	engine, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	result, err := engine.Verify(context.Background(), workscheduler.VerificationRequest{
		Requirement: workstore.ProofRequirement{Kind: "test", Verifier: engine.Name(), Command: "make test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var in struct {
		Stdout string `json:"stdout_excerpt"`
		Stderr string `json:"stderr_excerpt"`
	}
	if err := json.Unmarshal(result.InputJSON, &in); err != nil {
		t.Fatal(err)
	}
	return in.Stdout, in.Stderr
}

func TestExcerptDefaultsToTheHeadAndTakesACustomExcerpter(t *testing.T) {
	long := strings.Repeat("ok  pkg/a 0.1s\n", 600) + "--- FAIL: TestGreet\n"
	runner := fixedRunner{CommandResult{Stdout: long, Stderr: "make: *** [test] Error 1", ExitCode: 2}}

	head, _ := excerptsOf(t, Options{CommandRunner: runner})
	if strings.Contains(head, "TestGreet") || !strings.HasPrefix(head, "ok  pkg/a") {
		t.Fatalf("the shared default keeps the head: %q", head[:40])
	}

	tail, stderr := excerptsOf(t, Options{CommandRunner: runner, Excerpt: func(text string, limit int) string {
		if len(text) <= limit {
			return text
		}
		return text[len(text)-limit:]
	}})
	if !strings.HasSuffix(tail, "--- FAIL: TestGreet") || stderr != "make: *** [test] Error 1" {
		t.Fatalf("custom excerpt = %q / %q", tail[len(tail)-30:], stderr)
	}
}
