package tarsserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/mcp"
	"github.com/rs/zerolog"
)

const mcpDirPlaceholder = "${MCP_DIR}"
const mcpServerCreatorLLMSystemPrompt = "Create a runnable TARS MCP server draft. Return only a json object with name, description, language, use_case, tools, and assistant_message. language must be python or node. tools must be a concise array of MCP tool signatures with snake_case names, descriptions, and optional JSON-schema input_schema/output_schema. Do not include secrets or network-only assumptions."

type mcpServerCreatorSubmitter func(context.Context, mcpServerCreatorSubmitRequest) (mcpServerCreatorSubmitResponse, error)

type mcpServerCreatorDraftRequest struct {
	Prompt       string                                `json:"prompt,omitempty"`
	Conversation []mcpServerCreatorConversationMessage `json:"conversation,omitempty"`
	UseLLM       *bool                                 `json:"use_llm,omitempty"`
	Name         string                                `json:"name"`
	Description  string                                `json:"description"`
	Language     string                                `json:"language"`
	UseCase      string                                `json:"use_case"`
	Tools        []mcpServerCreatorToolSpec            `json:"tools,omitempty"`
}

type mcpServerCreatorConversationMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type mcpServerCreatorToolSpec struct {
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	InputSchema  map[string]any `json:"input_schema,omitempty"`
	OutputSchema map[string]any `json:"output_schema,omitempty"`
}

type mcpServerCreatorFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type mcpServerCreatorDraftResponse struct {
	Name             string                     `json:"name"`
	Description      string                     `json:"description"`
	Language         string                     `json:"language"`
	UseCase          string                     `json:"use_case"`
	Tools            []mcpServerCreatorToolSpec `json:"tools"`
	Files            []mcpServerCreatorFile     `json:"files"`
	DraftSource      string                     `json:"draft_source,omitempty"`
	AssistantMessage string                     `json:"assistant_message,omitempty"`
	Warnings         []string                   `json:"warnings,omitempty"`
}

type mcpServerCreatorSaveResponse struct {
	Saved bool     `json:"saved"`
	Path  string   `json:"path"`
	Files []string `json:"files"`
}

type mcpServerCreatorTestResponse struct {
	Success       bool                        `json:"success"`
	ExitCode      int                         `json:"exit_code"`
	Stdout        string                      `json:"stdout,omitempty"`
	Stderr        string                      `json:"stderr,omitempty"`
	Tools         []string                    `json:"tools"`
	CallResult    string                      `json:"call_result,omitempty"`
	ProtocolSteps []string                    `json:"protocol_steps"`
	SandboxPath   string                      `json:"sandbox_path"`
	SessionKind   string                      `json:"session_kind"`
	Hidden        bool                        `json:"hidden"`
	DurationMS    int64                       `json:"duration_ms"`
	ToolTrail     []mcpServerCreatorToolTrail `json:"tool_trail"`
}

type mcpServerCreatorToolTrail struct {
	Tool       string `json:"tool"`
	Command    string `json:"command"`
	Cwd        string `json:"cwd"`
	Status     string `json:"status"`
	ExitCode   int    `json:"exit_code"`
	DurationMS int64  `json:"duration_ms"`
}

type mcpServerCreatorSubmitRequest struct {
	Name     string `json:"name"`
	RepoPath string `json:"repo_path,omitempty"`
	Branch   string `json:"branch,omitempty"`
	Confirm  bool   `json:"confirm"`
}

type mcpServerCreatorSubmitResponse struct {
	Submitted bool     `json:"submitted"`
	Ready     bool     `json:"ready"`
	Message   string   `json:"message"`
	Commands  []string `json:"commands,omitempty"`
}

type mcpServerCreatorManifest struct {
	SchemaVersion int              `json:"schema_version,omitempty"`
	Server        config.MCPServer `json:"server"`
}

type mcpServerCreatorLLMDraft struct {
	Name             string                     `json:"name"`
	Description      string                     `json:"description"`
	Language         string                     `json:"language"`
	UseCase          string                     `json:"use_case"`
	Tools            []mcpServerCreatorToolSpec `json:"tools,omitempty"`
	AssistantMessage string                     `json:"assistant_message,omitempty"`
}

