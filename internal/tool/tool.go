package tool

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"

	"github.com/devlikebear/tarsncase/internal/llm"
)

type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type Result struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"is_error,omitempty"`
}

func (r Result) Text() string {
	if len(r.Content) == 0 {
		return ""
	}
	out := ""
	for i, block := range r.Content {
		if block.Text == "" {
			continue
		}
		if out != "" && i > 0 {
			out += "\n"
		}
		out += block.Text
	}
	return out
}

type Tool struct {
	Name        string
	Description string
	Parameters  json.RawMessage
	Execute     func(ctx context.Context, params json.RawMessage) (Result, error)
}

type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{
		tools: map[string]Tool{},
	}
}

func (r *Registry) Register(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name] = t
}

func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

func (r *Registry) All() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		list = append(list, t)
	}
	return list
}

func (r *Registry) Schemas() []llm.ToolSchema {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.tools) == 0 {
		return nil
	}

	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)

	schemas := make([]llm.ToolSchema, 0, len(names))
	for _, name := range names {
		t := r.tools[name]
		schemas = append(schemas, llm.ToolSchema{
			Type: "function",
			Function: llm.ToolFunctionSchema{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		})
	}
	return schemas
}

func (r *Registry) SchemasForNames(names []string) []llm.ToolSchema {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.tools) == 0 || len(names) == 0 {
		return nil
	}

	schemas := make([]llm.ToolSchema, 0, len(names))
	seen := map[string]struct{}{}
	for _, name := range names {
		key := strings.TrimSpace(name)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		t, ok := r.tools[key]
		if !ok {
			continue
		}
		schemas = append(schemas, llm.ToolSchema{
			Type: "function",
			Function: llm.ToolFunctionSchema{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		})
	}
	return schemas
}
