package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/llm"
	"github.com/devlikebear/tars/internal/memory"
	"github.com/spf13/cobra"
)

type doctorOptions struct {
	workspaceDir string
	configPath   string
	fix          bool
}

type doctorCheck struct {
	name   string
	status string
	detail string
}

type doctorReport struct {
	checks []doctorCheck
	hints  []string
}

var doctorRunner = runDoctorCommand

func defaultDoctorOptions() doctorOptions {
	return doctorOptions{
		workspaceDir: defaultWorkspaceDir(),
	}
}

func newDoctorCommand(stdout, stderr io.Writer) *cobra.Command {
	opts := defaultDoctorOptions()
	cmd := &cobra.Command{
		Use:          "doctor",
		Short:        "Check starter config, workspace, and BYOK readiness",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return doctorRunner(cmd.Context(), opts, stdout, stderr)
		},
	}
	cmd.Flags().StringVar(&opts.workspaceDir, "workspace-dir", opts.workspaceDir, "workspace directory")
	cmd.Flags().StringVar(&opts.configPath, "config", "", "config file path")
	cmd.Flags().BoolVar(&opts.fix, "fix", false, "create missing starter config and workspace files")
	return cmd
}

func runDoctorCommand(_ context.Context, opts doctorOptions, stdout, _ io.Writer) error {
	report, err := buildDoctorReport(opts)
	renderDoctorReport(stdout, report)
	return err
}

func buildDoctorReport(opts doctorOptions) (doctorReport, error) {
	report := doctorReport{}

	workspaceAbs, err := resolveWorkspaceDir(opts.workspaceDir)
	if err != nil {
		report.add("fail", "workspace", fmt.Sprintf("resolve workspace dir: %v", err))
		return report, fmt.Errorf("doctor found %d failing checks", report.failureCount())
	}
	configPath, err := resolveConfigPath(opts.configPath, workspaceAbs)
	if err != nil {
		report.add("fail", "config file", fmt.Sprintf("resolve config path: %v", err))
		return report, fmt.Errorf("doctor found %d failing checks", report.failureCount())
	}

	configExists, statErr := pathExists(configPath)
	switch {
	case statErr != nil:
		report.add("fail", "config file", fmt.Sprintf("stat %s: %v", configPath, statErr))
	case configExists:
		report.add("ok", "config file", configPath)
	case opts.fix:
		if err := writeStarterConfigFile(workspaceAbs, configPath); err != nil {
			report.add("fail", "config file", err.Error())
			return report, fmt.Errorf("doctor found %d failing checks", report.failureCount())
		}
		report.add("fixed", "config file", fmt.Sprintf("created starter config at %s", configPath))
	default:
		report.add("fail", "config file", fmt.Sprintf("missing: %s", configPath))
		report.addHint("run `tars doctor --fix` to create starter files")
	}

	cfg, cfgLoaded := config.Config{}, false
	if report.lastStatusFor("config file") != "fail" {
		cfg, err = config.Load(configPath)
		if err != nil {
			report.add("fail", "config load", err.Error())
		} else {
			cfgLoaded = true
			report.add("ok", "config load", fmt.Sprintf("loaded %s", configPath))
		}
	}

	runtimeWorkspaceAbs := workspaceAbs
	bundledPluginsDir := defaultStarterBundledPluginsDir()
	if cfgLoaded {
		runtimeWorkspaceAbs, err = resolveWorkspaceDir(cfg.WorkspaceDir)
		if err != nil {
			report.add("fail", "workspace", fmt.Sprintf("resolve workspace_dir from config: %v", err))
		}
		bundledPluginsDir = strings.TrimSpace(firstNonEmpty(cfg.PluginsBundledDir, bundledPluginsDir))
	}

	if report.lastStatusFor("workspace") != "fail" {
		missing := missingWorkspacePaths(runtimeWorkspaceAbs, bundledPluginsDir)
		switch {
		case len(missing) == 0:
			report.add("ok", "workspace", runtimeWorkspaceAbs)
		case opts.fix:
			if err := ensureStarterWorkspaceLayout(runtimeWorkspaceAbs, bundledPluginsDir); err != nil {
				report.add("fail", "workspace", err.Error())
			} else {
				report.add("fixed", "workspace", fmt.Sprintf("created starter workspace files at %s", runtimeWorkspaceAbs))
			}
		default:
			report.add("fail", "workspace", fmt.Sprintf("missing starter paths in %s: %s", runtimeWorkspaceAbs, strings.Join(missing, ", ")))
			report.addHint("run `tars doctor --fix` to create missing workspace files")
		}
	}

	if cfgLoaded {
		checkDoctorAPIAuth(&report, cfg)
		checkDoctorGatewayAgents(&report, cfg)
		checkDoctorLLMCredentials(&report, cfg, configPath)
		checkDoctorLLMRuntime(&report, cfg)
		checkDoctorSemanticMemory(&report, cfg, configPath)
	}

	if report.failureCount() > 0 {
		return report, fmt.Errorf("doctor found %d failing checks", report.failureCount())
	}
	return report, nil
}

