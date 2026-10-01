package tarsserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/proofverifier"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/workscheduler"
	"github.com/devlikebear/tars/internal/workstore"
)

type taskVerificationRequest struct {
	TaskID    string `json:"task_id,omitempty"`
	TimeoutMS int    `json:"timeout_ms,omitempty"`
}

type taskVerificationResult struct {
	Command     string `json:"command"`
	Status      string `json:"status"`
	ExitCode    int    `json:"exit_code"`
	TimedOut    bool   `json:"timed_out,omitempty"`
	EvidenceID  string `json:"evidence_id"`
	Summary     string `json:"summary,omitempty"`
	ProofState  string `json:"proof_state"`
	ProofOrigin string `json:"proof_origin"`
	VerifierID  string `json:"verifier_id"`
	// Output is the captured output (stderr, then stdout); not part of the
	// HTTP response, which keeps it in Summary.
	Output string `json:"-"`
}

type taskVerificationExecResponse struct {
	Command    string `json:"command"`
	ExitCode   int    `json:"exit_code"`
	Stdout     string `json:"stdout_excerpt,omitempty"`
	Stderr     string `json:"stderr_excerpt,omitempty"`
	DurationMS int64  `json:"duration_ms"`
	TimedOut   bool   `json:"timed_out,omitempty"`
	Message    string `json:"message,omitempty"`
}

func handleSessionTaskVerification(w http.ResponseWriter, r *http.Request, store *session.Store, sessionID string) {
	if _, err := store.Get(sessionID); err != nil {
		if strings.Contains(err.Error(), "session not found") {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "get session failed"})
		return
	}
	var req taskVerificationRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	run, err := verifySessionTask(r.Context(), store, sessionID, req.TaskID, nil, time.Duration(req.TimeoutMS)*time.Millisecond, nil)
	if err != nil {
		writeJSON(w, verificationErrorStatus(err), map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       run.allPassed,
		"task_id":  run.taskID,
		"results":  run.results,
		"plan":     run.tasks.Plan,
		"contract": run.tasks.Contract,
		"summary":  session.TaskSummary(run.tasks.Tasks),
	})
}

// verificationError is a verification that could not run, with the HTTP
// status the handler answers.
type verificationError struct {
	status int
	msg    string
}

func (e *verificationError) Error() string { return e.msg }

func verificationErrorStatus(err error) int {
	var verr *verificationError
	if errors.As(err, &verr) {
		return verr.status
	}
	return http.StatusInternalServerError
}

func badVerification(msg string) error {
	return &verificationError{status: http.StatusBadRequest, msg: msg}
}

// runTaskVerification runs a session's approved contract verification
// commands against one task (taskID, or the in-progress/first pending task
// when empty), records each command's evidence on that task, and reports
// whether all of them passed.
func runTaskVerification(ctx context.Context, store *session.Store, sessionID, taskID string, timeout time.Duration) ([]taskVerificationResult, bool, error) {
	run, err := verifySessionTask(ctx, store, sessionID, taskID, nil, timeout, nil)
	if err != nil {
		return nil, false, err
	}
	return run.results, run.allPassed, nil
}

// runTaskVerificationCommands is runTaskVerification for an explicit
// subset of commands: the focus build loop runs the plan's verify commands
// and keeps the end-to-end ones for review. The contract must still be
// approved; the commands go through the same verifier and evidence path.
// excerpt, when set, makes the output excerpts (proofverifier.Options.Excerpt);
// nil keeps the verifier's default.
func runTaskVerificationCommands(ctx context.Context, store *session.Store, sessionID, taskID string, commands []string, timeout time.Duration, excerpt func(string, int) string) ([]taskVerificationResult, bool, error) {
	if commands == nil {
		commands = []string{}
	}
	run, err := verifySessionTask(ctx, store, sessionID, taskID, commands, timeout, excerpt)
	if err != nil {
		return nil, false, err
	}
	return run.results, run.allPassed, nil
}

type taskVerificationRun struct {
	taskID    string
	results   []taskVerificationResult
	allPassed bool
	tasks     session.SessionTasks
}

