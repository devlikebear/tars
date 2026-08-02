// Package proofverifier provides deterministic, provenance-rich verification
// for durable scheduler completion gates.
package proofverifier

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/workscheduler"
	"github.com/devlikebear/tars/internal/workstore"
)

const (
	defaultName        = "deterministic"
	defaultTimeout     = 5 * time.Minute
	maximumURLBodySize = 4 << 20
)

type Options struct {
	Name                string
	ID                  string
	RootDir             string
	Timeout             time.Duration
	CommandRunner       CommandRunner
	HTTPClient          *http.Client
	LookupIP            func(context.Context, string) ([]net.IPAddr, error)
	AllowHTTP           bool
	AllowPrivateNetwork bool
	LLMJudge            LLMJudge
	Now                 func() time.Time
}

type CommandResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	TimedOut bool
	Duration time.Duration
}

type CommandRunner interface {
	Run(context.Context, string, string, time.Duration) (CommandResult, error)
}

type JudgeRequest struct {
	Objective    string
	Requirement  workstore.ProofRequirement
	WorkerOutput json.RawMessage
	MaxTokens    int64
	MaxCostUSD   float64
}

type JudgeResult struct {
	Status    workstore.ProofStatus
	Rationale string
	Model     string
	Tokens    int64
	CostUSD   float64
	InputJSON json.RawMessage
}

type LLMJudge interface {
	Judge(context.Context, JudgeRequest) (JudgeResult, error)
}

type Engine struct {
	name                string
	id                  string
	rootDir             string
	timeout             time.Duration
	runner              CommandRunner
	client              *http.Client
	lookupIP            func(context.Context, string) ([]net.IPAddr, error)
	allowHTTP           bool
	allowPrivateNetwork bool
	judge               LLMJudge
	now                 func() time.Time
	environmentJSON     json.RawMessage
}

func New(opts Options) (*Engine, error) {
	id := strings.TrimSpace(opts.ID)
	if id == "" {
		return nil, fmt.Errorf("proofverifier: verifier identity is required")
	}
	rootDir := strings.TrimSpace(opts.RootDir)
	if rootDir == "" {
		return nil, fmt.Errorf("proofverifier: root directory is required")
	}
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, fmt.Errorf("proofverifier: resolve root directory: %w", err)
	}
	info, err := os.Stat(absRoot)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("proofverifier: root directory is unavailable")
	}
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = defaultName
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	runner := opts.CommandRunner
	if runner == nil {
		runner = processCommandRunner{}
	}
	lookupIP := opts.LookupIP
	if lookupIP == nil {
		lookupIP = func(ctx context.Context, host string) ([]net.IPAddr, error) {
			return net.DefaultResolver.LookupIPAddr(ctx, host)
		}
	}
	environmentJSON, err := json.Marshal(map[string]any{
		"process_role": "independent-proof-verifier",
		"runner":       "subprocess",
		"root_dir":     absRoot,
		"goos":         runtime.GOOS,
		"goarch":       runtime.GOARCH,
	})
	if err != nil {
		return nil, fmt.Errorf("proofverifier: encode environment: %w", err)
	}
	engine := &Engine{
		name: name, id: id, rootDir: absRoot, timeout: timeout, runner: runner,
		lookupIP: lookupIP, allowHTTP: opts.AllowHTTP,
		allowPrivateNetwork: opts.AllowPrivateNetwork, judge: opts.LLMJudge,
		now: now, environmentJSON: environmentJSON,
	}
	engine.client = cloneHTTPClient(opts.HTTPClient, timeout, engine.validateURLTarget)
	return engine, nil
}

func (engine *Engine) Name() string { return engine.name }

func (engine *Engine) Identity() workscheduler.VerifierIdentity {
	return workscheduler.VerifierIdentity{
		ID: engine.id, EnvironmentJSON: append(json.RawMessage(nil), engine.environmentJSON...),
	}
}

func (engine *Engine) Verify(ctx context.Context, request workscheduler.VerificationRequest) (workscheduler.VerificationResult, error) {
	requirement := request.Requirement
	switch {
	case strings.TrimSpace(requirement.Command) != "":
		return engine.verifyCommand(ctx, requirement)
	case len(requirement.Paths) > 0:
		return engine.verifyArtifacts(ctx, requirement)
	case strings.TrimSpace(requirement.URL) != "":
		return engine.verifyURL(ctx, requirement)
	default:
		return engine.verifyWithLLM(ctx, request)
	}
}

