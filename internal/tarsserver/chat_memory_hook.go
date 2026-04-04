package tarsserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/memory"
)

type chatMemoryHookInput struct {
	WorkspaceDir     string
	SessionID        string
	ProjectID        string
	UserMessage      string
	AssistantMessage string
	AssistantTime    time.Time
	LLMClient        llm.Client
}

func applyPostChatMemoryHooks(input chatMemoryHookInput) error {
	dailyEntry := fmt.Sprintf(
		"chat session=%s user=%q assistant=%q",
		strings.TrimSpace(input.SessionID),
		trimForMemory(input.UserMessage, 120),
		trimForMemory(input.AssistantMessage, 160),
	)
	if err := memory.AppendDailyLog(input.WorkspaceDir, input.AssistantTime, dailyEntry); err != nil {
		return err
	}

	if shouldPromoteToMemory(input.UserMessage) {
		note := fmt.Sprintf("session %s user preference/fact: %s", strings.TrimSpace(input.SessionID), strings.TrimSpace(input.UserMessage))
		if err := memory.AppendMemoryNote(input.WorkspaceDir, input.AssistantTime, note); err != nil {
			return err
		}
		if err := appendExperienceIfNew(input.WorkspaceDir, memory.Experience{
			Timestamp:     input.AssistantTime.UTC(),
			Category:      "preference",
			Summary:       trimForMemory(strings.TrimSpace(input.UserMessage), 220),
			Tags:          []string{"manual-memory"},
			SourceSession: strings.TrimSpace(input.SessionID),
			ProjectID:     strings.TrimSpace(input.ProjectID),
			Importance:    8,
			Auto:          true,
		}); err != nil {
			return err
		}
	}

	for _, exp := range deriveAutoExperiences(
		strings.TrimSpace(input.SessionID),
		strings.TrimSpace(input.ProjectID),
		input.UserMessage,
		input.AssistantMessage,
		input.AssistantTime,
	) {
		if err := appendExperienceIfNew(input.WorkspaceDir, exp); err != nil {
			return err
		}
	}
	if err := maybeCompileKnowledgeBase(input); err != nil {
		return err
	}
	return nil
}

func maybeCompileKnowledgeBase(input chatMemoryHookInput) error {
	if input.LLMClient == nil {
		return nil
	}
	store := memory.NewKnowledgeStore(input.WorkspaceDir, nil)
	existing, err := store.List(memory.KnowledgeListOptions{Limit: 12})
	if err != nil {
		return nil
	}

	var refs strings.Builder
	for _, item := range existing {
		_, _ = fmt.Fprintf(&refs, "- slug=%s | title=%s | kind=%s | summary=%s\n", item.Slug, item.Title, item.Kind, trimForMemory(item.Summary, 120))
	}
	if refs.Len() == 0 {
		refs.WriteString("- (none)\n")
	}

	userPrompt := fmt.Sprintf(
		"Compile durable knowledge from the latest chat turn into a wiki-style knowledge base.\n"+
			"Return strict JSON with shape {\"notes\":[{\"slug\":\"lower-kebab-case\",\"title\":\"...\",\"kind\":\"preference|fact|decision|habit|workflow|topic|project_note|person|note\",\"summary\":\"...\",\"body\":\"...\",\"tags\":[\"...\"],\"aliases\":[\"...\"],\"links\":[{\"target\":\"slug\",\"relation\":\"related_to|depends_on|supports|part_of|contradicts\"}],\"project_id\":\"...\",\"source_session\":\"...\"}],\"edges\":[{\"source\":\"slug\",\"target\":\"slug\",\"relation\":\"...\"}]}.\n"+
			"Only keep durable knowledge worth reusing after chat/session reset.\n"+
			"Reuse existing slugs when the note already exists. Return empty arrays when nothing durable should be added.\n\n"+
			"Current note refs:\n%s\n"+
			"Session: %s\nProject: %s\nUser: %s\nAssistant: %s",
		refs.String(),
		strings.TrimSpace(input.SessionID),
		strings.TrimSpace(input.ProjectID),
		strings.TrimSpace(input.UserMessage),
		strings.TrimSpace(input.AssistantMessage),
	)
	resp, err := input.LLMClient.Chat(context.Background(), []llm.ChatMessage{
		{
			Role:    "system",
			Content: "You maintain a structured markdown knowledge base. Return strict JSON only.",
		},
		{
			Role:    "user",
			Content: userPrompt,
		},
	}, llm.ChatOptions{})
	if err != nil {
		return nil
	}
	raw := strings.TrimSpace(resp.Message.Content)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	var update memory.KnowledgeUpdate
	if err := json.Unmarshal([]byte(raw), &update); err != nil {
		return nil
	}
	for i := range update.Notes {
		if strings.TrimSpace(update.Notes[i].ProjectID) == "" {
			update.Notes[i].ProjectID = strings.TrimSpace(input.ProjectID)
		}
		if strings.TrimSpace(update.Notes[i].SourceSession) == "" {
			update.Notes[i].SourceSession = strings.TrimSpace(input.SessionID)
		}
		update.Notes[i].UpdatedAt = input.AssistantTime.UTC()
	}
	return store.ApplyUpdate(update, input.AssistantTime.UTC())
}

