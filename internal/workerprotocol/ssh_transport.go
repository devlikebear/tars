package workerprotocol

import (
	"context"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/devlikebear/tars/internal/secrets"
)

var sshUserPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9._-]{0,63}$`)

type SSHTransportOptions struct {
	SSHPath        string
	Host           string
	User           string
	Port           int
	IdentityFile   string
	KnownHostsFile string
	Runner         ProcessRunner
	Limits         WireLimits
}

type SSHTransport struct {
	sshPath        string
	host           string
	user           string
	port           int
	identityFile   string
	knownHostsFile string
	runner         ProcessRunner
	limits         WireLimits
}

func NewSSHTransport(opts SSHTransportOptions) (*SSHTransport, error) {
	host := strings.TrimSpace(strings.ToLower(opts.Host))
	user := strings.TrimSpace(opts.User)
	if !validSSHHost(host) || !sshUserPattern.MatchString(user) || opts.Port < 1 || opts.Port > 65535 ||
		!safeAbsoluteProcessPath(opts.SSHPath) || !safeAbsoluteProcessPath(opts.IdentityFile) || !safeAbsoluteProcessPath(opts.KnownHostsFile) {
		return nil, ErrTransportConfig
	}
	limits := opts.Limits
	if limits == (WireLimits{}) {
		limits = DefaultWireLimits()
	}
	if err := limits.Validate(); err != nil {
		return nil, err
	}
	if opts.Runner == nil {
		opts.Runner = OSProcessRunner{}
	}
	return &SSHTransport{
		sshPath: filepath.Clean(opts.SSHPath), host: host, user: user, port: opts.Port,
		identityFile: filepath.Clean(opts.IdentityFile), knownHostsFile: filepath.Clean(opts.KnownHostsFile),
		runner: opts.Runner, limits: limits,
	}, nil
}

func (transport *SSHTransport) Exchange(ctx context.Context, request WireRequest) (WireResponse, error) {
	if transport == nil || transport.runner == nil {
		return WireResponse{}, ErrTransportConfig
	}
	raw, err := encodeWireRequest(request, transport.limits)
	if err != nil {
		return WireResponse{}, err
	}
	result, err := transport.runner.Run(ctx, ProcessSpec{
		Path: transport.sshPath, Args: transport.arguments(), Stdin: raw,
		InheritEnv: false, MaxOutputBytes: transport.limits.MaxResponseBytes,
	})
	if int64(len(result.Stdout)) > transport.limits.MaxResponseBytes || int64(len(result.Stderr)) > transport.limits.MaxResponseBytes {
		return WireResponse{}, ErrTransportLimit
	}
	if err != nil {
		if errors.Is(err, ErrTransportLimit) {
			return WireResponse{}, err
		}
		stderr := redactSSHFailure(string(result.Stderr), request)
		if stderr == "" {
			return WireResponse{}, fmt.Errorf("workerprotocol: SSH worker exchange: %w", err)
		}
		return WireResponse{}, fmt.Errorf("workerprotocol: SSH worker exchange: %w: %s", err, stderr)
	}
	return decodeWireResponse(result.Stdout, request.RequestID, transport.limits)
}

func (transport *SSHTransport) arguments() []string {
	destinationHost := transport.host
	if net.ParseIP(destinationHost) != nil && strings.Contains(destinationHost, ":") {
		destinationHost = "[" + destinationHost + "]"
	}
	return []string{
		"-T", "-F", "/dev/null",
		"-o", "BatchMode=yes",
		"-o", "ClearAllForwardings=yes",
		"-o", "PermitLocalCommand=no",
		"-o", "RequestTTY=no",
		"-o", "StrictHostKeyChecking=yes",
		"-o", "IdentitiesOnly=yes",
		"-o", "UserKnownHostsFile=" + transport.knownHostsFile,
		"-i", transport.identityFile,
		"-p", strconv.Itoa(transport.port),
		transport.user + "@" + destinationHost,
		"tars", "worker", "serve", "--stdio", "--protocol", ProtocolVersionV1,
	}
}

func validSSHHost(host string) bool {
	if host == "" || strings.ContainsAny(host, " /@\\\r\n\t") {
		return false
	}
	if net.ParseIP(host) != nil {
		return true
	}
	return validEgressHost(host)
}

func safeAbsoluteProcessPath(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	return trimmed != "" && filepath.IsAbs(trimmed) && filepath.Clean(trimmed) == trimmed &&
		!strings.ContainsAny(trimmed, "\r\n\x00") && !strings.HasPrefix(filepath.Base(trimmed), "-")
}

func redactSSHFailure(stderr string, request WireRequest) string {
	redacted := secrets.RedactText(stderr)
	if request.Envelope.Type == MessageExecute {
		var payload ExecutePayload
		if jsonErr := decodePayload(request.Envelope.Payload, &payload); jsonErr == nil && payload.TaskToken != "" {
			redacted = strings.ReplaceAll(redacted, payload.TaskToken, "***")
		}
	}
	redacted = strings.Join(strings.Fields(redacted), " ")
	if len(redacted) > 512 {
		redacted = redacted[:512] + "..."
	}
	return redacted
}

var _ WorkerTransport = (*SSHTransport)(nil)
