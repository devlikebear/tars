package tarsserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/tool"
)

type taskVerificationRequest struct {
	TaskID    string `json:"task_id,omitempty"`
	TimeoutMS int    `json:"timeout_ms,omitempty"`
}

type taskVerificationResult struct {
	Command    string `json:"command"`
	Status     string `json:"status"`
	ExitCode   int    `json:"exit_code"`
	TimedOut   bool   `json:"timed_out,omitempty"`
	EvidenceID string `json:"evidence_id"`
	Summary    string `json:"summary,omitempty"`
}

type taskVerificationExecResponse struct {
	Command    string `json:"command"`
	ExitCode   int    `json:"exit_code"`
	Stdout     string `json:"stdout,omitempty"`
	Stderr     string `json:"stderr,omitempty"`
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
	st, err := store.GetTasks(sessionID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if st.Contract == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "approved task contract is required before running verification"})
		return
	}
	if !strings.EqualFold(strings.TrimSpace(st.Contract.Status), session.ContractStatusApproved) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "task contract must be approved before running verification"})
		return
	}
	if len(st.Contract.VerificationCommands) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "task contract has no verification commands"})
		return
	}
	taskIndex, err := selectVerificationTaskIndex(st.Tasks, req.TaskID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	workDir := store.WorkspaceDir()
	if currentDir, err := store.GetCurrentDir(sessionID); err == nil && strings.TrimSpace(currentDir) != "" {
		workDir = strings.TrimSpace(currentDir)
	}
	execTool := tool.NewExecTool(workDir)
	results := make([]taskVerificationResult, 0, len(st.Contract.VerificationCommands))
	allPassed := true
	for _, command := range st.Contract.VerificationCommands {
		command = strings.TrimSpace(command)
		if command == "" {
			continue
		}
		params := map[string]any{"command": command}
		if req.TimeoutMS > 0 {
			params["timeout_ms"] = req.TimeoutMS
		}
		rawParams, _ := json.Marshal(params)
		execResult, execErr := execTool.Execute(r.Context(), rawParams)
		if execErr != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": execErr.Error()})
			return
		}
		parsed := parseVerificationExecResponse(command, execResult)
		status := "passed"
		if execResult.IsError {
			status = "failed"
			allPassed = false
		}
		ev := session.TaskEvidence{
			ID:        session.NextEvidenceID(st.Tasks),
			Type:      session.EvidenceTypeTestResult,
			Title:     "Verification: " + command,
			Summary:   summarizeVerificationExec(parsed),
			Command:   command,
			Status:    status,
			CreatedAt: session.NowRFC3339(),
		}
		st.Tasks[taskIndex].Evidence = append(st.Tasks[taskIndex].Evidence, ev)
		results = append(results, taskVerificationResult{
			Command:    command,
			Status:     status,
			ExitCode:   parsed.ExitCode,
			TimedOut:   parsed.TimedOut,
			EvidenceID: ev.ID,
			Summary:    ev.Summary,
		})
	}
	if len(results) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "task contract has no runnable verification commands"})
		return
	}
	if err := store.SaveTasks(sessionID, st); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       allPassed,
		"task_id":  st.Tasks[taskIndex].ID,
		"results":  results,
		"plan":     st.Plan,
		"contract": st.Contract,
		"summary":  session.TaskSummary(st.Tasks),
	})
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

func parseVerificationExecResponse(command string, result tool.Result) taskVerificationExecResponse {
	parsed := taskVerificationExecResponse{Command: command}
	if err := json.Unmarshal([]byte(result.Text()), &parsed); err != nil {
		parsed.Message = strings.TrimSpace(result.Text())
		if result.IsError {
			parsed.ExitCode = -1
		}
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
