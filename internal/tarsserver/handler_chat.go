package tarsserver

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/agent"
	"github.com/devlikebear/tars/internal/agentruntime"
	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/extensions"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/devlikebear/tars/internal/ops"
	"github.com/devlikebear/tars/internal/prompt"
	"github.com/devlikebear/tars/internal/secrets"
	"github.com/devlikebear/tars/internal/session"
	"github.com/devlikebear/tars/internal/sessionoverride"
	"github.com/devlikebear/tars/internal/skill"
	"github.com/devlikebear/tars/internal/tool"
	"github.com/devlikebear/tars/internal/usage"
	"github.com/devlikebear/tars/internal/workscheduler"
	"github.com/devlikebear/tars/internal/workstore"
	"github.com/rs/zerolog"
)

func resolveChatSession(store *session.Store, sessionID string, mainSessionID string) (string, error) {
	// The public session API exposes the main session as id="main";
	// translate it back to the real internal ID so store.Get succeeds.
	trimmedID := strings.TrimSpace(sessionID)
	if strings.EqualFold(trimmedID, "main") {
		sessionID = strings.TrimSpace(mainSessionID)
	} else if strings.EqualFold(trimmedID, "new") {
		return createFallbackChatSession(store)
	}
	if strings.TrimSpace(sessionID) == "" {
		id := strings.TrimSpace(mainSessionID)
		if id == "" {
			return createFallbackChatSession(store)
		}
		if _, err := store.Get(id); err != nil {
			return createFallbackChatSession(store)
		}
		return id, nil
	}
	if _, err := store.Get(strings.TrimSpace(sessionID)); err != nil {
		// Requested session is stale; create a fresh session instead of
		// silently attaching the request to the main session.
		return createFallbackChatSession(store)
	}
	return strings.TrimSpace(sessionID), nil
}

func createFallbackChatSession(store *session.Store) (string, error) {
	sess, err := store.Create("chat")
	if err != nil {
		return "", err
	}
	return sess.ID, nil
}

func prepareChatContext(workspaceDir, userMessage string) (systemPrompt string, toolChoice *llm.ToolChoice, err error) {
	return prepareChatContextWithExtensions(workspaceDir, "", userMessage, extensions.Snapshot{}, nil)
}

type preparedChatContext struct {
	// SystemPrompt holds only the turn-stable region. Callers keep appending
	// their own static sections to it and must emit SystemPromptTail last —
	// see the ordering invariant on prompt.BuildResultFor.
	SystemPrompt string
	// SystemPromptTail is the per-turn region (prior-context recall, current
	// time). It closes the assembled system prompt so everything ahead of it
	// stays a matchable provider cache prefix.
	SystemPromptTail           string
	ToolChoice                 *llm.ToolChoice
	SystemPromptTokens         int
	RelevantMemoryCount        int
	RelevantMemoryTokens       int
	RelevantMemorySection      string
	RelevantMemoryItems        []prompt.RelevantMemoryItem
	RelevantMemoryBudgetTokens int
}

func prepareChatContextWithExtensions(
	workspaceDir string,
	sessionID string,
	userMessage string,
	extSnapshot extensions.Snapshot,
	invokedSkill *skill.Definition,
	semanticCfg ...memory.SemanticConfig,
) (systemPrompt string, toolChoice *llm.ToolChoice, err error) {
	details, err := prepareChatContextDetailsWithExtensions(workspaceDir, sessionID, userMessage, extSnapshot, invokedSkill, semanticCfg...)
	if err != nil {
		return "", nil, err
	}
	// Single-string callers (telegram, previews) get the dynamic tail folded
	// back on at the end — same order the split assembler produces.
	return details.SystemPrompt + details.SystemPromptTail, details.ToolChoice, nil
}

func prepareChatContextDetailsWithExtensions(
	workspaceDir string,
	sessionID string,
	userMessage string,
	extSnapshot extensions.Snapshot,
	invokedSkill *skill.Definition,
	semanticCfg ...memory.SemanticConfig,
) (preparedChatContext, error) {
	return prepareChatContextDetailsWithCache(workspaceDir, sessionID, userMessage, extSnapshot, invokedSkill, nil, firstSemanticConfig(semanticCfg...), nil, "", "")
}

func prepareChatContextDetailsWithCache(
	workspaceDir string,
	sessionID string,
	userMessage string,
	extSnapshot extensions.Snapshot,
	invokedSkill *skill.Definition,
	cache *memoryCache,
	semanticCfg memory.SemanticConfig,
	workDirs []string,
	currentDir string,
	planClarifyMode string,
) (preparedChatContext, error) {
	forceRelevantMemory := shouldForceMemoryToolCall(userMessage)
	extSnapshot = filterSkillSnapshotForProject(extSnapshot, workspaceDir)

	buildOpts := prompt.BuildOptions{
		WorkspaceDir:        workspaceDir,
		WorkDirs:            workDirs,
		CurrentDir:          currentDir,
		PlanClarifyMode:     planClarifyMode,
		Query:               userMessage,
		SessionID:           sessionID,
		ForceRelevantMemory: forceRelevantMemory,
	}

	// Cache-first strategy: reuse the recall payload to skip the expensive
	// semantic search, but always reassemble the prompt from the live options
	// so a cache hit and a cache miss produce byte-identical output.
	if cached, ok := cache.Get(userMessage, sessionID); ok {
		buildOpts.PresetRelevant = cached.Preset()
		return buildContextFromResult(workspaceDir, prompt.BuildResultFor(buildOpts), extSnapshot, invokedSkill, forceRelevantMemory), nil
	}

	buildOpts.MemorySearcher = buildSemanticMemoryService(workspaceDir, semanticCfg)
	buildResult := prompt.BuildResultFor(buildOpts)

	// Populate cache with search result
	cache.Put(userMessage, sessionID, memoryRecallFromResult(buildResult))

	return buildContextFromResult(workspaceDir, buildResult, extSnapshot, invokedSkill, forceRelevantMemory), nil
}

