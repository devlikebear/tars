package config

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const DefaultCasedConfigFilename = "config/cased.yaml"

type CasedConfig struct {
	APIAddr                 string
	APIAuthMode             string
	APIAuthToken            string
	APIUserToken            string
	APIAdminToken           string
	TargetCommand           string
	TargetArgs              []string
	TargetWorkingDir        string
	TargetEnv               map[string]string
	ProbeURL                string
	ProbeIntervalMS         int
	ProbeTimeoutMS          int
	ProbeFailThreshold      int
	ProbeStartGraceMS       int
	RestartMaxAttempts      int
	RestartBackoffMS        int
	RestartBackoffMaxMS     int
	RestartCooldownMS       int
	EventBufferSize         int
	EventPersistenceEnabled bool
	EventStorePath          string
	EventStoreMaxRecords    int
	Autostart               bool
}

func DefaultCased() CasedConfig {
	return CasedConfig{
		APIAddr:                 "127.0.0.1:43181",
		APIAuthMode:             "external-required",
		ProbeURL:                "http://127.0.0.1:43180/v1/healthz",
		ProbeIntervalMS:         5000,
		ProbeTimeoutMS:          1000,
		ProbeFailThreshold:      3,
		ProbeStartGraceMS:       15000,
		RestartMaxAttempts:      3,
		RestartBackoffMS:        1000,
		RestartBackoffMaxMS:     10000,
		RestartCooldownMS:       60000,
		EventBufferSize:         200,
		EventPersistenceEnabled: true,
		EventStorePath:          filepath.Join(".", "workspace", "_shared", "sentinel", "events.jsonl"),
		EventStoreMaxRecords:    5000,
		Autostart:               true,
	}
}

func ResolveCasedConfigPath(raw string) string {
	if v := strings.TrimSpace(raw); v != "" {
		return os.ExpandEnv(v)
	}
	if v := strings.TrimSpace(firstNonEmpty(os.Getenv("CASED_CONFIG"), os.Getenv("CASED_CONFIG_PATH"))); v != "" {
		return os.ExpandEnv(v)
	}
	if _, err := os.Stat(DefaultCasedConfigFilename); err == nil {
		return DefaultCasedConfigFilename
	}
	return ""
}

func LoadCased(path string) (CasedConfig, error) {
	cfg := DefaultCased()

	if strings.TrimSpace(path) != "" {
		if err := applyCasedYAML(&cfg, path); err != nil {
			return CasedConfig{}, err
		}
	}

	applyCasedEnv(&cfg)
	applyCasedDefaults(&cfg)
	if err := validateCased(cfg); err != nil {
		return CasedConfig{}, err
	}
	return cfg, nil
}

func applyCasedYAML(cfg *CasedConfig, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open config file %q: %w", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return fmt.Errorf("invalid config format at line %d", lineNum)
		}
		key = strings.TrimSpace(key)
		value = cleanYAMLValue(value)
		applyCasedPair(cfg, key, value)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read config file %q: %w", path, err)
	}
	return nil
}

