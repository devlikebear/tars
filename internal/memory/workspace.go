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

const defaultSoulTemplate = `# SOUL.md

## Persona
- Define the agent's communication style and tone.
- Set behavioral boundaries (topics to avoid, response length preferences).
- Describe the personality traits that should come through in interactions.
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
`

const defaultToolsTemplate = `# TOOLS.md

## Environment Tools
- Document environment-specific tools, CLI utilities, and integrations available to the agent.
- Note any tool restrictions or preferred usage patterns.
`

// EnsureWorkspace creates the minimum workspace layout used by tarsd.
func EnsureWorkspace(root string) error {
	if err := os.MkdirAll(filepath.Join(root, "memory"), 0o755); err != nil {
		return fmt.Errorf("create memory dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "_shared"), 0o755); err != nil {
		return fmt.Errorf("create shared dir: %w", err)
	}
	if err := ensureFile(filepath.Join(root, "HEARTBEAT.md"), defaultHeartbeatTemplate); err != nil {
		return err
	}
	if err := ensureFile(filepath.Join(root, "MEMORY.md"), defaultMemoryTemplate); err != nil {
		return err
	}
	if err := ensureFile(filepath.Join(root, "AGENTS.md"), defaultAgentsTemplate); err != nil {
		return err
	}
	if err := ensureFile(filepath.Join(root, "SOUL.md"), defaultSoulTemplate); err != nil {
		return err
	}
	if err := ensureFile(filepath.Join(root, "USER.md"), defaultUserTemplate); err != nil {
		return err
	}
	if err := ensureFile(filepath.Join(root, "IDENTITY.md"), defaultIdentityTemplate); err != nil {
		return err
	}
	if err := ensureFile(filepath.Join(root, "TOOLS.md"), defaultToolsTemplate); err != nil {
		return err
	}
	return nil
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
