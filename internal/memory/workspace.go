package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultHeartbeatTemplate = `# HEARTBEAT.md

## Heartbeat Guidance
- Check current work context from MEMORY.md and today's daily log.
- Decide next smallest actionable step.
- Write key decisions to today's daily log.
`

const defaultMemoryTemplate = `# MEMORY.md

## Long-Term Memory
- Keep only durable facts and preferences here.
`

const defaultAgentsTemplate = `# AGENTS.md

## Operating Guidelines
- Define how the agent should use workspace files (HEARTBEAT.md, MEMORY.md, daily logs).
- Specify memory read/write conventions (when to update MEMORY.md, when to append daily logs).
- Set boundaries for autonomous actions (what the agent may do without asking).
`

const defaultUserTemplate = `# USER.md

## User Profile
- Name:
- Preferred language:
- Key preferences and working style notes.
`

const defaultIdentityTemplate = `# IDENTITY.md

## Agent Identity
- Name: TARS
- Role: Personal AI assistant
- Personality traits and distinguishing characteristics.

## Communication Style
- Preferred tone, verbosity, and interaction style.
- Behavioral boundaries and topics to avoid.
- Traits that should consistently come through in responses.
`

const defaultToolsTemplate = `# TOOLS.md

## Environment Tools
- Document environment-specific tools, CLI utilities, and integrations available to the agent.
- Note any tool restrictions or preferred usage patterns.
`

// WorkspaceBootstrapFileSpec describes one top-level workspace bootstrap file.
type WorkspaceBootstrapFileSpec struct {
	Path            string
	Title           string
	Description     string
	DefaultContent  string
	EnsureByDefault bool
}

var workspaceBootstrapFileSpecs = []WorkspaceBootstrapFileSpec{
	{
		Path:            "USER.md",
		Title:           "User Identity",
		Description:     "Persistent user information such as name, timezone, preferences, and working style.",
		DefaultContent:  defaultUserTemplate,
		EnsureByDefault: true,
	},
	{
		Path:            "IDENTITY.md",
		Title:           "TARS Identity",
		Description:     "TARS persona, voice, behavioral boundaries, and self-identity.",
		DefaultContent:  defaultIdentityTemplate,
		EnsureByDefault: true,
	},
	{
		Path:            "HEARTBEAT.md",
		Title:           "Heartbeat Guidance",
		Description:     "Background/heartbeat operating guidance.",
		DefaultContent:  defaultHeartbeatTemplate,
		EnsureByDefault: true,
	},
	{
		Path:            "AGENTS.md",
		Title:           "Agent Guidelines",
		Description:     "Execution rules, autonomy boundaries, and workspace operating guidance for agents.",
		DefaultContent:  defaultAgentsTemplate,
		EnsureByDefault: true,
	},
	{
		Path:            "TOOLS.md",
		Title:           "Tool Guidance",
		Description:     "Available tools, constraints, and preferred usage patterns for agents.",
		DefaultContent:  defaultToolsTemplate,
		EnsureByDefault: true,
	},
}

const defaultKnowledgeIndexTemplate = `# Knowledge Base Index

## Purpose
- Durable wiki-style notes compiled from conversations and explicit memory operations.
- Keep note files in memory/wiki/notes and let the agent maintain them.
`

const defaultKnowledgeGraphTemplate = `{
  "nodes": [],
  "edges": []
}
`

// EnsureWorkspace creates the minimum workspace layout used by tars.
func EnsureWorkspace(root string) error {
	if err := os.MkdirAll(filepath.Join(root, "memory"), 0o755); err != nil {
		return fmt.Errorf("create memory dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "memory", "raw"), 0o755); err != nil {
		return fmt.Errorf("create memory raw dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "memory", "index"), 0o755); err != nil {
		return fmt.Errorf("create memory index dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "memory", "wiki"), 0o755); err != nil {
		return fmt.Errorf("create memory wiki dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "memory", "wiki", "notes"), 0o755); err != nil {
		return fmt.Errorf("create memory wiki notes dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "_shared"), 0o755); err != nil {
		return fmt.Errorf("create shared dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "artifacts"), 0o755); err != nil {
		return fmt.Errorf("create artifacts dir: %w", err)
	}
	if err := ensureFile(filepath.Join(root, "MEMORY.md"), defaultMemoryTemplate); err != nil {
		return err
	}
	for _, spec := range workspaceBootstrapFileSpecs {
		if !spec.EnsureByDefault {
			continue
		}
		if err := ensureFile(filepath.Join(root, spec.Path), spec.DefaultContent); err != nil {
			return err
		}
	}
	if err := ensureFile(filepath.Join(root, "memory", "wiki", "index.md"), defaultKnowledgeIndexTemplate); err != nil {
		return err
	}
	if err := ensureFile(filepath.Join(root, "memory", "wiki", "graph.json"), defaultKnowledgeGraphTemplate); err != nil {
		return err
	}
	return nil
}

func WorkspaceBootstrapFileSpecs() []WorkspaceBootstrapFileSpec {
	out := make([]WorkspaceBootstrapFileSpec, len(workspaceBootstrapFileSpecs))
	copy(out, workspaceBootstrapFileSpecs)
	return out
}

func WorkspaceBootstrapFileSpecFor(path string) (WorkspaceBootstrapFileSpec, bool) {
	for _, spec := range workspaceBootstrapFileSpecs {
		if strings.EqualFold(strings.TrimSpace(path), spec.Path) {
			return spec, true
		}
	}
	return WorkspaceBootstrapFileSpec{}, false
}

// AppendDailyLog appends one line into workspace/memory/YYYY-MM-DD.md.
func AppendDailyLog(root string, now time.Time, entry string) error {
	if err := EnsureWorkspace(root); err != nil {
		return err
	}
	path := filepath.Join(root, "memory", now.Format("2006-01-02")+".md")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open daily log: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteString(entry + "\n"); err != nil {
		return fmt.Errorf("append daily log: %w", err)
	}
	return nil
}

// AppendMemoryNote appends one bullet entry into workspace/MEMORY.md.
func AppendMemoryNote(root string, now time.Time, entry string) error {
	if strings.TrimSpace(entry) == "" {
		return nil
	}
	if err := EnsureWorkspace(root); err != nil {
		return err
	}

	path := filepath.Join(root, "MEMORY.md")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open memory file: %w", err)
	}
	defer f.Close()

	line := fmt.Sprintf("- %s %s\n", now.UTC().Format(time.RFC3339), strings.TrimSpace(entry))
	if _, err := f.WriteString(line); err != nil {
		return fmt.Errorf("append memory note: %w", err)
	}
	return nil
}

func ensureFile(path, defaultContent string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("create file %s: %w", path, err)
	}
	defer f.Close()
	if _, err := f.WriteString(defaultContent); err != nil {
		return fmt.Errorf("write default content %s: %w", path, err)
	}
	return nil
}
