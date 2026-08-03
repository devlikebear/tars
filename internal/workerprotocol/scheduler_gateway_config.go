package workerprotocol

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	schedulerGatewayConfigSchemaVersion = 1
	maxSchedulerGatewayConfigBytes      = 1 << 20
	SchedulerTransportInProcess         = "in-process"
	SchedulerTransportSSH               = "ssh"
)

type SchedulerInProcessConfig struct {
	WorkerConfigPath string `json:"worker_config_path"`
}

type SchedulerSSHConfig struct {
	SSHPath          string `json:"ssh_path"`
	Host             string `json:"host"`
	User             string `json:"user"`
	Port             int    `json:"port"`
	IdentityFile     string `json:"identity_file"`
	KnownHostsFile   string `json:"known_hosts_file"`
	WorkerConfigPath string `json:"worker_config_path,omitempty"`
}

// SchedulerGatewayConfig is stored in a private, owner-only JSON file. The
// signing key remains on the gateway; worker configuration carries only the
// corresponding public verification key.
type SchedulerGatewayConfig struct {
	SchemaVersion   int                      `json:"schema_version"`
	Adapter         string                   `json:"adapter"`
	WorkerID        string                   `json:"worker_id"`
	Transport       string                   `json:"transport"`
	PrivateKey      string                   `json:"private_key"`
	LeaseTTLSeconds int64                    `json:"lease_ttl_seconds"`
	TokenTTLSeconds int64                    `json:"token_ttl_seconds"`
	SyncMode        SyncMode                 `json:"sync_mode"`
	GitPath         string                   `json:"git_path,omitempty"`
	Policy          ExecutionPolicy          `json:"policy"`
	BundleLimits    WorkspaceBundleLimits    `json:"bundle_limits,omitempty"`
	WireLimits      WireLimits               `json:"wire_limits,omitempty"`
	Capabilities    WorkerCapabilities       `json:"capabilities,omitempty"`
	InProcess       SchedulerInProcessConfig `json:"in_process,omitempty"`
	SSH             SchedulerSSHConfig       `json:"ssh,omitempty"`
}

type ConfiguredSchedulerExecutorOptions struct {
	ConfigPath string
	SourceDir  string
	DataDir    string
	Controller *Controller
	Runner     ProcessRunner
}

