package remoteaccess

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

const (
	DefaultTailscaleBinary = "tailscale"
	DefaultHTTPSPort       = 443
	DefaultTargetURL       = "http://127.0.0.1:43180"
	// DefaultRelayHTTPSPort is the second serve port, for the phone preview
	// relay (docs/decisions/vibe-phone-preview.md): the app a vibe project
	// made runs on its own origin, never on the console's.
	DefaultRelayHTTPSPort = 8443
	// DefaultRelayTargetURL is where the server's preview relay listens.
	DefaultRelayTargetURL = "http://127.0.0.1:43181"
)

type Status struct {
	Installed   bool   `json:"installed"`
	LoggedIn    bool   `json:"logged_in"`
	HostName    string `json:"host_name"`
	TailnetURL  string `json:"tailnet_url"`
	ServeActive bool   `json:"serve_active"`
	ServePort   int    `json:"serve_port"`
	OwnedByTARS bool   `json:"owned_by_tars"`
	// StaleHost is the MagicDNS name TARS's serve handler still sits under
	// when it is not the machine's current one (the machine was renamed in
	// Tailscale): serve keeps the handler under the old name, and the
	// current name has no handler, so nothing reaches TARS. ServeActive is
	// false then; Enable adds the handler under the current name.
	StaleHost string `json:"stale_host,omitempty"`
	// Relay is the preview relay's serve port; set only when Options names
	// a relay target.
	Relay *RelayStatus `json:"relay,omitempty"`
}

// RelayStatus is Tailscale serve's mapping for the preview relay.
type RelayStatus struct {
	Active      bool `json:"active"`
	Port        int  `json:"port"`
	OwnedByTARS bool `json:"owned_by_tars"`
	// StaleHost is the old MagicDNS name the relay's handler sits under,
	// as Status.StaleHost.
	StaleHost string `json:"stale_host,omitempty"`
}

// RelayURL is the HTTPS origin a phone opens a preview at: the MagicDNS name
// on the relay's serve port. It is "" unless both TARS's own serve port and
// the relay's point at TARS — the relay is part of remote access, never a
// way in on its own.
func (s Status) RelayURL() string {
	if s.URL() == "" || s.Relay == nil || !s.Relay.Active || !s.Relay.OwnedByTARS || s.Relay.Port <= 0 {
		return ""
	}
	host := strings.TrimSuffix(strings.TrimSpace(s.TailnetURL), ".")
	if s.Relay.Port == DefaultHTTPSPort {
		return "https://" + host
	}
	return fmt.Sprintf("https://%s:%d", host, s.Relay.Port)
}

// URL is the HTTPS address a phone on the tailnet reaches this server at:
// the MagicDNS name on the port Tailscale serve forwards to TARS. It is ""
// unless serve is active and points at TARS, since any other address would
// lead nowhere or somewhere else.
func (s Status) URL() string {
	host := strings.TrimSuffix(strings.TrimSpace(s.TailnetURL), ".")
	if host == "" || !s.ServeActive || !s.OwnedByTARS {
		return ""
	}
	if s.ServePort > 0 && s.ServePort != DefaultHTTPSPort {
		return fmt.Sprintf("https://%s:%d", host, s.ServePort)
	}
	return "https://" + host
}

type Options struct {
	Binary    string
	Runner    Runner
	HTTPSPort int
	TargetURL string
	// RelayTargetURL, when set, makes Detect also report the preview
	// relay's serve port and EnableRelay/DisableRelay act on it.
	RelayTargetURL string
	// RelayHTTPSPort is the relay's serve port; DefaultRelayHTTPSPort when 0.
	RelayHTTPSPort int
}

type Desired struct {
	Enabled   bool
	HTTPSPort int
	TargetURL string
}

