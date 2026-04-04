package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/memory"
)

func NewMemoryKBListTool(workspaceDir string) Tool {
	return Tool{
		Name:        "memory_kb_list",
		Description: "List durable knowledge-base notes compiled into the workspace wiki.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "query":{"type":"string"},
    "kind":{"type":"string"},
    "tag":{"type":"string"},
    "project_id":{"type":"string"},
    "limit":{"type":"integer","minimum":1,"maximum":200}
  },
  "additionalProperties":false
}`),
		Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
			var input struct {
				Query     string `json:"query,omitempty"`
				Kind      string `json:"kind,omitempty"`
				Tag       string `json:"tag,omitempty"`
				ProjectID string `json:"project_id,omitempty"`
				Limit     int    `json:"limit,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return JSONTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			items, err := memory.NewKnowledgeStore(workspaceDir, nil).List(memory.KnowledgeListOptions{
				Query:     strings.TrimSpace(input.Query),
				Kind:      strings.TrimSpace(input.Kind),
				Tag:       strings.TrimSpace(input.Tag),
				ProjectID: strings.TrimSpace(input.ProjectID),
				Limit:     input.Limit,
			})
			if err != nil {
				return JSONTextResult(map[string]any{"message": err.Error()}, true), nil
			}
			return JSONTextResult(map[string]any{
				"count": len(items),
				"items": items,
			}, false), nil
		},
	}
}

func NewMemoryKBGetTool(workspaceDir string) Tool {
	return Tool{
		Name:        "memory_kb_get",
		Description: "Read one knowledge-base note by slug.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{"slug":{"type":"string"}},
  "required":["slug"],
  "additionalProperties":false
}`),
		Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
			var input struct {
				Slug string `json:"slug"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return JSONTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			item, err := memory.NewKnowledgeStore(workspaceDir, nil).Get(strings.TrimSpace(input.Slug))
			if err != nil {
				return JSONTextResult(map[string]any{"message": err.Error()}, true), nil
			}
			return JSONTextResult(item, false), nil
		},
	}
}

func NewMemoryKBUpsertTool(workspaceDir string, semantic *memory.Service) Tool {
	return Tool{
		Name:        "memory_kb_upsert",
		Description: "Create or update a structured knowledge-base note. Use this for durable wiki-style memory, links, and concept pages.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "slug":{"type":"string"},
    "title":{"type":"string"},
    "kind":{"type":"string"},
    "summary":{"type":"string"},
    "body":{"type":"string"},
    "tags":{"type":"array","items":{"type":"string"}},
    "aliases":{"type":"array","items":{"type":"string"}},
    "links":{"type":"array","items":{"type":"object","properties":{"target":{"type":"string"},"relation":{"type":"string"}},"required":["target"],"additionalProperties":false}},
    "project_id":{"type":"string"},
    "source_session":{"type":"string"}
  },
  "additionalProperties":false
}`),
		Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
			var input struct {
				Slug          string               `json:"slug,omitempty"`
				Title         *string              `json:"title,omitempty"`
				Kind          *string              `json:"kind,omitempty"`
				Summary       *string              `json:"summary,omitempty"`
				Body          *string              `json:"body,omitempty"`
				Tags          *[]string            `json:"tags,omitempty"`
				Aliases       *[]string            `json:"aliases,omitempty"`
				Links         *[]memory.KnowledgeLink `json:"links,omitempty"`
				ProjectID     *string              `json:"project_id,omitempty"`
				SourceSession *string              `json:"source_session,omitempty"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return JSONTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			item, err := memory.NewKnowledgeStore(workspaceDir, semantic).ApplyPatch(memory.KnowledgeNotePatch{
				Slug:          strings.TrimSpace(input.Slug),
				Title:         trimStringPtr(input.Title),
				Kind:          trimStringPtr(input.Kind),
				Summary:       trimStringPtr(input.Summary),
				Body:          trimStringPtr(input.Body),
				Tags:          normalizeStringSlicePtr(input.Tags),
				Aliases:       normalizeStringSlicePtr(input.Aliases),
				Links:         input.Links,
				ProjectID:     trimStringPtr(input.ProjectID),
				SourceSession: trimStringPtr(input.SourceSession),
				UpdatedAt:     time.Now().UTC(),
			})
			if err != nil {
				return JSONTextResult(map[string]any{"message": err.Error()}, true), nil
			}
			return JSONTextResult(item, false), nil
		},
	}
}

func NewMemoryKBDeleteTool(workspaceDir string, semantic *memory.Service) Tool {
	_ = semantic
	return Tool{
		Name:        "memory_kb_delete",
		Description: "Delete one knowledge-base note by slug.",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{"slug":{"type":"string"}},
  "required":["slug"],
  "additionalProperties":false
}`),
		Execute: func(_ context.Context, params json.RawMessage) (Result, error) {
			var input struct {
				Slug string `json:"slug"`
			}
			if err := json.Unmarshal(params, &input); err != nil {
				return JSONTextResult(map[string]any{"message": fmt.Sprintf("invalid arguments: %v", err)}, true), nil
			}
			if err := memory.NewKnowledgeStore(workspaceDir, nil).Delete(strings.TrimSpace(input.Slug)); err != nil {
				return JSONTextResult(map[string]any{"message": err.Error()}, true), nil
			}
			return JSONTextResult(map[string]any{
				"deleted": true,
				"slug":    strings.TrimSpace(input.Slug),
			}, false), nil
		},
	}
}

func trimStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	return &trimmed
}

func normalizeStringSlicePtr(value *[]string) *[]string {
	if value == nil {
		return nil
	}
	out := make([]string, 0, len(*value))
	for _, item := range *value {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return &out
}