// SubjectDigest recalculates the current deterministic subject without running
// a verification command. Comparing it with Proof.SubjectDigest detects stale
// file, commit, artifact, or URL evidence.
func (engine *Engine) SubjectDigest(ctx context.Context, requirement workstore.ProofRequirement) (string, json.RawMessage, error) {
	switch {
	case len(requirement.Paths) > 0:
		snapshot, err := engine.snapshotPaths(ctx, requirement.Paths)
		return snapshot.SubjectDigest, snapshot.ArtifactDigestsJSON, err
	case strings.TrimSpace(requirement.URL) != "":
		result, err := engine.fetchURL(ctx, requirement.URL)
		return result.SubjectDigest, result.ArtifactDigestsJSON, err
	case strings.TrimSpace(requirement.Command) != "":
		snapshot, err := engine.snapshotPaths(ctx, nil)
		return snapshot.SubjectDigest, snapshot.ArtifactDigestsJSON, err
	default:
		return "", nil, fmt.Errorf("proofverifier: no deterministic subject is declared")
	}
}

func (engine *Engine) verifyCommand(ctx context.Context, requirement workstore.ProofRequirement) (workscheduler.VerificationResult, error) {
	before, err := engine.snapshotPaths(ctx, requirement.Paths)
	if err != nil {
		return failedResult(engine.now(), "verification subject is unavailable", err.Error()), nil
	}
	command := strings.TrimSpace(requirement.Command)
	result, err := engine.runner.Run(ctx, engine.rootDir, command, engine.timeout)
	if err != nil {
		return workscheduler.VerificationResult{}, fmt.Errorf("proofverifier: run command: %w", err)
	}
	after, snapshotErr := engine.snapshotPaths(ctx, requirement.Paths)
	if snapshotErr != nil {
		return failedResult(engine.now(), "verification subject changed unexpectedly", snapshotErr.Error()), nil
	}
	status := workstore.ProofStatusPassed
	rationale := "command exited with code 0"
	if result.ExitCode != 0 || result.TimedOut {
		status = workstore.ProofStatusFailed
		if result.TimedOut {
			rationale = "command timed out"
		} else {
			rationale = fmt.Sprintf("command exited with code %d", result.ExitCode)
		}
	}
	inputJSON, _ := json.Marshal(map[string]any{
		"command": command, "exit_code": result.ExitCode, "timed_out": result.TimedOut,
		"duration_ms": result.Duration.Milliseconds(), "stdout_digest": digestText(result.Stdout),
		"stderr_digest": digestText(result.Stderr), "stdout_excerpt": truncateText(result.Stdout, 4096),
		"stderr_excerpt": truncateText(result.Stderr, 4096), "subject_before": before.SubjectDigest,
		"subject_after": after.SubjectDigest,
	})
	return workscheduler.VerificationResult{
		Status: status, Summary: "deterministic command verification",
		Rationale: rationale, SubjectDigest: after.SubjectDigest,
		InputJSON: inputJSON, ArtifactDigestsJSON: after.ArtifactDigestsJSON,
		ObservedAt: timePointer(engine.now().UTC()),
	}, nil
}

func (engine *Engine) verifyArtifacts(ctx context.Context, requirement workstore.ProofRequirement) (workscheduler.VerificationResult, error) {
	snapshot, err := engine.snapshotPaths(ctx, requirement.Paths)
	if err != nil {
		return failedResult(engine.now(), "artifact verification failed", err.Error()), nil
	}
	status := workstore.ProofStatusPassed
	rationale := fmt.Sprintf("verified %d artifact files", snapshot.Count)
	var expected struct {
		ExpectedDigests map[string]string `json:"expected_digests"`
	}
	if len(requirement.InputJSON) > 0 {
		if err := json.Unmarshal(requirement.InputJSON, &expected); err != nil {
			return failedResult(engine.now(), "artifact verification failed", "invalid expected digest policy"), nil
		}
	}
	for path, digest := range expected.ExpectedDigests {
		if snapshot.ByPath[filepath.ToSlash(filepath.Clean(path))] != strings.TrimSpace(digest) {
			status = workstore.ProofStatusFailed
			rationale = fmt.Sprintf("artifact digest mismatch for %s", path)
			break
		}
	}
	inputJSON, _ := json.Marshal(map[string]any{"paths": requirement.Paths, "expected_digests": expected.ExpectedDigests})
	return workscheduler.VerificationResult{
		Status: status, Summary: "artifact digest verification", Rationale: rationale,
		SubjectDigest: snapshot.SubjectDigest, InputJSON: inputJSON,
		ArtifactDigestsJSON: snapshot.ArtifactDigestsJSON,
		ObservedAt:          timePointer(engine.now().UTC()),
	}, nil
}

