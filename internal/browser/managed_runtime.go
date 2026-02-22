package browser

import (
	"context"
	"fmt"
	neturl "net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
)

const (
	managedStartTimeout  = 30 * time.Second
	managedActionTimeout = 20 * time.Second
)

type chromedpManagedRuntime struct {
	cfg     Config
	profile string

	mu            sync.Mutex
	started       bool
	allocCtx      context.Context
	allocCancel   context.CancelFunc
	browserCtx    context.Context
	browserCancel context.CancelFunc
}

func newChromedpManagedRuntime(cfg Config) managedRuntime {
	return &chromedpManagedRuntime{cfg: cfg, profile: "managed"}
}

func newChromedpRelayRuntime(cfg Config) managedRuntime {
	return &chromedpManagedRuntime{cfg: cfg, profile: "chrome"}
}

func (r *chromedpManagedRuntime) Start(_ context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started {
		return nil
	}

	switch strings.TrimSpace(strings.ToLower(r.profile)) {
	case "chrome":
		return r.startRelayProfile()
	default:
		return r.startManagedProfile()
	}
}

func (r *chromedpManagedRuntime) startManagedProfile() error {
	opts := []chromedp.ExecAllocatorOption{
		chromedp.NoDefaultBrowserCheck,
		chromedp.NoFirstRun,
		chromedp.Flag("disable-gpu", true),
	}
	if userDataDir := strings.TrimSpace(r.cfg.ManagedUserDataDir); userDataDir != "" {
		opts = append(opts, chromedp.UserDataDir(userDataDir))
	}
	if execPath := strings.TrimSpace(r.cfg.ManagedExecutablePath); execPath != "" {
		opts = append(opts, chromedp.ExecPath(execPath))
	}
	if r.cfg.ManagedHeadless {
		opts = append(opts, chromedp.Headless, chromedp.Flag("hide-scrollbars", true), chromedp.Flag("mute-audio", true))
	} else {
		opts = append(opts, chromedp.Flag("headless", false))
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	startCtx, startCancel := context.WithTimeout(browserCtx, managedStartTimeout)
	defer startCancel()
	if err := chromedp.Run(startCtx); err != nil {
		browserCancel()
		allocCancel()
		return fmt.Errorf("start managed browser: %w", err)
	}

	r.allocCtx = allocCtx
	r.allocCancel = allocCancel
	r.browserCtx = browserCtx
	r.browserCancel = browserCancel
	r.started = true
	return nil
}

func (r *chromedpManagedRuntime) startRelayProfile() error {
	relay := r.cfg.Relay
	if relay == nil {
		return fmt.Errorf("chrome relay is not configured")
	}
	if !relay.ExtensionConnected() {
		return fmt.Errorf("chrome relay extension is not connected")
	}
	rawURL := strings.TrimSpace(relay.CDPWebSocketURL())
	if rawURL == "" {
		return fmt.Errorf("chrome relay cdp url is not available")
	}
	wsURL, err := neturl.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("chrome relay cdp url is invalid: %w", err)
	}
	token := strings.TrimSpace(relay.RelayToken())
	if token != "" {
		values := wsURL.Query()
		if strings.TrimSpace(values.Get("token")) == "" {
			values.Set("token", token)
			wsURL.RawQuery = values.Encode()
		}
	}
	allocCtx, allocCancel := chromedp.NewRemoteAllocator(context.Background(), wsURL.String(), chromedp.NoModifyURL)
	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	startCtx, startCancel := context.WithTimeout(browserCtx, managedStartTimeout)
	defer startCancel()
	var handshake int
	if err := chromedp.Run(startCtx, chromedp.Evaluate(`1+1`, &handshake)); err != nil {
		browserCancel()
		allocCancel()
		return fmt.Errorf("start chrome relay browser: %w (relay cdp handshake failed: verify extension connection and relay protocol)", err)
	}
	if handshake != 2 {
		browserCancel()
		allocCancel()
		return fmt.Errorf("start chrome relay browser: relay cdp handshake failed: unexpected evaluate result %d", handshake)
	}
	r.allocCtx = allocCtx
	r.allocCancel = allocCancel
	r.browserCtx = browserCtx
	r.browserCancel = browserCancel
	r.started = true
	return nil
}

func (r *chromedpManagedRuntime) Stop(_ context.Context) error {
	r.mu.Lock()
	if !r.started {
		r.mu.Unlock()
		return nil
	}
	browserCancel := r.browserCancel
	allocCancel := r.allocCancel
	r.browserCancel = nil
	r.allocCancel = nil
	r.browserCtx = nil
	r.allocCtx = nil
	r.started = false
	r.mu.Unlock()

	if browserCancel != nil {
		browserCancel()
	}
	if allocCancel != nil {
		allocCancel()
	}
	return nil
}

func (r *chromedpManagedRuntime) Open(_ context.Context, rawURL string) error {
	browserCtx, err := r.requireStarted()
	if err != nil {
		return err
	}
	url := strings.TrimSpace(rawURL)
	if url == "" {
		return fmt.Errorf("url is required")
	}
	taskCtx, cancel := context.WithTimeout(browserCtx, managedActionTimeout)
	defer cancel()
	if err := chromedp.Run(taskCtx,
		chromedp.Navigate(url),
		chromedp.WaitReady("body", chromedp.ByQuery),
	); err != nil {
		return fmt.Errorf("open page: %w", err)
	}
	return nil
}

