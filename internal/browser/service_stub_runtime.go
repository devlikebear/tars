package browser

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

func (s *Service) Status() State {
	s.mu.RLock()
	state := s.state
	s.mu.RUnlock()
	state.ExtensionConnected = s.relayConnected()
	state.AttachedTabs = s.relayAttachedTabs()
	return state
}

func (s *Service) Profiles() []Profile {
	status := s.Status()
	profiles := []Profile{
		{Name: "managed", Driver: "chromedp", Default: status.Profile == "managed", Running: status.Running && status.Profile == "managed"},
		{Name: "chrome", Driver: "relay", Default: status.Profile == "chrome", Running: status.Running && status.Profile == "chrome", ExtensionConnected: status.ExtensionConnected},
	}
	return profiles
}

func (s *Service) Start(profile string) State {
	resolved := s.resolveProfile(profile)
	s.mu.RLock()
	currentProfile := strings.TrimSpace(s.state.Profile)
	wasRunning := s.state.Running
	s.mu.RUnlock()
	if wasRunning {
		currentRuntime := s.runtimeForProfile(currentProfile)
		targetRuntime := s.runtimeForProfile(resolved)
		if currentRuntime == targetRuntime {
			_ = targetRuntime.Stop(context.Background())
		} else {
			_ = currentRuntime.Stop(context.Background())
		}
	}
	runtime := s.runtimeForProfile(resolved)
	err := runtime.Start(context.Background())
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Profile = resolved
	s.state.Driver = driverForProfile(resolved)
	s.state.ExtensionConnected = s.relayConnected()
	if err != nil {
		s.state.Running = false
		s.state.AttachedTabs = 0
		s.state.LastError = err.Error()
		s.state.LastAction = "start"
		return s.state
	}
	s.state.Running = true
	s.state.AttachedTabs = s.relayAttachedTabs()
	s.state.LastError = ""
	s.state.LastAction = "start"
	return s.state
}

func (s *Service) Stop() State {
	_ = s.getManagedRuntime().Stop(context.Background())
	_ = s.getChromeRuntime().Stop(context.Background())

	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Running = false
	s.state.LastError = ""
	s.state.ExtensionConnected = s.relayConnected()
	s.state.AttachedTabs = s.relayAttachedTabs()
	s.state.LastAction = "stop"
	return s.state
}

func (s *Service) Open(rawURL string) (State, error) {
	s.mu.RLock()
	state := s.state
	s.mu.RUnlock()

	if !state.Running {
		return state, fmt.Errorf("browser is not running")
	}
	url := strings.TrimSpace(rawURL)
	if url == "" {
		return state, fmt.Errorf("url is required")
	}
	if err := s.runBrowserOperationWithRecovery(state.Profile, func(runtime managedRuntime) error {
		return runtime.Open(context.Background(), url)
	}); err != nil {
		return s.setBrowserError(err.Error())
	}
	s.mu.Lock()
	s.state.CurrentURL = url
	s.state.LastError = ""
	s.state.LastAction = "open"
	state = s.state
	s.mu.Unlock()
	return state, nil
}

func (s *Service) Snapshot() (State, error) {
	s.mu.RLock()
	state := s.state
	s.mu.RUnlock()
	if !state.Running {
		return state, fmt.Errorf("browser is not running")
	}
	var summary string
	err := s.runBrowserOperationWithRecovery(state.Profile, func(runtime managedRuntime) error {
		value, opErr := runtime.Snapshot(context.Background())
		if opErr != nil {
			return opErr
		}
		summary = value
		return nil
	})
	if err != nil {
		return s.setBrowserError(err.Error())
	}
	s.mu.Lock()
	if strings.TrimSpace(summary) == "" {
		s.state.LastSnapshot = "snapshot captured"
	} else {
		s.state.LastSnapshot = strings.TrimSpace(summary)
	}
	s.state.LastError = ""
	s.state.LastAction = "snapshot"
	state = s.state
	s.mu.Unlock()
	return state, nil
}

func (s *Service) Act(action string, target string, value string) (State, error) {
	s.mu.RLock()
	state := s.state
	s.mu.RUnlock()
	if !state.Running {
		return state, fmt.Errorf("browser is not running")
	}
	var lastAction string
	err := s.runBrowserOperationWithRecovery(state.Profile, func(runtime managedRuntime) error {
		value, opErr := runtime.Act(context.Background(), action, target, value)
		if opErr != nil {
			return opErr
		}
		lastAction = value
		return nil
	})
	if err != nil {
		return s.setBrowserError(err.Error())
	}
	s.mu.Lock()
	s.state.LastAction = strings.TrimSpace(lastAction)
	s.state.LastError = ""
	state = s.state
	s.mu.Unlock()
	return state, nil
}

