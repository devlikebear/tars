package browser

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (s *Service) Status() State {
	s.mu.RLock()
	state := s.state
	s.mu.RUnlock()
	state.ExtensionConnected = s.relayConnected()
	if state.ExtensionConnected {
		state.AttachedTabs = 1
	}
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
	s.mu.Lock()
	defer s.mu.Unlock()
	resolved := s.resolveProfile(profile)
	s.state.Running = true
	s.state.Profile = resolved
	s.state.Driver = driverForProfile(resolved)
	s.state.LastError = ""
	s.state.ExtensionConnected = s.relayConnected()
	if s.state.ExtensionConnected {
		s.state.AttachedTabs = 1
	} else {
		s.state.AttachedTabs = 0
	}
	s.state.LastAction = "start"
	return s.state
}

func (s *Service) Stop() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Running = false
	s.state.ExtensionConnected = s.relayConnected()
	s.state.AttachedTabs = 0
	s.state.LastAction = "stop"
	return s.state
}

func (s *Service) Open(rawURL string) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.state.Running {
		return s.state, fmt.Errorf("browser is not running")
	}
	url := strings.TrimSpace(rawURL)
	if url == "" {
		return s.state, fmt.Errorf("url is required")
	}
	s.state.CurrentURL = url
	s.state.LastAction = "open"
	return s.state, nil
}

func (s *Service) Snapshot() (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.state.Running {
		return s.state, fmt.Errorf("browser is not running")
	}
	if strings.TrimSpace(s.state.CurrentURL) == "" {
		s.state.LastSnapshot = "no page opened"
	} else {
		s.state.LastSnapshot = fmt.Sprintf("snapshot captured for %s", s.state.CurrentURL)
	}
	s.state.LastAction = "snapshot"
	return s.state, nil
}

func (s *Service) Act(action string, target string, value string) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.state.Running {
		return s.state, fmt.Errorf("browser is not running")
	}
	trimmed := strings.TrimSpace(action)
	if trimmed == "" {
		return s.state, fmt.Errorf("action is required")
	}
	s.state.LastAction = fmt.Sprintf("%s target=%s value=%s", trimmed, strings.TrimSpace(target), strings.TrimSpace(value))
	return s.state, nil
}

func (s *Service) Screenshot(name string) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.state.Running {
		return s.state, fmt.Errorf("browser is not running")
	}
	base := strings.TrimSpace(name)
	if base == "" {
		base = fmt.Sprintf("shot_%d.txt", time.Now().UnixNano())
	}
	file := base
	if strings.TrimSpace(s.cfg.WorkspaceDir) != "" {
		dir := filepath.Join(s.cfg.WorkspaceDir, "_shared", "browser")
		if err := os.MkdirAll(dir, 0o755); err == nil {
			file = filepath.Join(dir, base)
			_ = os.WriteFile(file, []byte("browser screenshot placeholder\nurl="+s.state.CurrentURL+"\n"), 0o644)
		}
	}
	s.state.LastScreenshot = file
	s.state.LastAction = "screenshot"
	return s.state, nil
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
	if s.state.ExtensionConnected {
		s.state.AttachedTabs = 1
	}
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
