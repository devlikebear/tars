package gateway

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (r *Runtime) BrowserStatus() BrowserState {
	if r == nil {
		return BrowserState{}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.browser
}

func (r *Runtime) BrowserStart() BrowserState {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.browser.Running = true
	return r.browser
}

func (r *Runtime) BrowserStop() BrowserState {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.browser.Running = false
	return r.browser
}

func (r *Runtime) BrowserOpen(url string) (BrowserState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.browser.Running {
		return r.browser, fmt.Errorf("browser is not running")
	}
	url = strings.TrimSpace(url)
	if url == "" {
		return r.browser, fmt.Errorf("url is required")
	}
	r.browser.CurrentURL = url
	r.browser.LastAction = "open"
	return r.browser, nil
}

func (r *Runtime) BrowserSnapshot() (BrowserState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.browser.Running {
		return r.browser, fmt.Errorf("browser is not running")
	}
	if strings.TrimSpace(r.browser.CurrentURL) == "" {
		r.browser.LastSnapshot = "no page opened"
	} else {
		r.browser.LastSnapshot = fmt.Sprintf("snapshot captured for %s", r.browser.CurrentURL)
	}
	r.browser.LastAction = "snapshot"
	return r.browser, nil
}

func (r *Runtime) BrowserAct(action string, target string, value string) (BrowserState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.browser.Running {
		return r.browser, fmt.Errorf("browser is not running")
	}
	action = strings.TrimSpace(action)
	if action == "" {
		return r.browser, fmt.Errorf("action is required")
	}
	r.browser.LastAction = fmt.Sprintf("%s target=%s value=%s", action, strings.TrimSpace(target), strings.TrimSpace(value))
	return r.browser, nil
}

func (r *Runtime) BrowserScreenshot(name string) (BrowserState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.browser.Running {
		return r.browser, fmt.Errorf("browser is not running")
	}
	base := strings.TrimSpace(name)
	if base == "" {
		base = fmt.Sprintf("shot_%d.txt", r.nowFn().UnixNano())
	}
	file := base
	if r.opts.WorkspaceDir != "" {
		dir := filepath.Join(r.opts.WorkspaceDir, "_shared", "browser")
		if err := os.MkdirAll(dir, 0o755); err == nil {
			file = filepath.Join(dir, base)
			_ = os.WriteFile(file, []byte("browser screenshot placeholder\nurl="+r.browser.CurrentURL+"\n"), 0o644)
		}
	}
	r.browser.LastScreenshot = file
	r.browser.LastAction = "screenshot"
	return r.browser, nil
}