func buildContextFromResult(
	workspaceDir string,
	buildResult prompt.BuildResult,
	extSnapshot extensions.Snapshot,
	invokedSkill *skill.Definition,
	forceRelevantMemory bool,
) preparedChatContext {
	systemPrompt := buildResult.StaticPrompt
	systemPrompt += "\n" + strings.TrimSpace(memoryToolSystemRule) + "\n"
	skillPrompt := skillPromptForChatContext(workspaceDir, extSnapshot)
	if strings.TrimSpace(skillPrompt) != "" {
		systemPrompt += "\n## Skills\n"
		systemPrompt += strings.TrimSpace(skillPrompt) + "\n"
		systemPrompt += "\n## Skill Usage Policy\n"
		systemPrompt += "- Skill body content is not preloaded in the prompt.\n"
		systemPrompt += "- If you need a skill, call read_file with the listed skill path first.\n"
	}
	if invokedSkill != nil {
		readPath := skillRuntimeReadPathForPrompt(workspaceDir, invokedSkill.RuntimePath)
		systemPrompt += "\n## Invoked Skill\n"
		systemPrompt += fmt.Sprintf(
			"User invoked /%s.\nBefore responding, call read_file on path %q to load this skill.\n",
			strings.TrimSpace(invokedSkill.Name),
			readPath,
		)
	}
	var toolChoice *llm.ToolChoice
	if forceRelevantMemory {
		toolChoice = llm.ToolChoiceRequired()
	}
	return preparedChatContext{
		SystemPrompt:               systemPrompt,
		SystemPromptTail:           buildResult.DynamicTail,
		ToolChoice:                 toolChoice,
		SystemPromptTokens:         promptTokenEstimate(systemPrompt + buildResult.DynamicTail),
		RelevantMemoryCount:        buildResult.RelevantMemoryCount,
		RelevantMemoryTokens:       buildResult.RelevantTokens,
		RelevantMemorySection:      buildResult.RelevantSection,
		RelevantMemoryItems:        append([]prompt.RelevantMemoryItem(nil), buildResult.RelevantMemoryItems...),
		RelevantMemoryBudgetTokens: buildResult.RelevantBudgetTokens,
	}
}

func appendInvokedCommandPrompt(systemPrompt, workspaceDir string, command *skill.Definition) string {
	if command == nil {
		return systemPrompt
	}
	readPath := skillRuntimeReadPathForPrompt(workspaceDir, command.RuntimePath)
	systemPrompt += "\n## Invoked Command\n"
	systemPrompt += fmt.Sprintf(
		"User invoked /%s.\nBefore responding, call read_file on path %q to load this command.\nCommands are explicit-only and should not be used unless the user invokes them.\n",
		strings.TrimSpace(command.Name),
		readPath,
	)
	return systemPrompt
}

func skillPromptForChatContext(workspaceDir string, extSnapshot extensions.Snapshot) string {
	if len(extSnapshot.Skills) == 0 {
		return strings.TrimSpace(extSnapshot.SkillPrompt)
	}
	skills := append([]skill.Definition(nil), extSnapshot.Skills...)
	for i := range skills {
		skills[i].RuntimePath = skillRuntimeReadPathForPrompt(workspaceDir, skills[i].RuntimePath)
	}
	return skill.FormatAvailableSkills(skills)
}

func skillRuntimeReadPathForPrompt(workspaceDir, runtimePath string) string {
	path := strings.TrimSpace(runtimePath)
	if path == "" {
		return ""
	}
	cleaned := filepath.ToSlash(filepath.Clean(path))
	if filepath.IsAbs(path) {
		return cleaned
	}
	workspaceBase := filepath.Base(filepath.Clean(strings.TrimSpace(workspaceDir)))
	if workspaceBase == "" || workspaceBase == "." || workspaceBase == string(filepath.Separator) {
		return cleaned
	}
	firstSegment := strings.SplitN(cleaned, "/", 2)[0]
	if firstSegment == workspaceBase {
		return cleaned
	}
	return filepath.ToSlash(filepath.Join(workspaceBase, filepath.FromSlash(cleaned)))
}

func filterSkillSnapshotForProject(snapshot extensions.Snapshot, _ string) extensions.Snapshot {
	// No project-level skill filtering after project package removal.
	return snapshot
}

func loadSessionHistory(transcriptPath string, maxTokens int) ([]session.Message, error) {
	snapshot, err := loadSessionHistorySnapshot(transcriptPath, maxTokens)
	if err != nil {
		return nil, err
	}
	return snapshot.Messages, nil
}

func loadSessionHistorySnapshot(transcriptPath string, maxTokens int) (session.HistorySnapshot, error) {
	return session.LoadHistorySnapshot(transcriptPath, maxTokens)
}

func buildLLMMessages(systemPrompt string, history []session.Message, userMessage string) []llm.ChatMessage {
	return buildLLMMessagesWithBlocks(systemPrompt, history, userMessage, nil)
}

func buildLLMMessagesWithBlocks(systemPrompt string, history []session.Message, userMessage string, contentBlocks []llm.ContentBlock) []llm.ChatMessage {
	return buildLLMMessagesWithTail(systemPrompt, "", history, userMessage, contentBlocks)
}

// buildLLMMessagesWithTail emits the turn-stable prompt and the per-turn tail
// as two adjacent system messages. Providers concatenate system messages in
// order, so the rendered prompt is unchanged — but keeping them separate lets
// the Anthropic client put its cache_control breakpoint at the end of the
// stable block instead of after the volatile one, where it could never hit.
// An empty tail collapses back to a single system message.
func buildLLMMessagesWithTail(systemPrompt, systemTail string, history []session.Message, userMessage string, contentBlocks []llm.ContentBlock) []llm.ChatMessage {
	llmMessages := make([]llm.ChatMessage, 0, len(history)+3)
	llmMessages = append(llmMessages, llm.ChatMessage{Role: "system", Content: systemPrompt})
	if strings.TrimSpace(systemTail) != "" {
		llmMessages = append(llmMessages, llm.ChatMessage{Role: "system", Content: systemTail})
	}
	llmMessages = append(llmMessages, buildLLMMessageHistory(history)...)
	msg := llm.ChatMessage{Role: "user", Content: userMessage, ContentBlocks: contentBlocks}
	llmMessages = append(llmMessages, msg)
	return llmMessages
}

// systemPromptTokens estimates the whole system prompt, which the assembler
// may have split across a stable message and a per-turn tail.
func systemPromptTokens(msgs []llm.ChatMessage) int {
	total := 0
	for _, msg := range msgs {
		if !strings.EqualFold(strings.TrimSpace(msg.Role), "system") {
			break
		}
		total += promptTokenEstimate(msg.Content)
	}
	return total
}