func newMCPServerCreatorAPIHandler(workspaceDir string, logger zerolog.Logger, submitter mcpServerCreatorSubmitter, routers ...llm.Router) http.Handler {
	var router llm.Router
	if len(routers) > 0 {
		router = routers[0]
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/admin/mcp-servers/draft", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		var req mcpServerCreatorDraftRequest
		if !decodeJSONBody(w, r, &req) {
			return
		}
		draft, err := buildMCPServerCreatorDraft(r.Context(), router, req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, draft)
	})
	mux.HandleFunc("/v1/admin/mcp-servers/save-local", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		var draft mcpServerCreatorDraftResponse
		if !decodeJSONBody(w, r, &draft) {
			return
		}
		saved, err := saveMCPServerCreatorDraft(workspaceDir, draft)
		if err != nil {
			logger.Error().Err(err).Str("mcp_server", draft.Name).Msg("save mcp server creator draft failed")
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, saved)
	})
	mux.HandleFunc("/v1/admin/mcp-servers/test", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		var draft mcpServerCreatorDraftResponse
		if !decodeJSONBody(w, r, &draft) {
			return
		}
		result, err := testMCPServerCreatorDraft(r.Context(), workspaceDir, draft)
		if err != nil {
			logger.Error().Err(err).Str("mcp_server", draft.Name).Msg("test mcp server creator draft failed")
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
	mux.HandleFunc("/v1/admin/mcp-servers/submit-pr", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodPost) {
			return
		}
		var req mcpServerCreatorSubmitRequest
		if !decodeJSONBody(w, r, &req) {
			return
		}
		if err := validateSkillCreatorName(req.Name); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if submitter != nil {
			resp, err := submitter(r.Context(), req)
			if err != nil {
				logger.Error().Err(err).Str("mcp_server", req.Name).Msg("submit mcp server draft PR failed")
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, resp)
			return
		}
		resp := mcpServerCreatorSubmitResponse{
			Ready:   true,
			Message: "Local MCP server draft is ready. Draft PR submission requires a configured tars-skills checkout and explicit confirmation.",
			Commands: []string{
				fmt.Sprintf("cp -R %s %s", shellQuote(filepath.ToSlash(filepath.Join(workspaceDir, "mcp-servers", req.Name))), shellQuote("tars-skills/mcp-servers/")),
				fmt.Sprintf("gh pr create --draft --title %q", "Add "+req.Name+" MCP server"),
			},
		}
		writeJSON(w, http.StatusOK, resp)
	})
	return mux
}

func buildMCPServerCreatorDraft(ctx context.Context, router llm.Router, req mcpServerCreatorDraftRequest) (mcpServerCreatorDraftResponse, error) {
	resolved, source, assistant, warnings, err := resolveMCPServerCreatorDraftRequest(ctx, router, req)
	if err != nil {
		return mcpServerCreatorDraftResponse{}, err
	}
	draft, err := buildMCPServerCreatorDraftFromResolved(resolved)
	if err != nil {
		return mcpServerCreatorDraftResponse{}, err
	}
	draft.DraftSource = source
	draft.AssistantMessage = assistant
	draft.Warnings = append(draft.Warnings, warnings...)
	return draft, nil
}

func buildMCPServerCreatorDraftFromResolved(req mcpServerCreatorDraftRequest) (mcpServerCreatorDraftResponse, error) {
	name := strings.TrimSpace(req.Name)
	if err := validateSkillCreatorName(name); err != nil {
		return mcpServerCreatorDraftResponse{}, err
	}
	description := strings.TrimSpace(req.Description)
	if description == "" {
		return mcpServerCreatorDraftResponse{}, fmt.Errorf("description is required")
	}
	useCase := strings.TrimSpace(req.UseCase)
	if useCase == "" {
		return mcpServerCreatorDraftResponse{}, fmt.Errorf("use_case is required")
	}
	language, err := normalizeMCPServerCreatorLanguage(req.Language)
	if err != nil {
		return mcpServerCreatorDraftResponse{}, err
	}
	tools, warnings := normalizeMCPServerCreatorTools(req.Tools, name, useCase)
	files, err := renderMCPServerCreatorFiles(name, description, language, useCase, tools)
	if err != nil {
		return mcpServerCreatorDraftResponse{}, err
	}
	return mcpServerCreatorDraftResponse{
		Name:        name,
		Description: description,
		Language:    language,
		UseCase:     useCase,
		Tools:       tools,
		Files:       files,
		Warnings:    warnings,
	}, nil
}

