package extensions

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/plugin"
	"github.com/devlikebear/tars/internal/skill"
	"github.com/devlikebear/tars/internal/skillhub"
	"github.com/devlikebear/tars/internal/tool"
	"github.com/fsnotify/fsnotify"
	zlog "github.com/rs/zerolog/log"
)

type Source = plugin.Source

const (
	SourceWorkspace = plugin.SourceWorkspace
	SourceUser      = plugin.SourceUser
	SourceBundled   = plugin.SourceBundled
)

type PluginSourceDir struct {
	Source Source
	Dir    string
}

type MPRuntime interface {
	SetServers(servers []config.MCPServer)
	BuildTools(ctx context.Context) ([]tool.Tool, error)
}

type Options struct {
	WorkspaceDir           string
	SkillsEnabled          bool
	PluginsEnabled         bool
	PluginsAllowMCPServers bool
	SkillSources           []skill.SourceDir
	PluginSources          []PluginSourceDir
	MCPBaseServers         []config.MCPServer
	MCPRuntime             MPRuntime
	WatchSkills            bool
	WatchPlugins           bool
	WatchDebounce          time.Duration
	PluginConfig           map[string]map[string]any // per-plugin config keyed by plugin ID

	// LifecycleToolResolver is consulted by plugin lifecycle hooks to
	// look up the builtin tool to invoke. May be nil; nil disables
	// lifecycle hook execution and turns each declared hook into a
	// single skip diagnostic. Wiring is the caller's responsibility —
	// extensions/Manager intentionally does not own the user-surface
	// registry.
	LifecycleToolResolver LifecycleToolResolver
}

type Snapshot struct {
	Version     int64
	Skills      []skill.Definition
	Plugins     []plugin.Definition
	SkillPrompt string
	MCPServers  []config.MCPServer
	Diagnostics []string
}

type Manager struct {
	opts          Options
	mu            sync.RWMutex
	snapshot      Snapshot
	chatTools     []tool.Tool
	version       atomic.Int64
	disabledStore *disabledStore

	watcherMu sync.Mutex
	watcher   *fsnotify.Watcher
	stopWatch context.CancelFunc
}

func NewManager(opts Options) (*Manager, error) {
	if strings.TrimSpace(opts.WorkspaceDir) == "" {
		return nil, fmt.Errorf("workspace dir is required")
	}
	if opts.WatchDebounce <= 0 {
		opts.WatchDebounce = 200 * time.Millisecond
	}
	return &Manager{
		opts:          opts,
		disabledStore: newDisabledStore(opts.WorkspaceDir),
	}, nil
}

// DisabledSet returns the current disabled extensions.
func (m *Manager) DisabledSet() DisabledSet {
	ds, _ := m.disabledStore.Load()
	return ds
}

// SetDisabled enables or disables an extension and reloads.
func (m *Manager) SetDisabled(ctx context.Context, kind, name string, disabled bool) error {
	if err := m.disabledStore.SetDisabled(kind, name, disabled); err != nil {
		return err
	}
	return m.Reload(ctx)
}

func (m *Manager) Start(ctx context.Context) error {
	finishStart := beginExtensionsStartupStep("extensions_start")
	if err := m.reload(ctx, reloadOptions{AsyncMCPTools: true}); err != nil {
		finishStart(err)
		return err
	}

	if err := runExtensionsStartupStep("extensions_on_start_hooks", func() error {
		// Run on_start lifecycle hooks (non-fatal)
		m.mu.RLock()
		plugins := append([]plugin.Definition(nil), m.snapshot.Plugins...)
		m.mu.RUnlock()
		if diags := runLifecycleHooks(ctx, plugins, "on_start", 0, m.opts.LifecycleToolResolver); len(diags) > 0 {
			m.mu.Lock()
			m.snapshot.Diagnostics = append(m.snapshot.Diagnostics, diags...)
			m.mu.Unlock()
		}
		return nil
	}); err != nil {
		finishStart(err)
		return err
	}

	if err := runExtensionsStartupStep("extensions_watcher_setup", func() error {
		if !m.opts.WatchSkills && !m.opts.WatchPlugins {
			return nil
		}
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			return fmt.Errorf("create extension watcher: %w", err)
		}

		dirs := m.watchDirs()
		for _, dir := range dirs {
			if err := addWatchRecursive(watcher, dir); err != nil {
				_ = watcher.Close()
				return err
			}
		}

		watchCtx, cancel := context.WithCancel(ctx)
		m.watcherMu.Lock()
		m.watcher = watcher
		m.stopWatch = cancel
		m.watcherMu.Unlock()
		go m.watchLoop(watchCtx, watcher)
		return nil
	}); err != nil {
		finishStart(err)
		return err
	}
	finishStart(nil)
	return nil
}