func OpenConfiguredSchedulerExecutor(opts ConfiguredSchedulerExecutorOptions) (*SchedulerExecutor, error) {
	if opts.Controller == nil {
		return nil, fmt.Errorf("workerprotocol: configured scheduler executor requires a shared controller")
	}
	config, err := loadSchedulerGatewayConfig(opts.ConfigPath)
	if err != nil {
		return nil, err
	}
	sourceDir, dataDir, err := secureSchedulerDirectories(opts.SourceDir, opts.DataDir, opts.ConfigPath)
	if err != nil {
		return nil, err
	}
	privateKey, err := base64.RawStdEncoding.DecodeString(strings.TrimSpace(config.PrivateKey))
	config.PrivateKey = ""
	if err != nil || len(privateKey) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("%w: gateway task signing key is invalid", ErrTransportConfig)
	}
	issuer, err := NewTaskTokenIssuer(TaskTokenIssuerOptions{
		PrivateKey: ed25519.PrivateKey(privateKey), MaxTTL: time.Duration(config.LeaseTTLSeconds) * time.Second,
	})
	for index := range privateKey {
		privateKey[index] = 0
	}
	if err != nil {
		return nil, err
	}

	var transport WorkerTransport
	capabilities := config.Capabilities
	endpoint := ""
	switch config.Transport {
	case SchedulerTransportInProcess:
		worker, limits, openErr := OpenConfiguredWorkerService(config.InProcess.WorkerConfigPath, opts.Runner)
		if openErr != nil {
			return nil, openErr
		}
		snapshot := worker.Snapshot()
		if snapshot.WorkerID != config.WorkerID || worker.VerificationKeyID() != issuer.KeyID() ||
			time.Duration(config.TokenTTLSeconds)*time.Second > worker.MaxTaskTokenTTL() {
			return nil, fmt.Errorf("%w: in-process worker identity or task verification profile differs from gateway", ErrTransportConfig)
		}
		if schedulerPathsOverlap(sourceDir, worker.RootDir()) {
			return nil, fmt.Errorf("%w: worker root must be outside the source workspace", ErrTransportConfig)
		}
		transport, err = NewInProcessTransport(worker, limits)
		capabilities = snapshot.Capabilities
		endpoint = "local://" + config.WorkerID
	case SchedulerTransportSSH:
		transport, err = NewSSHTransport(SSHTransportOptions{
			SSHPath: config.SSH.SSHPath, Host: config.SSH.Host, User: config.SSH.User, Port: config.SSH.Port,
			IdentityFile: config.SSH.IdentityFile, KnownHostsFile: config.SSH.KnownHostsFile,
			WorkerConfigPath: config.SSH.WorkerConfigPath, Runner: opts.Runner, Limits: config.WireLimits,
		})
		endpoint = fmt.Sprintf("ssh://%s@%s:%d", config.SSH.User, config.SSH.Host, config.SSH.Port)
	default:
		return nil, fmt.Errorf("%w: unsupported scheduler worker transport %q", ErrTransportConfig, config.Transport)
	}
	if err != nil {
		return nil, err
	}
	if !capabilities.EgressPolicy || !capabilities.ResourceLimits || !capabilities.ArtifactScan {
		return nil, fmt.Errorf("%w: scheduler worker must enforce egress, resources, and artifact scanning", ErrInvalidPolicy)
	}
	store, err := NewFileRemoteRunStore(filepath.Join(dataDir, "runs"))
	if err != nil {
		return nil, err
	}
	quarantine, err := NewArtifactQuarantine(ArtifactQuarantineOptions{RootDir: filepath.Join(dataDir, "artifacts")})
	if err != nil {
		return nil, err
	}
	coordinator, err := NewGatewayCoordinator(GatewayCoordinatorOptions{
		Controller: opts.Controller, WorkerID: config.WorkerID, TransportName: config.Transport, Endpoint: endpoint,
		Capabilities: capabilities, Transport: transport, TokenIssuer: issuer, Quarantine: quarantine, ResultRecorder: store,
		LeaseTTL: time.Duration(config.LeaseTTLSeconds) * time.Second,
		TokenTTL: time.Duration(config.TokenTTLSeconds) * time.Second,
	})
	if err != nil {
		return nil, err
	}
	return NewSchedulerExecutor(SchedulerExecutorOptions{
		Adapter: config.Adapter, SourceDir: sourceDir, SyncMode: config.SyncMode, GitPath: config.GitPath,
		Policy: config.Policy, BundleLimits: config.BundleLimits, Coordinator: coordinator, Store: store,
	})
}