type Runner interface {
	Run(ctx context.Context, name string, args ...string) (stdout string, stderr string, err error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) (string, string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func Detect(ctx context.Context, opts Options) (Status, error) {
	opts = normalizeOptions(opts)
	statusOut, statusErrOut, err := opts.Runner.Run(ctx, opts.Binary, "status", "--json")
	if errors.Is(err, exec.ErrNotFound) {
		return Status{Installed: false}, nil
	}
	if err != nil {
		return Status{Installed: true}, commandError("tailscale status --json", statusErrOut, err)
	}
	status, err := parseStatusJSON(statusOut)
	if err != nil {
		return Status{Installed: true}, err
	}
	status.Installed = true
	if !status.LoggedIn {
		return status, nil
	}

	raw, err := readServeConfig(ctx, opts)
	if err != nil {
		return status, err
	}
	serve, err := parseServeConfig(raw, opts.HTTPSPort, opts.TargetURL, status.TailnetURL)
	if err != nil {
		return status, err
	}
	status.ServeActive = serve.active
	status.ServePort = serve.port
	status.OwnedByTARS = serve.owned
	status.StaleHost = serve.staleHost
	if opts.RelayTargetURL != "" {
		relay, err := parseServeConfig(raw, opts.RelayHTTPSPort, opts.RelayTargetURL, status.TailnetURL)
		if err != nil {
			return status, err
		}
		status.Relay = &RelayStatus{Active: relay.active, Port: opts.RelayHTTPSPort, OwnedByTARS: relay.owned, StaleHost: relay.staleHost}
	}
	return status, nil
}

func readServeConfig(ctx context.Context, opts Options) (string, error) {
	statusOut, statusErrOut, err := opts.Runner.Run(ctx, opts.Binary, "serve", "status", "--json")
	if err == nil {
		return statusOut, nil
	}
	statusErr := commandError("tailscale serve status --json", statusErrOut, err)

	configOut, configErrOut, configErr := opts.Runner.Run(ctx, opts.Binary, "serve", "get-config", "--all")
	if configErr != nil {
		return "", fmt.Errorf("%v; fallback %v", statusErr, commandError("tailscale serve get-config --all", configErrOut, configErr))
	}
	return configOut, nil
}

// ErrRelayPortTaken means the relay's serve port is TARS's own serve port
// or is used by another serve target; the relay is then not served.
var ErrRelayPortTaken = errors.New("tailscale serve relay port is used by a different target")

// EnableRelay maps the relay's serve port to Options.RelayTargetURL. It
// is the second half of turning remote access on (one switch, two ports):
// callers run it after Enable succeeded. Verified on tailscale 1.104 that
// serve keeps 443 and 8443 active side by side on one node.
func EnableRelay(ctx context.Context, opts Options) error {
	opts = normalizeOptions(opts)
	if opts.RelayTargetURL == "" {
		return nil
	}
	if opts.RelayHTTPSPort == opts.HTTPSPort {
		return ErrRelayPortTaken
	}
	status, err := Detect(ctx, opts)
	if err != nil {
		return err
	}
	if !status.Installed || !status.LoggedIn {
		return errors.New("tailscale is not ready")
	}
	if status.Relay.Active && status.Relay.OwnedByTARS {
		return nil
	}
	if status.Relay.Active {
		return ErrRelayPortTaken
	}
	_, stderr, err := opts.Runner.Run(ctx, opts.Binary, "serve", fmt.Sprintf("--https=%d", opts.RelayHTTPSPort), "--bg", opts.RelayTargetURL)
	if err != nil {
		return commandError("tailscale serve relay enable", stderr, err)
	}
	return nil
}

// DisableRelay removes the relay's serve port when it points at the relay;
// a port another target uses is left alone.
func DisableRelay(ctx context.Context, opts Options) error {
	opts = normalizeOptions(opts)
	if opts.RelayTargetURL == "" || opts.RelayHTTPSPort == opts.HTTPSPort {
		return nil
	}
	status, err := Detect(ctx, opts)
	if err != nil {
		return err
	}
	if !status.Installed || status.Relay == nil || !status.Relay.Active || !status.Relay.OwnedByTARS {
		return nil
	}
	_, stderr, err := opts.Runner.Run(ctx, opts.Binary, "serve", fmt.Sprintf("--https=%d", opts.RelayHTTPSPort), "off")
	if err != nil {
		return commandError("tailscale serve relay disable", stderr, err)
	}
	return nil
}

func Enable(ctx context.Context, opts Options) error {
	opts = normalizeOptions(opts)
	status, err := Detect(ctx, opts)
	if err != nil {
		return err
	}
	if !status.Installed {
		return errors.New("tailscale is not installed")
	}
	if !status.LoggedIn {
		return errors.New("tailscale is not logged in")
	}
	if status.ServeActive && status.OwnedByTARS {
		return nil
	}
	if status.ServeActive && !status.OwnedByTARS {
		return fmt.Errorf("tailscale serve https port %d is already used by a different target", opts.HTTPSPort)
	}
	_, stderr, err := opts.Runner.Run(ctx, opts.Binary, "serve", fmt.Sprintf("--https=%d", opts.HTTPSPort), "--bg", opts.TargetURL)
	if err != nil {
		return commandError("tailscale serve enable", stderr, err)
	}
	return nil
}

func Disable(ctx context.Context, opts Options) error {
	opts = normalizeOptions(opts)
	status, err := Detect(ctx, opts)
	if err != nil {
		return err
	}
	if !status.Installed {
		return errors.New("tailscale is not installed")
	}
	if !status.ServeActive {
		return nil
	}
	if !status.OwnedByTARS {
		return fmt.Errorf("tailscale serve https port %d is used by a different target", opts.HTTPSPort)
	}
	_, stderr, err := opts.Runner.Run(ctx, opts.Binary, "serve", fmt.Sprintf("--https=%d", opts.HTTPSPort), "off")
	if err != nil {
		return commandError("tailscale serve disable", stderr, err)
	}
	return nil
}

func Reconcile(ctx context.Context, desired Desired, opts Options) error {
	if desired.HTTPSPort > 0 {
		opts.HTTPSPort = desired.HTTPSPort
	}
	if strings.TrimSpace(desired.TargetURL) != "" {
		opts.TargetURL = desired.TargetURL
	}
	if desired.Enabled {
		return Enable(ctx, opts)
	}
	return Disable(ctx, opts)
}

func normalizeOptions(opts Options) Options {
	if strings.TrimSpace(opts.Binary) == "" {
		opts.Binary = DefaultTailscaleBinary
	}
	if opts.Runner == nil {
		opts.Runner = ExecRunner{}
	}
	if opts.HTTPSPort <= 0 {
		opts.HTTPSPort = DefaultHTTPSPort
	}
	if strings.TrimSpace(opts.TargetURL) == "" {
		opts.TargetURL = DefaultTargetURL
	}
	opts.TargetURL = strings.TrimSpace(opts.TargetURL)
	opts.RelayTargetURL = strings.TrimSpace(opts.RelayTargetURL)
	if opts.RelayHTTPSPort <= 0 {
		opts.RelayHTTPSPort = DefaultRelayHTTPSPort
	}
	return opts
}

func parseStatusJSON(raw string) (Status, error) {
	var payload struct {
		BackendState string `json:"BackendState"`
		Self         *struct {
			HostName string `json:"HostName"`
			DNSName  string `json:"DNSName"`
		} `json:"Self"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return Status{}, fmt.Errorf("parse tailscale status json: %w", err)
	}
	status := Status{
		LoggedIn: strings.EqualFold(strings.TrimSpace(payload.BackendState), "running") && payload.Self != nil,
	}
	if payload.Self != nil {
		status.HostName = strings.TrimSpace(payload.Self.HostName)
		status.TailnetURL = strings.TrimSuffix(strings.TrimSpace(payload.Self.DNSName), ".")
	}
	return status, nil
}

type serveConfigStatus struct {
	active bool
	owned  bool
	port   int
	// staleHost is a host other than the current MagicDNS name whose
	// handler on the port points at the target; set only when the current
	// name has no handler on the port.
	staleHost string
}

// parseServeConfig reads the serve config's handler for httpsPort under
// currentHost, the machine's MagicDNS name. Serve keys its handlers by
// "host:port" and keeps a handler under the name it was added with, so
// after the machine is renamed in Tailscale a handler under the old name
// still shows up but answers nothing: TLS to the current name finds no
// handler. Only a handler under currentHost counts; a TARS handler under
// another name is reported as staleHost. With no currentHost known, any
// host counts, as before.
func parseServeConfig(raw string, httpsPort int, targetURL, currentHost string) (serveConfigStatus, error) {
	if strings.TrimSpace(raw) == "" {
		return serveConfigStatus{}, nil
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return serveConfigStatus{}, fmt.Errorf("parse tailscale serve config: %w", err)
	}
	web, _ := payload["Web"].(map[string]any)
	current := normalizeHost(currentHost)
	var out serveConfigStatus
	var stale []string
	for hostPort, value := range web {
		host, port, ok := parseHostPort(hostPort)
		if !ok || port != httpsPort {
			continue
		}
		owned := containsProxyTarget(value, targetURL)
		if current != "" && normalizeHost(host) != current {
			if owned {
				stale = append(stale, normalizeHost(host))
			}
			continue
		}
		out.active = true
		out.port = port
		if owned {
			out.owned = true
			return out, nil
		}
	}
	if !out.active && len(stale) > 0 {
		sort.Strings(stale)
		out.staleHost = stale[0]
	}
	return out, nil
}

func normalizeHost(host string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
}

func parseHostPort(value string) (string, int, bool) {
	host, portString, err := net.SplitHostPort(value)
	if err != nil {
		idx := strings.LastIndex(value, ":")
		if idx < 0 || idx == len(value)-1 {
			return "", 0, false
		}
		host, portString = value[:idx], value[idx+1:]
	}
	port, err := strconv.Atoi(portString)
	if err != nil {
		return "", 0, false
	}
	return host, port, true
}

func containsProxyTarget(value any, targetURL string) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if strings.EqualFold(key, "proxy") {
				if proxy, ok := child.(string); ok && strings.TrimSpace(proxy) == targetURL {
					return true
				}
			}
			if containsProxyTarget(child, targetURL) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if containsProxyTarget(child, targetURL) {
				return true
			}
		}
	}
	return false
}

func commandError(command, stderr string, err error) error {
	message := strings.TrimSpace(stderr)
	if message == "" {
		message = err.Error()
	}
	return fmt.Errorf("%s failed: %s", command, message)
}