func (engine *Engine) verifyURL(ctx context.Context, requirement workstore.ProofRequirement) (workscheduler.VerificationResult, error) {
	result, err := engine.fetchURL(ctx, requirement.URL)
	if err != nil {
		return failedResult(engine.now(), "URL verification failed", err.Error()), nil
	}
	status := workstore.ProofStatusPassed
	rationale := fmt.Sprintf("HTTPS endpoint returned status %d", result.StatusCode)
	if result.StatusCode < http.StatusOK || result.StatusCode >= http.StatusMultipleChoices {
		status = workstore.ProofStatusFailed
		rationale = fmt.Sprintf("HTTPS endpoint returned status %d", result.StatusCode)
	}
	return workscheduler.VerificationResult{
		Status: status, Summary: "URL verification", Rationale: rationale,
		SubjectDigest: result.SubjectDigest, InputJSON: result.InputJSON,
		ArtifactDigestsJSON: result.ArtifactDigestsJSON,
		ObservedAt:          timePointer(engine.now().UTC()),
	}, nil
}

func (engine *Engine) verifyWithLLM(ctx context.Context, request workscheduler.VerificationRequest) (workscheduler.VerificationResult, error) {
	policy := request.Execution.Claim.Schedule.Policy.Proof
	subjectDigest := digestJSON(map[string]any{"requirement": request.Requirement, "worker_output": request.Result.OutputJSON})
	if !policy.AllowLLMFallback || engine.judge == nil {
		return workscheduler.VerificationResult{
			Status: workstore.ProofStatusPending, Summary: "semantic verification pending",
			Rationale:     "no deterministic verifier is declared and LLM fallback is not available",
			SubjectDigest: subjectDigest,
		}, nil
	}
	if policy.MaxLLMTokens <= 0 || policy.MaxLLMCostUSD <= 0 {
		return workscheduler.VerificationResult{
			Status: workstore.ProofStatusPending, Summary: "semantic verification pending",
			Rationale: "LLM judge budgets are not configured", SubjectDigest: subjectDigest,
		}, nil
	}
	judged, err := engine.judge.Judge(ctx, JudgeRequest{
		Objective: request.Execution.Work.Objective, Requirement: request.Requirement,
		WorkerOutput: request.Result.OutputJSON, MaxTokens: policy.MaxLLMTokens,
		MaxCostUSD: policy.MaxLLMCostUSD,
	})
	if err != nil {
		return workscheduler.VerificationResult{}, fmt.Errorf("proofverifier: LLM judge: %w", err)
	}
	status := judged.Status
	if status != workstore.ProofStatusPassed && status != workstore.ProofStatusFailed {
		status = workstore.ProofStatusPending
	}
	if judged.Tokens > policy.MaxLLMTokens || judged.CostUSD > policy.MaxLLMCostUSD {
		status = workstore.ProofStatusPending
	}
	inputJSON, _ := json.Marshal(map[string]any{
		"model": judged.Model, "tokens": judged.Tokens, "cost_usd": judged.CostUSD,
		"judge_input": judged.InputJSON,
	})
	return workscheduler.VerificationResult{
		Status: status, Summary: "bounded LLM judge verification",
		Rationale: strings.TrimSpace(judged.Rationale), SubjectDigest: subjectDigest,
		InputJSON: inputJSON, ObservedAt: timePointer(engine.now().UTC()),
		UsedLLM: true, Tokens: judged.Tokens, CostUSD: judged.CostUSD,
	}, nil
}

type fileDigest struct {
	Path      string `json:"path"`
	Digest    string `json:"digest"`
	SizeBytes int64  `json:"size_bytes"`
}

type pathSnapshot struct {
	SubjectDigest       string
	ArtifactDigestsJSON json.RawMessage
	ByPath              map[string]string
	Count               int
}