func applyCasedEnv(cfg *CasedConfig) {
	if v := firstNonEmpty(os.Getenv("CASED_API_ADDR"), os.Getenv("TARSD_CASED_API_ADDR")); v != "" {
		cfg.APIAddr = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("CASED_API_AUTH_MODE"), os.Getenv("TARSD_CASED_API_AUTH_MODE")); v != "" {
		cfg.APIAuthMode = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("CASED_API_AUTH_TOKEN"), os.Getenv("TARSD_CASED_API_AUTH_TOKEN")); v != "" {
		cfg.APIAuthToken = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("CASED_API_USER_TOKEN"), os.Getenv("TARSD_CASED_API_USER_TOKEN")); v != "" {
		cfg.APIUserToken = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("CASED_API_ADMIN_TOKEN"), os.Getenv("TARSD_CASED_API_ADMIN_TOKEN")); v != "" {
		cfg.APIAdminToken = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("CASED_TARGET_COMMAND"), os.Getenv("TARSD_CASED_TARGET_COMMAND")); v != "" {
		cfg.TargetCommand = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("CASED_TARGET_ARGS_JSON"), os.Getenv("TARSD_CASED_TARGET_ARGS_JSON")); v != "" {
		cfg.TargetArgs = parseStringArrayJSON(v, cfg.TargetArgs)
	}
	if v := firstNonEmpty(os.Getenv("CASED_TARGET_WORKING_DIR"), os.Getenv("TARSD_CASED_TARGET_WORKING_DIR")); v != "" {
		cfg.TargetWorkingDir = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("CASED_TARGET_ENV_JSON"), os.Getenv("TARSD_CASED_TARGET_ENV_JSON")); v != "" {
		cfg.TargetEnv = parseStringMapJSON(v, cfg.TargetEnv)
	}
	if v := firstNonEmpty(os.Getenv("CASED_PROBE_URL"), os.Getenv("TARSD_CASED_PROBE_URL")); v != "" {
		cfg.ProbeURL = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("CASED_PROBE_INTERVAL_MS"), os.Getenv("TARSD_CASED_PROBE_INTERVAL_MS")); v != "" {
		cfg.ProbeIntervalMS = parsePositiveInt(v, cfg.ProbeIntervalMS)
	}
	if v := firstNonEmpty(os.Getenv("CASED_PROBE_TIMEOUT_MS"), os.Getenv("TARSD_CASED_PROBE_TIMEOUT_MS")); v != "" {
		cfg.ProbeTimeoutMS = parsePositiveInt(v, cfg.ProbeTimeoutMS)
	}
	if v := firstNonEmpty(os.Getenv("CASED_PROBE_FAIL_THRESHOLD"), os.Getenv("TARSD_CASED_PROBE_FAIL_THRESHOLD")); v != "" {
		cfg.ProbeFailThreshold = parsePositiveInt(v, cfg.ProbeFailThreshold)
	}
	if v := firstNonEmpty(os.Getenv("CASED_PROBE_START_GRACE_MS"), os.Getenv("TARSD_CASED_PROBE_START_GRACE_MS")); v != "" {
		cfg.ProbeStartGraceMS = parsePositiveInt(v, cfg.ProbeStartGraceMS)
	}
	if v := firstNonEmpty(os.Getenv("CASED_RESTART_MAX_ATTEMPTS"), os.Getenv("TARSD_CASED_RESTART_MAX_ATTEMPTS")); v != "" {
		cfg.RestartMaxAttempts = parsePositiveInt(v, cfg.RestartMaxAttempts)
	}
	if v := firstNonEmpty(os.Getenv("CASED_RESTART_BACKOFF_MS"), os.Getenv("TARSD_CASED_RESTART_BACKOFF_MS")); v != "" {
		cfg.RestartBackoffMS = parsePositiveInt(v, cfg.RestartBackoffMS)
	}
	if v := firstNonEmpty(os.Getenv("CASED_RESTART_BACKOFF_MAX_MS"), os.Getenv("TARSD_CASED_RESTART_BACKOFF_MAX_MS")); v != "" {
		cfg.RestartBackoffMaxMS = parsePositiveInt(v, cfg.RestartBackoffMaxMS)
	}
	if v := firstNonEmpty(os.Getenv("CASED_RESTART_COOLDOWN_MS"), os.Getenv("TARSD_CASED_RESTART_COOLDOWN_MS")); v != "" {
		cfg.RestartCooldownMS = parsePositiveInt(v, cfg.RestartCooldownMS)
	}
	if v := firstNonEmpty(os.Getenv("CASED_EVENT_BUFFER_SIZE"), os.Getenv("TARSD_CASED_EVENT_BUFFER_SIZE")); v != "" {
		cfg.EventBufferSize = parsePositiveInt(v, cfg.EventBufferSize)
	}
	if v := firstNonEmpty(os.Getenv("CASED_EVENT_PERSISTENCE_ENABLED"), os.Getenv("TARSD_CASED_EVENT_PERSISTENCE_ENABLED")); v != "" {
		cfg.EventPersistenceEnabled = parseBool(v, cfg.EventPersistenceEnabled)
	}
	if v := firstNonEmpty(os.Getenv("CASED_EVENT_STORE_PATH"), os.Getenv("TARSD_CASED_EVENT_STORE_PATH")); v != "" {
		cfg.EventStorePath = strings.TrimSpace(v)
	}
	if v := firstNonEmpty(os.Getenv("CASED_EVENT_STORE_MAX_RECORDS"), os.Getenv("TARSD_CASED_EVENT_STORE_MAX_RECORDS")); v != "" {
		cfg.EventStoreMaxRecords = parsePositiveInt(v, cfg.EventStoreMaxRecords)
	}
	if v := firstNonEmpty(os.Getenv("CASED_AUTOSTART"), os.Getenv("TARSD_CASED_AUTOSTART")); v != "" {
		cfg.Autostart = parseBool(v, cfg.Autostart)
	}
}

