package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func TestGatewayAgentsWatcher_NoAgentsDirIsNoop(t *testing.T) {
	workspace := t.TempDir()
	var mu sync.Mutex
	refreshCount := 0
	watcher := newGatewayAgentsWatcher(gatewayAgentsWatcherOptions{
		WorkspaceDir: workspace,
		Debounce:     50 * time.Millisecond,
		Logger:       zerolog.New(io.Discard),
		Refresh: func(_ string) {
			mu.Lock()
			refreshCount++
			mu.Unlock()
		},
	})

	started, err := watcher.Start(context.Background())
	if err != nil {
		t.Fatalf("start watcher: %v", err)
	}
	if started {
		t.Fatalf("expected watcher not started when agents dir is missing")
	}
	watcher.Close()

	mu.Lock()
	defer mu.Unlock()
	if refreshCount != 0 {
		t.Fatalf("expected no refresh call, got %d", refreshCount)
	}
}

func TestGatewayAgentsWatcher_RefreshOnAgentFileChanges(t *testing.T) {
	workspace := t.TempDir()
	agentsDir := filepath.Join(workspace, "agents", "researcher")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatalf("mkdir agents dir: %v", err)
	}

	var mu sync.Mutex
	refreshCount := 0
	reasons := []string{}
	watcher := newGatewayAgentsWatcher(gatewayAgentsWatcherOptions{
		WorkspaceDir: workspace,
		Debounce:     80 * time.Millisecond,
		Logger:       zerolog.New(io.Discard),
		Refresh: func(reason string) {
			mu.Lock()
			refreshCount++
			reasons = append(reasons, reason)
			mu.Unlock()
		},
	})

	started, err := watcher.Start(context.Background())
	if err != nil {
		t.Fatalf("start watcher: %v", err)
	}
	if !started {
		t.Fatalf("expected watcher to start")
	}
	defer watcher.Close()

	agentPath := filepath.Join(agentsDir, "AGENT.md")
	if err := os.WriteFile(agentPath, []byte("first"), 0o644); err != nil {
		t.Fatalf("write create: %v", err)
	}
	waitForRefreshCount(t, &mu, &refreshCount, 1)

	if err := os.WriteFile(agentPath, []byte("second"), 0o644); err != nil {
		t.Fatalf("write modify: %v", err)
	}
	waitForRefreshCount(t, &mu, &refreshCount, 2)

	tempPath := filepath.Join(agentsDir, "TEMP.md")
	if err := os.WriteFile(tempPath, []byte("third"), 0o644); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	if err := os.Rename(tempPath, filepath.Join(agentsDir, "AGENT.md")); err != nil {
		t.Fatalf("rename to AGENT.md: %v", err)
	}
	waitForRefreshCount(t, &mu, &refreshCount, 3)

	if err := os.Remove(agentPath); err != nil {
		t.Fatalf("remove agent file: %v", err)
	}
	waitForRefreshCount(t, &mu, &refreshCount, 4)

	mu.Lock()
	defer mu.Unlock()
	for _, reason := range reasons {
		if reason != "watch" {
			t.Fatalf("expected watch reason, got %q", reason)
		}
	}
}

func TestGatewayAgentsWatcher_DebounceBurstEvents(t *testing.T) {
	workspace := t.TempDir()
	agentsDir := filepath.Join(workspace, "agents", "researcher")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatalf("mkdir agents dir: %v", err)
	}

	var mu sync.Mutex
	refreshCount := 0
	debounce := 300 * time.Millisecond
	watcher := newGatewayAgentsWatcher(gatewayAgentsWatcherOptions{
		WorkspaceDir: workspace,
		Debounce:     debounce,
		Logger:       zerolog.New(io.Discard),
		Refresh: func(_ string) {
			mu.Lock()
			refreshCount++
			mu.Unlock()
		},
	})
	started, err := watcher.Start(context.Background())
	if err != nil {
		t.Fatalf("start watcher: %v", err)
	}
	if !started {
		t.Fatalf("expected watcher to start")
	}
	defer watcher.Close()

	agentPath := filepath.Join(agentsDir, "AGENT.md")
	if err := os.WriteFile(agentPath, []byte("0"), 0o644); err != nil {
		t.Fatalf("write create: %v", err)
	}
	for i := 1; i <= 3; i++ {
		if err := os.WriteFile(agentPath, []byte(time.Now().String()), 0o644); err != nil {
			t.Fatalf("burst write %d: %v", i, err)
		}
	}

	waitForRefreshCount(t, &mu, &refreshCount, 1)
	time.Sleep(debounce + 200*time.Millisecond)

	mu.Lock()
	got := refreshCount
	mu.Unlock()
	if got != 1 {
		t.Fatalf("expected debounced refresh once, got %d", got)
	}
}

func waitForRefreshCount(t *testing.T, mu *sync.Mutex, counter *int, expected int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		current := *counter
		mu.Unlock()
		if current >= expected {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	mu.Lock()
	current := *counter
	mu.Unlock()
	t.Fatalf("expected refresh count >= %d, got %d", expected, current)
}
