package heartbeat

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/memory"
)

type AskFunc func(ctx context.Context, prompt string) (string, error)

type TurnRecord struct {
	OccurredAt   time.Time
	Prompt       string
	Response     string
	Acknowledged bool
}

type RunResult struct {
	Response     string
	Skipped      bool
	SkipReason   string
	Logged       bool
	Acknowledged bool
}

type Policy struct {
	AckToken           string
	ActiveHours        string
	Timezone           string
	ShouldRun          func(ctx context.Context, now time.Time) (bool, string)
	LoadSessionContext func(ctx context.Context, now time.Time) (string, error)
	AppendSessionTurn  func(ctx context.Context, turn TurnRecord) error
}

const defaultAckToken = "HEARTBEAT_OK"

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
	result, err := RunOnceWithLLMResultWithPolicy(ctx, workspaceDir, now, ask, Policy{})
	if err != nil {
		return "", err
	}
	return result.Response, nil
}

// RunOnceWithLLMResultWithPolicy runs one heartbeat turn with optional policy controls.
func RunOnceWithLLMResultWithPolicy(ctx context.Context, workspaceDir string, now time.Time, ask AskFunc, policy Policy) (RunResult, error) {
	if ask == nil {
		return RunResult{}, fmt.Errorf("ask function is required")
	}
	if ok, reason, err := shouldRun(ctx, now, policy); err != nil {
		return RunResult{}, err
	} else if !ok {
		return RunResult{
			Skipped:    true,
			SkipReason: reason,
		}, nil
	}
	prompt, rawHeartbeat, err := buildPrompt(workspaceDir, now, policy.LoadSessionContext, ctx)
	if err != nil {
		return RunResult{}, err
	}

	response, err := ask(ctx, prompt)
	if err != nil {
		return RunResult{}, fmt.Errorf("ask llm: %w", err)
	}
	result := RunResult{Response: response}
	result.Acknowledged = isAcknowledged(response, policy.AckToken)

	if !result.Acknowledged {
		if err := memory.AppendDailyLog(
			workspaceDir,
			now,
			fmt.Sprintf("heartbeat tick | prompt=%q", strings.TrimSpace(rawHeartbeat)),
		); err != nil {
			return RunResult{}, fmt.Errorf("append heartbeat log: %w", err)
		}

		if err := memory.AppendDailyLog(
			workspaceDir,
			now,
			fmt.Sprintf("heartbeat llm response | text=%q", strings.TrimSpace(response)),
		); err != nil {
			return RunResult{}, fmt.Errorf("append heartbeat llm response: %w", err)
		}
		result.Logged = true

		if policy.AppendSessionTurn != nil {
			if err := policy.AppendSessionTurn(ctx, TurnRecord{
				OccurredAt:   now.UTC(),
				Prompt:       strings.TrimSpace(rawHeartbeat),
				Response:     strings.TrimSpace(response),
				Acknowledged: result.Acknowledged,
			}); err != nil {
				return RunResult{}, fmt.Errorf("append heartbeat session turn: %w", err)
			}
		}
	}
	return result, nil
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
	return RunLoopWithLLMWithPolicy(ctx, workspaceDir, interval, maxHeartbeats, nowFn, ask, Policy{})
}

func RunLoopWithLLMWithPolicy(
	ctx context.Context,
	workspaceDir string,
	interval time.Duration,
	maxHeartbeats int,
	nowFn func() time.Time,
	ask AskFunc,
	policy Policy,
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
			if _, err := RunOnceWithLLMResultWithPolicy(ctx, workspaceDir, nowFn(), ask, policy); err != nil {
				return count, err
			}
			count++
			if maxHeartbeats > 0 && count >= maxHeartbeats {
				return count, nil
			}
		}
	}
}

func buildPrompt(
	workspaceDir string,
	now time.Time,
	loadSessionContext func(ctx context.Context, now time.Time) (string, error),
	ctx context.Context,
) (string, string, error) {
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
	mainSessionContext := ""
	if loadSessionContext != nil {
		contextRaw, err := loadSessionContext(ctx, now)
		if err != nil {
			return "", "", fmt.Errorf("load main session context: %w", err)
		}
		mainSessionContext = strings.TrimSpace(contextRaw)
	}

	prompt := fmt.Sprintf(
		"HEARTBEAT:\n%s\n\nMEMORY:\n%s\n\nTODAY_LOG:\n%s\n\nDecide one next actionable step in one short sentence.",
		strings.TrimSpace(string(heartbeatRaw)),
		strings.TrimSpace(string(memoryRaw)),
		strings.TrimSpace(string(todayRaw)),
	)
	if mainSessionContext != "" {
		prompt += "\n\nMAIN_SESSION_CONTEXT:\n" + mainSessionContext
	}

	return prompt, string(heartbeatRaw), nil
}

func shouldRun(ctx context.Context, now time.Time, policy Policy) (bool, string, error) {
	if policy.ShouldRun != nil {
		ok, reason := policy.ShouldRun(ctx, now)
		if !ok {
			reason = strings.TrimSpace(reason)
			if reason == "" {
				reason = "heartbeat gate blocked run"
			}
			return false, reason, nil
		}
	}
	window := strings.TrimSpace(policy.ActiveHours)
	if window == "" {
		return true, "", nil
	}
	ok, err := withinActiveHours(now, window, strings.TrimSpace(policy.Timezone))
	if err != nil {
		return false, "", err
	}
	if !ok {
		return false, "outside active hours", nil
	}
	return true, "", nil
}

func withinActiveHours(now time.Time, activeHours, timezone string) (bool, error) {
	location := now.Location()
	if timezone != "" {
		loc, err := time.LoadLocation(timezone)
		if err != nil {
			return false, fmt.Errorf("invalid heartbeat timezone %q: %w", timezone, err)
		}
		location = loc
	}
	local := now.In(location)
	start, end, err := parseActiveHourRange(activeHours)
	if err != nil {
		return false, err
	}
	current := local.Hour()*60 + local.Minute()
	if start == end {
		return true, nil
	}
	if start < end {
		return current >= start && current < end, nil
	}
	return current >= start || current < end, nil
}

func parseActiveHourRange(raw string) (int, int, error) {
	parts := strings.Split(strings.TrimSpace(raw), "-")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid active_hours: %q (expected HH:MM-HH:MM)", raw)
	}
	start, err := parseClockMinutes(parts[0])
	if err != nil {
		return 0, 0, err
	}
	end, err := parseClockMinutes(parts[1])
	if err != nil {
		return 0, 0, err
	}
	return start, end, nil
}

func parseClockMinutes(raw string) (int, error) {
	value := strings.TrimSpace(raw)
	t, err := time.Parse("15:04", value)
	if err != nil {
		return 0, fmt.Errorf("invalid active_hours clock %q (expected HH:MM)", value)
	}
	return t.Hour()*60 + t.Minute(), nil
}

func isAcknowledged(response, token string) bool {
	trimmed := strings.TrimSpace(response)
	if trimmed == "" {
		return false
	}
	if strings.TrimSpace(token) == "" {
		token = defaultAckToken
	}
	return strings.HasPrefix(trimmed, token) || strings.HasSuffix(trimmed, token)
}
