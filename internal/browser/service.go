package browser

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// SecretReader resolves secret values for login automation.
type SecretReader interface {
	ReadKV(ctx context.Context, secretPath string) (map[string]string, error)
}

// OTPRequester requests a one-time passcode from an approved channel.
type OTPRequester interface {
	RequestOTP(ctx context.Context, siteID string, timeout time.Duration) (string, error)
}

// Config controls browser runtime behavior.
type Config struct {
	WorkspaceDir           string
	DefaultProfile         string
	ManagedHeadless        bool
	ManagedExecutablePath  string
	ManagedUserDataDir     string
	SiteFlowsDir           string
	AutoLoginSiteAllowlist []string
	Vault                  SecretReader
	OTP                    OTPRequester
}

// State is the current browser runtime state.
type State struct {
	Running        bool   `json:"running"`
	Profile        string `json:"profile,omitempty"`
	Driver         string `json:"driver,omitempty"`
	CurrentURL     string `json:"current_url,omitempty"`
	LastSnapshot   string `json:"last_snapshot,omitempty"`
	LastAction     string `json:"last_action,omitempty"`
	LastScreenshot string `json:"last_screenshot,omitempty"`
	ExtensionConnected bool `json:"extension_connected,omitempty"`
	AttachedTabs       int  `json:"attached_tabs,omitempty"`
	LastError      string `json:"last_error,omitempty"`
}

// Profile reports available browser profiles.
type Profile struct {
	Name               string `json:"name"`
	Driver             string `json:"driver"`
	Default            bool   `json:"default"`
	Running            bool   `json:"running"`
	ExtensionConnected bool   `json:"extension_connected,omitempty"`
}

// LoginResult reports login outcome without exposing secrets.
type LoginResult struct {
	SiteID  string `json:"site_id"`
	Profile string `json:"profile"`
	Mode    string `json:"mode"`
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// CheckResult reports check action outcome.
type CheckResult struct {
	SiteID     string `json:"site_id"`
	Profile    string `json:"profile"`
	CheckCount int    `json:"check_count"`
	Passed     bool   `json:"passed"`
	Message    string `json:"message"`
}

// RunResult reports flow run outcome.
type RunResult struct {
	SiteID    string `json:"site_id"`
	Profile   string `json:"profile"`
	Action    string `json:"action"`
	StepCount int    `json:"step_count"`
	Success   bool   `json:"success"`
	Message   string `json:"message"`
}

// Service provides profile-aware browser operations.
type Service struct {
	cfg       Config
	allowAuto map[string]struct{}
	managed   managedRuntime
	runner    flowRunner

	mu    sync.RWMutex
	state State
}

type managedRuntime interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Open(ctx context.Context, rawURL string) error
	Snapshot(ctx context.Context) (string, error)
	Act(ctx context.Context, action string, target string, value string) (string, error)
	Screenshot(ctx context.Context, path string) error
}

type flowRunner interface {
	Execute(ctx context.Context, req flowRunRequest) (flowRunResponse, error)
}

func NewService(cfg Config) *Service {
	defaultProfile := strings.TrimSpace(strings.ToLower(cfg.DefaultProfile))
	if defaultProfile == "" {
		defaultProfile = "managed"
	}
	if strings.TrimSpace(cfg.SiteFlowsDir) == "" {
		cfg.SiteFlowsDir = filepath.Join(strings.TrimSpace(cfg.WorkspaceDir), "automation", "sites")
	}
	if strings.TrimSpace(cfg.ManagedUserDataDir) == "" {
		cfg.ManagedUserDataDir = filepath.Join(strings.TrimSpace(cfg.WorkspaceDir), "_shared", "browser", "managed")
	}
	allow := map[string]struct{}{}
	for _, siteID := range cfg.AutoLoginSiteAllowlist {
		trimmed := strings.TrimSpace(strings.ToLower(siteID))
		if trimmed == "" {
			continue
		}
		allow[trimmed] = struct{}{}
	}
	return &Service{
		cfg:       cfg,
		allowAuto: allow,
		managed:   newPlaywrightManagedRuntime(cfg),
		runner:    newPlaywrightFlowRunner(cfg),
		state: State{
			Profile: defaultProfile,
			Driver:  driverForProfile(defaultProfile),
		},
	}
}

