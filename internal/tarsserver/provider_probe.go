package tarsserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/devlikebear/tars/internal/llmdefaults"
)

// The Config page's "Test connection" probes every provider alias in the
// pool. HTTP providers are tested by listing their models. CLI providers
// have no model list worth asking for, so the probe checks what a turn needs
// without spending usage: the binary, its version, and its sign-in.

const (
	providerProbeOK    = "ok"
	providerProbeInfo  = "info"
	providerProbeWarn  = "warn"
	providerProbeError = "error"

	providerProbeCLIMissing     = "cli_missing"
	providerProbeNotLoggedIn    = "not_logged_in"
	providerProbeAuthUnknown    = "auth_unknown"
	providerProbeAuthFailed     = "auth_check_failed"
	providerProbeVersionOld     = "version_old"
	providerProbeModelsFailed   = "models_failed"
	providerProbeModelsWarning  = "models_warning"
	providerProbeModelsNoneSeen = "models_empty"

	// antigravityCLIMinVersion is the first agy that reports tool_info and
	// cache_read_tokens; older ones run turns but leave both silently empty.
	antigravityCLIMinVersion = "1.1.12"

	providerProbeCLITimeout = 15 * time.Second
	providerProbeTimeout    = 30 * time.Second
	providerProbeOutputCap  = 400
)

type providerProbeResult struct {
	Alias   string `json:"alias"`
	Kind    string `json:"kind"`
	Default bool   `json:"default"`
	// Status is ok, info (configured but not fully verifiable), warn
	// (works with a caveat) or error (a turn would fail).
	Status string `json:"status"`
	// Problem names what the status is about, for the console to phrase.
	Problem    string `json:"problem,omitempty"`
	Detail     string `json:"detail,omitempty"`
	CLIPath    string `json:"cli_path,omitempty"`
	Version    string `json:"version,omitempty"`
	MinVersion string `json:"min_version,omitempty"`
	AuthMethod string `json:"auth_method,omitempty"`
	ModelCount int    `json:"model_count,omitempty"`
}

type providerProbeResponse struct {
	Results []providerProbeResult `json:"results"`
}

// providerCLIRunner runs a CLI and returns its combined stdout; stderr is
// folded into the error.
type providerCLIRunner func(ctx context.Context, path string, args ...string) ([]byte, error)

func runProviderCLI(ctx context.Context, path string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, providerProbeCLITimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, args...) // NOSONAR: the binary is the provider's own CLI resolved like a chat turn does; the arguments are fixed.
	cmd.Env = append(os.Environ(), "NO_COLOR=1")
	cmd.WaitDelay = releaseKillGrace
	killProcessGroupOnCancel(cmd, releaseKillGrace)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return stdout.Bytes(), fmt.Errorf("%s timed out after %s", strings.Join(args, " "), providerProbeCLITimeout)
	}
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if msg == "" {
			return stdout.Bytes(), err
		}
		return stdout.Bytes(), fmt.Errorf("%w: %s", err, clipProbeOutput(msg))
	}
	return stdout.Bytes(), nil
}

// probeAll tests every alias in the pool concurrently, the default tier's
// alias first and the rest by name.
func (s *providerModelsService) probeAll(ctx context.Context) providerProbeResponse {
	ctx, cancel := context.WithTimeout(ctx, providerProbeTimeout)
	defer cancel()
	defaultAlias := ""
	if resolved, ok := s.defaultResolved(); ok {
		defaultAlias = resolved.ProviderAlias
	}
	aliases := make([]string, 0, len(s.cfg.LLMProviders))
	for alias := range s.cfg.LLMProviders {
		aliases = append(aliases, alias)
	}
	sort.Slice(aliases, func(i, j int) bool {
		if (aliases[i] == defaultAlias) != (aliases[j] == defaultAlias) {
			return aliases[i] == defaultAlias
		}
		return aliases[i] < aliases[j]
	})
	results := make([]providerProbeResult, len(aliases))
	var wg sync.WaitGroup
	for i, alias := range aliases {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result := s.probe(ctx, alias)
			result.Default = alias == defaultAlias
			results[i] = result
		}()
	}
	wg.Wait()
	return providerProbeResponse{Results: results}
}

func (s *providerModelsService) probe(ctx context.Context, alias string) providerProbeResult {
	kind := normalizeProviderValue(s.cfg.LLMProviders[alias].Kind)
	result := providerProbeResult{Alias: alias, Kind: kind}
	switch kind {
	case llmdefaults.ProviderClaudeCodeCLI:
		return s.probeClaudeCodeCLI(ctx, result)
	case llmdefaults.ProviderAntigravityCLI:
		return s.probeAntigravityCLI(ctx, result)
	}
	info, err := s.models(ctx, alias)
	switch {
	case err != nil:
		result.Status, result.Problem, result.Detail = providerProbeError, providerProbeModelsFailed, clipProbeOutput(err.Error())
	case strings.TrimSpace(info.Warning) != "":
		result.Status, result.Problem, result.Detail = providerProbeWarn, providerProbeModelsWarning, clipProbeOutput(info.Warning)
		result.ModelCount = len(info.Models)
	case len(info.Models) == 0:
		result.Status, result.Problem = providerProbeWarn, providerProbeModelsNoneSeen
	default:
		result.Status, result.ModelCount = providerProbeOK, len(info.Models)
	}
	return result
}