func checkDoctorAPIAuth(report *doctorReport, cfg config.Config) {
	mode := strings.TrimSpace(strings.ToLower(cfg.APIAuthMode))
	switch mode {
	case "off", "external-required":
		if !cfg.APIAllowInsecureLocalAuth {
			report.add("fail", "api auth", fmt.Sprintf("api_auth_mode=%s requires api_allow_insecure_local_auth=true", mode))
			report.addHint("set `api_allow_insecure_local_auth: true` only for explicit localhost-only development")
			return
		}
		report.add("warn", "api auth", fmt.Sprintf("api_auth_mode=%s is localhost-only and should not be exposed publicly", mode))
	default:
		report.add("ok", "api auth", fmt.Sprintf("api_auth_mode=%s", firstNonEmpty(mode, "required")))
	}
}

func checkDoctorLLMCredentials(report *doctorReport, cfg config.Config, configPath string) {
	if strings.TrimSpace(strings.ToLower(cfg.LLMAuthMode)) != "api-key" {
		report.add("ok", "llm credentials", fmt.Sprintf("provider=%s auth=%s", cfg.LLMProvider, cfg.LLMAuthMode))
		return
	}
	if strings.TrimSpace(cfg.LLMAPIKey) != "" {
		report.add("ok", "llm credentials", fmt.Sprintf("provider=%s api key configured", cfg.LLMProvider))
		return
	}

	hint := llmCredentialHint(strings.TrimSpace(strings.ToLower(cfg.LLMProvider)), configPath)
	report.add("fail", "llm credentials", fmt.Sprintf("provider=%s auth=api-key requires credentials", firstNonEmpty(cfg.LLMProvider, "unknown")))
	report.addHint(hint)
}

func checkDoctorGatewayAgents(report *doctorReport, cfg config.Config) {
	if !cfg.GatewayEnabled {
		return
	}
	defaultAgent := strings.TrimSpace(cfg.GatewayDefaultAgent)
	if defaultAgent == "" || strings.EqualFold(defaultAgent, "default") {
		report.add("ok", "gateway agents", "in-process default gateway agent available")
		return
	}

	enabled := map[string]config.GatewayAgent{}
	for _, agent := range cfg.GatewayAgents {
		if !agent.Enabled {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(agent.Name))
		if name == "" {
			continue
		}
		enabled[name] = agent
	}
	spec, ok := enabled[strings.ToLower(defaultAgent)]
	if !ok {
		report.add("fail", "gateway agents", fmt.Sprintf("gateway_default_agent=%q is not registered as an enabled gateway agent", defaultAgent))
		report.addHint("clear `gateway_default_agent` to use the in-process default agent, or register the named gateway agent")
		return
	}
	if detail := validateDoctorGatewayAgent(cfg.WorkspaceDir, spec); detail != "" {
		report.add("fail", "gateway agents", detail)
		report.addHint("fix the missing gateway command/path, or clear `gateway_default_agent` to fall back to the in-process default agent")
		return
	}
	report.add("ok", "gateway agents", fmt.Sprintf("gateway_default_agent=%s", defaultAgent))
}

func checkDoctorLLMRuntime(report *doctorReport, cfg config.Config) {
	switch strings.TrimSpace(strings.ToLower(cfg.LLMProvider)) {
	case "claude-code-cli":
		path, err := llm.FindClaudeCodeCLIPath()
		if err != nil {
			report.add("fail", "llm runtime", err.Error())
			report.addHint("install Claude Code or set `CLAUDE_CODE_CLI_PATH` to the local `claude` binary")
			return
		}
		report.add("ok", "llm runtime", fmt.Sprintf("provider=%s cli=%s", cfg.LLMProvider, path))
	}
}

func checkDoctorSemanticMemory(report *doctorReport, cfg config.Config, configPath string) {
	semanticCfg := memory.SemanticConfig{
		Enabled:         cfg.MemorySemanticEnabled,
		EmbedProvider:   cfg.MemoryEmbedProvider,
		EmbedBaseURL:    cfg.MemoryEmbedBaseURL,
		EmbedAPIKey:     cfg.MemoryEmbedAPIKey,
		EmbedModel:      cfg.MemoryEmbedModel,
		EmbedDimensions: cfg.MemoryEmbedDimensions,
	}
	if !semanticCfg.Enabled {
		report.add("ok", "semantic memory", "disabled")
		return
	}
	if err := memory.ValidateSemanticConfig(semanticCfg); err != nil {
		report.add("fail", "semantic memory", err.Error())
		report.addHint(fmt.Sprintf(
			"set `memory_embed_provider` to one of [%s], or disable semantic memory in %s",
			strings.Join(memory.SupportedEmbedProviders(), ", "),
			configPath,
		))
		return
	}
	report.add("ok", "semantic memory", fmt.Sprintf("provider=%s model=%s", semanticCfg.EmbedProvider, semanticCfg.EmbedModel))
}

