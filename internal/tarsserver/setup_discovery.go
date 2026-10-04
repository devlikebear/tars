package tarsserver

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/devlikebear/tars/internal/auth"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/llmdefaults"
)

type setupCandidate struct {
	Kind        string            `json:"kind"`
	Installed   bool              `json:"installed"`
	Ready       bool              `json:"ready"`
	Problem     string            `json:"problem,omitempty"`
	CLIPath     string            `json:"cli_path,omitempty"`
	Version     string            `json:"version,omitempty"`
	Source      string            `json:"source,omitempty"`
	Models      []string          `json:"models"`
	Recommended map[string]string `json:"recommended"`
}
type setupDiscoveryResponse struct {
	Candidates []setupCandidate `json:"candidates"`
}
type setupDiscovery struct {
	service         *providerModelsService
	findCodex       func() (string, error)
	codexCredential func() (auth.ProviderCredential, error)
	codexModels     func(context.Context) (modelsAPIInfo, error)
}

func newSetupDiscovery(service *providerModelsService) *setupDiscovery {
	d := &setupDiscovery{service: service, findCodex: findLocalCodex}
	d.codexCredential = func() (auth.ProviderCredential, error) {
		return auth.ResolveProviderCredential(auth.ProviderAuthConfig{Provider: llmdefaults.ProviderOpenAICodex, AuthMode: "oauth"})
	}
	d.codexModels = d.models
	return d
}
func findLocalCodex() (string, error) {
	if path, err := exec.LookPath("codex"); err == nil {
		return path, nil
	}
	home, _ := os.UserHomeDir()
	for _, dir := range []string{filepath.Join(home, ".local", "bin"), "/opt/homebrew/bin", "/usr/local/bin"} {
		if path, err := exec.LookPath(filepath.Join(dir, "codex")); err == nil {
			return path, nil
		}
	}
	return "", exec.ErrNotFound
}
func (d *setupDiscovery) models(ctx context.Context) (modelsAPIInfo, error) {
	models, err := d.service.fetcher.FetchModels(ctx, llm.ProviderOptions{Provider: llmdefaults.ProviderOpenAICodex, AuthMode: "oauth", OAuthProvider: llmdefaults.OpenAICodexOAuthProvider, BaseURL: llmdefaults.OpenAICodexBaseURL})
	if err == nil && len(models) > 0 {
		if d.service.cache != nil {
			_ = d.service.cache.put(llmdefaults.ProviderOpenAICodex, llmdefaults.OpenAICodexBaseURL, "oauth", models, d.service.nowFn())
		}
		return modelsAPIInfo{Source: "live", Models: models}, nil
	}
	if d.service.cache != nil && !setupAuthRejected(err) {
		if entry, ok := d.service.cache.get(llmdefaults.ProviderOpenAICodex, llmdefaults.OpenAICodexBaseURL, "oauth"); ok && len(entry.Models) > 0 {
			return modelsAPIInfo{Source: "cache", Stale: true, Models: entry.Models}, nil
		}
	}
	return modelsAPIInfo{Source: "fallback", Models: []string{llmdefaults.OpenAICodexModel}}, err
}
func (d *setupDiscovery) discover(ctx context.Context) setupDiscoveryResponse {
	ctx, cancel := context.WithTimeout(ctx, providerProbeTimeout)
	defer cancel()
	candidates := make([]setupCandidate, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		probe := d.service.probeClaudeCodeCLI(ctx, providerProbeResult{Kind: llmdefaults.ProviderClaudeCodeCLI})
		candidates[0] = setupCandidate{Kind: probe.Kind, Installed: probe.CLIPath != "", Ready: probe.Status == providerProbeOK, Problem: probe.Problem, CLIPath: probe.CLIPath, Version: probe.Version, Source: "cli", Models: []string{"opus", "sonnet", "haiku"}, Recommended: map[string]string{"heavy": "opus", "standard": "sonnet", "light": "haiku"}}
	}()
	go func() {
		defer wg.Done()
		candidate := setupCandidate{Kind: llmdefaults.ProviderOpenAICodex, Models: []string{}, Recommended: map[string]string{}}
		path, err := d.findCodex()
		candidate.Installed = err == nil
		candidate.CLIPath = path
		if candidate.Installed {
			if out, runErr := d.service.runCLI(ctx, path, "--version"); runErr == nil {
				candidate.Version = parseCLIVersion(string(out))
			}
		}
		cred, authErr := d.codexCredential()
		if authErr != nil || strings.TrimSpace(cred.AccessToken) == "" {
			candidate.Problem = providerProbeNotLoggedIn
			if !candidate.Installed {
				candidate.Problem = providerProbeCLIMissing
			}
			candidates[1] = candidate
			return
		}
		info, modelErr := d.codexModels(ctx)
		candidate.Ready = true
		candidate.Models = info.Models
		candidate.Source = info.Source
		candidate.Recommended = recommendCodexModels(info.Models)
		if modelErr != nil {
			candidate.Problem = providerProbeModelsFailed
			if setupAuthRejected(modelErr) {
				candidate.Ready = false
				candidate.Problem = providerProbeNotLoggedIn
			}
		}
		candidates[1] = candidate
	}()
	wg.Wait()
	return setupDiscoveryResponse{Candidates: candidates}
}

// recommendCodexModels keeps backend ordering: the first visible model is
// its recommendation. The largest variant is preferred for heavy work and
// smaller ones for light work.
func recommendCodexModels(models []string) map[string]string {
	out := map[string]string{}
	if len(models) == 0 {
		return out
	}
	out["heavy"], out["standard"], out["light"] = models[0], models[0], models[0]
	for _, model := range models {
		if strings.Contains(model, "astra") {
			out["heavy"] = model
			break
		}
	}
	for _, model := range models {
		if strings.Contains(model, "sol") {
			out["standard"] = model
			break
		}
	}
	for _, model := range models {
		if strings.Contains(model, "luna") || strings.Contains(model, "mini") {
			out["light"] = model
			break
		}
	}
	return out
}

func setupAuthRejected(err error) bool {
	var providerErr *llm.ProviderError
	return errors.As(err, &providerErr) && (providerErr.StatusCode == http.StatusUnauthorized || providerErr.StatusCode == http.StatusForbidden)
}