// verifySessionTask runs commands (the contract's when nil) and saves their
// evidence.
func verifySessionTask(ctx context.Context, store *session.Store, sessionID, taskID string, commands []string, timeout time.Duration, excerpt func(string, int) string) (taskVerificationRun, error) {
	if _, err := store.Get(sessionID); err != nil {
		if strings.Contains(err.Error(), "session not found") {
			return taskVerificationRun{}, &verificationError{status: http.StatusNotFound, msg: "session not found"}
		}
		return taskVerificationRun{}, &verificationError{status: http.StatusInternalServerError, msg: "get session failed"}
	}
	st, err := store.GetTasks(sessionID)
	if err != nil {
		return taskVerificationRun{}, err
	}
	if st.Contract == nil {
		return taskVerificationRun{}, badVerification("approved task contract is required before running verification")
	}
	if !strings.EqualFold(strings.TrimSpace(st.Contract.Status), session.ContractStatusApproved) {
		return taskVerificationRun{}, badVerification("task contract must be approved before running verification")
	}
	if commands == nil {
		commands = st.Contract.VerificationCommands
		if len(commands) == 0 {
			return taskVerificationRun{}, badVerification("task contract has no verification commands")
		}
	}
	taskIndex, err := selectVerificationTaskIndex(st.Tasks, taskID)
	if err != nil {
		return taskVerificationRun{}, badVerification(err.Error())
	}
	workDir := store.WorkspaceDir()
	if currentDir, err := store.GetCurrentDir(sessionID); err == nil && strings.TrimSpace(currentDir) != "" {
		workDir = strings.TrimSpace(currentDir)
	}
	verifier, err := proofverifier.New(proofverifier.Options{
		ID: "session-proof-verifier", RootDir: workDir, Timeout: timeout, Excerpt: excerpt,
	})
	if err != nil {
		return taskVerificationRun{}, err
	}
	identity := verifier.Identity()
	reporterID := "session-task:" + st.Tasks[taskIndex].ID
	proofPolicy := workstore.StepProofPolicy{Required: true, FailureState: workstore.WorkStateReview}
	if st.Contract.ProofPolicy != nil {
		proofPolicy.Required = st.Contract.ProofPolicy.Required
		proofPolicy.FailureState = workstore.WorkState(st.Contract.ProofPolicy.FailureState)
		proofPolicy.AllowLLMFallback = st.Contract.ProofPolicy.AllowLLMFallback
		proofPolicy.MaxLLMTokens = st.Contract.ProofPolicy.MaxLLMTokens
		proofPolicy.MaxLLMCostUSD = st.Contract.ProofPolicy.MaxLLMCostUSD
	}
	results := make([]taskVerificationResult, 0, len(commands))
	allPassed := true
	for _, command := range commands {
		command = strings.TrimSpace(command)
		if command == "" {
			continue
		}
		verified, verifyErr := verifier.Verify(ctx, workscheduler.VerificationRequest{
			Execution: workscheduler.Execution{
				Work:  workstore.Work{Objective: st.Contract.Goal},
				Claim: workstore.StepClaim{Schedule: workstore.StepSchedule{Policy: workstore.StepSchedulePolicy{Proof: proofPolicy}}},
			},
			Result: workscheduler.ExecutionResult{Succeeded: true},
			Requirement: workstore.ProofRequirement{
				Kind: session.EvidenceTypeTestResult, Verifier: verifier.Name(), Command: command,
			},
		})
		if verifyErr != nil {
			return taskVerificationRun{}, verifyErr
		}
		parsed := parseVerificationProofInput(command, verified.InputJSON)
		status := string(verified.Status)
		if verified.Status != workstore.ProofStatusPassed {
			allPassed = false
		}
		observedAt := session.NowRFC3339()
		if verified.ObservedAt != nil {
			observedAt = verified.ObservedAt.UTC().Format(time.RFC3339Nano)
		}
		summary := summarizeVerificationExec(parsed)
		if strings.TrimSpace(verified.Rationale) != "" {
			if summary == "" {
				summary = verified.Rationale
			} else {
				summary = verified.Rationale + " | " + summary
			}
		}
		ev := session.TaskEvidence{
			ID: session.NextEvidenceID(st.Tasks), Type: session.EvidenceTypeTestResult,
			Title: "Verification: " + command, Summary: summary, Command: command, Status: status,
			ProofState: status, ProofOrigin: string(workstore.ProofOriginIndependentVerifier),
			ReporterID: reporterID, VerifierID: identity.ID, Verifier: verifier.Name(),
			EnvironmentJSON: identity.EnvironmentJSON, InputJSON: verified.InputJSON,
			ArtifactDigestsJSON: verified.ArtifactDigestsJSON,
			SubjectDigest:       verified.SubjectDigest, Rationale: verified.Rationale,
			ObservedAt: observedAt, CreatedAt: session.NowRFC3339(), UpdatedAt: session.NowRFC3339(),
		}
		st.Tasks[taskIndex].Evidence = append(st.Tasks[taskIndex].Evidence, ev)
		results = append(results, taskVerificationResult{
			Command:    command,
			Status:     status,
			ExitCode:   parsed.ExitCode,
			TimedOut:   parsed.TimedOut,
			EvidenceID: ev.ID,
			Summary:    ev.Summary,
			ProofState: ev.ProofState, ProofOrigin: ev.ProofOrigin, VerifierID: ev.VerifierID,
			Output: verificationOutput(parsed),
		})
	}
	if len(results) == 0 {
		return taskVerificationRun{}, badVerification("task contract has no runnable verification commands")
	}
	if err := store.SaveTasks(sessionID, st); err != nil {
		return taskVerificationRun{}, err
	}
	return taskVerificationRun{taskID: st.Tasks[taskIndex].ID, results: results, allPassed: allPassed, tasks: st}, nil
}