// insertSystemMessageBeforeUser inserts an extra system-role message
// immediately before the final user message in msgs. If no user message is
// found (defensive) the system message is appended to the end. The original
// slice header is not mutated; a new slice is returned.
func insertSystemMessageBeforeUser(msgs []llm.ChatMessage, content string) []llm.ChatMessage {
	if strings.TrimSpace(content) == "" {
		return msgs
	}
	insertAt := len(msgs)
	for i := len(msgs) - 1; i >= 0; i-- {
		if strings.EqualFold(strings.TrimSpace(msgs[i].Role), "user") {
			insertAt = i
			break
		}
	}
	out := make([]llm.ChatMessage, 0, len(msgs)+1)
	out = append(out, msgs[:insertAt]...)
	out = append(out, llm.ChatMessage{Role: "system", Content: content})
	out = append(out, msgs[insertAt:]...)
	return out
}

type toolReplayRecord struct {
	id      string
	name    string
	args    string
	content string
}

func buildLLMMessageHistory(history []session.Message) []llm.ChatMessage {
	llmMessages := make([]llm.ChatMessage, 0, len(history)+1)
	pendingByID := map[string]toolReplayRecord{}
	pendingOrder := make([]string, 0, 4)

	appendToolOutput := func(toolCallIDs []string) {
		if len(toolCallIDs) == 0 {
			return
		}
		for _, id := range toolCallIDs {
			if strings.TrimSpace(id) == "" {
				continue
			}
			record := pendingByID[id]
			if strings.TrimSpace(record.id) == "" {
				continue
			}
			llmMessages = append(llmMessages, llm.ChatMessage{
				Role:       "tool",
				Content:    record.content,
				ToolCallID: record.id,
			})
		}
		pendingByID = map[string]toolReplayRecord{}
		pendingOrder = pendingOrder[:0]
	}

	discardToolOutput := func() {
		pendingByID = map[string]toolReplayRecord{}
		pendingOrder = pendingOrder[:0]
	}

	appendAssistantWithPendingTools := func(m session.Message) {
		if len(pendingOrder) == 0 {
			llmMessages = append(llmMessages, llm.ChatMessage{
				Role:       strings.TrimSpace(m.Role),
				Content:    m.Content,
				ToolCallID: strings.TrimSpace(m.ToolCallID),
			})
			return
		}

		toolCalls := make([]llm.ToolCall, 0, len(pendingOrder))
		outputOrder := make([]string, 0, len(pendingOrder))
		for _, id := range pendingOrder {
			record := pendingByID[id]
			name := strings.TrimSpace(record.name)
			if name == "" {
				continue
			}
			toolCalls = append(toolCalls, llm.ToolCall{
				ID:        id,
				Name:      name,
				Arguments: strings.TrimSpace(record.args),
			})
			outputOrder = append(outputOrder, id)
		}
		if len(toolCalls) == 0 {
			llmMessages = append(llmMessages, llm.ChatMessage{
				Role:       "assistant",
				Content:    m.Content,
				ToolCallID: strings.TrimSpace(m.ToolCallID),
			})
			discardToolOutput()
			return
		}

		llmMessages = append(llmMessages, llm.ChatMessage{
			Role:       "assistant",
			Content:    m.Content,
			ToolCalls:  toolCalls,
			ToolCallID: strings.TrimSpace(m.ToolCallID),
		})
		appendToolOutput(outputOrder)
	}

	for _, m := range history {
		role := strings.TrimSpace(m.Role)
		if role == "" {
			continue
		}
		if role == "tool" {
			callID := strings.TrimSpace(m.ToolCallID)
			if callID == "" {
				continue
			}
			if _, ok := pendingByID[callID]; !ok {
				pendingOrder = append(pendingOrder, callID)
			}
			pendingByID[callID] = toolReplayRecord{
				id:      callID,
				name:    m.ToolName,
				args:    m.ToolArgs,
				content: m.Content,
			}
			continue
		}
		if role == "assistant" {
			appendAssistantWithPendingTools(m)
			continue
		}

		discardToolOutput()
		llmMessages = append(llmMessages, llm.ChatMessage{
			Role:       role,
			Content:    m.Content,
			ToolCallID: strings.TrimSpace(m.ToolCallID),
		})
	}
	discardToolOutput()

	return llmMessages
}

// attachmentsToContentBlocks converts chat attachments to LLM content blocks.
// Text files are injected as text blocks, images as image blocks, PDFs as document blocks.
func attachmentsToContentBlocks(attachments []chatAttachment) []llm.ContentBlock {
	if len(attachments) == 0 {
		return nil
	}
	blocks := make([]llm.ContentBlock, 0, len(attachments))
	for _, a := range attachments {
		mime := strings.TrimSpace(a.MimeType)
		data := strings.TrimSpace(a.Data)
		if data == "" {
			continue
		}

		switch {
		case strings.HasPrefix(mime, "text/") || isTextMime(mime):
			// Decode base64 text content and inject as text block
			decoded, err := base64Decode(data)
			if err != nil {
				continue
			}
			label := strings.TrimSpace(a.Name)
			if label == "" {
				label = "attachment"
			}
			blocks = append(blocks, llm.ContentBlock{
				Type: "text",
				Text: fmt.Sprintf("--- File: %s ---\n%s\n--- End of file ---", label, string(decoded)),
			})
		case strings.HasPrefix(mime, "image/"):
			blocks = append(blocks, llm.ContentBlock{
				Type:      "image",
				MediaType: mime,
				Data:      data,
			})
		case mime == "application/pdf":
			blocks = append(blocks, llm.ContentBlock{
				Type:      "document",
				MediaType: mime,
				Data:      data,
			})
		default:
			// Unknown binary — try as text
			decoded, err := base64Decode(data)
			if err != nil {
				continue
			}
			label := strings.TrimSpace(a.Name)
			if label == "" {
				label = "attachment"
			}
			blocks = append(blocks, llm.ContentBlock{
				Type: "text",
				Text: fmt.Sprintf("--- File: %s ---\n%s\n--- End of file ---", label, string(decoded)),
			})
		}
	}
	return blocks
}

func isTextMime(mime string) bool {
	textTypes := []string{
		"application/json", "application/xml", "application/yaml",
		"application/x-yaml", "application/javascript", "application/typescript",
		"application/toml", "application/x-sh",
	}
	for _, t := range textTypes {
		if mime == t {
			return true
		}
	}
	return false
}

func base64Decode(data string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(data)
}

func statusPreview(value string, maxLen int) string {
	return secrets.RedactPreview(value, maxLen)
}

func statusPreviewForTool(toolName string, value string, maxLen int) string {
	if strings.EqualFold(strings.TrimSpace(toolName), "subagents_run") {
		if compact, ok := compactSubagentsRunPreview(value); ok {
			return secrets.RedactPreview(compact, 4000)
		}
	}
	return statusPreview(value, maxLen)
}

