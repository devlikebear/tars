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
	workerServiceConfigSchemaVersion = 1
	maxWorkerServiceConfigBytes      = 1 << 20
)

type ContainerWorkerConfig struct {
	RuntimePath    string   `json:"runtime_path"`
	Image          string   `json:"image"`
	Command        []string `json:"command"`
	CPUs           string   `json:"cpus,omitempty"`
	PIDsLimit      int      `json:"pids_limit,omitempty"`
	SupportsResume bool     `json:"supports_resume,omitempty"`
}

type WorkerServiceConfig struct {
	SchemaVersion      int                   `json:"schema_version"`
	WorkerID           string                `json:"worker_id"`
	RootDir            string                `json:"root_dir"`
	StatePath          string                `json:"state_path,omitempty"`
	PublicKey          string                `json:"public_key"`
	MaxTokenTTLSeconds int64                 `json:"max_token_ttl_seconds"`
	WireLimits         WireLimits            `json:"wire_limits"`
	Container          ContainerWorkerConfig `json:"container"`
}

func OpenConfiguredWorkerService(configPath string, runner ProcessRunner) (*ReferenceWorker, WireLimits, error) {
	config, err := loadWorkerServiceConfig(configPath)
	if err != nil {
		return nil, WireLimits{}, err
	}
	publicKey, err := base64.RawStdEncoding.DecodeString(strings.TrimSpace(config.PublicKey))
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		return nil, WireLimits{}, fmt.Errorf("%w: worker public verification key is invalid", ErrTransportConfig)
	}
	verifier, err := NewTaskTokenVerifier(ed25519.PublicKey(publicKey), time.Duration(config.MaxTokenTTLSeconds)*time.Second, time.Now)
	if err != nil {
		return nil, WireLimits{}, err
	}
	executor, err := NewContainerReferenceExecutor(ContainerReferenceExecutorOptions{
		RuntimePath: config.Container.RuntimePath, Image: config.Container.Image,
		Command: config.Container.Command, CPUs: config.Container.CPUs, PIDsLimit: config.Container.PIDsLimit,
		Runner: runner, SupportsResume: config.Container.SupportsResume,
	})
	if err != nil {
		return nil, WireLimits{}, err
	}
	worker, err := NewReferenceWorker(ReferenceWorkerOptions{
		WorkerID: config.WorkerID, RootDir: config.RootDir, StatePath: config.StatePath,
		TokenVerifier: verifier, Executor: executor,
	})
	if err != nil {
		return nil, WireLimits{}, err
	}
	return worker, config.WireLimits, nil
}

func loadWorkerServiceConfig(configPath string) (WorkerServiceConfig, error) {
	path := strings.TrimSpace(configPath)
	if path == "" || !filepath.IsAbs(path) {
		return WorkerServiceConfig{}, fmt.Errorf("%w: absolute worker config path is required", ErrTransportConfig)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return WorkerServiceConfig{}, fmt.Errorf("workerprotocol: inspect worker config: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o022 != 0 {
		return WorkerServiceConfig{}, fmt.Errorf("%w: worker config must be a non-writable regular file", ErrTransportConfig)
	}
	file, err := os.Open(path)
	if err != nil {
		return WorkerServiceConfig{}, fmt.Errorf("workerprotocol: open worker config: %w", err)
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxWorkerServiceConfigBytes+1))
	if err != nil {
		return WorkerServiceConfig{}, fmt.Errorf("workerprotocol: read worker config: %w", err)
	}
	if len(raw) > maxWorkerServiceConfigBytes {
		return WorkerServiceConfig{}, ErrTransportLimit
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var config WorkerServiceConfig
	if err := decoder.Decode(&config); err != nil {
		return WorkerServiceConfig{}, fmt.Errorf("%w: decode worker config: %v", ErrTransportConfig, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return WorkerServiceConfig{}, fmt.Errorf("%w: worker config contains trailing data", ErrTransportConfig)
	}
	if config.SchemaVersion != workerServiceConfigSchemaVersion || !validProtocolIdentifier(config.WorkerID) ||
		!filepath.IsAbs(config.RootDir) || (config.StatePath != "" && !filepath.IsAbs(config.StatePath)) ||
		config.MaxTokenTTLSeconds <= 0 || config.MaxTokenTTLSeconds > 3600 {
		return WorkerServiceConfig{}, ErrTransportConfig
	}
	if config.WireLimits == (WireLimits{}) {
		config.WireLimits = DefaultWireLimits()
	}
	if err := config.WireLimits.Validate(); err != nil {
		return WorkerServiceConfig{}, err
	}
	return config, nil
}