func (r *chromedpManagedRuntime) Snapshot(_ context.Context) (string, error) {
	browserCtx, err := r.requireStarted()
	if err != nil {
		return "", err
	}
	var bodyText string
	taskCtx, cancel := context.WithTimeout(browserCtx, managedActionTimeout)
	defer cancel()
	if err := chromedp.Run(taskCtx,
		chromedp.Evaluate(`(() => {
			const value = document && document.body ? document.body.innerText : "";
			return String(value || "").trim().slice(0, 1000);
		})()`, &bodyText),
	); err != nil {
		return "", fmt.Errorf("capture snapshot: %w", err)
	}
	bodyText = strings.TrimSpace(bodyText)
	if bodyText == "" {
		return "snapshot captured", nil
	}
	return bodyText, nil
}

func (r *chromedpManagedRuntime) Act(_ context.Context, action string, target string, value string) (string, error) {
	browserCtx, err := r.requireStarted()
	if err != nil {
		return "", err
	}
	normalizedAction := strings.TrimSpace(strings.ToLower(action))
	taskCtx, cancel := context.WithTimeout(browserCtx, managedActionTimeout)
	defer cancel()

	switch normalizedAction {
	case "":
		return "", fmt.Errorf("action is required")
	case "click":
		selector := strings.TrimSpace(target)
		if selector == "" {
			return "", fmt.Errorf("target selector is required for click")
		}
		if err := chromedp.Run(taskCtx, chromedp.Click(selector, chromedp.ByQuery)); err != nil {
			return "", fmt.Errorf("click action failed: %w", err)
		}
		return fmt.Sprintf("click target=%s", selector), nil
	case "type":
		selector := strings.TrimSpace(target)
		if selector == "" {
			return "", fmt.Errorf("target selector is required for type")
		}
		if err := chromedp.Run(taskCtx,
			chromedp.SetValue(selector, "", chromedp.ByQuery),
			chromedp.SendKeys(selector, strings.TrimSpace(value), chromedp.ByQuery),
		); err != nil {
			return "", fmt.Errorf("type action failed: %w", err)
		}
		return fmt.Sprintf("type target=%s", selector), nil
	case "wait":
		selector := strings.TrimSpace(target)
		if selector != "" {
			if err := chromedp.Run(taskCtx, chromedp.WaitVisible(selector, chromedp.ByQuery)); err != nil {
				return "", fmt.Errorf("wait action failed: %w", err)
			}
			return fmt.Sprintf("wait target=%s", selector), nil
		}
		waitFor := parseManagedWaitDuration(value)
		if err := chromedp.Run(taskCtx, chromedp.Sleep(waitFor)); err != nil {
			return "", fmt.Errorf("wait sleep failed: %w", err)
		}
		return fmt.Sprintf("wait duration=%s", waitFor.String()), nil
	case "evaluate":
		expr := strings.TrimSpace(value)
		if expr == "" {
			expr = strings.TrimSpace(target)
		}
		if expr == "" {
			return "", fmt.Errorf("javascript expression is required for evaluate")
		}
		var output any
		if err := chromedp.Run(taskCtx, chromedp.Evaluate(expr, &output)); err != nil {
			return "", fmt.Errorf("evaluate action failed: %w", err)
		}
		return fmt.Sprintf("evaluate result=%v", output), nil
	default:
		return "", fmt.Errorf("unsupported action: %s", normalizedAction)
	}
}

func (r *chromedpManagedRuntime) Screenshot(_ context.Context, path string) error {
	browserCtx, err := r.requireStarted()
	if err != nil {
		return err
	}
	targetPath := strings.TrimSpace(path)
	if targetPath == "" {
		return fmt.Errorf("screenshot path is required")
	}
	taskCtx, cancel := context.WithTimeout(browserCtx, managedActionTimeout)
	defer cancel()

	var png []byte
	if err := chromedp.Run(taskCtx, chromedp.CaptureScreenshot(&png)); err != nil {
		return fmt.Errorf("capture screenshot: %w", err)
	}
	if len(png) == 0 {
		return fmt.Errorf("capture screenshot: empty png payload")
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return fmt.Errorf("prepare screenshot directory: %w", err)
	}
	if err := os.WriteFile(targetPath, png, 0o644); err != nil {
		return fmt.Errorf("write screenshot: %w", err)
	}
	return nil
}

func (r *chromedpManagedRuntime) requireStarted() (context.Context, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.started || r.browserCtx == nil {
		return nil, fmt.Errorf("browser is not running")
	}
	return r.browserCtx, nil
}

func parseManagedWaitDuration(value string) time.Duration {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return time.Second
	}
	if d, err := time.ParseDuration(raw); err == nil && d > 0 {
		return d
	}
	if ms, err := strconv.Atoi(raw); err == nil && ms > 0 {
		return time.Duration(ms) * time.Millisecond
	}
	return time.Second
}