func (m *Manager) Close() {
	// Run on_stop lifecycle hooks before shutdown
	m.mu.RLock()
	plugins := append([]plugin.Definition(nil), m.snapshot.Plugins...)
	m.mu.RUnlock()
	_ = runLifecycleHooks(context.Background(), plugins, "on_stop", 0, m.opts.LifecycleToolResolver)

	m.watcherMu.Lock()
	defer m.watcherMu.Unlock()
	if m.stopWatch != nil {
		m.stopWatch()
		m.stopWatch = nil
	}
	if m.watcher != nil {
		_ = m.watcher.Close()
		m.watcher = nil
	}
}

func (m *Manager) Reload(ctx context.Context) error {
	return m.reload(ctx, reloadOptions{})
}

type reloadOptions struct {
	AsyncMCPTools bool
}

func (m *Manager) reload(ctx context.Context, opts reloadOptions) error {
	finishReload := beginExtensionsStartupStep("extensions_reload")
	failReload := func(err error) error {
		finishReload(err)
		return err
	}

	plugins := plugin.Snapshot{}
	var err error
	if m.opts.PluginsEnabled {
		if err := runExtensionsStartupStep("extensions_plugin_load", func() error {
			plugins, err = plugin.Load(plugin.LoadOptions{
				Sources:      toPluginSources(m.opts.PluginSources),
				Availability: plugin.AvailabilityOptions{},
			})
			return err
		}); err != nil {
			return failReload(err)
		}
	}

	skills := skill.Snapshot{}
	if m.opts.SkillsEnabled {
		if err := runExtensionsStartupStep("extensions_skill_load", func() error {
			skillSources := mergeSkillSources(m.opts.SkillSources, plugins.Plugins, plugins.SkillDirs)
			skills, err = skill.Load(skill.LoadOptions{
				Sources: skillSources,
				Availability: skill.AvailabilityOptions{
					InstalledPlugins: pluginIDs(plugins.Plugins),
				},
			})
			return err
		}); err != nil {
			return failReload(err)
		}
		if err := runExtensionsStartupStep("extensions_skill_mirror", func() error {
			skills, err = skill.MirrorToWorkspace(m.opts.WorkspaceDir, skills)
			return err
		}); err != nil {
			return failReload(err)
		}
	}

	pluginMCPServers := []config.MCPServer{}
	if m.opts.PluginsAllowMCPServers {
		pluginMCPServers = plugins.MCPServers
		// Surface that an externally-installed plugin's mcp_servers
		// declarations are now active. The default for this flag is
		// false; flipping it on lets a plugin manifest spawn external
		// processes (RF-008 same threat surface as the legacy shell
		// hook), so the operator should see this in startup logs.
		if len(pluginMCPServers) > 0 {
			names := make([]string, 0, len(pluginMCPServers))
			for _, s := range pluginMCPServers {
				if name := strings.TrimSpace(s.Name); name != "" {
					names = append(names, name)
				}
			}
			zlog.Logger.Warn().
				Bool("plugins_allow_mcp_servers", true).
				Strs("plugin_mcp_servers", names).
				Msg("extensions: plugin-declared MCP servers enabled; verify each plugin source is trusted")
		}
	}
	var hubMCPServers []config.MCPServer
	var hubDiagnostics []string
	var workspaceMCPServers []config.MCPServer
	var workspaceMCPDiagnostics []string
	var mcpServers []config.MCPServer
	var mcpDiagnostics []string
	if err := runExtensionsStartupStep("extensions_mcp_servers_load", func() error {
		hubMCPServers, hubDiagnostics = skillhub.LoadInstalledMCPServers(m.opts.WorkspaceDir)
		workspaceMCPServers, workspaceMCPDiagnostics = skillhub.LoadWorkspaceMCPServers(m.opts.WorkspaceDir)
		mcpServers, mcpDiagnostics = mergeMCPServers(
			mcpServerGroup{label: "config", servers: m.opts.MCPBaseServers},
			mcpServerGroup{label: "plugin", servers: pluginMCPServers},
			mcpServerGroup{label: "hub", servers: hubMCPServers},
			mcpServerGroup{label: "workspace", servers: workspaceMCPServers},
		)
		return nil
	}); err != nil {
		return failReload(err)
	}
	mcpTools := make([]tool.Tool, 0)
	mcpToolsPending := false
	if m.opts.MCPRuntime != nil {
		m.opts.MCPRuntime.SetServers(mcpServers)
		if opts.AsyncMCPTools {
			mcpToolsPending = true
			zlog.Logger.Debug().
				Int("mcp_servers", len(mcpServers)).
				Msg("extensions mcp tools build deferred")
		} else {
			finishMCPTools := beginExtensionsStartupStep("extensions_mcp_tools_build")
			mcpTools, err = m.opts.MCPRuntime.BuildTools(ctx)
			finishMCPTools(err)
			if err != nil {
				// MCP server failures should not block startup; record diagnostic and continue.
				mcpDiagnostics = append(mcpDiagnostics, fmt.Sprintf("mcp tools build failed: %v", err))
				mcpTools = nil
			}
		}
	}

	// Filter out user-disabled extensions
	var disabled DisabledSet
	if err := runExtensionsStartupStep("extensions_disabled_load", func() error {
		disabled, err = m.disabledStore.Load()
		return err
	}); err != nil {
		return failReload(err)
	}
	{
		filtered := make([]skill.Definition, 0, len(skills.Skills))
		for _, s := range skills.Skills {
			if disabled.isSkillDisabled(s.Name) {
				skills.Diagnostics = append(skills.Diagnostics, skill.Diagnostic{
					Path:    s.FilePath,
					Message: "disabled by user",
				})
				continue
			}
			filtered = append(filtered, s)
		}
		skills.Skills = filtered
	}
	{
		filtered := make([]plugin.Definition, 0, len(plugins.Plugins))
		for _, p := range plugins.Plugins {
			if disabled.isPluginDisabled(p.ID) {
				plugins.Diagnostics = append(plugins.Diagnostics, plugin.Diagnostic{
					Path:    p.RootDir,
					Message: "disabled by user",
				})
				continue
			}
			filtered = append(filtered, p)
		}
		plugins.Plugins = filtered
	}
	{
		filtered := make([]config.MCPServer, 0, len(mcpServers))
		for _, s := range mcpServers {
			if disabled.isMCPDisabled(s.Name) {
				mcpDiagnostics = append(mcpDiagnostics, "mcp server "+s.Name+": disabled by user")
				continue
			}
			filtered = append(filtered, s)
		}
		mcpServers = filtered
	}

	diagnostics := make([]string, 0, len(skills.Diagnostics)+len(plugins.Diagnostics)+len(hubDiagnostics)+len(workspaceMCPDiagnostics)+len(mcpDiagnostics))
	for _, d := range skills.Diagnostics {
		diagnostics = append(diagnostics, formatDiagnostic(d.Path, d.Message))
	}
	for _, d := range plugins.Diagnostics {
		diagnostics = append(diagnostics, formatDiagnostic(d.Path, d.Message))
	}
	diagnostics = append(diagnostics, hubDiagnostics...)
	diagnostics = append(diagnostics, workspaceMCPDiagnostics...)
	diagnostics = append(diagnostics, mcpDiagnostics...)

	nextVersion := m.version.Add(1)
	nextSnapshot := Snapshot{
		Version:     nextVersion,
		Skills:      append([]skill.Definition(nil), skills.Skills...),
		Plugins:     append([]plugin.Definition(nil), plugins.Plugins...),
		SkillPrompt: skill.FormatAvailableSkills(skills.Skills),
		MCPServers:  append([]config.MCPServer(nil), mcpServers...),
		Diagnostics: diagnostics,
	}

	// Collect tool provider tools (stub — Phase 2 will implement actual providers)
	var providerTools []tool.Tool
	var providerDiags []string
	if err := runExtensionsStartupStep("extensions_tool_provider_collect", func() error {
		providerTools, providerDiags = m.collectToolProviderTools(ctx, plugins.Plugins)
		return nil
	}); err != nil {
		return failReload(err)
	}
	if len(providerDiags) > 0 {
		nextSnapshot.Diagnostics = append(nextSnapshot.Diagnostics, providerDiags...)
	}

	if err := runExtensionsStartupStep("extensions_snapshot_publish", func() error {
		m.mu.Lock()
		m.snapshot = nextSnapshot
		allTools := append([]tool.Tool(nil), mcpTools...)
		allTools = append(allTools, providerTools...)
		m.chatTools = allTools
		m.mu.Unlock()
		return nil
	}); err != nil {
		return failReload(err)
	}
	if mcpToolsPending {
		m.buildMCPToolsAsync(ctx, nextSnapshot.Version, providerTools)
	}
	finishReload(nil)
	return nil
}

