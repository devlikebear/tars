package browser

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type flowRunRequest struct {
	Mode           string            `json:"mode"`
	SiteID         string            `json:"site_id,omitempty"`
	Profile        string            `json:"profile,omitempty"`
	URL            string            `json:"url,omitempty"`
	AllowedHosts   []string          `json:"allowed_hosts,omitempty"`
	Headless       bool              `json:"headless"`
	ExecutablePath string            `json:"executable_path,omitempty"`
	UserDataDir    string            `json:"user_data_dir,omitempty"`
	WorkspaceDir   string            `json:"workspace_dir,omitempty"`
	Action         string            `json:"action,omitempty"`
	Target         string            `json:"target,omitempty"`
	Value          string            `json:"value,omitempty"`
	ScreenshotPath string            `json:"screenshot_path,omitempty"`
	Login          SiteLogin         `json:"login,omitempty"`
	Checks         []SiteCheck       `json:"checks,omitempty"`
	Steps          []SiteStep        `json:"steps,omitempty"`
	Credentials    map[string]string `json:"credentials,omitempty"`
	OTPCode        string            `json:"otp_code,omitempty"`
}

type flowRunResponse struct {
	CurrentURL     string `json:"current_url,omitempty"`
	Snapshot       string `json:"snapshot,omitempty"`
	LastAction     string `json:"last_action,omitempty"`
	ScreenshotPath string `json:"screenshot_path,omitempty"`
	Message        string `json:"message,omitempty"`
	Passed         bool   `json:"passed,omitempty"`
}

func runPlaywrightRequest(ctx context.Context, req flowRunRequest) (flowRunResponse, error) {
	scriptPath := defaultPlaywrightRunnerPath()
	payload, err := json.Marshal(req)
	if err != nil {
		return flowRunResponse{}, fmt.Errorf("marshal playwright request: %w", err)
	}
	cmd := exec.CommandContext(ctx, "node", scriptPath)
	cmd.Dir = filepath.Dir(filepath.Dir(scriptPath))
	cmd.Stdin = bytes.NewReader(payload)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = strings.TrimSpace(stdout.String())
		}
		if message == "" {
			message = err.Error()
		}
		return flowRunResponse{}, fmt.Errorf("playwright runner failed: %s", message)
	}
	var response flowRunResponse
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		return flowRunResponse{}, fmt.Errorf("decode playwright response: %w", err)
	}
	return response, nil
}

func defaultPlaywrightRunnerPath() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "scripts/playwright_browser_runner.mjs"
	}
	return filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(file))), "scripts", "playwright_browser_runner.mjs")
}
