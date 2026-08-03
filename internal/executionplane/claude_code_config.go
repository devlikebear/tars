package executionplane

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	claudeCodeHarnessConfigSchemaVersion = 1
	maxClaudeCodeHarnessConfigBytes      = 1 << 20
)

// ClaudeCodeHarnessConfig deliberately has no arbitrary argv, environment,
// settings, plugin, MCP, or credential fields. The owner can only choose the
// model, finite execution budgets, and a bounded coding-tool policy.
type ClaudeCodeHarnessConfig struct {
	SchemaVersion  int      `json:"schema_version"`
	Adapter        string   `json:"adapter"`
	Model          string   `json:"model"`
	TimeoutSeconds int      `json:"timeout_seconds"`
	MaxTurns       int      `json:"max_turns"`
	MaxBudgetUSD   float64  `json:"max_budget_usd"`
	Tools          []string `json:"tools"`
	AllowedTools   []string `json:"allowed_tools"`
}

func OpenConfiguredClaudeCodeWorker(configPath string) (*ClaudeCodeWorker, error) {
	config, err := loadClaudeCodeHarnessConfig(configPath)
	if err != nil {
		return nil, err
	}
	return NewClaudeCodeWorker(ClaudeCodeWorkerOptions{
		Model: config.Model, Timeout: time.Duration(config.TimeoutSeconds) * time.Second,
		MaxTurns: config.MaxTurns, MaxBudgetUSD: config.MaxBudgetUSD,
		Tools: config.Tools, AllowedTools: config.AllowedTools,
	})
}

func loadClaudeCodeHarnessConfig(configPath string) (ClaudeCodeHarnessConfig, error) {
	path := strings.TrimSpace(configPath)
	if path == "" || !filepath.IsAbs(path) {
		return ClaudeCodeHarnessConfig{}, fmt.Errorf("executionplane: absolute Claude Code harness config path is required")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return ClaudeCodeHarnessConfig{}, fmt.Errorf("executionplane: inspect Claude Code harness config: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return ClaudeCodeHarnessConfig{}, fmt.Errorf("executionplane: Claude Code harness config must be an owner-only regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return ClaudeCodeHarnessConfig{}, fmt.Errorf("executionplane: open Claude Code harness config: %w", err)
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil || !openedInfo.Mode().IsRegular() || openedInfo.Mode().Perm()&0o077 != 0 || !os.SameFile(info, openedInfo) {
		return ClaudeCodeHarnessConfig{}, fmt.Errorf("executionplane: Claude Code harness config changed during validation")
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxClaudeCodeHarnessConfigBytes+1))
	if err != nil {
		return ClaudeCodeHarnessConfig{}, fmt.Errorf("executionplane: read Claude Code harness config: %w", err)
	}
	if len(raw) > maxClaudeCodeHarnessConfigBytes {
		return ClaudeCodeHarnessConfig{}, fmt.Errorf("executionplane: Claude Code harness config exceeds %d bytes", maxClaudeCodeHarnessConfigBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var config ClaudeCodeHarnessConfig
	if err := decoder.Decode(&config); err != nil {
		return ClaudeCodeHarnessConfig{}, fmt.Errorf("executionplane: decode Claude Code harness config: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return ClaudeCodeHarnessConfig{}, fmt.Errorf("executionplane: Claude Code harness config contains trailing data")
	}
	if config.SchemaVersion != claudeCodeHarnessConfigSchemaVersion || strings.TrimSpace(config.Adapter) != claudeCodeWorkerName {
		return ClaudeCodeHarnessConfig{}, fmt.Errorf("executionplane: incompatible Claude Code harness config")
	}
	config.Model = strings.TrimSpace(config.Model)
	if config.Model == "" || len(config.Model) > 128 || strings.ContainsAny(config.Model, "\r\n\x00") {
		return ClaudeCodeHarnessConfig{}, fmt.Errorf("executionplane: invalid Claude Code harness model")
	}
	return config, nil
}