func applyCasedPair(cfg *CasedConfig, key, value string) {
	switch key {
	case "api_addr":
		cfg.APIAddr = strings.TrimSpace(value)
	case "api_auth_mode":
		cfg.APIAuthMode = strings.TrimSpace(value)
	case "api_auth_token":
		cfg.APIAuthToken = strings.TrimSpace(value)
	case "api_user_token":
		cfg.APIUserToken = strings.TrimSpace(value)
	case "api_admin_token":
		cfg.APIAdminToken = strings.TrimSpace(value)
	case "target_command":
		cfg.TargetCommand = strings.TrimSpace(value)
	case "target_args_json":
		cfg.TargetArgs = parseStringArrayJSON(value, cfg.TargetArgs)
	case "target_working_dir":
		cfg.TargetWorkingDir = strings.TrimSpace(value)
	case "target_env_json":
		cfg.TargetEnv = parseStringMapJSON(value, cfg.TargetEnv)
	case "probe_url":
		cfg.ProbeURL = strings.TrimSpace(value)
	case "probe_interval_ms":
		cfg.ProbeIntervalMS = parsePositiveInt(value, cfg.ProbeIntervalMS)
	case "probe_timeout_ms":
		cfg.ProbeTimeoutMS = parsePositiveInt(value, cfg.ProbeTimeoutMS)
	case "probe_fail_threshold":
		cfg.ProbeFailThreshold = parsePositiveInt(value, cfg.ProbeFailThreshold)
	case "probe_start_grace_ms":
		cfg.ProbeStartGraceMS = parsePositiveInt(value, cfg.ProbeStartGraceMS)
	case "restart_max_attempts":
		cfg.RestartMaxAttempts = parsePositiveInt(value, cfg.RestartMaxAttempts)
	case "restart_backoff_ms":
		cfg.RestartBackoffMS = parsePositiveInt(value, cfg.RestartBackoffMS)
	case "restart_backoff_max_ms":
		cfg.RestartBackoffMaxMS = parsePositiveInt(value, cfg.RestartBackoffMaxMS)
	case "restart_cooldown_ms":
		cfg.RestartCooldownMS = parsePositiveInt(value, cfg.RestartCooldownMS)
	case "event_buffer_size":
		cfg.EventBufferSize = parsePositiveInt(value, cfg.EventBufferSize)
	case "event_persistence_enabled":
		cfg.EventPersistenceEnabled = parseBool(value, cfg.EventPersistenceEnabled)
	case "event_store_path":
		cfg.EventStorePath = strings.TrimSpace(value)
	case "event_store_max_records":
		cfg.EventStoreMaxRecords = parsePositiveInt(value, cfg.EventStoreMaxRecords)
	case "autostart":
		cfg.Autostart = parseBool(value, cfg.Autostart)
	}
}