func (s *Service) Screenshot(name string) (State, error) {
	s.mu.RLock()
	state := s.state
	workspaceDir := strings.TrimSpace(s.cfg.WorkspaceDir)
	s.mu.RUnlock()
	if !state.Running {
		return state, fmt.Errorf("browser is not running")
	}
	targetPath := resolveScreenshotPath(workspaceDir, name)
	if err := s.runBrowserOperationWithRecovery(state.Profile, func(runtime managedRuntime) error {
		return runtime.Screenshot(context.Background(), targetPath)
	}); err != nil {
		return s.setBrowserError(err.Error())
	}
	s.mu.Lock()
	s.state.LastScreenshot = targetPath
	s.state.LastError = ""
	s.state.LastAction = "screenshot"
	state = s.state
	s.mu.Unlock()
	return state, nil
}

func (s *Service) runBrowserOperationWithRecovery(profile string, op func(runtime managedRuntime) error) error {
	if s == nil || op == nil {
		return fmt.Errorf("browser operation is not available")
	}
	runtime := s.runtimeForProfile(profile)
	err := op(runtime)
	if err == nil {
		return nil
	}
	if !shouldRecoverBrowserRuntimeError(profile, err) {
		return err
	}
	_ = runtime.Stop(context.Background())
	if restartErr := runtime.Start(context.Background()); restartErr != nil {
		return fmt.Errorf("%w (recovery restart failed: %v)", err, restartErr)
	}
	if retryErr := op(runtime); retryErr != nil {
		return retryErr
	}
	return nil
}

func shouldRecoverBrowserRuntimeError(profile string, err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	if message == "" {
		return false
	}
	return strings.Contains(message, "context canceled") ||
		strings.Contains(message, "connection reset by peer") ||
		strings.Contains(message, "websocket: close") ||
		strings.Contains(message, "target closed")
}

func resolveScreenshotPath(workspaceDir, name string) string {
	base := strings.TrimSpace(name)
	if base == "" {
		base = fmt.Sprintf("shot_%d.png", time.Now().UnixNano())
	}
	ext := strings.TrimSpace(strings.ToLower(filepath.Ext(base)))
	if ext != ".png" {
		stem := strings.TrimSuffix(base, filepath.Ext(base))
		stem = strings.TrimSpace(stem)
		if stem == "" {
			stem = fmt.Sprintf("shot_%d", time.Now().UnixNano())
		}
		base = stem + ".png"
	}
	if strings.TrimSpace(workspaceDir) == "" || filepath.IsAbs(base) {
		return base
	}
	dir := filepath.Join(workspaceDir, "_shared", "browser")
	return filepath.Join(dir, base)
}

func (s *Service) setBrowserError(message string) (State, error) {
	err := errors.New(strings.TrimSpace(message))
	s.mu.Lock()
	s.state.LastError = err.Error()
	state := s.state
	s.mu.Unlock()
	return state, err
}

func (s *Service) getManagedRuntime() managedRuntime {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.managed == nil {
		s.managed = newChromedpManagedRuntime(s.cfg)
	}
	return s.managed
}

func (s *Service) getChromeRuntime() managedRuntime {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.chrome == nil {
		s.chrome = newChromedpRelayRuntime(s.cfg)
	}
	return s.chrome
}

func (s *Service) runtimeForProfile(profile string) managedRuntime {
	if strings.TrimSpace(strings.ToLower(profile)) == "chrome" {
		return s.getChromeRuntime()
	}
	return s.getManagedRuntime()
}

func (s *Service) resolveProfile(profile string) string {
	candidate := strings.TrimSpace(strings.ToLower(profile))
	if candidate == "" {
		candidate = strings.TrimSpace(strings.ToLower(s.cfg.DefaultProfile))
	}
	if candidate == "" {
		candidate = "managed"
	}
	switch candidate {
	case "managed", "chrome":
		return candidate
	default:
		return "managed"
	}
}

func (s *Service) setLastAction(action string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.LastAction = action
	s.state.ExtensionConnected = s.relayConnected()
	s.state.AttachedTabs = s.relayAttachedTabs()
}

func driverForProfile(profile string) string {
	if strings.TrimSpace(strings.ToLower(profile)) == "chrome" {
		return "relay"
	}
	return "chromedp"
}

func (s *Service) relayConnected() bool {
	if s == nil || s.cfg.Relay == nil {
		return false
	}
	return s.cfg.Relay.ExtensionConnected()
}

func (s *Service) relayAttachedTabs() int {
	if s == nil || s.cfg.Relay == nil {
		return 0
	}
	return s.cfg.Relay.AttachedTabs()
}