func (engine *Engine) snapshotPaths(ctx context.Context, requested []string) (pathSnapshot, error) {
	paths := requested
	if len(paths) == 0 {
		tracked, err := engine.workspaceFiles(ctx)
		if err != nil {
			return pathSnapshot{}, err
		}
		paths = tracked
	}
	files := make(map[string]fileDigest)
	for _, requestedPath := range paths {
		if err := ctx.Err(); err != nil {
			return pathSnapshot{}, err
		}
		absolute, relative, err := engine.confinedPath(requestedPath)
		if err != nil {
			return pathSnapshot{}, err
		}
		info, err := os.Lstat(absolute)
		if err != nil {
			return pathSnapshot{}, fmt.Errorf("path %q: %w", requestedPath, err)
		}
		if info.IsDir() {
			err = filepath.WalkDir(absolute, func(path string, entry os.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if path != absolute && entry.IsDir() && excludedDirectory(entry.Name()) {
					return filepath.SkipDir
				}
				if entry.IsDir() {
					return nil
				}
				return engine.addFileDigest(files, path)
			})
			if err != nil {
				return pathSnapshot{}, fmt.Errorf("walk path %q: %w", relative, err)
			}
			continue
		}
		if err := engine.addFileDigest(files, absolute); err != nil {
			return pathSnapshot{}, err
		}
	}
	ordered := make([]fileDigest, 0, len(files))
	for _, item := range files {
		ordered = append(ordered, item)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Path < ordered[j].Path })
	encoded, err := json.Marshal(ordered)
	if err != nil {
		return pathSnapshot{}, fmt.Errorf("encode artifact digests: %w", err)
	}
	byPath := make(map[string]string, len(ordered))
	for _, item := range ordered {
		byPath[item.Path] = item.Digest
	}
	return pathSnapshot{
		SubjectDigest: digestBytes(encoded), ArtifactDigestsJSON: encoded,
		ByPath: byPath, Count: len(ordered),
	}, nil
}

func (engine *Engine) workspaceFiles(ctx context.Context) ([]string, error) {
	command := exec.CommandContext(ctx, "git", "-C", engine.rootDir, "ls-files", "-co", "--exclude-standard", "-z")
	raw, err := command.Output()
	if err == nil {
		parts := strings.Split(string(raw), "\x00")
		paths := make([]string, 0, len(parts))
		for _, path := range parts {
			if strings.TrimSpace(path) != "" {
				paths = append(paths, path)
			}
		}
		if len(paths) > 0 {
			return paths, nil
		}
	}
	return []string{"."}, nil
}

func (engine *Engine) confinedPath(path string) (string, string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", "", fmt.Errorf("proofverifier: empty artifact path")
	}
	absolute := path
	if !filepath.IsAbs(absolute) {
		absolute = filepath.Join(engine.rootDir, absolute)
	}
	absolute = filepath.Clean(absolute)
	relative, err := filepath.Rel(engine.rootDir, absolute)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("proofverifier: path %q escapes verifier root", path)
	}
	return absolute, filepath.ToSlash(relative), nil
}

func (engine *Engine) addFileDigest(files map[string]fileDigest, absolute string) error {
	relative, err := filepath.Rel(engine.rootDir, absolute)
	if err != nil {
		return err
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return err
	}
	var raw []byte
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(absolute)
		if err != nil {
			return err
		}
		raw = []byte("symlink:" + target)
	} else if info.Mode().IsRegular() {
		raw, err = os.ReadFile(absolute)
		if err != nil {
			return err
		}
	} else {
		return nil
	}
	key := filepath.ToSlash(relative)
	files[key] = fileDigest{Path: key, Digest: digestBytes(raw), SizeBytes: info.Size()}
	return nil
}

func excludedDirectory(name string) bool {
	switch name {
	case ".git", ".tars", "node_modules":
		return true
	default:
		return false
	}
}

type urlSnapshot struct {
	StatusCode          int
	SubjectDigest       string
	InputJSON           json.RawMessage
	ArtifactDigestsJSON json.RawMessage
}