func compactSubagentsRunPreview(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", false
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return "", false
	}
	if tasks, ok := payload["tasks"].([]any); ok {
		return compactSubagentsRunArgs(payload, tasks)
	}
	if subagents, ok := payload["subagents"].([]any); ok {
		return compactSubagentsRunResult(payload, subagents)
	}
	return "", false
}

func compactSubagentsRunArgs(payload map[string]any, tasks []any) (string, bool) {
	out := map[string]any{
		"count": len(tasks),
		"tasks": compactSubagentTaskArgs(tasks),
	}
	if count := compactPreviewCount(payload["count"]); count > 0 {
		out["count"] = count
	}
	if agent := compactPreviewString(payload["agent"], 80); agent != "" {
		out["agent"] = agent
	}
	if mode := compactPreviewString(payload["mode"], 40); mode != "" {
		out["mode"] = mode
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return "", false
	}
	return string(raw), true
}

func compactSubagentTaskArgs(tasks []any) []map[string]any {
	out := make([]map[string]any, 0, len(tasks))
	for _, item := range tasks {
		record, ok := item.(map[string]any)
		if !ok {
			continue
		}
		task := map[string]any{}
		if title := compactPreviewString(record["title"], 120); title != "" {
			task["title"] = title
		} else {
			task["title"] = "subagent"
		}
		if tier := compactPreviewString(record["tier"], 40); tier != "" {
			task["tier"] = tier
		}
		out = append(out, task)
	}
	return out
}

func compactSubagentsRunResult(payload map[string]any, subagents []any) (string, bool) {
	out := map[string]any{
		"count":     len(subagents),
		"subagents": compactSubagentResults(subagents),
	}
	if count := compactPreviewCount(payload["count"]); count > 0 {
		out["count"] = count
	}
	if agent := compactPreviewString(payload["agent"], 80); agent != "" {
		out["agent"] = agent
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return "", false
	}
	return string(raw), true
}

func compactSubagentResults(subagents []any) []map[string]any {
	out := make([]map[string]any, 0, len(subagents))
	for _, item := range subagents {
		record, ok := item.(map[string]any)
		if !ok {
			continue
		}
		run := map[string]any{}
		for _, key := range []string{"run_id", "session_id", "agent", "title", "status", "tier", "consensus_mode"} {
			if value := compactPreviewString(record[key], 120); value != "" {
				run[key] = value
			}
		}
		if summary := compactPreviewString(record["summary"], 160); summary != "" {
			run["summary"] = summary
		}
		if errMsg := compactPreviewString(record["error"], 160); errMsg != "" {
			run["error"] = errMsg
		}
		out = append(out, run)
	}
	return out
}

func compactPreviewString(value any, maxLen int) string {
	text, ok := value.(string)
	if !ok {
		return ""
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if maxLen <= 0 || len(text) <= maxLen {
		return text
	}
	runes := []rune(text)
	if len(runes) <= maxLen {
		return text
	}
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-3]) + "..."
}

func compactPreviewCount(value any) int {
	switch typed := value.(type) {
	case float64:
		if typed > 0 {
			return int(typed)
		}
	case int:
		if typed > 0 {
			return typed
		}
	}
	return 0
}

func resolveInvokedSkill(message string, manager *extensions.Manager) *skill.Definition {
	if manager == nil {
		return nil
	}
	return resolveInvokedSkillFromSnapshot(message, manager.Snapshot())
}

func resolveInvokedSkillFromSnapshot(message string, snapshot extensions.Snapshot) *skill.Definition {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" || !strings.HasPrefix(trimmed, "/") {
		return nil
	}
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return nil
	}
	name := strings.TrimPrefix(fields[0], "/")
	if strings.TrimSpace(name) == "" || strings.Contains(name, "/") {
		return nil
	}
	return findInvocableSkillInSnapshot(snapshot, name)
}

func resolveSkillForMessage(message string, manager *extensions.Manager, workspaceDir, sessionID string) *skill.Definition {
	resolved := resolveSkillSelection(message, manager, workspaceDir, sessionID)
	return resolved.Definition
}

type skillSelection struct {
	Definition *skill.Definition
	Reason     string
}

type commandSelection struct {
	Definition *skill.Definition
	Reason     string
}

func resolveSkillSelection(message string, manager *extensions.Manager, workspaceDir, sessionID string, sessionConfig ...session.SessionToolConfig) skillSelection {
	snapshot := extensions.Snapshot{}
	if manager != nil {
		snapshot = manager.Snapshot()
	}
	return resolveSkillSelectionFromSnapshot(message, snapshot, workspaceDir, sessionID, sessionConfig...)
}

func resolveSkillSelectionFromSnapshot(message string, snapshot extensions.Snapshot, workspaceDir, sessionID string, sessionConfig ...session.SessionToolConfig) skillSelection {
	if invoked := resolveInvokedSkillFromSnapshot(message, snapshot); invoked != nil {
		if len(sessionConfig) > 0 {
			filtered := applySessionSkillConfig([]skill.Definition{*invoked}, sessionConfig[0])
			if len(filtered) == 0 {
				return skillSelection{}
			}
		}
		return skillSelection{Definition: invoked, Reason: "explicit_command"}
	}
	projectStart := findProjectStartSkillInSnapshot(snapshot)
	if projectStart == nil {
		return skillSelection{}
	}
	if hasActiveProjectBrief(workspaceDir, sessionID) {
		if len(sessionConfig) > 0 {
			filtered := applySessionSkillConfig([]skill.Definition{*projectStart}, sessionConfig[0])
			if len(filtered) == 0 {
				return skillSelection{}
			}
		}
		return skillSelection{Definition: projectStart, Reason: "active_brief"}
	}
	return skillSelection{}
}

func resolveCommandSelectionFromDefinitions(message string, commands []skill.Definition, sessionConfig ...session.SessionToolConfig) commandSelection {
	snapshot := extensions.Snapshot{Skills: commands}
	invoked := resolveInvokedSkillFromSnapshot(message, snapshot)
	if invoked == nil {
		return commandSelection{}
	}
	if len(sessionConfig) > 0 {
		filtered := applySessionCommandConfig([]skill.Definition{*invoked}, sessionConfig[0])
		if len(filtered) == 0 {
			return commandSelection{}
		}
	}
	return commandSelection{Definition: invoked, Reason: "explicit_command"}
}

func findProjectStartSkill(manager *extensions.Manager) *skill.Definition {
	if manager == nil {
		return nil
	}
	return findProjectStartSkillInSnapshot(manager.Snapshot())
}