func (s *providerModelsService) probeCLIPath(result *providerProbeResult) bool {
	path, err := s.findCLI(result.Kind)
	if err != nil {
		result.Status, result.Problem, result.Detail = providerProbeError, providerProbeCLIMissing, clipProbeOutput(err.Error())
		return false
	}
	result.CLIPath = path
	return true
}

// probeClaudeCodeCLI asks `claude auth status`, which reads the local login
// without a model call. A CLI too old to have the subcommand is reported as
// unverified rather than broken.
func (s *providerModelsService) probeClaudeCodeCLI(ctx context.Context, result providerProbeResult) providerProbeResult {
	if !s.probeCLIPath(&result) {
		return result
	}
	if out, err := s.runCLI(ctx, result.CLIPath, "--version"); err == nil {
		result.Version = parseCLIVersion(string(out))
	}
	out, err := s.runCLI(ctx, result.CLIPath, "auth", "status", "--json")
	var status struct {
		LoggedIn   *bool  `json:"loggedIn"`
		AuthMethod string `json:"authMethod"`
	}
	// A logged-out CLI exits non-zero but still prints the JSON.
	if jsonErr := json.Unmarshal(bytes.TrimSpace(out), &status); jsonErr != nil || status.LoggedIn == nil {
		result.Status, result.Problem = providerProbeInfo, providerProbeAuthUnknown
		if err != nil {
			result.Detail = clipProbeOutput(err.Error())
		}
		return result
	}
	result.AuthMethod = strings.TrimSpace(status.AuthMethod)
	if !*status.LoggedIn {
		result.Status, result.Problem = providerProbeError, providerProbeNotLoggedIn
		return result
	}
	result.Status = providerProbeOK
	return result
}

// probeAntigravityCLI checks the version floor and runs `agy models`, which
// needs the CLI's own Google sign-in but spends no usage.
func (s *providerModelsService) probeAntigravityCLI(ctx context.Context, result providerProbeResult) providerProbeResult {
	if !s.probeCLIPath(&result) {
		return result
	}
	if out, err := s.runCLI(ctx, result.CLIPath, "--version"); err == nil {
		result.Version = parseCLIVersion(string(out))
	}
	out, err := s.runCLI(ctx, result.CLIPath, "models")
	if err != nil {
		result.Status, result.Problem, result.Detail = providerProbeError, providerProbeAuthFailed, clipProbeOutput(err.Error())
		return result
	}
	result.ModelCount = countAgyModels(string(out))
	if result.Version != "" && compareCLIVersions(result.Version, antigravityCLIMinVersion) < 0 {
		result.Status, result.Problem, result.MinVersion = providerProbeWarn, providerProbeVersionOld, antigravityCLIMinVersion
		return result
	}
	if result.ModelCount == 0 {
		result.Status, result.Problem = providerProbeWarn, providerProbeModelsNoneSeen
		return result
	}
	result.Status = providerProbeOK
	return result
}

// countAgyModels counts the `<slug>\t<label>` lines of `agy models`; the
// progress line before them has no tab.
func countAgyModels(out string) int {
	n := 0
	for _, line := range strings.Split(out, "\n") {
		if slug, _, ok := strings.Cut(strings.TrimSpace(line), "\t"); ok && strings.TrimSpace(slug) != "" {
			n++
		}
	}
	return n
}

var cliVersionPattern = regexp.MustCompile(`\d+\.\d+\.\d+`)

func parseCLIVersion(out string) string {
	return cliVersionPattern.FindString(out)
}

// compareCLIVersions compares dotted numeric versions; a missing part is 0.
func compareCLIVersions(a, b string) int {
	ap, bp := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < max(len(ap), len(bp)); i++ {
		var x, y int
		if i < len(ap) {
			x, _ = strconv.Atoi(ap[i])
		}
		if i < len(bp) {
			y, _ = strconv.Atoi(bp[i])
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

func clipProbeOutput(s string) string {
	runes := []rune(strings.TrimSpace(s))
	if len(runes) <= providerProbeOutputCap {
		return string(runes)
	}
	return string(runes[:providerProbeOutputCap]) + "…"
}

func handleProviderProbe(service *providerModelsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeMethodNotAllowed(w)
			return
		}
		if service == nil {
			writeError(w, http.StatusInternalServerError, "providers_unavailable", "provider metadata service is not configured")
			return
		}
		writeJSON(w, http.StatusOK, service.probeAll(r.Context()))
	}
}
