package browser

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

type playwrightManagedRuntime struct {
	cfg Config

	mu         sync.Mutex
	started    bool
	currentURL string
}

func newPlaywrightManagedRuntime(cfg Config) managedRuntime {
	return &playwrightManagedRuntime{cfg: cfg}
}

func (r *playwrightManagedRuntime) Start(_ context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.started = true
	return nil
}

func (r *playwrightManagedRuntime) Stop(_ context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.started = false
	r.currentURL = ""
	return nil
}

func (r *playwrightManagedRuntime) Open(ctx context.Context, rawURL string) error {
	url := strings.TrimSpace(rawURL)
	if url == "" {
		return fmt.Errorf("url is required")
	}
	r.mu.Lock()
	started := r.started
	r.mu.Unlock()
	if !started {
		return fmt.Errorf("browser is not running")
	}
	response, err := runPlaywrightRequest(ctx, flowRunRequest{
		Mode:           "open",
		URL:            url,
		Headless:       r.cfg.ManagedHeadless,
		ExecutablePath: strings.TrimSpace(r.cfg.ManagedExecutablePath),
		UserDataDir:    strings.TrimSpace(r.cfg.ManagedUserDataDir),
		WorkspaceDir:   strings.TrimSpace(r.cfg.WorkspaceDir),
	})
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.currentURL = firstNonEmptyTrimmed(response.CurrentURL, url)
	r.mu.Unlock()
	return nil
}

func (r *playwrightManagedRuntime) Snapshot(ctx context.Context) (string, error) {
	url, err := r.requireURL()
	if err != nil {
		return "", err
	}
	response, err := runPlaywrightRequest(ctx, flowRunRequest{
		Mode:           "snapshot",
		URL:            url,
		Headless:       r.cfg.ManagedHeadless,
		ExecutablePath: strings.TrimSpace(r.cfg.ManagedExecutablePath),
		UserDataDir:    strings.TrimSpace(r.cfg.ManagedUserDataDir),
		WorkspaceDir:   strings.TrimSpace(r.cfg.WorkspaceDir),
	})
	if err != nil {
		return "", err
	}
	return firstNonEmptyTrimmed(response.Snapshot, response.Message, "snapshot captured"), nil
}

func (r *playwrightManagedRuntime) Act(ctx context.Context, action string, target string, value string) (string, error) {
	url, err := r.requireURL()
	if err != nil {
		return "", err
	}
	response, err := runPlaywrightRequest(ctx, flowRunRequest{
		Mode:           "act",
		URL:            url,
		Action:         strings.TrimSpace(action),
		Target:         strings.TrimSpace(target),
		Value:          strings.TrimSpace(value),
		Headless:       r.cfg.ManagedHeadless,
		ExecutablePath: strings.TrimSpace(r.cfg.ManagedExecutablePath),
		UserDataDir:    strings.TrimSpace(r.cfg.ManagedUserDataDir),
		WorkspaceDir:   strings.TrimSpace(r.cfg.WorkspaceDir),
	})
	if err != nil {
		return "", err
	}
	if nextURL := strings.TrimSpace(response.CurrentURL); nextURL != "" {
		r.mu.Lock()
		r.currentURL = nextURL
		r.mu.Unlock()
	}
	return firstNonEmptyTrimmed(response.LastAction, response.Message, strings.TrimSpace(action)), nil
}

func (r *playwrightManagedRuntime) Screenshot(ctx context.Context, path string) error {
	url, err := r.requireURL()
	if err != nil {
		return err
	}
	_, err = runPlaywrightRequest(ctx, flowRunRequest{
		Mode:           "screenshot",
		URL:            url,
		ScreenshotPath: strings.TrimSpace(path),
		Headless:       r.cfg.ManagedHeadless,
		ExecutablePath: strings.TrimSpace(r.cfg.ManagedExecutablePath),
		UserDataDir:    strings.TrimSpace(r.cfg.ManagedUserDataDir),
		WorkspaceDir:   strings.TrimSpace(r.cfg.WorkspaceDir),
	})
	return err
}

func (r *playwrightManagedRuntime) requireURL() (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.started {
		return "", fmt.Errorf("browser is not running")
	}
	if strings.TrimSpace(r.currentURL) == "" {
		return "", fmt.Errorf("browser current url is not set")
	}
	return r.currentURL, nil
}