func (m *Manager) buildMCPToolsAsync(ctx context.Context, snapshotVersion int64, providerTools []tool.Tool) {
	if m == nil || m.opts.MCPRuntime == nil {
		return
	}
	providerTools = append([]tool.Tool(nil), providerTools...)
	go func() {
		finishMCPTools := beginExtensionsStartupStep("extensions_mcp_tools_build_async")
		mcpTools, err := m.opts.MCPRuntime.BuildTools(ctx)
		finishMCPTools(err)
		if err != nil && ctx.Err() != nil {
			return
		}

		m.mu.Lock()
		defer m.mu.Unlock()
		if m.snapshot.Version != snapshotVersion {
			return
		}
		m.snapshot.Version = m.version.Add(1)
		if err != nil {
			m.snapshot.Diagnostics = append(m.snapshot.Diagnostics, fmt.Sprintf("mcp tools build failed: %v", err))
			m.chatTools = append([]tool.Tool(nil), providerTools...)
			return
		}
		allTools := append([]tool.Tool(nil), mcpTools...)
		allTools = append(allTools, providerTools...)
		m.chatTools = allTools
		zlog.Logger.Info().
			Int("mcp_tools", len(mcpTools)).
			Msg("extensions mcp tools ready")
	}()
}

func runExtensionsStartupStep(step string, fn func() error) error {
	finish := beginExtensionsStartupStep(step)
	err := fn()
	finish(err)
	return err
}