func (engine *Engine) fetchURL(ctx context.Context, rawURL string) (urlSnapshot, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return urlSnapshot{}, fmt.Errorf("parse URL: %w", err)
	}
	if err := engine.validateURLTarget(ctx, parsed); err != nil {
		return urlSnapshot{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return urlSnapshot{}, fmt.Errorf("create URL request: %w", err)
	}
	response, err := engine.client.Do(request)
	if err != nil {
		return urlSnapshot{}, fmt.Errorf("request URL: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(response.Body, maximumURLBodySize+1))
	if err != nil {
		return urlSnapshot{}, fmt.Errorf("read URL response: %w", err)
	}
	if len(body) > maximumURLBodySize {
		return urlSnapshot{}, fmt.Errorf("URL response exceeds %d bytes", maximumURLBodySize)
	}
	bodyDigest := digestBytes(body)
	inputJSON, _ := json.Marshal(map[string]any{
		"url": response.Request.URL.String(), "status_code": response.StatusCode,
		"etag": response.Header.Get("ETag"), "last_modified": response.Header.Get("Last-Modified"),
		"body_digest": bodyDigest,
	})
	artifacts, _ := json.Marshal([]fileDigest{{Path: response.Request.URL.String(), Digest: bodyDigest, SizeBytes: int64(len(body))}})
	return urlSnapshot{
		StatusCode: response.StatusCode, SubjectDigest: digestBytes(inputJSON),
		InputJSON: inputJSON, ArtifactDigestsJSON: artifacts,
	}, nil
}

func (engine *Engine) validateURLTarget(ctx context.Context, target *url.URL) error {
	if target == nil || strings.TrimSpace(target.Hostname()) == "" {
		return fmt.Errorf("proofverifier: URL host is required")
	}
	if target.Scheme != "https" && (!engine.allowHTTP || target.Scheme != "http") {
		return fmt.Errorf("proofverifier: only HTTPS verification URLs are allowed")
	}
	addresses, err := engine.lookupIP(ctx, target.Hostname())
	if err != nil {
		return fmt.Errorf("proofverifier: resolve URL host: %w", err)
	}
	if len(addresses) == 0 {
		return fmt.Errorf("proofverifier: URL host has no addresses")
	}
	if engine.allowPrivateNetwork {
		return nil
	}
	for _, address := range addresses {
		ip := address.IP
		if ip == nil || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
			return fmt.Errorf("proofverifier: URL resolves to a non-public address")
		}
	}
	return nil
}

func cloneHTTPClient(source *http.Client, timeout time.Duration, validate func(context.Context, *url.URL) error) *http.Client {
	client := http.Client{Timeout: timeout}
	if source != nil {
		client = *source
		if client.Timeout <= 0 {
			client.Timeout = timeout
		}
	}
	previous := client.CheckRedirect
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("proofverifier: too many URL redirects")
		}
		if err := validate(request.Context(), request.URL); err != nil {
			return err
		}
		if previous != nil {
			return previous(request, via)
		}
		return nil
	}
	return &client
}

type processCommandRunner struct{}

func (processCommandRunner) Run(ctx context.Context, directory, command string, timeout time.Duration) (CommandResult, error) {
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	startedAt := time.Now()
	cmd := exec.CommandContext(commandCtx, "/bin/sh", "-lc", command)
	cmd.Dir = directory
	cmd.Env = verifierEnvironment()
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := CommandResult{Stdout: stdout.String(), Stderr: stderr.String(), Duration: time.Since(startedAt)}
	if commandCtx.Err() != nil {
		result.ExitCode = -1
		result.TimedOut = errors.Is(commandCtx.Err(), context.DeadlineExceeded)
		return result, nil
	}
	if err == nil {
		return result, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		return result, nil
	}
	return CommandResult{}, err
}

func verifierEnvironment() []string {
	environment := make([]string, 0, len(os.Environ())+1)
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		upper := strings.ToUpper(name)
		if strings.Contains(upper, "API_KEY") || strings.Contains(upper, "TOKEN") || strings.Contains(upper, "SECRET") || strings.Contains(upper, "PASSWORD") || strings.Contains(upper, "CREDENTIAL") {
			continue
		}
		environment = append(environment, entry)
	}
	return append(environment, "TARS_EXECUTION_ROLE=proof-verifier")
}

func failedResult(now time.Time, summary, rationale string) workscheduler.VerificationResult {
	return workscheduler.VerificationResult{
		Status: workstore.ProofStatusFailed, Summary: summary,
		Rationale: strings.TrimSpace(rationale), ObservedAt: timePointer(now.UTC()),
	}
}

func digestText(value string) string { return digestBytes([]byte(value)) }

func digestBytes(value []byte) string {
	digest := sha256.Sum256(value)
	return fmt.Sprintf("sha256:%x", digest[:])
}

func digestJSON(value any) string {
	raw, _ := json.Marshal(value)
	return digestBytes(raw)
}

func timePointer(value time.Time) *time.Time { return &value }

func truncateText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}