func llmCredentialHint(provider, configPath string) string {
	switch provider {
	case "openai":
		return fmt.Sprintf("export OPENAI_API_KEY='your-api-key' or set llm_api_key in %s", configPath)
	case "anthropic":
		return fmt.Sprintf("export ANTHROPIC_API_KEY='your-api-key' or set llm_api_key in %s", configPath)
	case "gemini", "gemini-native":
		return fmt.Sprintf("export GEMINI_API_KEY='your-api-key' or set llm_api_key in %s", configPath)
	case "openai-codex":
		return fmt.Sprintf("set llm_auth_mode: oauth or configure OPENAI_CODEX_OAUTH_TOKEN in %s", configPath)
	case "claude-code-cli":
		return fmt.Sprintf("install Claude Code or set CLAUDE_CODE_CLI_PATH; no api key is required in %s", configPath)
	default:
		return fmt.Sprintf("set TARS_LLM_API_KEY or llm_api_key in %s", configPath)
	}
}

func validateDoctorGatewayAgent(workspaceDir string, spec config.GatewayAgent) string {
	name := strings.TrimSpace(spec.Name)
	command := strings.TrimSpace(os.ExpandEnv(spec.Command))
	if name == "" || command == "" {
		return fmt.Sprintf("gateway agent %q is missing a command", name)
	}
	workDir := strings.TrimSpace(os.ExpandEnv(spec.WorkingDir))
	if workDir == "" {
		workDir = strings.TrimSpace(workspaceDir)
	} else if !filepath.IsAbs(workDir) && strings.TrimSpace(workspaceDir) != "" {
		workDir = filepath.Join(strings.TrimSpace(workspaceDir), workDir)
	}
	if detail := validateDoctorCommandPath(command, workDir, name); detail != "" {
		return detail
	}
	for _, rawArg := range spec.Args {
		arg := strings.TrimSpace(os.ExpandEnv(rawArg))
		if arg == "" {
			continue
		}
		if !doctorLooksLikeLocalPath(arg) {
			continue
		}
		path := arg
		if !filepath.IsAbs(path) {
			path = filepath.Join(workDir, path)
		}
		if _, err := os.Stat(path); err != nil {
			return fmt.Sprintf("gateway agent %q references missing path %s", name, path)
		}
	}
	return ""
}

func validateDoctorCommandPath(command, workDir, agentName string) string {
	if doctorLooksLikeLocalPath(command) {
		path := command
		if !filepath.IsAbs(path) {
			path = filepath.Join(workDir, path)
		}
		if _, err := os.Stat(path); err != nil {
			return fmt.Sprintf("gateway agent %q command not found: %s", agentName, path)
		}
		return ""
	}
	if _, err := exec.LookPath(command); err != nil {
		return fmt.Sprintf("gateway agent %q command %q is not available on PATH", agentName, command)
	}
	return ""
}

func doctorLooksLikeLocalPath(value string) bool {
	value = strings.TrimSpace(value)
	return filepath.IsAbs(value) || strings.HasPrefix(value, "./") || strings.HasPrefix(value, "../")
}

func missingWorkspacePaths(root string, bundledPluginsDir string) []string {
	required := []string{
		filepath.Join(root, "memory"),
		filepath.Join(root, "memory", "raw"),
		filepath.Join(root, "memory", "wiki"),
		filepath.Join(root, "memory", "wiki", "notes"),
		filepath.Join(root, "memory", "wiki", "index.md"),
		filepath.Join(root, "memory", "wiki", "graph.json"),
		filepath.Join(root, "_shared"),
		filepath.Join(root, "MEMORY.md"),
		filepath.Join(root, "AGENTS.md"),
	}
	required = append(required, bundledWorkspacePluginManifestPaths(root, bundledPluginsDir)...)
	missing := make([]string, 0)
	for _, path := range required {
		exists, err := pathExists(path)
		if err != nil || !exists {
			missing = append(missing, path)
		}
	}
	return missing
}

func pathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func renderDoctorReport(stdout io.Writer, report doctorReport) {
	summary := "PASS"
	if report.failureCount() > 0 {
		summary = "FAIL"
	}
	_, _ = fmt.Fprintf(stdout, "doctor: %s\n", summary)
	for _, check := range report.checks {
		_, _ = fmt.Fprintf(stdout, "[%s] %s: %s\n", check.status, check.name, check.detail)
	}
	if len(report.hints) > 0 {
		_, _ = fmt.Fprintln(stdout, "")
		_, _ = fmt.Fprintln(stdout, "Next:")
		for _, hint := range uniqueStrings(report.hints) {
			_, _ = fmt.Fprintf(stdout, "- %s\n", hint)
		}
	}
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func (r *doctorReport) add(status, name, detail string) {
	r.checks = append(r.checks, doctorCheck{
		name:   name,
		status: status,
		detail: strings.TrimSpace(detail),
	})
}

func (r *doctorReport) addHint(hint string) {
	r.hints = append(r.hints, strings.TrimSpace(hint))
}

func (r doctorReport) failureCount() int {
	count := 0
	for _, check := range r.checks {
		if check.status == "fail" {
			count++
		}
	}
	return count
}

func (r doctorReport) lastStatusFor(name string) string {
	for i := len(r.checks) - 1; i >= 0; i-- {
		if r.checks[i].name == name {
			return r.checks[i].status
		}
	}
	return ""
}