func findProjectStartSkillInSnapshot(snapshot extensions.Snapshot) *skill.Definition {
	for _, name := range []string{"project-start", "project_start"} {
		if skillDef := findSkillInSnapshot(snapshot, name); skillDef != nil {
			return skillDef
		}
	}
	return nil
}

func findInvocableSkillInSnapshot(snapshot extensions.Snapshot, name string) *skill.Definition {
	def := findSkillInSnapshot(snapshot, name)
	if def == nil || !def.UserInvocable {
		return nil
	}
	return def
}

func findSkillInSnapshot(snapshot extensions.Snapshot, name string) *skill.Definition {
	key := normalizeSkillLookupKey(name)
	if key == "" {
		return nil
	}
	for _, def := range snapshot.Skills {
		if skillDefinitionMatchesLookup(def, key) {
			copySkill := def
			return &copySkill
		}
	}
	return nil
}

func normalizeSkillLookupKey(name string) string {
	key := strings.TrimSpace(name)
	key = strings.TrimPrefix(key, "/")
	return strings.ToLower(strings.TrimSpace(key))
}

func skillDefinitionMatchesLookup(def skill.Definition, key string) bool {
	if key == "" {
		return false
	}
	if normalizeSkillLookupKey(def.Name) == key {
		return true
	}
	if normalizeSkillLookupKey(def.Slash) == key {
		return true
	}
	for _, alias := range def.Aliases {
		if normalizeSkillLookupKey(alias) == key {
			return true
		}
	}
	return false
}

func hasActiveProjectBrief(_, _ string) bool {
	// Project briefs are no longer available after project package removal.
	return false
}

func latestTurnUsedTools(messages []session.Message) []string {
	if len(messages) == 0 {
		return nil
	}
	start := -1
	for i := len(messages) - 1; i >= 0; i-- {
		if strings.TrimSpace(messages[i].Role) == "user" {
			start = i
			break
		}
	}
	if start < 0 {
		return nil
	}
	used := make([]string, 0)
	seen := map[string]struct{}{}
	for i := start + 1; i < len(messages); i++ {
		if strings.TrimSpace(messages[i].Role) == "user" {
			break
		}
		if strings.TrimSpace(messages[i].Role) != "tool" {
			continue
		}
		name := strings.TrimSpace(messages[i].ToolName)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		used = append(used, name)
	}
	return used
}

// ToolCallRecord holds a tool invocation for transcript persistence.
type ToolCallRecord struct {
	ToolName    string
	ToolCallID  string
	ToolArgs    string
	ToolResult  string
	ToolIsError bool
}

func setupAgentLoop(
	client llm.Client,
	registry *tool.Registry,
	sessionID string,
	historyLen int,
	usageTracker *usage.Tracker,
	logger zerolog.Logger,
	sendStatus func(string, string, string, string, string, string, ...bool),
	afterTool func(ctx context.Context, evt agent.Event),
) (*agent.Loop, *[]ToolCallRecord) {
	toolCalls := &[]ToolCallRecord{}
	if _, ok := registry.Get("session_status"); !ok {
		registry.Register(tool.NewSessionStatusTool(func(_ context.Context) (tool.SessionStatus, error) {
			return tool.SessionStatus{
				SessionID:       sessionID,
				HistoryMessages: historyLen + 1,
			}, nil
		}))
	}

	counterHook := agent.NewCounterHook()
	auditHook := agent.NewAuditHook(64)
	logHook := agent.HookFunc(func(ctx context.Context, evt agent.Event) {
		logger.Debug().
			Str("event", string(evt.Type)).
			Int("iteration", evt.Iteration).
			Int("message_count", evt.MessageCount).
			Str("tool_name", evt.ToolName).
			Str("tool_call_id", evt.ToolCallID).
			Msg("agent loop event")
		switch evt.Type {
		case agent.EventLoopStart:
			sendStatus("loop_start", "agent loop started", "", "", "", "")
		case agent.EventBeforeLLM:
			sendStatus("before_llm", "calling llm", "", "", "", "")
		case agent.EventAfterLLM:
			sendStatus("after_llm", "llm response received", "", "", "", "")
		case agent.EventBeforeTool:
			sendStatus(
				"before_tool_call",
				"executing tool",
				evt.ToolName,
				evt.ToolCallID,
				statusPreviewForTool(evt.ToolName, evt.ToolArgs, 180),
				"",
			)
		case agent.EventAfterTool:
			sendStatus(
				"after_tool_call",
				"tool completed",
				evt.ToolName,
				evt.ToolCallID,
				statusPreviewForTool(evt.ToolName, evt.ToolArgs, 180),
				statusPreviewForTool(evt.ToolName, evt.ToolResult, 180),
				evt.ToolIsError,
			)
			recordToolUsageSignal(ctx, usageTracker, sessionID, evt)
			*toolCalls = append(*toolCalls, ToolCallRecord{
				ToolName:    evt.ToolName,
				ToolCallID:  evt.ToolCallID,
				ToolArgs:    statusPreviewForTool(evt.ToolName, evt.ToolArgs, 500),
				ToolResult:  statusPreviewForTool(evt.ToolName, evt.ToolResult, 500),
				ToolIsError: evt.ToolIsError,
			})
			if afterTool != nil {
				afterTool(ctx, evt)
			}
		case agent.EventProviderTool:
			// Upstream provider (claude-code-cli today) already executed the
			// tool inside its own subprocess. We surface it to the chat
			// stream so the console can show "Claude Code ran Bash(ls)" and
			// to the persisted transcript so the audit trail survives the
			// session — but we don't issue a usage signal (no TARS-side
			// invocation occurred) and we don't fire the afterTool hook
			// (that hook assumes TARS ran the tool).
			sendStatus(
				"provider_tool",
				"upstream tool executed",
				evt.ToolName,
				evt.ToolCallID,
				statusPreviewForTool(evt.ToolName, evt.ToolArgs, 180),
				"",
			)
			*toolCalls = append(*toolCalls, ToolCallRecord{
				ToolName:    evt.ToolName,
				ToolCallID:  evt.ToolCallID,
				ToolArgs:    statusPreviewForTool(evt.ToolName, evt.ToolArgs, 500),
				ToolResult:  "(executed by upstream provider)",
				ToolIsError: false,
			})
		case agent.EventLoopEnd:
			sendStatus("loop_end", "agent loop completed", "", "", "", "")
			logger.Debug().
				Str("session_id", sessionID).
				Any("event_counts", counterHook.Snapshot()).
				Int("audit_entries", len(auditHook.Entries())).
				Msg("agent loop summary")
		case agent.EventLoopError:
			msg := "agent loop error"
			if evt.Err != nil {
				msg = evt.Err.Error()
			}
			sendStatus("error", msg, evt.ToolName, evt.ToolCallID, "", "")
		}
	})
	return agent.NewLoop(client, registry, counterHook, auditHook, logHook), toolCalls
}

