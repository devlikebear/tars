package gateway

import (
	"context"
	"fmt"
	"strings"

	"github.com/devlikebear/tarsncase/internal/browser"
)

func (r *Runtime) BrowserStatus() BrowserState {
	if r == nil {
		return BrowserState{}
	}
	state := browser.State{}
	if r.browserService != nil {
		state = r.browserService.Status()
	}
	converted := toGatewayBrowserState(state)
	r.mu.Lock()
	r.browser = converted
	r.mu.Unlock()
	return converted
}

func (r *Runtime) BrowserProfiles() []BrowserProfile {
	if r == nil || r.browserService == nil {
		return []BrowserProfile{{Name: "managed", Driver: "chromedp", Default: true}}
	}
	source := r.browserService.Profiles()
	out := make([]BrowserProfile, 0, len(source))
	for _, item := range source {
		out = append(out, BrowserProfile{
			Name:               strings.TrimSpace(item.Name),
			Driver:             strings.TrimSpace(item.Driver),
			Default:            item.Default,
			Running:            item.Running,
			ExtensionConnected: item.ExtensionConnected,
		})
	}
	return out
}

func (r *Runtime) BrowserStart() BrowserState {
	return r.BrowserStartWithProfile("")
}

func (r *Runtime) BrowserStartWithProfile(profile string) BrowserState {
	if r == nil {
		return BrowserState{}
	}
	if r.browserService == nil {
		return BrowserState{}
	}
	state := toGatewayBrowserState(r.browserService.Start(profile))
	r.mu.Lock()
	r.browser = state
	r.stateVersion++
	r.mu.Unlock()
	r.persistSnapshot()
	return state
}

func (r *Runtime) BrowserStop() BrowserState {
	if r == nil {
		return BrowserState{}
	}
	if r.browserService == nil {
		return BrowserState{}
	}
	state := toGatewayBrowserState(r.browserService.Stop())
	r.mu.Lock()
	r.browser = state
	r.stateVersion++
	r.mu.Unlock()
	r.persistSnapshot()
	return state
}

func (r *Runtime) BrowserOpen(url string) (BrowserState, error) {
	if r == nil || r.browserService == nil {
		return BrowserState{}, fmt.Errorf("browser runtime is not configured")
	}
	state, err := r.browserService.Open(url)
	converted := toGatewayBrowserState(state)
	r.mu.Lock()
	r.browser = converted
	r.mu.Unlock()
	if err == nil {
		r.persistSnapshot()
	}
	return converted, err
}

func (r *Runtime) BrowserSnapshot() (BrowserState, error) {
	if r == nil || r.browserService == nil {
		return BrowserState{}, fmt.Errorf("browser runtime is not configured")
	}
	state, err := r.browserService.Snapshot()
	converted := toGatewayBrowserState(state)
	r.mu.Lock()
	r.browser = converted
	r.mu.Unlock()
	if err == nil {
		r.persistSnapshot()
	}
	return converted, err
}

func (r *Runtime) BrowserAct(action string, target string, value string) (BrowserState, error) {
	if r == nil || r.browserService == nil {
		return BrowserState{}, fmt.Errorf("browser runtime is not configured")
	}
	state, err := r.browserService.Act(action, target, value)
	converted := toGatewayBrowserState(state)
	r.mu.Lock()
	r.browser = converted
	r.mu.Unlock()
	if err == nil {
		r.persistSnapshot()
	}
	return converted, err
}

func (r *Runtime) BrowserScreenshot(name string) (BrowserState, error) {
	if r == nil || r.browserService == nil {
		return BrowserState{}, fmt.Errorf("browser runtime is not configured")
	}
	state, err := r.browserService.Screenshot(name)
	converted := toGatewayBrowserState(state)
	r.mu.Lock()
	r.browser = converted
	r.mu.Unlock()
	if err == nil {
		r.persistSnapshot()
	}
	return converted, err
}

func (r *Runtime) BrowserLogin(ctx context.Context, siteID string, profile string) (browser.LoginResult, error) {
	if r == nil || r.browserService == nil {
		return browser.LoginResult{}, fmt.Errorf("browser runtime is not configured")
	}
	result, err := r.browserService.Login(ctx, siteID, profile)
	r.BrowserStatus()
	return result, err
}

func (r *Runtime) BrowserCheck(ctx context.Context, siteID string, profile string) (browser.CheckResult, error) {
	if r == nil || r.browserService == nil {
		return browser.CheckResult{}, fmt.Errorf("browser runtime is not configured")
	}
	result, err := r.browserService.Check(ctx, siteID, profile)
	r.BrowserStatus()
	return result, err
}

func (r *Runtime) BrowserRun(ctx context.Context, siteID string, flowAction string, profile string) (browser.RunResult, error) {
	if r == nil || r.browserService == nil {
		return browser.RunResult{}, fmt.Errorf("browser runtime is not configured")
	}
	result, err := r.browserService.Run(ctx, siteID, flowAction, profile)
	r.BrowserStatus()
	return result, err
}

func toGatewayBrowserState(source browser.State) BrowserState {
	return BrowserState{
		Running:            source.Running,
		Profile:            strings.TrimSpace(source.Profile),
		Driver:             strings.TrimSpace(source.Driver),
		CurrentURL:         source.CurrentURL,
		LastSnapshot:       source.LastSnapshot,
		LastAction:         source.LastAction,
		LastScreenshot:     source.LastScreenshot,
		ExtensionConnected: source.ExtensionConnected,
		AttachedTabs:       source.AttachedTabs,
		LastError:          source.LastError,
	}
}
