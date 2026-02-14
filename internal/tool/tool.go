package tool

import (
	"context"
	"encoding/json"
	"sync"
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