func recordToolUsageSignal(ctx context.Context, tracker *usage.Tracker, fallbackSessionID string, evt agent.Event) {
	if tracker == nil || strings.TrimSpace(evt.ToolName) == "" {
		return
	}
	meta := usage.CallMetaFromContext(ctx)
	sessionID := strings.TrimSpace(meta.SessionID)
	if sessionID == "" {
		sessionID = strings.TrimSpace(fallbackSessionID)
	}
	dimensions := toolSignalDimensions(evt.ToolName, evt.ToolArgs)
	if evt.ToolIsError {
		dimensions["error"] = "true"
	}
	_ = tracker.RecordSignal(usage.SignalEntry{
		Name:       "tool_call",
		Source:     meta.Source,
		SessionID:  sessionID,
		RunID:      meta.RunID,
		Dimensions: dimensions,
	})
}

func toolSignalDimensions(toolName string, rawArgs string) map[string]string {
	dimensions := map[string]string{
		"tool": strings.TrimSpace(toolName),
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(rawArgs)), &args); err != nil {
		return dimensions
	}
	if action, ok := stringValue(args["action"]); ok {
		dimensions["action"] = action
	}
	if mode, ok := stringValue(args["mode"]); ok {
		dimensions["mode"] = mode
	}
	if tasks, ok := args["tasks"].([]any); ok {
		dimensions["task_count"] = fmt.Sprintf("%d", len(tasks))
	}
	if steps, ok := args["steps"].([]any); ok {
		dimensions["step_count"] = fmt.Sprintf("%d", len(steps))
	}
	return dimensions
}

func stringValue(value any) (string, bool) {
	text, ok := value.(string)
	if !ok {
		return "", false
	}
	text = strings.TrimSpace(text)
	return text, text != ""
}

func resolveAgentMaxIterations(value int) int {
	if value <= 0 {
		return agent.DefaultMaxLoopIters
	}
	return value
}

func promptTokenEstimate(content string) int {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return 0
	}
	tokens := len(trimmed) / 4
	if tokens < 1 {
		return 1
	}
	return tokens
}

func sumHistoryTokens(messages []session.Message) int {
	total := 0
	for _, m := range messages {
		total += promptTokenEstimate(m.Content)
	}
	return total
}

type chatToolingOptions struct {
	ProcessManager              *tool.ProcessManager
	Extensions                  *extensions.Manager
	AgentRuntime                *agentruntime.Runtime
	WorkScheduler               *workscheduler.Scheduler
	WorkLedger                  *workstore.Store
	AutomationToolsForWorkspace func(workspaceID string) []tool.Tool
	ToolsDefaultSet             string
	ToolsAllowHighRiskUser      bool
	MemorySemanticConfig        memory.SemanticConfig
	MemoryCache                 *memoryCache
	APIMaxInflightChat          int
	UsageTracker                *usage.Tracker
	OpsManager                  *ops.Manager
	Compaction                  chatCompactionOptions
	// ExecMaxTimeoutMS forwards config.ToolsExecMaxTimeoutMS into the
	// per-call cap on the exec tool. 0 falls back to the tool default.
	ExecMaxTimeoutMS int
	// ExecMaxBackgroundTimeoutMS forwards
	// config.ToolsProcessMaxTimeoutMS into the per-call cap on
	// background (process-managed) commands.
	ExecMaxBackgroundTimeoutMS int
	// PlanClarifyMode forwards config.PlanClarifyMode into the prompt
	// builder so the Planning section's clarifying-questions stance can
	// be tuned without forking the prompt source.
	PlanClarifyMode string
	// ClaudeCodeCLIPermissionMode forwards config.ClaudeCodeCLIPermissionMode
	// into the chat loop so it can be passed as --permission-mode when the
	// active tier uses the claude-code-cli provider. Empty falls back to
	// "auto" inside the provider.
	ClaudeCodeCLIPermissionMode string
	StyleDefaults               sessionStyleValues
	// OverrideService resolves session-cwd .tars/settings*.json overrides
	// into an EffectiveConfig used for tool gating and prompt override.
	// nil disables overrides (chat falls back to raw session fields).
	OverrideService *sessionoverride.Service
}

type chatCompactionOptions struct {
	TriggerTokens      int
	KeepRecentTokens   int
	KeepRecentFraction float64
	LLMMode            string
	LLMTimeoutSeconds  int
}

func defaultChatToolingOptions() chatToolingOptions {
	defaults := config.Default()
	return chatToolingOptions{
		StyleDefaults: sessionStyleDefaultsFromConfig(defaults),
		Compaction: chatCompactionOptions{
			TriggerTokens:      defaults.CompactionTriggerTokens,
			KeepRecentTokens:   defaults.CompactionKeepRecentTokens,
			KeepRecentFraction: defaults.CompactionKeepRecentFraction,
			LLMMode:            defaults.CompactionLLMMode,
			LLMTimeoutSeconds:  defaults.CompactionLLMTimeoutSeconds,
		},
	}
}

func toolNamesFromSchemas(schemas []llm.ToolSchema) []string {
	out := make([]string, 0, len(schemas))
	for _, schema := range schemas {
		name := strings.TrimSpace(schema.Function.Name)
		if name == "" {
			continue
		}
		out = append(out, name)
	}
	return out
}

func skillNamesFromDefinitions(defs []skill.Definition) []string {
	out := make([]string, 0, len(defs))
	for _, def := range defs {
		name := strings.TrimSpace(def.Name)
		if name == "" {
			continue
		}
		out = append(out, name)
	}
	return out
}

func skillNameOrEmpty(def *skill.Definition) string {
	if def == nil {
		return ""
	}
	return strings.TrimSpace(def.Name)
}

func newChatAPIHandler(workspaceDir string, store *session.Store, client llm.Client, logger zerolog.Logger) http.Handler {
	return newChatAPIHandlerWithRuntimeConfig(
		workspaceDir,
		store,
		client,
		nil,
		logger,
		agent.DefaultMaxLoopIters,
		nil,
		"",
		defaultChatToolingOptions(),
	)
}