func resolveMCPServerCreatorDraftRequest(ctx context.Context, router llm.Router, req mcpServerCreatorDraftRequest) (mcpServerCreatorDraftRequest, string, string, []string, error) {
	prompt := naturalMCPServerCreatorPrompt(req)
	useLLM := req.UseLLM == nil || *req.UseLLM
	if prompt != "" && useLLM && router != nil {
		llmDraft, err := draftMCPServerCreatorWithLLM(ctx, router, req, prompt)
		if err == nil {
			return mergeMCPServerCreatorLLMDraft(req, llmDraft, prompt), "llm", strings.TrimSpace(llmDraft.AssistantMessage), nil, nil
		}
		resolved := heuristicMCPServerCreatorDraftRequest(req, prompt)
		return resolved, "heuristic", "", []string{"LLM draft failed; generated a local draft instead: " + err.Error()}, nil
	}
	if prompt != "" {
		warnings := []string{}
		if useLLM && router == nil {
			warnings = append(warnings, "LLM is not configured; generated a local draft instead.")
		}
		return heuristicMCPServerCreatorDraftRequest(req, prompt), "heuristic", "", warnings, nil
	}
	return req, "template", "", nil, nil
}

func draftMCPServerCreatorWithLLM(ctx context.Context, router llm.Router, req mcpServerCreatorDraftRequest, prompt string) (mcpServerCreatorLLMDraft, error) {
	client, _, err := router.ClientFor(llm.RoleAgentRuntimePlanner)
	if err != nil {
		return mcpServerCreatorLLMDraft{}, err
	}
	payload := map[string]any{
		"request":             prompt,
		"conversation":        normalizeMCPServerCreatorConversation(req.Conversation),
		"name_hint":           strings.TrimSpace(req.Name),
		"description_hint":    strings.TrimSpace(req.Description),
		"language_hint":       strings.TrimSpace(req.Language),
		"use_case_hint":       strings.TrimSpace(req.UseCase),
		"tool_signature_hint": req.Tools,
		"supported_languages": []string{"python", "node"},
		"response_format":     "json",
	}
	raw, _ := json.Marshal(payload)
	messages := []llm.ChatMessage{
		{Role: "system", Content: mcpServerCreatorLLMSystemPrompt},
		{Role: "user", Content: string(raw)},
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	resp, err := client.Chat(ctx, messages, llm.ChatOptions{
		ResponseFormat: &llm.ResponseFormat{Type: llm.ResponseFormatJSONObject},
	})
	if err != nil {
		return mcpServerCreatorLLMDraft{}, err
	}
	var draft mcpServerCreatorLLMDraft
	if err := json.Unmarshal([]byte(strings.TrimSpace(resp.Message.Content)), &draft); err != nil {
		return mcpServerCreatorLLMDraft{}, fmt.Errorf("decode llm draft: %w", err)
	}
	return draft, nil
}

func mergeMCPServerCreatorLLMDraft(req mcpServerCreatorDraftRequest, llmDraft mcpServerCreatorLLMDraft, prompt string) mcpServerCreatorDraftRequest {
	out := req
	out.Prompt = prompt
	if strings.TrimSpace(llmDraft.Name) != "" {
		out.Name = strings.TrimSpace(llmDraft.Name)
		if err := validateSkillCreatorName(out.Name); err != nil {
			out.Name = deriveMCPServerCreatorName(out.Name + " " + prompt)
		}
	}
	if strings.TrimSpace(llmDraft.Description) != "" {
		out.Description = strings.TrimSpace(llmDraft.Description)
	}
	if strings.TrimSpace(llmDraft.Language) != "" {
		out.Language = strings.TrimSpace(llmDraft.Language)
	}
	if strings.TrimSpace(llmDraft.UseCase) != "" {
		out.UseCase = strings.TrimSpace(llmDraft.UseCase)
	}
	if len(llmDraft.Tools) > 0 {
		out.Tools = llmDraft.Tools
	}
	return heuristicMCPServerCreatorDraftRequest(out, prompt)
}

func heuristicMCPServerCreatorDraftRequest(req mcpServerCreatorDraftRequest, prompt string) mcpServerCreatorDraftRequest {
	out := req
	if strings.TrimSpace(out.Name) == "" {
		out.Name = deriveMCPServerCreatorName(prompt)
	}
	if strings.TrimSpace(out.Description) == "" {
		out.Description = deriveMCPServerCreatorDescription(prompt)
	}
	if strings.TrimSpace(out.UseCase) == "" {
		out.UseCase = prompt
	}
	if strings.TrimSpace(out.Language) == "" {
		out.Language = "python"
	}
	return out
}

func naturalMCPServerCreatorPrompt(req mcpServerCreatorDraftRequest) string {
	if prompt := strings.TrimSpace(req.Prompt); prompt != "" {
		return prompt
	}
	parts := make([]string, 0, len(req.Conversation))
	for _, msg := range normalizeMCPServerCreatorConversation(req.Conversation) {
		parts = append(parts, msg.Role+": "+msg.Content)
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func normalizeMCPServerCreatorConversation(messages []mcpServerCreatorConversationMessage) []mcpServerCreatorConversationMessage {
	out := make([]mcpServerCreatorConversationMessage, 0, len(messages))
	for _, msg := range messages {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		if role != "user" && role != "assistant" {
			continue
		}
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}
		out = append(out, mcpServerCreatorConversationMessage{Role: role, Content: content})
	}
	return out
}

func deriveMCPServerCreatorName(prompt string) string {
	lower := strings.ToLower(strings.TrimSpace(prompt))
	var b strings.Builder
	lastDash := false
	for _, r := range lower {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_' || r == ' ' || r == '\t' || r == '\n':
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
		if b.Len() >= 48 {
			break
		}
	}
	name := strings.Trim(b.String(), "-")
	if name == "" {
		name = "custom-mcp-server"
	}
	if !strings.Contains(name, "mcp") && len(name) <= 44 {
		name += "-mcp"
	}
	name = strings.Trim(name, "-")
	if err := validateSkillCreatorName(name); err != nil {
		return "custom-mcp-server"
	}
	return name
}

func deriveMCPServerCreatorDescription(prompt string) string {
	fragment := sentenceFragment(prompt)
	fragment = truncateRunes(fragment, 140)
	if fragment == "" {
		return "Generated MCP server."
	}
	return "Generated MCP server for " + fragment + "."
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(string(runes[:limit]))
}

func saveMCPServerCreatorDraft(workspaceDir string, draft mcpServerCreatorDraftResponse) (mcpServerCreatorSaveResponse, error) {
	if strings.TrimSpace(workspaceDir) == "" {
		return mcpServerCreatorSaveResponse{}, fmt.Errorf("workspace directory is required")
	}
	cleanFiles, err := cleanMCPServerCreatorDraftFiles(draft)
	if err != nil {
		return mcpServerCreatorSaveResponse{}, err
	}
	targetDir := filepath.Join(workspaceDir, "mcp-servers", draft.Name)
	saved, err := writeMCPServerCreatorDraftFiles(targetDir, cleanFiles)
	if err != nil {
		return mcpServerCreatorSaveResponse{}, err
	}
	return mcpServerCreatorSaveResponse{Saved: true, Path: targetDir, Files: saved}, nil
}

func testMCPServerCreatorDraft(ctx context.Context, workspaceDir string, draft mcpServerCreatorDraftResponse) (mcpServerCreatorTestResponse, error) {
	if strings.TrimSpace(workspaceDir) == "" {
		return mcpServerCreatorTestResponse{}, fmt.Errorf("workspace directory is required")
	}
	start := time.Now()
	cleanFiles, err := cleanMCPServerCreatorDraftFiles(draft)
	if err != nil {
		return mcpServerCreatorTestResponse{}, err
	}
	baseDir := filepath.Join(workspaceDir, "tmp", "mcp-server-tests")
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return mcpServerCreatorTestResponse{}, fmt.Errorf("create mcp server test directory: %w", err)
	}
	sandboxPath, err := os.MkdirTemp(baseDir, draft.Name+"-")
	if err != nil {
		return mcpServerCreatorTestResponse{}, fmt.Errorf("create mcp server test sandbox: %w", err)
	}
	targetDir := filepath.Join(sandboxPath, "mcp-servers", draft.Name)
	if _, err := writeMCPServerCreatorDraftFiles(targetDir, cleanFiles); err != nil {
		return mcpServerCreatorTestResponse{}, err
	}
	toolTrail := []mcpServerCreatorToolTrail{}
	var stdout string
	var stderr string
	setupCtx, setupCancel := context.WithTimeout(ctx, 60*time.Second)
	setupTrail, setupStdout, setupStderr, setupErr := setupMCPServerCreatorSandboxDependencies(setupCtx, targetDir, cleanFiles)
	setupCancel()
	toolTrail = append(toolTrail, setupTrail...)
	stdout = setupStdout
	stderr = setupStderr
	if setupErr != nil {
		duration := time.Since(start)
		return mcpServerCreatorTestResponse{
			Success:     false,
			ExitCode:    1,
			Stdout:      stdout,
			Stderr:      setupErr.Error() + stderrSuffix(stderr),
			SandboxPath: sandboxPath,
			SessionKind: "worker",
			Hidden:      true,
			DurationMS:  duration.Milliseconds(),
			ToolTrail:   toolTrail,
		}, nil
	}
	server, err := loadMCPServerCreatorManifest(targetDir, cleanFiles, draft.Name)
	if err != nil {
		return mcpServerCreatorTestResponse{}, err
	}

	runCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	client := mcp.NewClient([]config.MCPServer{server})
	client.SetCommandAllowlist([]string{server.Command})
	defer client.Close()

	status := "pass"
	exitCode := 0
	tools, listErr := client.ListTools(runCtx)
	if listErr != nil {
		status = "fail"
		exitCode = 1
		if runCtx.Err() == context.DeadlineExceeded {
			status = "timeout"
			exitCode = -1
		}
		stderr = strings.TrimSpace(strings.Join(nonEmptyStrings(stderr, listErr.Error()), "\n"))
	}
	toolNames := make([]string, 0, len(tools))
	for _, tool := range tools {
		toolNames = append(toolNames, tool.Name)
	}

	callResult := ""
	protocolSteps := []string{"tools/list"}
	if listErr == nil {
		var callErr error
		protocolSteps = append(protocolSteps, "tools/call")
		callResult, callErr = callFirstMCPServerCreatorTool(runCtx, client, server.Name, tools, draft)
		if callErr != nil {
			status = "fail"
			exitCode = 1
			if runCtx.Err() == context.DeadlineExceeded {
				status = "timeout"
				exitCode = -1
			}
			stderr = strings.TrimSpace(strings.Join(nonEmptyStrings(stderr, callErr.Error()), "\n"))
		}
	}
	duration := time.Since(start)
	success := status == "pass"
	toolTrail = append(toolTrail, mcpServerCreatorToolTrail{
		Tool:       "mcp_stdio",
		Command:    mcpServerCreatorCommandString(server),
		Cwd:        targetDir,
		Status:     status,
		ExitCode:   exitCode,
		DurationMS: duration.Milliseconds(),
	})
	return mcpServerCreatorTestResponse{
		Success:       success,
		ExitCode:      exitCode,
		Stdout:        stdout,
		Stderr:        stderr,
		Tools:         toolNames,
		CallResult:    callResult,
		ProtocolSteps: protocolSteps,
		SandboxPath:   sandboxPath,
		SessionKind:   "worker",
		Hidden:        true,
		DurationMS:    duration.Milliseconds(),
		ToolTrail:     toolTrail,
	}, nil
}

func setupMCPServerCreatorSandboxDependencies(ctx context.Context, targetDir string, files []mcpServerCreatorFile) ([]mcpServerCreatorToolTrail, string, string, error) {
	if !mcpServerCreatorHasFile(files, "package.json") {
		return nil, "", "", nil
	}
	start := time.Now()
	cmd := exec.CommandContext(ctx, "npm", "install", "--no-audit", "--no-fund", "--package-lock=false", "--loglevel=error")
	cmd.Dir = targetDir
	cmd.Env = append(os.Environ(), "npm_config_update_notifier=false")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	status := "pass"
	exitCode := 0
	if err != nil {
		status = "fail"
		exitCode = 1
		if ctx.Err() == context.DeadlineExceeded {
			status = "timeout"
			exitCode = -1
		} else if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}
	trail := []mcpServerCreatorToolTrail{
		{
			Tool:       "npm_install",
			Command:    "npm install --no-audit --no-fund --package-lock=false --loglevel=error",
			Cwd:        targetDir,
			Status:     status,
			ExitCode:   exitCode,
			DurationMS: time.Since(start).Milliseconds(),
		},
	}
	if err != nil {
		return trail, stdout.String(), stderr.String(), fmt.Errorf("npm install failed: %w", err)
	}
	return trail, stdout.String(), stderr.String(), nil
}

func mcpServerCreatorHasFile(files []mcpServerCreatorFile, path string) bool {
	for _, file := range files {
		if file.Path == path {
			return true
		}
	}
	return false
}

func stderrSuffix(stderr string) string {
	stderr = strings.TrimSpace(stderr)
	if stderr == "" {
		return ""
	}
	return "\n" + stderr
}

func nonEmptyStrings(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func cleanMCPServerCreatorDraftFiles(draft mcpServerCreatorDraftResponse) ([]mcpServerCreatorFile, error) {
	if err := validateSkillCreatorName(draft.Name); err != nil {
		return nil, err
	}
	if len(draft.Files) == 0 {
		return nil, fmt.Errorf("at least one file is required")
	}
	cleanFiles := make([]mcpServerCreatorFile, 0, len(draft.Files))
	hasManifest := false
	for _, file := range draft.Files {
		rel, err := cleanSkillCreatorFilePath(file.Path)
		if err != nil {
			return nil, err
		}
		if rel == "tars.mcp.json" {
			hasManifest = true
		}
		cleanFiles = append(cleanFiles, mcpServerCreatorFile{Path: rel, Content: file.Content})
	}
	if !hasManifest {
		return nil, fmt.Errorf("tars.mcp.json is required")
	}
	return cleanFiles, nil
}

func writeMCPServerCreatorDraftFiles(targetDir string, cleanFiles []mcpServerCreatorFile) ([]string, error) {
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return nil, fmt.Errorf("create mcp server directory: %w", err)
	}
	saved := make([]string, 0, len(cleanFiles))
	for _, file := range cleanFiles {
		rel := file.Path
		target := filepath.Join(targetDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return nil, fmt.Errorf("create parent directory for %s: %w", rel, err)
		}
		mode := os.FileMode(0o644)
		if isMCPServerCreatorExecutable(rel) {
			mode = 0o755
		}
		if err := os.WriteFile(target, []byte(file.Content), mode); err != nil {
			return nil, fmt.Errorf("write %s: %w", rel, err)
		}
		saved = append(saved, rel)
	}
	sort.Strings(saved)
	return saved, nil
}

func loadMCPServerCreatorManifest(targetDir string, files []mcpServerCreatorFile, fallbackName string) (config.MCPServer, error) {
	var manifestFile *mcpServerCreatorFile
	for i := range files {
		if files[i].Path == "tars.mcp.json" {
			manifestFile = &files[i]
			break
		}
	}
	if manifestFile == nil {
		return config.MCPServer{}, fmt.Errorf("tars.mcp.json is required")
	}
	var manifest mcpServerCreatorManifest
	if err := json.Unmarshal([]byte(manifestFile.Content), &manifest); err != nil {
		return config.MCPServer{}, fmt.Errorf("decode tars.mcp.json: %w", err)
	}
	if manifest.SchemaVersion == 0 {
		manifest.SchemaVersion = 1
	}
	if manifest.SchemaVersion != 1 {
		return config.MCPServer{}, fmt.Errorf("unsupported mcp manifest schema_version %d", manifest.SchemaVersion)
	}
	server := config.NormalizeMCPServer(manifest.Server)
	if strings.TrimSpace(server.Name) == "" {
		server.Name = fallbackName
	}
	if strings.TrimSpace(server.Command) == "" {
		return config.MCPServer{}, fmt.Errorf("mcp server command is required")
	}
	server.Source = "draft"
	server.Command = strings.ReplaceAll(server.Command, mcpDirPlaceholder, targetDir)
	for i, arg := range server.Args {
		server.Args[i] = strings.ReplaceAll(arg, mcpDirPlaceholder, targetDir)
	}
	for key, value := range server.Env {
		server.Env[key] = strings.ReplaceAll(value, mcpDirPlaceholder, targetDir)
	}
	return server, nil
}

func callFirstMCPServerCreatorTool(ctx context.Context, client *mcp.Client, serverName string, infos []mcp.ToolInfo, draft mcpServerCreatorDraftResponse) (string, error) {
	if len(infos) == 0 {
		return "", fmt.Errorf("tools/list returned no tools")
	}
	wrappers, err := client.BuildTools(ctx)
	if err != nil {
		return "", err
	}
	targetInfo := infos[0]
	targetName := mcp.MCPToolName(serverName, targetInfo.Name)
	for _, wrapper := range wrappers {
		if wrapper.Name != targetName {
			continue
		}
		args := sampleMCPServerCreatorArgs(targetInfo.Name, draft)
		raw, err := json.Marshal(args)
		if err != nil {
			return "", fmt.Errorf("marshal tool args: %w", err)
		}
		result, err := wrapper.Execute(ctx, raw)
		if err != nil {
			return "", err
		}
		if result.IsError {
			return result.Text(), fmt.Errorf("tool %s returned an error: %s", targetInfo.Name, result.Text())
		}
		return result.Text(), nil
	}
	return "", fmt.Errorf("tool wrapper not found for %s", targetInfo.Name)
}

func sampleMCPServerCreatorArgs(toolName string, draft mcpServerCreatorDraftResponse) map[string]any {
	for _, tool := range draft.Tools {
		if tool.Name == toolName {
			return sampleMCPArgsFromSchema(tool.InputSchema)
		}
	}
	return map[string]any{"request": "hello from TARS MCP Creator"}
}

func sampleMCPArgsFromSchema(schema map[string]any) map[string]any {
	if len(schema) == 0 {
		return map[string]any{"request": "hello from TARS MCP Creator"}
	}
	properties, _ := schema["properties"].(map[string]any)
	required := stringSliceFromAny(schema["required"])
	if len(required) == 0 {
		required = []string{"request"}
	}
	out := map[string]any{}
	for _, name := range required {
		prop, _ := properties[name].(map[string]any)
		out[name] = sampleMCPValueFromSchema(prop)
	}
	if len(out) == 0 {
		out["request"] = "hello from TARS MCP Creator"
	}
	return out
}

func sampleMCPValueFromSchema(schema map[string]any) any {
	t, _ := schema["type"].(string)
	switch t {
	case "integer":
		return 1
	case "number":
		return 1
	case "boolean":
		return true
	case "array":
		return []any{}
	case "object":
		return map[string]any{}
	default:
		return "hello from TARS MCP Creator"
	}
}

func stringSliceFromAny(value any) []string {
	raw, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
			out = append(out, strings.TrimSpace(text))
		}
	}
	return out
}