// verificationOutput is a command's captured output, stderr first: what a
// failure card and the next fix turn quote.
func verificationOutput(result taskVerificationExecResponse) string {
	parts := make([]string, 0, 3)
	for _, text := range []string{result.Stderr, result.Stdout, result.Message} {
		if text = strings.TrimSpace(text); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

func selectVerificationTaskIndex(tasks []session.Task, requestedTaskID string) (int, error) {
	if len(tasks) == 0 {
		return -1, fmt.Errorf("at least one task is required before running verification")
	}
	requestedTaskID = strings.TrimSpace(requestedTaskID)
	if requestedTaskID != "" {
		for i := range tasks {
			if tasks[i].ID == requestedTaskID {
				return i, nil
			}
		}
		return -1, fmt.Errorf("task %q not found", requestedTaskID)
	}
	for i := range tasks {
		if strings.EqualFold(strings.TrimSpace(tasks[i].Status), "in_progress") {
			return i, nil
		}
	}
	for i := range tasks {
		if strings.EqualFold(strings.TrimSpace(tasks[i].Status), "pending") {
			return i, nil
		}
	}
	return 0, nil
}

func parseVerificationProofInput(command string, raw json.RawMessage) taskVerificationExecResponse {
	parsed := taskVerificationExecResponse{Command: command}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		parsed.Message = "verification provenance was unavailable"
	}
	if strings.TrimSpace(parsed.Command) == "" {
		parsed.Command = command
	}
	return parsed
}

func summarizeVerificationExec(result taskVerificationExecResponse) string {
	parts := []string{fmt.Sprintf("exit_code=%d", result.ExitCode)}
	if result.TimedOut {
		parts = append(parts, "timed_out=true")
	}
	if result.DurationMS > 0 {
		parts = append(parts, fmt.Sprintf("duration_ms=%d", result.DurationMS))
	}
	if text := strings.TrimSpace(result.Stdout); text != "" {
		parts = append(parts, "stdout: "+text)
	}
	if text := strings.TrimSpace(result.Stderr); text != "" {
		parts = append(parts, "stderr: "+text)
	}
	if text := strings.TrimSpace(result.Message); text != "" {
		parts = append(parts, "message: "+text)
	}
	return strings.Join(parts, " | ")
}
