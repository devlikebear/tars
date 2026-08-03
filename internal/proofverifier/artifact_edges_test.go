package proofverifier

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/workscheduler"
	"github.com/devlikebear/tars/internal/workstore"
)

func TestEngineVerifiesConfinedArtifactTreesAndExpectedDigests(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for _, directory := range []string{"reports/nested", "reports/.git", "reports/.tars", "reports/node_modules"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		"reports/a.txt":               "alpha\n",
		"reports/nested/b.txt":        "beta\n",
		"reports/.git/hidden":         "ignored\n",
		"reports/.tars/private":       "ignored\n",
		"reports/node_modules/module": "ignored\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("a.txt", filepath.Join(root, "reports", "link.txt")); err != nil {
		t.Fatal(err)
	}
	engine, err := New(Options{ID: "artifact-verifier", RootDir: root, Now: func() time.Time {
		return time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	}})
	if err != nil {
		t.Fatal(err)
	}
	if engine.Name() != defaultName || engine.Identity().ID != "artifact-verifier" || len(engine.Identity().EnvironmentJSON) == 0 {
		t.Fatalf("engine name/identity = %q/%+v", engine.Name(), engine.Identity())
	}

	requirement := workstore.ProofRequirement{Paths: []string{"reports"}}
	result, err := engine.Verify(context.Background(), workscheduler.VerificationRequest{Requirement: requirement})
	if err != nil || result.Status != workstore.ProofStatusPassed || !strings.Contains(result.Rationale, "verified 3 artifact files") {
		t.Fatalf("artifact verification = %+v err=%v", result, err)
	}
	digest, artifacts, err := engine.SubjectDigest(context.Background(), requirement)
	if err != nil || digest == "" || len(artifacts) == 0 || digest != result.SubjectDigest {
		t.Fatalf("artifact subject digest=%q artifacts=%s err=%v", digest, artifacts, err)
	}

	requirement.InputJSON = []byte(`{"expected_digests":{"reports/a.txt":"` + digestBytes([]byte("alpha\n")) + `"}}`)
	result, err = engine.Verify(context.Background(), workscheduler.VerificationRequest{Requirement: requirement})
	if err != nil || result.Status != workstore.ProofStatusPassed {
		t.Fatalf("matching digest verification = %+v err=%v", result, err)
	}
	requirement.InputJSON = []byte(`{"expected_digests":{"reports/a.txt":"sha256:wrong"}}`)
	result, err = engine.Verify(context.Background(), workscheduler.VerificationRequest{Requirement: requirement})
	if err != nil || result.Status != workstore.ProofStatusFailed || !strings.Contains(result.Rationale, "digest mismatch") {
		t.Fatalf("mismatched digest verification = %+v err=%v", result, err)
	}
	requirement.InputJSON = []byte(`{`)
	result, err = engine.Verify(context.Background(), workscheduler.VerificationRequest{Requirement: requirement})
	if err != nil || result.Status != workstore.ProofStatusFailed || !strings.Contains(result.Rationale, "invalid expected digest") {
		t.Fatalf("invalid digest policy = %+v err=%v", result, err)
	}

	for _, path := range []string{"", "../escape", "missing.txt"} {
		result, err := engine.Verify(context.Background(), workscheduler.VerificationRequest{
			Requirement: workstore.ProofRequirement{Paths: []string{path}},
		})
		if err != nil || result.Status != workstore.ProofStatusFailed {
			t.Fatalf("unsafe artifact path %q result=%+v err=%v", path, result, err)
		}
	}
	if _, _, err := engine.SubjectDigest(context.Background(), workstore.ProofRequirement{}); err == nil {
		t.Fatal("empty deterministic subject succeeded")
	}
	if !excludedDirectory(".git") || !excludedDirectory(".tars") || !excludedDirectory("node_modules") || excludedDirectory("src") {
		t.Fatal("excluded directory policy mismatch")
	}
}

