package heartbeat

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devlikebear/tarsncase/internal/memory"
)

type AskFunc func(ctx context.Context, prompt string) (string, error)

// RunOnce reads HEARTBEAT.md and appends one heartbeat event to today's daily log.
func RunOnce(workspaceDir string, now time.Time) error {
	heartbeatPath := filepath.Join(workspaceDir, "HEARTBEAT.md")
	content, err := os.ReadFile(heartbeatPath)
	if err != nil {
		return fmt.Errorf("read HEARTBEAT.md: %w", err)
	}

	if err := memory.AppendDailyLog(
		workspaceDir,
		now,
		fmt.Sprintf("heartbeat tick | prompt=%q", strings.TrimSpace(string(content))),
	); err != nil {
		return fmt.Errorf("append heartbeat log: %w", err)
	}

	return nil
}

// RunOnceWithLLM reads heartbeat context, asks LLM, and appends events to daily log.
func RunOnceWithLLM(ctx context.Context, workspaceDir string, now time.Time, ask AskFunc) error {
	_, err := RunOnceWithLLMResult(ctx, workspaceDir, now, ask)
	return err
}

// RunOnceWithLLMResult runs one heartbeat turn and returns the raw LLM response text.
func RunOnceWithLLMResult(ctx context.Context, workspaceDir string, now time.Time, ask AskFunc) (string, error) {
	if ask == nil {
		return "", fmt.Errorf("ask function is required")
	}
	prompt, rawHeartbeat, err := buildPrompt(workspaceDir, now)
	if err != nil {
		return "", err
	}

	response, err := ask(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("ask llm: %w", err)
	}

	if err := memory.AppendDailyLog(
		workspaceDir,
		now,
		fmt.Sprintf("heartbeat tick | prompt=%q", strings.TrimSpace(rawHeartbeat)),
	); err != nil {
		return "", fmt.Errorf("append heartbeat log: %w", err)
	}

	if err := memory.AppendDailyLog(
		workspaceDir,
		now,
		fmt.Sprintf("heartbeat llm response | text=%q", strings.TrimSpace(response)),
	); err != nil {
		return "", fmt.Errorf("append heartbeat llm response: %w", err)
	}
	return response, nil
}

// RunLoop executes RunOnce in a ticker loop.
// If maxHeartbeats is greater than zero, loop exits after maxHeartbeats ticks.
func RunLoop(
	ctx context.Context,
	workspaceDir string,
	interval time.Duration,
	maxHeartbeats int,
	nowFn func() time.Time,
) (int, error) {
	if interval <= 0 {
		return 0, fmt.Errorf("heartbeat interval must be > 0")
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	count := 0
	for {
		select {
		case <-ctx.Done():
			return count, nil
		case <-ticker.C:
			if err := RunOnce(workspaceDir, nowFn()); err != nil {
				return count, err
			}
			count++
			if maxHeartbeats > 0 && count >= maxHeartbeats {
				return count, nil
			}
		}
	}
}

// RunLoopWithLLM executes RunOnceWithLLM in a ticker loop.
func RunLoopWithLLM(
	ctx context.Context,
	workspaceDir string,
	interval time.Duration,
	maxHeartbeats int,
	nowFn func() time.Time,
	ask AskFunc,
) (int, error) {
	if interval <= 0 {
		return 0, fmt.Errorf("heartbeat interval must be > 0")
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	count := 0
	for {
		select {
		case <-ctx.Done():
			return count, nil
		case <-ticker.C:
			if err := RunOnceWithLLM(ctx, workspaceDir, nowFn(), ask); err != nil {
				return count, err
			}
			count++
			if maxHeartbeats > 0 && count >= maxHeartbeats {
				return count, nil
			}
		}
	}
}

func buildPrompt(workspaceDir string, now time.Time) (string, string, error) {
	heartbeatPath := filepath.Join(workspaceDir, "HEARTBEAT.md")
	heartbeatRaw, err := os.ReadFile(heartbeatPath)
	if err != nil {
		return "", "", fmt.Errorf("read HEARTBEAT.md: %w", err)
	}

	memoryPath := filepath.Join(workspaceDir, "MEMORY.md")
	memoryRaw, err := os.ReadFile(memoryPath)
	if err != nil {
		return "", "", fmt.Errorf("read MEMORY.md: %w", err)
	}

	todayPath := filepath.Join(workspaceDir, "memory", now.Format("2006-01-02")+".md")
	todayRaw, _ := os.ReadFile(todayPath)

	prompt := fmt.Sprintf(
		"HEARTBEAT:\n%s\n\nMEMORY:\n%s\n\nTODAY_LOG:\n%s\n\nDecide one next actionable step in one short sentence.",
		strings.TrimSpace(string(heartbeatRaw)),
		strings.TrimSpace(string(memoryRaw)),
		strings.TrimSpace(string(todayRaw)),
	)

	return prompt, string(heartbeatRaw), nil
}