func (s *Service) Login(ctx context.Context, siteID string, profile string) (LoginResult, error) {
	flow, err := s.loadFlow(siteID)
	if err != nil {
		return LoginResult{}, err
	}
	resolvedProfile, err := s.resolveProfileForFlow(profile, flow)
	if err != nil {
		return LoginResult{}, err
	}
	s.Start(resolvedProfile)
	mode := strings.TrimSpace(strings.ToLower(flow.Login.Mode))
	if mode == "" {
		mode = "manual"
	}
	result := LoginResult{SiteID: flow.ID, Profile: resolvedProfile, Mode: mode}
	if mode == "manual" {
		result.Success = true
		result.Message = "manual login required in browser session"
		s.setLastAction(fmt.Sprintf("login site=%s mode=manual", flow.ID))
		return result, nil
	}
	if mode != "vault_form" && mode != "env_form" {
		return LoginResult{}, fmt.Errorf("unsupported login mode: %s", mode)
	}

	values, source, err := s.resolveLoginCredentials(ctx, flow)
	if err != nil {
		return LoginResult{}, err
	}
	if strings.TrimSpace(values["username"]) == "" || strings.TrimSpace(values["password"]) == "" {
		return LoginResult{}, fmt.Errorf("login credentials must include username/password")
	}
	otpCode := ""
	if flow.Login.OTPRequired {
		if s.cfg.OTP == nil {
			return LoginResult{}, fmt.Errorf("otp requester is not configured")
		}
		timeout := 300 * time.Second
		if flow.Login.OTPTimeoutSec > 0 {
			timeout = time.Duration(flow.Login.OTPTimeoutSec) * time.Second
		}
		code, err := s.cfg.OTP.RequestOTP(ctx, flow.ID, timeout)
		if err != nil {
			return LoginResult{}, fmt.Errorf("otp request failed: %w", err)
		}
		if strings.TrimSpace(code) == "" {
			return LoginResult{}, fmt.Errorf("otp code is empty")
		}
		otpCode = strings.TrimSpace(code)
	}
	if s.runner == nil {
		return LoginResult{}, fmt.Errorf("browser flow runner is not configured")
	}
	runResult, err := s.runner.Execute(ctx, flowRunRequest{
		Mode:           "login",
		SiteID:         flow.ID,
		Profile:        resolvedProfile,
		Headless:       s.cfg.ManagedHeadless,
		ExecutablePath: strings.TrimSpace(s.cfg.ManagedExecutablePath),
		UserDataDir:    s.profileUserDataDir(resolvedProfile, flow.ID),
		URL:            firstNonEmptyTrimmed(flow.Login.URL, flow.URL),
		AllowedHosts:   normalizeAllowedHosts(flow.AllowedHosts),
		Login:          flow.Login,
		Credentials:    values,
		OTPCode:        otpCode,
		WorkspaceDir:   strings.TrimSpace(s.cfg.WorkspaceDir),
	})
	if err != nil {
		return LoginResult{}, err
	}
	result.Success = true
	result.Message = firstNonEmptyTrimmed(runResult.Message, "auto login completed using "+source+" credentials")
	s.setLastAction(fmt.Sprintf("login site=%s mode=%s source=%s", flow.ID, mode, source))
	return result, nil
}

func (s *Service) resolveLoginCredentials(ctx context.Context, flow SiteFlow) (map[string]string, string, error) {
	mode := strings.TrimSpace(strings.ToLower(flow.Login.Mode))
	if mode == "" {
		mode = "manual"
	}
	if _, ok := s.allowAuto[strings.ToLower(flow.ID)]; !ok {
		return nil, "", fmt.Errorf("auto login is not allowed for site: %s", flow.ID)
	}

	readEnv := func() (map[string]string, bool) {
		prefix := strings.TrimSpace(flow.Login.EnvPrefix)
		values := resolveEnvLoginValues(prefix)
		username := strings.TrimSpace(values["username"])
		password := strings.TrimSpace(values["password"])
		if username == "" || password == "" {
			return nil, false
		}
		return values, true
	}

	if mode == "env_form" {
		values, ok := readEnv()
		if !ok {
			return nil, "", fmt.Errorf("env credentials are not configured for site: %s", flow.ID)
		}
		return values, "env", nil
	}

	vaultPath := strings.TrimSpace(flow.Login.VaultPath)
	if s.cfg.Vault != nil && vaultPath != "" {
		values, err := s.cfg.Vault.ReadKV(ctx, vaultPath)
		if err == nil && strings.TrimSpace(values["username"]) != "" && strings.TrimSpace(values["password"]) != "" {
			return values, "vault", nil
		}
	}
	if values, ok := readEnv(); ok {
		return values, "env", nil
	}
	return nil, "", fmt.Errorf("vault/env credentials are not configured for site: %s", flow.ID)
}

