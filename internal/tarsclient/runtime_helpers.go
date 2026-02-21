package tarsclient

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

func parseSpawnCommand(raw string) (spawnCommand, error) {
	args := strings.Fields(strings.TrimSpace(raw))
	cmd := spawnCommand{}
	message := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--wait":
			cmd.Wait = true
		case "--agent", "--title", "--session":
			if i+1 >= len(args) {
				return spawnCommand{}, fmt.Errorf("%s requires a value", a)
			}
			v := args[i+1]
			i++
			switch a {
			case "--agent":
				cmd.Agent = v
			case "--title":
				cmd.Title = v
			case "--session":
				cmd.SessionID = v
			}
		default:
			if strings.HasPrefix(a, "--") {
				return spawnCommand{}, fmt.Errorf("unknown option: %s", a)
			}
			message = append(message, a)
		}
	}
	cmd.Message = strings.TrimSpace(strings.Join(message, " "))
	if cmd.Message == "" {
		return spawnCommand{}, fmt.Errorf("spawn message is required")
	}
	return cmd, nil
}

func isRunTerminal(status string) bool {
	s := strings.ToLower(strings.TrimSpace(status))
	switch s {
	case "completed", "failed", "canceled", "cancelled":
		return true
	default:
		return false
	}
}

func waitRun(ctx context.Context, api runtimeClient, runID string, interval time.Duration) (agentRun, error) {
	if interval <= 0 {
		interval = time.Second
	}
	for {
		run, err := api.getRun(ctx, runID)
		if err != nil {
			return agentRun{}, err
		}
		if isRunTerminal(run.Status) {
			return run, nil
		}
		select {
		case <-ctx.Done():
			return agentRun{}, ctx.Err()
		case <-time.After(interval):
		}
	}
}

func parseOptionalLimit(v string, fallback int) (int, error) {
	val := strings.TrimSpace(v)
	if val == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(val)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("limit must be a positive integer")
	}
	return n, nil
}

func parseProfileFlag(args []string) (string, error) {
	profile := ""
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch {
		case arg == "":
			continue
		case arg == "--profile":
			if i+1 >= len(args) {
				return "", fmt.Errorf("usage: --profile <name>")
			}
			profile = strings.TrimSpace(args[i+1])
			i++
		case strings.HasPrefix(arg, "--profile="):
			profile = strings.TrimSpace(strings.TrimPrefix(arg, "--profile="))
		default:
			return "", fmt.Errorf("unknown option: %s", arg)
		}
	}
	return profile, nil
}