func beginExtensionsStartupStep(step string) func(error) {
	startedAt := time.Now()
	zlog.Logger.Debug().Str("step", step).Msg("extensions startup step started")
	return func(err error) {
		event := zlog.Logger.Debug().
			Str("step", step).
			Int64("duration_ms", time.Since(startedAt).Milliseconds())
		if err != nil {
			event.Err(err).Msg("extensions startup step failed")
			return
		}
		event.Msg("extensions startup step completed")
	}
}

func (m *Manager) Snapshot() Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	copySnapshot := Snapshot{
		Version:     m.snapshot.Version,
		SkillPrompt: m.snapshot.SkillPrompt,
		Skills:      append([]skill.Definition(nil), m.snapshot.Skills...),
		Plugins:     append([]plugin.Definition(nil), m.snapshot.Plugins...),
		MCPServers:  append([]config.MCPServer(nil), m.snapshot.MCPServers...),
		Diagnostics: append([]string(nil), m.snapshot.Diagnostics...),
	}
	return copySnapshot
}

func (m *Manager) ChatTools() []tool.Tool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]tool.Tool(nil), m.chatTools...)
}

func (m *Manager) FindSkill(name string) (skill.Definition, bool) {
	key := normalizeSkillLookupKey(name)
	if key == "" {
		return skill.Definition{}, false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, def := range m.snapshot.Skills {
		if skillDefinitionMatchesLookup(def, key) {
			return def, true
		}
	}
	return skill.Definition{}, false
}

func normalizeSkillLookupKey(name string) string {
	key := strings.TrimSpace(name)
	key = strings.TrimPrefix(key, "/")
	return strings.ToLower(strings.TrimSpace(key))
}

func skillDefinitionMatchesLookup(def skill.Definition, key string) bool {
	if key == "" {
		return false
	}
	if normalizeSkillLookupKey(def.Name) == key {
		return true
	}
	if normalizeSkillLookupKey(def.Slash) == key {
		return true
	}
	for _, alias := range def.Aliases {
		if normalizeSkillLookupKey(alias) == key {
			return true
		}
	}
	return false
}

func (m *Manager) watchLoop(ctx context.Context, watcher *fsnotify.Watcher) {
	debounce := m.opts.WatchDebounce
	if debounce <= 0 {
		debounce = 200 * time.Millisecond
	}
	var timer *time.Timer
	var timerCh <-chan time.Time

	resetTimer := func() {
		if timer == nil {
			timer = time.NewTimer(debounce)
			timerCh = timer.C
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(debounce)
		timerCh = timer.C
	}

	for {
		select {
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Create != 0 {
				info, err := os.Stat(event.Name)
				if err == nil && info.IsDir() {
					_ = addWatchRecursive(watcher, event.Name)
				}
			}
			resetTimer()
		case <-timerCh:
			_ = m.Reload(context.Background())
			timerCh = nil
		case _, ok := <-watcher.Errors:
			if !ok {
				return
			}
		}
	}
}

func (m *Manager) watchDirs() []string {
	dirs := make([]string, 0, len(m.opts.SkillSources)+len(m.opts.PluginSources))
	if m.opts.WatchSkills {
		for _, source := range m.opts.SkillSources {
			if strings.TrimSpace(source.Dir) == "" {
				continue
			}
			dirs = append(dirs, source.Dir)
		}
	}
	if m.opts.WatchPlugins {
		for _, source := range m.opts.PluginSources {
			if strings.TrimSpace(source.Dir) == "" {
				continue
			}
			dirs = append(dirs, source.Dir)
		}
	}
	return uniqueStrings(dirs)
}

func pluginIDs(defs []plugin.Definition) []string {
	if len(defs) == 0 {
		return nil
	}
	out := make([]string, 0, len(defs))
	for _, def := range defs {
		if strings.TrimSpace(def.ID) == "" {
			continue
		}
		out = append(out, def.ID)
	}
	return out
}

func addWatchRecursive(w *fsnotify.Watcher, root string) error {
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat watch dir %q: %w", root, err)
	}
	if !info.IsDir() {
		return nil
	}
	if err := w.Add(root); err != nil {
		return fmt.Errorf("watch dir %q: %w", root, err)
	}
	return filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if !d.IsDir() || path == root {
			return nil
		}
		_ = w.Add(path)
		return nil
	})
}

