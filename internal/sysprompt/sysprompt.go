package sysprompt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/memory"
)

type Scope string

const (
	ScopeWorkspace Scope = "workspace"
	ScopeAgent     Scope = "agent"
)

type FileSpec struct {
	Scope          Scope
	Path           string
	Title          string
	Description    string
	DefaultContent string
}

type FileState struct {
	Scope          Scope    `json:"scope"`
	Path           string   `json:"path"`
	Title          string   `json:"title"`
	Description    string   `json:"description"`
	Exists         bool     `json:"exists"`
	Editable       bool     `json:"editable"`
	SizeBytes      int64    `json:"size_bytes,omitempty"`
	UpdatedAt      string   `json:"updated_at,omitempty"`
	Content        string   `json:"content,omitempty"`
	StarterContent string   `json:"starter_content,omitempty"`
	PromptTargets  []string `json:"prompt_targets,omitempty"`
}

func specs() []FileSpec {
	lookup := map[string]memory.WorkspaceBootstrapFileSpec{}
	for _, spec := range memory.WorkspaceBootstrapFileSpecs() {
		lookup[spec.Path] = spec
	}
	return []FileSpec{
		workspaceSpec("USER.md", lookup),
		workspaceSpec("IDENTITY.md", lookup),
		agentSpec("AGENTS.md", lookup),
		agentSpec("TOOLS.md", lookup),
	}
}

func Specs(scope Scope) []FileSpec {
	out := []FileSpec{}
	for _, spec := range specs() {
		if scope != "" && spec.Scope != scope {
			continue
		}
		out = append(out, spec)
	}
	return out
}

func workspaceSpec(path string, lookup map[string]memory.WorkspaceBootstrapFileSpec) FileSpec {
	spec := lookup[path]
	return FileSpec{
		Scope:          ScopeWorkspace,
		Path:           path,
		Title:          spec.Title,
		Description:    spec.Description,
		DefaultContent: spec.DefaultContent,
	}
}

func agentSpec(path string, lookup map[string]memory.WorkspaceBootstrapFileSpec) FileSpec {
	spec := lookup[path]
	return FileSpec{
		Scope:          ScopeAgent,
		Path:           path,
		Title:          spec.Title,
		Description:    spec.Description,
		DefaultContent: spec.DefaultContent,
	}
}

func List(root string, scope Scope) ([]FileState, error) {
	items := []FileState{}
	for _, spec := range specs() {
		if scope != "" && spec.Scope != scope {
			continue
		}
		state, err := inspect(root, spec, false)
		if err != nil {
			return nil, err
		}
		items = append(items, state)
	}
	return items, nil
}

func Get(root string, scope Scope, path string) (FileState, error) {
	spec, ok := specFor(scope, path)
	if !ok {
		return FileState{}, os.ErrNotExist
	}
	return inspect(root, spec, true)
}

func Save(root string, scope Scope, path string, content string) (FileState, error) {
	spec, ok := specFor(scope, path)
	if !ok {
		return FileState{}, os.ErrNotExist
	}
	trimmedRoot := strings.TrimSpace(root)
	if trimmedRoot == "" {
		return FileState{}, fmt.Errorf("workspace root is required")
	}
	absPath := filepath.Join(trimmedRoot, spec.Path)
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return FileState{}, err
	}
	if err := os.WriteFile(absPath, []byte(content), 0o644); err != nil {
		return FileState{}, err
	}
	return inspect(root, spec, true)
}

func specFor(scope Scope, path string) (FileSpec, bool) {
	target := strings.TrimSpace(path)
	for _, spec := range specs() {
		if scope != "" && spec.Scope != scope {
			continue
		}
		if strings.EqualFold(spec.Path, target) {
			return spec, true
		}
	}
	return FileSpec{}, false
}

func inspect(root string, spec FileSpec, includeContent bool) (FileState, error) {
	absPath := filepath.Join(strings.TrimSpace(root), spec.Path)
	state := FileState{
		Scope:          spec.Scope,
		Path:           spec.Path,
		Title:          spec.Title,
		Description:    spec.Description,
		Editable:       true,
		PromptTargets:  promptTargetsFor(spec.Scope),
		StarterContent: spec.DefaultContent,
	}
	info, err := os.Stat(absPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return FileState{}, err
		}
		if includeContent {
			state.Content = ""
		}
		return state, nil
	}
	state.Exists = true
	state.SizeBytes = info.Size()
	state.UpdatedAt = info.ModTime().UTC().Format(time.RFC3339)
	if includeContent {
		raw, readErr := os.ReadFile(absPath)
		if readErr != nil {
			return FileState{}, readErr
		}
		state.Content = string(raw)
	}
	return state, nil
}

func promptTargetsFor(scope Scope) []string {
	switch scope {
	case ScopeAgent:
		return []string{"sub_agent"}
	default:
		return []string{"main_agent"}
	}
}