func resolveEnvLoginValues(prefix string) map[string]string {
	base := strings.TrimSpace(strings.ToUpper(prefix))
	lookup := func(keys ...string) string {
		for _, key := range keys {
			v := strings.TrimSpace(os.Getenv(key))
			if v != "" {
				return v
			}
		}
		return ""
	}
	candidatesUser := []string{"LOGIN_USERNAME", "BROWSER_LOGIN_USERNAME", "USERNAME"}
	candidatesPass := []string{"LOGIN_PASSWORD", "BROWSER_LOGIN_PASSWORD", "PASSWORD"}
	if base != "" {
		candidatesUser = append([]string{base + "_USERNAME"}, candidatesUser...)
		candidatesPass = append([]string{base + "_PASSWORD"}, candidatesPass...)
	}
	return map[string]string{
		"username": lookup(candidatesUser...),
		"password": lookup(candidatesPass...),
	}
}

func (s *Service) Check(ctx context.Context, siteID string, profile string) (CheckResult, error) {
	flow, err := s.loadFlow(siteID)
	if err != nil {
		return CheckResult{}, err
	}
	resolvedProfile, err := s.resolveProfileForFlow(profile, flow)
	if err != nil {
		return CheckResult{}, err
	}
	s.Start(resolvedProfile)
	if s.runner == nil {
		return CheckResult{}, fmt.Errorf("browser flow runner is not configured")
	}
	runResult, err := s.runner.Execute(ctx, flowRunRequest{
		Mode:           "check",
		SiteID:         flow.ID,
		Profile:        resolvedProfile,
		Headless:       s.cfg.ManagedHeadless,
		ExecutablePath: strings.TrimSpace(s.cfg.ManagedExecutablePath),
		UserDataDir:    s.profileUserDataDir(resolvedProfile, flow.ID),
		URL:            s.checkURL(flow),
		AllowedHosts:   normalizeAllowedHosts(flow.AllowedHosts),
		Checks:         flow.Checks,
		WorkspaceDir:   strings.TrimSpace(s.cfg.WorkspaceDir),
	})
	if err != nil {
		return CheckResult{}, err
	}
	res := CheckResult{
		SiteID:     flow.ID,
		Profile:    resolvedProfile,
		CheckCount: len(flow.Checks),
		Passed:     runResult.Passed,
		Message:    firstNonEmptyTrimmed(runResult.Message, fmt.Sprintf("check completed (%d checks)", len(flow.Checks))),
	}
	s.setLastAction(fmt.Sprintf("check site=%s", flow.ID))
	return res, nil
}

func (s *Service) Run(ctx context.Context, siteID string, flowAction string, profile string) (RunResult, error) {
	flow, err := s.loadFlow(siteID)
	if err != nil {
		return RunResult{}, err
	}
	resolvedProfile, err := s.resolveProfileForFlow(profile, flow)
	if err != nil {
		return RunResult{}, err
	}
	action := strings.TrimSpace(flowAction)
	if action == "" {
		return RunResult{}, fmt.Errorf("flow action is required")
	}
	steps, ok := flow.Actions[action]
	if !ok {
		return RunResult{}, fmt.Errorf("flow action not found: %s", action)
	}
	if err := validateActionAllowedHosts(flow, action, steps.Steps); err != nil {
		return RunResult{}, err
	}
	s.Start(resolvedProfile)
	if s.runner == nil {
		return RunResult{}, fmt.Errorf("browser flow runner is not configured")
	}
	runResult, err := s.runner.Execute(ctx, flowRunRequest{
		Mode:           "run",
		SiteID:         flow.ID,
		Profile:        resolvedProfile,
		Headless:       s.cfg.ManagedHeadless,
		ExecutablePath: strings.TrimSpace(s.cfg.ManagedExecutablePath),
		UserDataDir:    s.profileUserDataDir(resolvedProfile, flow.ID),
		URL:            flow.URL,
		AllowedHosts:   normalizeAllowedHosts(flow.AllowedHosts),
		Steps:          steps.Steps,
		WorkspaceDir:   strings.TrimSpace(s.cfg.WorkspaceDir),
	})
	if err != nil {
		return RunResult{}, err
	}
	result := RunResult{
		SiteID:    flow.ID,
		Profile:   resolvedProfile,
		Action:    action,
		StepCount: len(steps.Steps),
		Success:   true,
		Message:   firstNonEmptyTrimmed(runResult.Message, fmt.Sprintf("flow action %s completed (%d steps)", action, len(steps.Steps))),
	}
	s.setLastAction(fmt.Sprintf("run site=%s action=%s", flow.ID, action))
	return result, nil
}

func (s *Service) loadFlow(siteID string) (SiteFlow, error) {
	all, err := LoadSiteFlows(s.cfg.SiteFlowsDir)
	if err != nil {
		return SiteFlow{}, err
	}
	target := strings.TrimSpace(strings.ToLower(siteID))
	if target == "" {
		return SiteFlow{}, fmt.Errorf("site_id is required")
	}
	for _, flow := range all {
		if strings.ToLower(strings.TrimSpace(flow.ID)) == target {
			if !flow.Enabled {
				return SiteFlow{}, fmt.Errorf("site flow is disabled: %s", flow.ID)
			}
			flow.Profile = strings.TrimSpace(strings.ToLower(flow.Profile))
			flow.AllowedHosts = normalizeAllowedHosts(flow.AllowedHosts)
			return flow, nil
		}
	}
	return SiteFlow{}, fmt.Errorf("site flow not found: %s", siteID)
}