func normalizeMCPServerCreatorLanguage(language string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "", "python", "py", "fastmcp":
		return "python", nil
	case "node", "javascript", "js", "typescript", "ts":
		return "node", nil
	default:
		return "", fmt.Errorf("language must be python or node")
	}
}

func normalizeMCPServerCreatorTools(explicit []mcpServerCreatorToolSpec, name string, useCase string) ([]mcpServerCreatorToolSpec, []string) {
	warnings := []string{}
	if len(explicit) == 0 {
		return []mcpServerCreatorToolSpec{
			{
				Name:         "run_" + identifierFromKebab(name),
				Description:  "Run " + sentenceFragment(useCase),
				InputSchema:  defaultMCPInputSchema(),
				OutputSchema: defaultMCPOutputSchema(),
			},
		}, warnings
	}
	seen := map[string]bool{}
	tools := make([]mcpServerCreatorToolSpec, 0, len(explicit))
	for _, tool := range explicit {
		name := normalizeMCPToolName(tool.Name)
		if name == "" {
			warnings = append(warnings, "Skipped a tool with an invalid name.")
			continue
		}
		if seen[name] {
			warnings = append(warnings, "Skipped duplicate tool "+name+".")
			continue
		}
		seen[name] = true
		description := strings.TrimSpace(tool.Description)
		if description == "" {
			description = "Run " + name
		}
		input := tool.InputSchema
		if len(input) == 0 {
			input = defaultMCPInputSchema()
		}
		output := tool.OutputSchema
		if len(output) == 0 {
			output = defaultMCPOutputSchema()
		}
		tools = append(tools, mcpServerCreatorToolSpec{Name: name, Description: description, InputSchema: input, OutputSchema: output})
	}
	if len(tools) == 0 {
		tools = []mcpServerCreatorToolSpec{
			{Name: "run_" + identifierFromKebab(name), Description: "Run " + sentenceFragment(useCase), InputSchema: defaultMCPInputSchema(), OutputSchema: defaultMCPOutputSchema()},
		}
	}
	return tools, warnings
}

func defaultMCPInputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"request": map[string]any{
				"type":        "string",
				"description": "Natural-language input for this tool.",
			},
		},
		"required": []any{"request"},
	}
}

func defaultMCPOutputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"result": map[string]any{
				"type":        "string",
				"description": "Tool result.",
			},
		},
		"required": []any{"result"},
	}
}

func renderMCPServerCreatorFiles(name, description, language, useCase string, tools []mcpServerCreatorToolSpec) ([]mcpServerCreatorFile, error) {
	manifestCommand := "python3"
	manifestArgs := []string{mcpDirPlaceholder + "/server.py"}
	if language == "node" {
		manifestCommand = "node"
		manifestArgs = []string{mcpDirPlaceholder + "/server.mjs"}
	}
	manifest := mcpServerCreatorManifest{
		SchemaVersion: 1,
		Server: config.MCPServer{
			Name:      name,
			Transport: config.MCPTransportStdio,
			Command:   manifestCommand,
			Args:      manifestArgs,
		},
	}
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	files := []mcpServerCreatorFile{
		{Path: "README.md", Content: renderMCPServerCreatorReadme(name, description, language, useCase, tools)},
		{Path: "tars.mcp.json", Content: string(manifestData) + "\n"},
	}
	if language == "node" {
		files = append(files,
			mcpServerCreatorFile{Path: "package.json", Content: renderMCPServerCreatorPackageJSON(name, description)},
			mcpServerCreatorFile{Path: "server.mjs", Content: renderMCPServerCreatorNodeServer(name, tools)},
		)
	} else {
		files = append(files,
			mcpServerCreatorFile{Path: "requirements.txt", Content: "mcp[cli]\n"},
			mcpServerCreatorFile{Path: "server.py", Content: renderMCPServerCreatorPythonServer(name, tools)},
		)
	}
	return files, nil
}