func toPluginSources(sources []PluginSourceDir) []plugin.SourceDir {
	out := make([]plugin.SourceDir, 0, len(sources))
	for _, source := range sources {
		out = append(out, plugin.SourceDir{
			Source: source.Source,
			Dir:    source.Dir,
		})
	}
	return out
}

func mergeSkillSources(base []skill.SourceDir, plugins []plugin.Definition, pluginSkillDirs []string) []skill.SourceDir {
	out := append([]skill.SourceDir(nil), base...)
	if len(pluginSkillDirs) == 0 || len(plugins) == 0 {
		return sortSkillSources(out)
	}

	dirSource := map[string]skill.Source{}
	for _, pluginDef := range plugins {
		for _, rel := range pluginDef.Skills {
			absPath, err := filepath.Abs(filepath.Join(pluginDef.RootDir, rel))
			if err != nil {
				continue
			}
			if _, ok := dirSource[absPath]; ok {
				continue
			}
			dirSource[absPath] = toSkillSource(pluginDef.Source)
		}
	}
	for _, dir := range pluginSkillDirs {
		absPath, err := filepath.Abs(dir)
		if err != nil {
			continue
		}
		source := dirSource[absPath]
		if source == "" {
			source = skill.SourceBundled
		}
		out = append(out, skill.SourceDir{
			Source: source,
			Dir:    absPath,
		})
	}
	return sortSkillSources(out)
}