func (s *Service) resolveProfileForFlow(requested string, flow SiteFlow) (string, error) {
	flowProfile := strings.TrimSpace(strings.ToLower(flow.Profile))
	if flowProfile == "" {
		return s.resolveProfile(requested), nil
	}
	resolvedFlowProfile := s.resolveProfile(flowProfile)
	requestedProfile := strings.TrimSpace(requested)
	if requestedProfile == "" {
		return resolvedFlowProfile, nil
	}
	resolvedRequested := s.resolveProfile(requestedProfile)
	if resolvedRequested != resolvedFlowProfile {
		return "", fmt.Errorf("profile %s is not allowed for site flow %s (required: %s)", resolvedRequested, flow.ID, resolvedFlowProfile)
	}
	return resolvedFlowProfile, nil
}

func (s *Service) profileUserDataDir(profile, siteID string) string {
	base := strings.TrimSpace(s.cfg.ManagedUserDataDir)
	if base == "" {
		base = filepath.Join(strings.TrimSpace(s.cfg.WorkspaceDir), "_shared", "browser", "managed")
	}
	profilePart := sanitizeBrowserPathComponent(profile)
	if profilePart == "" {
		profilePart = "managed"
	}
	sitePart := sanitizeBrowserPathComponent(siteID)
	if sitePart == "" {
		sitePart = "default"
	}
	return filepath.Join(base, profilePart, sitePart)
}

func (s *Service) checkURL(flow SiteFlow) string {
	if value := strings.TrimSpace(flow.URL); value != "" {
		return value
	}
	for _, key := range []string{"check", "status", "health"} {
		action, ok := flow.Actions[key]
		if !ok {
			continue
		}
		if value := firstOpenStep(action.Steps); value != "" {
			return value
		}
	}
	return ""
}

func firstOpenStep(steps []SiteStep) string {
	for _, step := range steps {
		if value := strings.TrimSpace(step.Open); value != "" {
			return value
		}
	}
	return ""
}

func firstNonEmptyTrimmed(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func sanitizeBrowserPathComponent(value string) string {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if trimmed == "" {
		return ""
	}
	replacer := strings.NewReplacer(
		"/", "-",
		"\\", "-",
		":", "-",
		" ", "-",
	)
	trimmed = replacer.Replace(trimmed)
	trimmed = strings.Trim(trimmed, "-.")
	return trimmed
}

func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func validateActionAllowedHosts(flow SiteFlow, action string, steps []SiteStep) error {
	if len(flow.AllowedHosts) == 0 {
		return nil
	}
	for index, step := range steps {
		rawURL := strings.TrimSpace(step.Open)
		if rawURL == "" {
			continue
		}
		parsed, err := url.Parse(rawURL)
		if err != nil {
			return fmt.Errorf("allowed_hosts policy rejected invalid open step URL for action %s step %d: %w", action, index+1, err)
		}
		host := strings.TrimSpace(strings.ToLower(parsed.Hostname()))
		if host == "" {
			return fmt.Errorf("allowed_hosts policy rejected open step URL without host for action %s step %d", action, index+1)
		}
		if !hostAllowed(host, flow.AllowedHosts) {
			return fmt.Errorf("allowed_hosts policy blocked open step host %s for site %s", host, flow.ID)
		}
	}
	return nil
}

func normalizeAllowedHosts(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		host := strings.TrimSpace(strings.ToLower(value))
		host = strings.TrimPrefix(host, "http://")
		host = strings.TrimPrefix(host, "https://")
		host = strings.TrimSuffix(host, "/")
		if host == "" {
			continue
		}
		if _, exists := seen[host]; exists {
			continue
		}
		seen[host] = struct{}{}
		out = append(out, host)
	}
	sort.Strings(out)
	return out
}

func hostAllowed(host string, allowlist []string) bool {
	normalizedHost := strings.TrimSpace(strings.ToLower(host))
	if normalizedHost == "" {
		return false
	}
	for _, pattern := range allowlist {
		candidate := strings.TrimSpace(strings.ToLower(pattern))
		if candidate == "" {
			continue
		}
		if candidate == normalizedHost {
			return true
		}
		if strings.HasPrefix(candidate, "*.") {
			suffix := strings.TrimPrefix(candidate, "*.")
			if suffix != "" && (normalizedHost == suffix || strings.HasSuffix(normalizedHost, "."+suffix)) {
				return true
			}
		}
	}
	return false
}