func shouldPromoteToMemory(userMessage string) bool {
	lower := strings.ToLower(strings.TrimSpace(userMessage))
	return strings.HasPrefix(lower, "remember ") ||
		strings.HasPrefix(lower, "remember:") ||
		strings.HasPrefix(lower, "기억해") ||
		strings.HasPrefix(lower, "메모해")
}

func trimForMemory(s string, max int) string {
	v := strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if max <= 0 || len(v) <= max {
		return v
	}
	return v[:max] + "..."
}

func deriveAutoExperiences(sessionID, projectID, userMessage, assistantMessage string, now time.Time) []memory.Experience {
	out := make([]memory.Experience, 0, 2)
	if exp, ok := deriveUserAutoExperience(sessionID, projectID, userMessage, now); ok {
		out = append(out, exp)
	}
	if exp, ok := deriveAssistantAutoExperience(sessionID, projectID, assistantMessage, now); ok {
		out = append(out, exp)
	}
	return out
}

func deriveUserAutoExperience(sessionID, projectID, userMessage string, now time.Time) (memory.Experience, bool) {
	lowerUser := strings.ToLower(strings.TrimSpace(userMessage))
	exp := memory.Experience{
		Timestamp:     now.UTC(),
		SourceSession: strings.TrimSpace(sessionID),
		ProjectID:     strings.TrimSpace(projectID),
		Importance:    6,
		Auto:          true,
	}

	switch {
	case strings.Contains(lowerUser, "prefer") || strings.Contains(lowerUser, "선호") || strings.Contains(lowerUser, "취향"):
		exp.Category = "preference"
		exp.Summary = trimForMemory(strings.TrimSpace(userMessage), 220)
		exp.Tags = []string{"auto", "user-preference"}
		return exp, exp.Summary != ""
	default:
		return memory.Experience{}, false
	}
}

func deriveAssistantAutoExperience(sessionID, projectID, assistantMessage string, now time.Time) (memory.Experience, bool) {
	lowerAssistant := strings.ToLower(strings.TrimSpace(assistantMessage))
	exp := memory.Experience{
		Timestamp:     now.UTC(),
		SourceSession: strings.TrimSpace(sessionID),
		ProjectID:     strings.TrimSpace(projectID),
		Importance:    6,
		Auto:          true,
	}

	switch {
	case strings.Contains(lowerAssistant, "completed") || strings.Contains(lowerAssistant, "완료"):
		exp.Category = "task_completed"
		exp.Summary = trimForMemory(strings.TrimSpace(assistantMessage), 220)
		exp.Tags = []string{"auto", "task"}
		exp.Importance = 7
		return exp, exp.Summary != ""
	case strings.Contains(lowerAssistant, "fixed") || strings.Contains(lowerAssistant, "resolved") || strings.Contains(lowerAssistant, "해결"):
		exp.Category = "error_resolved"
		exp.Summary = trimForMemory(strings.TrimSpace(assistantMessage), 220)
		exp.Tags = []string{"auto", "error"}
		exp.Importance = 7
		return exp, exp.Summary != ""
	default:
		return memory.Experience{}, false
	}
}

func deriveAutoExperience(sessionID, projectID, userMessage, assistantMessage string, now time.Time) (memory.Experience, bool) {
	if exp, ok := deriveUserAutoExperience(sessionID, projectID, userMessage, now); ok {
		return exp, true
	}
	return deriveAssistantAutoExperience(sessionID, projectID, assistantMessage, now)
}

func appendExperienceIfNew(workspaceDir string, exp memory.Experience) error {
	if strings.TrimSpace(exp.Summary) == "" {
		return nil
	}
	existing, err := memory.SearchExperiences(workspaceDir, memory.SearchOptions{
		Query:     strings.TrimSpace(exp.Summary),
		ProjectID: strings.TrimSpace(exp.ProjectID),
		Limit:     6,
	})
	if err == nil {
		normalizedSummary := strings.ToLower(strings.TrimSpace(exp.Summary))
		for _, item := range existing {
			if strings.ToLower(strings.TrimSpace(item.Summary)) == normalizedSummary && strings.TrimSpace(item.Category) == strings.TrimSpace(exp.Category) {
				return nil
			}
		}
	}
	return memory.AppendExperience(workspaceDir, exp)
}