func newChatAPIHandlerWithOptions(
	workspaceDir string,
	store *session.Store,
	client llm.Client,
	logger zerolog.Logger,
	maxIterations int,
	extraTools ...tool.Tool,
) http.Handler {
	return newChatAPIHandlerWithRuntimeConfig(
		workspaceDir,
		store,
		client,
		nil,
		logger,
		maxIterations,
		nil,
		"",
		defaultChatToolingOptions(),
		extraTools...,
	)
}

func newChatAPIHandlerWithRuntime(
	workspaceDir string,
	store *session.Store,
	client llm.Client,
	logger zerolog.Logger,
	maxIterations int,
	activity *runtimeActivity,
	extraTools ...tool.Tool,
) http.Handler {
	return newChatAPIHandlerWithRuntimeConfig(
		workspaceDir,
		store,
		client,
		nil,
		logger,
		maxIterations,
		activity,
		"",
		defaultChatToolingOptions(),
		extraTools...,
	)
}

func newChatAPIHandlerWithRuntimeConfig(
	workspaceDir string,
	store *session.Store,
	client llm.Client,
	router llm.Router,
	logger zerolog.Logger,
	maxIterations int,
	activity *runtimeActivity,
	mainSessionID string,
	tooling chatToolingOptions,
	extraTools ...tool.Tool,
) http.Handler {
	maxIters := resolveAgentMaxIterations(maxIterations)
	chatLimiter := newInflightLimiter(tooling.APIMaxInflightChat, 2)
	cancelRegistry := newChatCancelRegistry()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat", func(w http.ResponseWriter, r *http.Request) {
		handleChatRequest(w, r, chatHandlerDeps{
			workspaceDir:   workspaceDir,
			store:          store,
			client:         client,
			router:         router,
			logger:         logger,
			maxIters:       maxIters,
			chatLimiter:    chatLimiter,
			activity:       activity,
			mainSessionID:  strings.TrimSpace(mainSessionID),
			tooling:        tooling,
			extraTools:     extraTools,
			cancelRegistry: cancelRegistry,
		})
	})
	mux.HandleFunc("/v1/chat/cancel", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeMethodNotAllowed(w)
			return
		}
		sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
		if sessionID == "" {
			writeError(w, http.StatusBadRequest, "", "session_id is required")
			return
		}
		if cancelRegistry.Cancel(sessionID) {
			writeJSON(w, http.StatusOK, map[string]bool{"cancelled": true})
		} else {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "no active chat for session"})
		}
	})
	mux.HandleFunc("/v1/chat/mentions/files", func(w http.ResponseWriter, r *http.Request) {
		handleChatFileMentionCandidates(w, r, chatHandlerDeps{
			workspaceDir:  workspaceDir,
			store:         store,
			client:        client,
			router:        router,
			logger:        logger,
			tooling:       tooling,
			mainSessionID: strings.TrimSpace(mainSessionID),
		})
	})
	mux.HandleFunc("/v1/chat/tools", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowed(w)
			return
		}
		reqStore, requestWorkspaceDir, _, err := resolveSessionStoreForRequest(workspaceDir, store, r)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "", "resolve workspace failed")
			return
		}
		registry := buildChatToolRegistry(
			reqStore, "", "", requestWorkspaceDir, tool.SingleDirPolicy(requestWorkspaceDir), nil, chatHandlerDeps{
				workspaceDir:  workspaceDir,
				store:         store,
				client:        client,
				router:        router,
				logger:        logger,
				tooling:       tooling,
				extraTools:    extraTools,
				mainSessionID: strings.TrimSpace(mainSessionID),
			},
		)
		type toolInfo struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			HighRisk    bool   `json:"high_risk"`
			Group       string `json:"group,omitempty"`
		}
		schemas := registry.Schemas()
		tools := make([]toolInfo, 0, len(schemas))
		for _, s := range schemas {
			canonical := tool.CanonicalToolName(s.Function.Name)
			tools = append(tools, toolInfo{
				Name:        s.Function.Name,
				Description: s.Function.Description,
				HighRisk:    isHighRiskToolName(s.Function.Name),
				Group:       tool.ToolGroupForName(canonical),
			})
		}
		// Include skills and MCP info if available
		type chatToolsResponse struct {
			Tools    []toolInfo         `json:"tools"`
			Skills   []string           `json:"skills,omitempty"`
			Commands []skill.Definition `json:"commands,omitempty"`
			MCP      []string           `json:"mcp_servers,omitempty"`
		}
		resp := chatToolsResponse{Tools: tools}
		if tooling.Extensions != nil {
			snap := tooling.Extensions.Snapshot()
			if sessionID := strings.TrimSpace(r.URL.Query().Get("session_id")); sessionID != "" {
				if sess, err := reqStore.Get(sessionID); err == nil {
					snap = augmentSnapshotWithCwdSkills(snap, sess.CurrentDir)
					commands, commandDiags := loadSessionCwdCommands(sess.CurrentDir)
					snap.Diagnostics = append(snap.Diagnostics, commandDiags...)
					resp.Commands = commands
				}
			}
			for _, sk := range snap.Skills {
				resp.Skills = append(resp.Skills, sk.Name)
			}
			for _, server := range snap.MCPServers {
				if name := strings.TrimSpace(server.Name); name != "" {
					resp.MCP = append(resp.MCP, name)
				}
			}
		} else if sessionID := strings.TrimSpace(r.URL.Query().Get("session_id")); sessionID != "" {
			if sess, err := reqStore.Get(sessionID); err == nil {
				snap := augmentSnapshotWithCwdSkills(extensions.Snapshot{}, sess.CurrentDir)
				commands, _ := loadSessionCwdCommands(sess.CurrentDir)
				resp.Commands = commands
				for _, sk := range snap.Skills {
					resp.Skills = append(resp.Skills, sk.Name)
				}
			}
		}
		writeJSON(w, http.StatusOK, resp)
	})
	mux.HandleFunc("/v1/chat/prior-context/preview", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeMethodNotAllowed(w)
			return
		}
		var req struct {
			SessionID string `json:"session_id"`
			Query     string `json:"query"`
		}
		if !decodeJSONBody(w, r, &req) {
			return
		}
		sessionID := strings.TrimSpace(req.SessionID)
		if sessionID == "" {
			writeError(w, http.StatusBadRequest, "", "session_id is required")
			return
		}
		reqStore, requestWorkspaceDir, _, err := resolveSessionStoreForRequest(workspaceDir, store, r)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "", "resolve workspace failed")
			return
		}
		sess, err := reqStore.Get(sessionID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
			return
		}
		query := strings.TrimSpace(req.Query)
		preview := prompt.BuildPriorContextPreview(prompt.BuildOptions{
			WorkspaceDir:   requestWorkspaceDir,
			WorkDirs:       sess.WorkDirs,
			CurrentDir:     sess.CurrentDir,
			Query:          query,
			SessionID:      sessionID,
			MemorySearcher: buildSemanticMemoryService(requestWorkspaceDir, tooling.MemorySemanticConfig),
		}, 0)
		budgetPercent := 0
		if preview.RelevantBudgetTokens > 0 && preview.RelevantTokens > 0 {
			budgetPercent = int((float64(preview.RelevantTokens) / float64(preview.RelevantBudgetTokens)) * 100)
			if budgetPercent == 0 {
				budgetPercent = 1
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"session_id":             sessionID,
			"query":                  query,
			"mode":                   preview.Mode,
			"section":                preview.Section,
			"items":                  preview.Items,
			"below_threshold_items":  preview.BelowThreshold,
			"recent_fallback_items":  preview.RecentFallback,
			"relevant_tokens":        preview.RelevantTokens,
			"relevant_memory_count":  len(preview.Items),
			"relevant_budget_tokens": preview.RelevantBudgetTokens,
			"budget_percent":         budgetPercent,
			"generated_at":           time.Now().UTC().Format(time.RFC3339),
		})
	})
	mux.HandleFunc("/v1/chat/context", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowed(w)
			return
		}
		sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
		if sessionID == "" {
			writeError(w, http.StatusBadRequest, "", "session_id is required")
			return
		}
		reqStore, requestWorkspaceDir, _, err := resolveSessionStoreForRequest(workspaceDir, store, r)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "", "resolve workspace failed")
			return
		}
		sess, err := reqStore.Get(sessionID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
			return
		}
		transcriptPath := reqStore.TranscriptPath(sessionID)
		historySnapshot, err := loadSessionHistorySnapshot(transcriptPath, chatHistoryMaxTokens)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "", "load history failed")
			return
		}
		extSnapshot := extensions.Snapshot{}
		if tooling.Extensions != nil {
			extSnapshot = tooling.Extensions.Snapshot()
		}
		extSnapshot = augmentSnapshotWithCwdSkills(extSnapshot, sess.CurrentDir)
		commands, commandDiags := loadSessionCwdCommands(sess.CurrentDir)
		extSnapshot.Diagnostics = append(extSnapshot.Diagnostics, commandDiags...)
		var sessionToolConfigs []session.SessionToolConfig
		effTC, effPrompt, present := effectiveSessionView(tooling.OverrideService, sess)
		if present {
			sessionToolConfigs = append(sessionToolConfigs, effTC)
		}
		extSnapshot = filterExtensionsSnapshotForSession(extSnapshot, sessionToolConfigs...)
		if len(sessionToolConfigs) > 0 {
			commands = applySessionCommandConfig(commands, sessionToolConfigs[0])
		}
		// Build PathPolicy from session work_dirs for context preview
		var previewPolicy tool.PathPolicy
		if len(sess.WorkDirs) > 0 {
			previewPolicy = tool.NewPathPolicy(requestWorkspaceDir, sess.WorkDirs, sess.CurrentDir)
		} else {
			previewPolicy = tool.SingleDirPolicy(requestWorkspaceDir)
		}
		contextDetails, err := prepareChatContextDetailsWithCache(
			requestWorkspaceDir, sessionID, "(context preview)",
			extSnapshot, nil, tooling.MemoryCache, tooling.MemorySemanticConfig,
			sess.WorkDirs, sess.CurrentDir, tooling.PlanClarifyMode,
		)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "", "prepare context failed")
			return
		}
		systemPrompt := contextDetails.SystemPrompt
		if strings.TrimSpace(effPrompt) != "" {
			systemPrompt += "\n\n## Session Prompt Override\n" + strings.TrimSpace(effPrompt) + "\n"
		}
		style := effectiveSessionStyle(tooling.StyleDefaults, sess.StyleControl)
		systemPrompt += formatSessionStylePrompt(style, sess.AutomationConsent)
		// Mirror the live assembler: the per-turn tail closes the prompt.
		systemPrompt += contextDetails.SystemPromptTail
		registry := buildChatToolRegistry(
			reqStore, "", sessionID, requestWorkspaceDir, previewPolicy, historySnapshot.Messages, chatHandlerDeps{
				workspaceDir:  workspaceDir,
				store:         store,
				client:        client,
				router:        router,
				logger:        logger,
				tooling:       tooling,
				extraTools:    extraTools,
				mainSessionID: strings.TrimSpace(mainSessionID),
			},
		)
		injectedSchemas := resolveInjectedToolSchemas(
			registry,
			tooling.ToolsDefaultSet,
			nil,
			"admin",
			tooling.ToolsAllowHighRiskUser,
			sessionToolConfigs...,
		)
		writeJSON(w, http.StatusOK, map[string]any{
			"session_id":                      sessionID,
			"system_prompt":                   systemPrompt,
			"system_prompt_tokens":            promptTokenEstimate(systemPrompt),
			"history_tokens":                  sumHistoryTokens(historySnapshot.Messages),
			"history_messages":                len(historySnapshot.Messages),
			"tool_count":                      len(injectedSchemas),
			"tool_names":                      toolNamesFromSchemas(injectedSchemas),
			"skill_count":                     len(extSnapshot.Skills),
			"skill_names":                     skillNamesFromDefinitions(extSnapshot.Skills),
			"command_count":                   len(commands),
			"command_names":                   skillNamesFromDefinitions(commands),
			"memory_count":                    contextDetails.RelevantMemoryCount,
			"memory_tokens":                   contextDetails.RelevantMemoryTokens,
			"compaction_trigger_tokens":       tooling.Compaction.TriggerTokens,
			"compaction_keep_recent_tokens":   tooling.Compaction.KeepRecentTokens,
			"compaction_keep_recent_fraction": tooling.Compaction.KeepRecentFraction,
			"compaction_last_mode":            strings.TrimSpace(sess.LastCompactionMode),
			"used_tool_names":                 latestTurnUsedTools(historySnapshot.Messages),
			"mentioned_subagent_count":        0,
			"mentioned_subagents":             []string{},
			"prompt_override":                 sess.PromptOverride,
			"style_control":                   sess.StyleControl,
			"style_effective":                 style,
		})
	})
	return mux
}