func loadSchedulerGatewayConfig(configPath string) (SchedulerGatewayConfig, error) {
	path := strings.TrimSpace(configPath)
	if path == "" || !filepath.IsAbs(path) {
		return SchedulerGatewayConfig{}, fmt.Errorf("%w: absolute scheduler gateway config path is required", ErrTransportConfig)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return SchedulerGatewayConfig{}, fmt.Errorf("workerprotocol: inspect scheduler gateway config: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return SchedulerGatewayConfig{}, fmt.Errorf("%w: scheduler gateway config must be an owner-only regular file", ErrTransportConfig)
	}
	file, err := os.Open(path)
	if err != nil {
		return SchedulerGatewayConfig{}, fmt.Errorf("workerprotocol: open scheduler gateway config: %w", err)
	}
	defer func() { _ = file.Close() }()
	raw, err := io.ReadAll(io.LimitReader(file, maxSchedulerGatewayConfigBytes+1))
	if err != nil {
		return SchedulerGatewayConfig{}, fmt.Errorf("workerprotocol: read scheduler gateway config: %w", err)
	}
	if len(raw) > maxSchedulerGatewayConfigBytes {
		return SchedulerGatewayConfig{}, ErrTransportLimit
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var config SchedulerGatewayConfig
	if err := decoder.Decode(&config); err != nil {
		return SchedulerGatewayConfig{}, fmt.Errorf("%w: decode scheduler gateway config: %v", ErrTransportConfig, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return SchedulerGatewayConfig{}, fmt.Errorf("%w: scheduler gateway config contains trailing data", ErrTransportConfig)
	}
	if err := validateSchedulerGatewayConfig(&config); err != nil {
		return SchedulerGatewayConfig{}, err
	}
	return config, nil
}

func validateSchedulerGatewayConfig(config *SchedulerGatewayConfig) error {
	if config == nil || config.SchemaVersion != schedulerGatewayConfigSchemaVersion ||
		!validProtocolIdentifier(config.Adapter) || !validProtocolIdentifier(config.WorkerID) ||
		strings.TrimSpace(config.PrivateKey) == "" || config.LeaseTTLSeconds <= 0 || config.LeaseTTLSeconds > 3600 ||
		config.TokenTTLSeconds <= 0 || config.TokenTTLSeconds > config.LeaseTTLSeconds ||
		(config.SyncMode != SyncModeDirectory && config.SyncMode != SyncModeGit) {
		return ErrTransportConfig
	}
	if config.Policy.Egress.Mode == "" && len(config.Policy.Egress.AllowHosts) == 0 && config.Policy.Limits == (ResourceLimits{}) {
		config.Policy = DefaultExecutionPolicy()
	}
	if err := config.Policy.Validate(); err != nil {
		return err
	}
	if config.Policy.Egress.Mode != EgressDeny {
		return fmt.Errorf("%w: configured scheduler workers require default-deny egress", ErrInvalidPolicy)
	}
	if config.BundleLimits == (WorkspaceBundleLimits{}) {
		config.BundleLimits = DefaultWorkspaceBundleLimits()
	}
	if err := config.BundleLimits.Validate(); err != nil {
		return err
	}
	if config.SyncMode == SyncModeGit && !safeAbsoluteProcessPath(config.GitPath) {
		return fmt.Errorf("%w: Git sync requires an absolute executable", ErrTransportConfig)
	}
	switch config.Transport {
	case SchedulerTransportInProcess:
		if !filepath.IsAbs(strings.TrimSpace(config.InProcess.WorkerConfigPath)) {
			return fmt.Errorf("%w: in-process worker config path must be absolute", ErrTransportConfig)
		}
	case SchedulerTransportSSH:
		if !config.Capabilities.EgressPolicy || !config.Capabilities.ResourceLimits || !config.Capabilities.ArtifactScan {
			return fmt.Errorf("%w: SSH worker capabilities must be declared", ErrTransportConfig)
		}
		if config.WireLimits == (WireLimits{}) {
			config.WireLimits = DefaultWireLimits()
		}
		if err := config.WireLimits.Validate(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("%w: invalid scheduler transport", ErrTransportConfig)
	}
	return nil
}

func secureSchedulerDirectories(sourceDir, dataDir, configPath string) (string, string, error) {
	source, err := filepath.Abs(strings.TrimSpace(sourceDir))
	if err != nil || strings.TrimSpace(sourceDir) == "" {
		return "", "", fmt.Errorf("%w: scheduler source directory is invalid", ErrTransportConfig)
	}
	source, err = filepath.EvalSymlinks(source)
	if err != nil {
		return "", "", fmt.Errorf("workerprotocol: resolve scheduler source directory: %w", err)
	}
	data, err := filepath.Abs(strings.TrimSpace(dataDir))
	if err != nil || strings.TrimSpace(dataDir) == "" {
		return "", "", fmt.Errorf("%w: scheduler data directory is invalid", ErrTransportConfig)
	}
	if err := os.MkdirAll(data, 0o700); err != nil {
		return "", "", fmt.Errorf("workerprotocol: create remote scheduler data directory: %w", err)
	}
	data, err = filepath.EvalSymlinks(data)
	if err != nil {
		return "", "", fmt.Errorf("workerprotocol: resolve remote scheduler data directory: %w", err)
	}
	canonicalConfig, err := filepath.EvalSymlinks(filepath.Clean(configPath))
	if err != nil {
		return "", "", fmt.Errorf("workerprotocol: resolve scheduler gateway config: %w", err)
	}
	if schedulerPathsOverlap(source, data) || sameOrWithinWorkspace(source, canonicalConfig) {
		return "", "", fmt.Errorf("%w: scheduler data and private config must be outside the source workspace", ErrTransportConfig)
	}
	if err := os.Chmod(data, 0o700); err != nil {
		return "", "", fmt.Errorf("workerprotocol: secure remote scheduler data directory: %w", err)
	}
	return source, data, nil
}

func schedulerPathsOverlap(left, right string) bool {
	return sameOrWithinWorkspace(left, right) || sameOrWithinWorkspace(right, left)
}