type mcpServerGroup struct {
	label   string
	servers []config.MCPServer
}

func mergeMCPServers(groups ...mcpServerGroup) ([]config.MCPServer, []string) {
	out := make([]config.MCPServer, 0)
	diagnostics := make([]string, 0)
	index := map[string]int{}
	owners := make([]string, 0)
	for _, group := range groups {
		for _, server := range group.servers {
			name := strings.ToLower(strings.TrimSpace(server.Name))
			if name == "" {
				continue
			}
			server.Source = group.label
			if idx, ok := index[name]; ok {
				diagnostics = append(diagnostics, fmt.Sprintf("mcp server %q from %s overrides %s source", server.Name, group.label, owners[idx]))
				out[idx] = server
				owners[idx] = group.label
				continue
			}
			index[name] = len(out)
			out = append(out, server)
			owners = append(owners, group.label)
		}
	}
	return out, diagnostics
}

func toSkillSource(source plugin.Source) skill.Source {
	switch source {
	case plugin.SourceWorkspace:
		return skill.SourceWorkspace
	case plugin.SourceUser:
		return skill.SourceUser
	default:
		return skill.SourceBundled
	}
}

func sortSkillSources(sources []skill.SourceDir) []skill.SourceDir {
	sort.SliceStable(sources, func(i, j int) bool {
		return sourceRank(sources[i].Source) < sourceRank(sources[j].Source)
	})
	return sources
}

func sourceRank(source skill.Source) int {
	switch source {
	case skill.SourceBundled:
		return 0
	case skill.SourceUser:
		return 1
	case skill.SourceWorkspace:
		return 2
	default:
		return 3
	}
}

func formatDiagnostic(path string, message string) string {
	path = strings.TrimSpace(path)
	message = strings.TrimSpace(message)
	if path == "" {
		return message
	}
	return fmt.Sprintf("%s: %s", path, message)
}

// collectToolProviderTools collects tools from plugins that declare a
// tools_provider. RF-007 removed the builtin plugin path entirely, so
// only mcp_server (already routed through MCPRuntime) is recognized;
// every other tools_provider value yields a diagnostic.
func (m *Manager) collectToolProviderTools(_ context.Context, plugins []plugin.Definition) ([]tool.Tool, []string) {
	var tools []tool.Tool
	var diagnostics []string

	for _, p := range plugins {
		if p.ToolsProvider == nil {
			continue
		}
		switch p.ToolsProvider.Type {
		case "mcp_server":
			// Already handled by MCPRuntime — skip to avoid double-registration.
		default:
			diagnostics = append(diagnostics, fmt.Sprintf(
				"plugin %q: tools_provider type %q is not supported (use mcp_server; "+
					"the builtin and script paths were removed in RF-007)",
				p.ID, p.ToolsProvider.Type,
			))
		}
	}
	return tools, diagnostics
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}