func renderMCPServerCreatorReadme(name, description, language, useCase string, tools []mcpServerCreatorToolSpec) string {
	var b strings.Builder
	b.WriteString("# " + titleFromKebab(name) + "\n\n")
	b.WriteString(description + "\n\n")
	b.WriteString("## Runtime\n\n")
	if language == "node" {
		b.WriteString("- Install: `npm install`\n")
		b.WriteString("- Start: `node server.mjs`\n")
	} else {
		b.WriteString("- Install: `python3 -m pip install -r requirements.txt`\n")
		b.WriteString("- Start: `python3 server.py`\n")
	}
	b.WriteString("\n## Use Case\n\n")
	b.WriteString(useCase + "\n\n")
	b.WriteString("## Tools\n\n")
	for _, tool := range tools {
		b.WriteString("- `" + tool.Name + "` — " + tool.Description + "\n")
	}
	return b.String()
}

func renderMCPServerCreatorPackageJSON(name, description string) string {
	data := map[string]any{
		"name":        name,
		"version":     "0.1.0",
		"description": description,
		"type":        "module",
		"scripts": map[string]string{
			"start": "node server.mjs",
		},
		"dependencies": map[string]string{
			"@modelcontextprotocol/sdk": "^1.29.0",
			"zod":                       "^3.25.0",
		},
	}
	encoded, _ := json.MarshalIndent(data, "", "  ")
	return string(encoded) + "\n"
}