func applyCasedDefaults(cfg *CasedConfig) {
	cfg.APIAddr = strings.TrimSpace(cfg.APIAddr)
	if cfg.APIAddr == "" {
		cfg.APIAddr = "127.0.0.1:43181"
	}
	cfg.APIAuthMode = strings.TrimSpace(strings.ToLower(cfg.APIAuthMode))
	switch cfg.APIAuthMode {
	case "off", "external-required", "required":
	default:
		cfg.APIAuthMode = "external-required"
	}
	cfg.APIAuthToken = strings.TrimSpace(cfg.APIAuthToken)
	cfg.APIUserToken = strings.TrimSpace(cfg.APIUserToken)
	cfg.APIAdminToken = strings.TrimSpace(cfg.APIAdminToken)
	cfg.TargetCommand = strings.TrimSpace(cfg.TargetCommand)
	cfg.TargetWorkingDir = strings.TrimSpace(cfg.TargetWorkingDir)
	cfg.ProbeURL = strings.TrimSpace(cfg.ProbeURL)
	if cfg.ProbeURL == "" {
		cfg.ProbeURL = "http://127.0.0.1:43180/v1/healthz"
	}
	if cfg.ProbeIntervalMS <= 0 {
		cfg.ProbeIntervalMS = 5000
	}
	if cfg.ProbeTimeoutMS <= 0 {
		cfg.ProbeTimeoutMS = 1000
	}
	if cfg.ProbeFailThreshold <= 0 {
		cfg.ProbeFailThreshold = 3
	}
	if cfg.ProbeStartGraceMS <= 0 {
		cfg.ProbeStartGraceMS = 15000
	}
	if cfg.RestartMaxAttempts <= 0 {
		cfg.RestartMaxAttempts = 3
	}
	if cfg.RestartBackoffMS <= 0 {
		cfg.RestartBackoffMS = 1000
	}
	if cfg.RestartBackoffMaxMS <= 0 {
		cfg.RestartBackoffMaxMS = 10000
	}
	if cfg.RestartCooldownMS <= 0 {
		cfg.RestartCooldownMS = 60000
	}
	if cfg.EventBufferSize <= 0 {
		cfg.EventBufferSize = 200
	}
	cfg.EventStorePath = strings.TrimSpace(cfg.EventStorePath)
	if cfg.EventStorePath == "" {
		cfg.EventStorePath = filepath.Join(".", "workspace", "_shared", "sentinel", "events.jsonl")
	}
	if cfg.EventStoreMaxRecords <= 0 {
		cfg.EventStoreMaxRecords = 5000
	}
	if len(cfg.TargetEnv) > 0 {
		normalized := make(map[string]string, len(cfg.TargetEnv))
		keys := make([]string, 0, len(cfg.TargetEnv))
		for key := range cfg.TargetEnv {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			trimmed := strings.TrimSpace(key)
			if trimmed == "" {
				continue
			}
			normalized[trimmed] = cfg.TargetEnv[key]
		}
		cfg.TargetEnv = normalized
	}
}

func validateCased(cfg CasedConfig) error {
	if strings.TrimSpace(cfg.TargetCommand) == "" {
		return fmt.Errorf("target_command is required")
	}
	return nil
}

func parseStringArrayJSON(raw string, fallback []string) []string {
	var parsed []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &parsed); err != nil {
		return fallback
	}
	out := make([]string, 0, len(parsed))
	for _, item := range parsed {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return fallback
	}
	return out
}

func parseStringMapJSON(raw string, fallback map[string]string) map[string]string {
	var parsed map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &parsed); err != nil {
		return fallback
	}
	out := make(map[string]string, len(parsed))
	for key, value := range parsed {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			continue
		}
		out[trimmed] = value
	}
	if len(out) == 0 {
		return fallback
	}
	return out
}