func TestEngineDiscoversGitWorkspaceAndProcessRunnerOutcomes(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if output, err := (processCommandRunner{}).Run(context.Background(), root, "printf success", time.Second); err != nil || output.Stdout != "success" || output.ExitCode != 0 {
		t.Fatalf("successful process verification = %+v err=%v", output, err)
	}
	if output, err := (processCommandRunner{}).Run(context.Background(), root, "printf failed >&2; exit 4", time.Second); err != nil || output.Stderr != "failed" || output.ExitCode != 4 {
		t.Fatalf("failed process verification = %+v err=%v", output, err)
	}
	if output, err := (processCommandRunner{}).Run(context.Background(), root, "sleep 1", 10*time.Millisecond); err != nil || !output.TimedOut || output.ExitCode != -1 {
		t.Fatalf("timed process verification = %+v err=%v", output, err)
	}
	if env := verifierEnvironment(); len(env) == 0 || !containsEnvironmentKey(env, "PATH") {
		t.Fatalf("verifier environment = %v", env)
	}
	if truncateText("abcdef", 3) != "abc..." || truncateText("abc", 3) != "abc" || truncateText("abc", 0) != "..." {
		t.Fatal("truncateText did not enforce limits")
	}

	for _, args := range [][]string{{"init"}, {"config", "user.name", "TARS Test"}, {"config", "user.email", "tars@example.test"}} {
		command := append([]string{"-C", root}, args...)
		if result, err := (processCommandRunner{}).Run(context.Background(), root, "git "+strings.Join(command, " "), time.Second); err != nil || result.ExitCode != 0 {
			t.Fatalf("git %v result=%+v err=%v", args, result, err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "tracked.txt"), []byte("tracked\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if result, err := (processCommandRunner{}).Run(context.Background(), root, "git add tracked.txt", time.Second); err != nil || result.ExitCode != 0 {
		t.Fatalf("git add result=%+v err=%v", result, err)
	}
	if result, err := (processCommandRunner{}).Run(context.Background(), root, "git commit -m initial", time.Second); err != nil || result.ExitCode != 0 {
		t.Fatalf("git commit result=%+v err=%v", result, err)
	}
	if err := os.WriteFile(filepath.Join(root, "untracked.txt"), []byte("untracked\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	engine, err := New(Options{ID: "git-verifier", RootDir: root})
	if err != nil {
		t.Fatal(err)
	}
	paths, err := engine.workspaceFiles(context.Background())
	if err != nil || len(paths) != 2 {
		t.Fatalf("workspace files = %v err=%v", paths, err)
	}
	snapshot, err := engine.snapshotPaths(context.Background(), nil)
	if err != nil || snapshot.Count != 2 {
		t.Fatalf("workspace snapshot = %+v err=%v", snapshot, err)
	}
}

func TestEngineURLTargetAndConstructionFailClosed(t *testing.T) {
	t.Parallel()

	if _, err := New(Options{}); err == nil {
		t.Fatal("empty verifier options accepted")
	}
	if _, err := New(Options{ID: "verifier", RootDir: filepath.Join(t.TempDir(), "missing")}); err == nil {
		t.Fatal("missing verifier root accepted")
	}
	lookupErr := errors.New("lookup failed")
	engine, err := New(Options{
		ID: "url-verifier", RootDir: t.TempDir(),
		LookupIP: func(context.Context, string) ([]net.IPAddr, error) { return nil, lookupErr },
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []*url.URL{nil, {Scheme: "ftp", Host: "example.test"}, {Scheme: "https", Host: "example.test"}} {
		if err := engine.validateURLTarget(context.Background(), target); err == nil {
			t.Fatalf("unsafe URL target accepted: %+v", target)
		}
	}
	engine.lookupIP = func(context.Context, string) ([]net.IPAddr, error) { return nil, nil }
	if err := engine.validateURLTarget(context.Background(), &url.URL{Scheme: "https", Host: "example.test"}); err == nil {
		t.Fatal("addressless URL accepted")
	}
	engine.lookupIP = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	}
	if err := engine.validateURLTarget(context.Background(), &url.URL{Scheme: "https", Host: "example.test"}); err == nil {
		t.Fatal("private URL accepted")
	}
	engine.allowPrivateNetwork = true
	if err := engine.validateURLTarget(context.Background(), &url.URL{Scheme: "https", Host: "example.test"}); err != nil {
		t.Fatalf("explicit private URL allowance = %v", err)
	}

	client := cloneHTTPClient(&http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return errors.New("original redirect policy")
	}}, time.Second, func(context.Context, *url.URL) error { return nil })
	request := &http.Request{URL: &url.URL{Scheme: "https", Host: "example.test"}}
	if err := client.CheckRedirect(request, make([]*http.Request, 5)); err == nil || !strings.Contains(err.Error(), "too many") {
		t.Fatalf("redirect limit error = %v", err)
	}
	if err := client.CheckRedirect(request, nil); err == nil || !strings.Contains(err.Error(), "original") {
		t.Fatalf("original redirect error = %v", err)
	}
}

func containsEnvironmentKey(environment []string, key string) bool {
	for _, value := range environment {
		if strings.HasPrefix(value, key+"=") {
			return true
		}
	}
	return false
}