func renderMCPServerCreatorPythonServer(name string, tools []mcpServerCreatorToolSpec) string {
	var b strings.Builder
	b.WriteString(`#!/usr/bin/env python3
from mcp.server.fastmcp import FastMCP


mcp = FastMCP(` + fmt.Sprintf("%q", titleFromKebab(name)) + `)

`)
	for _, tool := range tools {
		b.WriteString(fmt.Sprintf(`@mcp.tool()
def %s(request: str) -> dict[str, str]:
    """%s"""
    return {"result": f"TODO: implement %s for: {request or '(no request provided)'}"}

`, tool.Name, pythonDocstring(tool.Description), tool.Name))
	}
	b.WriteString(`
if __name__ == "__main__":
    mcp.run(transport="stdio")
`)
	return b.String()
}

func renderMCPServerCreatorNodeServer(name string, tools []mcpServerCreatorToolSpec) string {
	var b strings.Builder
	b.WriteString(`#!/usr/bin/env node
import { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js'
import { StdioServerTransport } from '@modelcontextprotocol/sdk/server/stdio.js'
import { z } from 'zod'

const server = new McpServer({ name: ` + fmt.Sprintf("%q", name) + `, version: '0.1.0' })

`)
	for _, tool := range tools {
		b.WriteString(fmt.Sprintf(`server.registerTool(
  %q,
  {
    title: %q,
    description: %q,
    inputSchema: {
      request: z.string().describe('Natural-language input for this tool.'),
    },
    outputSchema: {
      result: z.string(),
    },
  },
  async ({ request }) => {
    const output = { result: `+"`TODO: implement %s for: ${request || '(no request provided)'}`"+` }
    return {
      content: [{ type: 'text', text: JSON.stringify(output) }],
      structuredContent: output,
    }
  },
)

`, tool.Name, titleFromKebab(tool.Name), tool.Description, tool.Name))
	}
	b.WriteString(`await server.connect(new StdioServerTransport())
`)
	return b.String()
}

func normalizeMCPToolName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return ""
	}
	var b strings.Builder
	lastUnderscore := false
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastUnderscore = false
		case r == '_' || r == '-' || r == ' ':
			if !lastUnderscore {
				b.WriteByte('_')
				lastUnderscore = true
			}
		default:
			return ""
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return ""
	}
	if out[0] >= '0' && out[0] <= '9' {
		out = "tool_" + out
	}
	return out
}

func identifierFromKebab(name string) string {
	return normalizeMCPToolName(strings.ReplaceAll(name, "-", "_"))
}

func pythonDocstring(value string) string {
	value = strings.ReplaceAll(strings.TrimSpace(value), `"""`, `\"\"\"`)
	value = strings.ReplaceAll(value, "\n", " ")
	return value
}

func isMCPServerCreatorExecutable(path string) bool {
	return strings.HasSuffix(path, ".sh") || strings.HasSuffix(path, ".py") || strings.HasSuffix(path, ".mjs") || strings.HasSuffix(path, ".js")
}

func mcpServerCreatorCommandString(server config.MCPServer) string {
	parts := []string{shellQuote(server.Command)}
	for _, arg := range server.Args {
		parts = append(parts, shellQuote(arg))
	}
	return strings.Join(parts, " ")
}
