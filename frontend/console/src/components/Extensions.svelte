<script lang="ts">
  import { onMount } from 'svelte'
  import {
    getHubRegistry,
    getHubInstalled,
    getHubSkillContent,
    getSkillDetail,
    getDisabledExtensions,
    setExtensionDisabled,
    hubInstall,
    hubUninstall,
    hubUpdate,
    listSkills,
    listPlugins,
    listMCPServers,
    reloadExtensions,
  } from '../lib/api'
  import { renderMarkdown } from '../lib/markdown'
  import type {
    HubRegistry,
    HubRegistryEntry,
    HubInstalledItem,
    SkillDef,
    PluginDef,
    MCPServerStatus,
  } from '../lib/types'

  type Tab = 'hub' | 'installed'

  let tab: Tab = $state('installed')
  let loading = $state(true)
  let error = $state('')
  let success = $state('')

  // Hub tab
  let registry: HubRegistry | null = $state(null)
  let installedNames: Set<string> = $state(new Set())   // all loaded (runtime + hub)
  let hubInstalledNames: Set<string> = $state(new Set()) // hub DB only (can uninstall)
  let hubLoading = $state(false)
  let busyItem = $state('')

  // Installed tab
  let skills: SkillDef[] = $state([])
  let plugins: PluginDef[] = $state([])
  let mcpServers: MCPServerStatus[] = $state([])
  let installed: { skills: HubInstalledItem[]; plugins: HubInstalledItem[]; mcps: HubInstalledItem[] } = $state({ skills: [], plugins: [], mcps: [] })
  let disabledSkills: Set<string> = $state(new Set())
  let disabledPlugins: Set<string> = $state(new Set())
  let disabledMCPs: Set<string> = $state(new Set())
  let reloading = $state(false)
  let updating = $state(false)
  let togglingItem = $state('')

  // Version tracking for update detection
  let installedVersions: Map<string, string> = $state(new Map())
  let registryVersions: Map<string, string> = $state(new Map())

  function hasUpdate(type: string, name: string): boolean {
    const instVer = installedVersions.get(type + ':' + name)
    const regVer = registryVersions.get(type + ':' + name)
    if (!instVer || !regVer) return false
    return instVer !== regVer
  }

  function registryVersion(type: string, name: string): string {
    return registryVersions.get(type + ':' + name) || ''
  }

  let updateCount = $derived.by(() => {
    let count = 0
    for (const [key] of installedVersions) {
      const reg = registryVersions.get(key)
      if (reg && reg !== installedVersions.get(key)) count++
    }
    return count
  })

  // Detail panel
  let detailKey: string | null = $state(null)
  let detailContent = $state('')
  let detailLoading = $state(false)
  let detailMeta: Record<string, string> = $state({})

  async function toggleDetail(kind: string, name: string, source: 'installed' | 'hub') {
    const key = `${source}-${kind}:${name}`
    if (detailKey === key) { detailKey = null; return }
    detailKey = key
    detailContent = ''
    detailMeta = {}
    detailLoading = true
    try {
      if (kind === 'skill' && source === 'installed') {
        const detail = await getSkillDetail(name)
        detailContent = detail.content || 'No content available.'
        detailMeta = { source: detail.source || '', invocable: detail.user_invocable ? 'Yes — use /' + name + ' in chat' : 'No — system use only' }
      } else if (kind === 'skill' && source === 'hub') {
        const result = await getHubSkillContent(name)
        detailContent = result.content || 'No content available.'
        detailMeta = { version: result.version }
      } else {
        detailContent = 'Detail view is available for skills.'
      }
    } catch { detailContent = 'Failed to load details.' }
    finally { detailLoading = false }
  }

  function isDetailOpen(kind: string, name: string, source: 'installed' | 'hub'): boolean {
    return detailKey === `${source}-${kind}:${name}`
  }

  async function loadInstalled() {
    loading = true
    error = ''
    try {
      const [s, p, m, inst, dis] = await Promise.all([listSkills(), listPlugins(), listMCPServers(), getHubInstalled(), getDisabledExtensions()])
      skills = s
      plugins = p
      mcpServers = m
      installed = inst
      // Hub DB items (can be uninstalled via hub)
      const hubNames = new Set<string>()
      for (const i of inst.skills) hubNames.add('skill:' + i.name)
      for (const i of inst.plugins) hubNames.add('plugin:' + i.name)
      for (const i of inst.mcps) hubNames.add('mcp:' + i.name)
      hubInstalledNames = hubNames

      // All loaded names (hub + runtime) for "Installed" badge in Hub tab
      const names = new Set(hubNames)
      for (const sk of s) names.add('skill:' + sk.name)
      for (const pl of p) names.add('plugin:' + (pl.id || pl.name))
      for (const mc of m) names.add('mcp:' + mc.name)
      installedNames = names
      disabledSkills = new Set((dis.skills ?? []).map((n: string) => n.toLowerCase()))
      disabledPlugins = new Set((dis.plugins ?? []).map((n: string) => n.toLowerCase()))
      disabledMCPs = new Set((dis.mcp_servers ?? []).map((n: string) => n.toLowerCase()))

      // Track installed versions for update detection
      const versions = new Map<string, string>()
      for (const i of inst.skills) versions.set('skill:' + i.name, i.version || '')
      for (const i of inst.plugins) versions.set('plugin:' + i.name, i.version || '')
      for (const i of inst.mcps) versions.set('mcp:' + i.name, i.version || '')
      installedVersions = versions
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load extensions'
    } finally {
      loading = false
    }
  }

  async function loadHub() {
    hubLoading = true
    error = ''
    try {
      const raw = await getHubRegistry()
      registry = {
        version: raw.version ?? 0,
        skills: raw.skills ?? [],
        plugins: raw.plugins ?? [],
        mcp_servers: raw.mcp_servers ?? [],
      }
      // Track registry versions for update detection
      const regVers = new Map<string, string>()
      for (const e of registry.skills) regVers.set('skill:' + e.name, e.version || '')
      for (const e of registry.plugins) regVers.set('plugin:' + e.name, e.version || '')
      for (const e of registry.mcp_servers) regVers.set('mcp:' + e.name, e.version || '')
      registryVersions = regVers
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to fetch registry'
    } finally {
      hubLoading = false
    }
  }

  function isInstalled(type: string, name: string): boolean {
    return installedNames.has(type + ':' + name)
  }

  function isHubInstalled(type: string, name: string): boolean {
    return hubInstalledNames.has(type + ':' + name)
  }

  async function handleInstall(type: string, name: string) {
    busyItem = type + ':' + name
    error = ''
    success = ''
    try {
      await hubInstall(type, name)
      installedNames = new Set([...installedNames, type + ':' + name])
      success = `Installed ${type} "${name}"`
      await loadInstalled()
    } catch (e) {
      error = e instanceof Error ? e.message : 'Install failed'
    } finally {
      busyItem = ''
    }
  }

  async function handleUninstall(type: string, name: string) {
    busyItem = type + ':' + name
    error = ''
    success = ''
    try {
      await hubUninstall(type, name)
      const next = new Set(installedNames)
      next.delete(type + ':' + name)
      installedNames = next
      success = `Uninstalled ${type} "${name}"`
      await loadInstalled()
    } catch (e) {
      error = e instanceof Error ? e.message : 'Uninstall failed'
    } finally {
      busyItem = ''
    }
  }

  function isDisabledExt(kind: string, name: string): boolean {
    const key = name.toLowerCase()
    if (kind === 'skill') return disabledSkills.has(key)
    if (kind === 'plugin') return disabledPlugins.has(key)
    if (kind === 'mcp') return disabledMCPs.has(key)
    return false
  }

  async function handleToggle(kind: string, name: string) {
    const currently = isDisabledExt(kind, name)
    togglingItem = kind + ':' + name
    error = ''
    success = ''
    try {
      await setExtensionDisabled(kind, name, !currently)
      success = `${name} ${currently ? 'enabled' : 'disabled'}. Extensions reloaded.`
      await loadInstalled()
    } catch (e) {
      error = e instanceof Error ? e.message : 'Toggle failed'
    } finally {
      togglingItem = ''
    }
  }

  async function handleReload() {
    reloading = true
    error = ''
    success = ''
    try {
      const result = await reloadExtensions()
      success = `Reloaded: ${result.skills} skills, ${result.plugins} plugins, ${result.mcp_count} MCP servers`
      await loadInstalled()
    } catch (e) {
      error = e instanceof Error ? e.message : 'Reload failed'
    } finally {
      reloading = false
    }
  }

  async function handleUpdateAll() {
    updating = true
    error = ''
    success = ''
    try {
      const result = await hubUpdate()
      const total = (result.updated_skills?.length ?? 0) + (result.updated_plugins?.length ?? 0)
      success = total > 0 ? `Updated ${total} packages` : 'Everything is up to date'
      await loadInstalled()
    } catch (e) {
      error = e instanceof Error ? e.message : 'Update failed'
    } finally {
      updating = false
    }
  }

  function switchTab(t: Tab) {
    tab = t
    if (t === 'hub' && !registry) void loadHub()
  }

  onMount(() => {
    void loadInstalled()
    void loadHub() // fetch registry for version comparison
  })
</script>

<div class="ext-page">
  <div class="page-header">
    <div>
      <h2>Extensions</h2>
      <p class="page-subtitle">Manage skills, plugins, and MCP servers</p>
    </div>
    <div class="page-actions">
      <div class="view-toggle">
        <button class="toggle-btn" class:active={tab === 'installed'} onclick={() => switchTab('installed')}>Installed</button>
        <button class="toggle-btn" class:active={tab === 'hub'} onclick={() => switchTab('hub')}>Hub</button>
      </div>
    </div>
  </div>

  {#if error}
    <div class="message message-error">{error}</div>
  {/if}
  {#if success}
    <div class="message message-success">{success}</div>
  {/if}

  {#if tab === 'installed'}
    <!-- Installed Extensions -->
    <div class="ext-toolbar">
      <button class="btn btn-ghost btn-sm" disabled={reloading} onclick={handleReload}>
        {reloading ? 'Reloading...' : 'Reload'}
      </button>
      <button class="btn btn-ghost btn-sm" disabled={updating || updateCount === 0} onclick={handleUpdateAll}>
        {updating ? 'Updating...' : 'Update All'}
      </button>
      {#if updateCount > 0}
        <span class="badge badge-warning">{updateCount} update{updateCount > 1 ? 's' : ''}</span>
      {/if}
    </div>

    {#if loading}
      <div class="ext-loading">Loading extensions...</div>
    {:else}
      <!-- Skills -->
      <section class="card ext-section">
        <div class="card-header">
          <span class="card-title">Skills</span>
          <span class="badge badge-default">{skills.length}</span>
        </div>
        {#if skills.length === 0}
          <div class="empty-state"><p>No skills loaded.</p></div>
        {:else}
          <div class="ext-list">
            {#each skills as s}
              <div class="ext-item-wrapper">
                <div class="ext-item">
                  <div class="ext-item-info">
                    <button class="ext-name-btn" onclick={() => toggleDetail('skill', s.name, 'installed')}>
                      <strong>{s.name}</strong>
                      <span class="detail-chevron" class:open={isDetailOpen('skill', s.name, 'installed')}>{'\u25b8'}</span>
                    </button>
                    <span class="ext-desc">{s.description || '\u2014'}</span>
                    <div class="ext-meta">
                      {#if s.source}<span class="badge badge-default">{s.source}</span>{/if}
                      {#if s.user_invocable}<span class="badge badge-accent" title="User can invoke via /skill-name in chat">/{s.name}</span>{/if}
                    </div>
                  </div>
                  <div class="ext-item-actions">
                    <button class="toggle-switch" class:on={!isDisabledExt('skill', s.name)} disabled={togglingItem === 'skill:' + s.name} onclick={() => handleToggle('skill', s.name)}>{isDisabledExt('skill', s.name) ? 'OFF' : 'ON'}</button>
                    {#if hasUpdate('skill', s.name)}
                      <button class="btn btn-warning btn-sm" disabled={busyItem === 'skill:' + s.name} onclick={() => handleInstall('skill', s.name)} title="Update to v{registryVersion('skill', s.name)}">
                        {busyItem === 'skill:' + s.name ? '...' : 'Update'}
                      </button>
                    {/if}
                    {#if isHubInstalled('skill', s.name)}
                      <button class="btn btn-danger btn-sm" disabled={busyItem === 'skill:' + s.name} onclick={() => handleUninstall('skill', s.name)}>{busyItem === 'skill:' + s.name ? '...' : 'Uninstall'}</button>
                    {/if}
                  </div>
                </div>
                {#if isDetailOpen('skill', s.name, 'installed')}
                  <div class="ext-detail">
                    {#if detailLoading}<div class="ext-detail-loading">Loading...</div>
                    {:else}
                      {#if Object.keys(detailMeta).length > 0}
                        <div class="ext-detail-meta">{#each Object.entries(detailMeta) as [k, v]}{#if v}<span><strong>{k}:</strong> {v}</span>{/if}{/each}</div>
                      {/if}
                      <div class="ext-detail-content ext-md">{@html renderMarkdown(detailContent)}</div>
                    {/if}
                  </div>
                {/if}
              </div>
            {/each}
          </div>
        {/if}
      </section>

      <!-- Plugins -->
      <section class="card ext-section">
        <div class="card-header">
          <span class="card-title">Plugins</span>
          <span class="badge badge-default">{plugins.length}</span>
        </div>
        {#if plugins.length === 0}
          <div class="empty-state"><p>No plugins loaded.</p></div>
        {:else}
          <div class="ext-list">
            {#each plugins as p}
              <div class="ext-item">
                <div class="ext-item-info">
                  <strong>{p.name || p.id}</strong>
                  <span class="ext-desc">{p.description || '\u2014'}</span>
                  {#if p.version}<span class="ext-meta-tag">v{p.version}</span>{/if}
                </div>
                <div class="ext-item-actions">
                  <button
                    class="toggle-switch"
                    class:on={!isDisabledExt('plugin', p.id || p.name)}
                    disabled={togglingItem === 'plugin:' + (p.id || p.name)}
                    title={isDisabledExt('plugin', p.id || p.name) ? 'Enable' : 'Disable'}
                    onclick={() => handleToggle('plugin', p.id || p.name)}
                  >{isDisabledExt('plugin', p.id || p.name) ? 'OFF' : 'ON'}</button>
                  {#if hasUpdate('plugin', p.id || p.name)}
                    <button class="btn btn-warning btn-sm" disabled={busyItem === 'plugin:' + (p.id || p.name)} onclick={() => handleInstall('plugin', p.id || p.name)} title="Update to v{registryVersion('plugin', p.id || p.name)}">
                      {busyItem === 'plugin:' + (p.id || p.name) ? '...' : 'Update'}
                    </button>
                  {/if}
                  {#if isHubInstalled('plugin', p.id || p.name)}
                    <button class="btn btn-danger btn-sm" disabled={busyItem === 'plugin:' + (p.id || p.name)} onclick={() => handleUninstall('plugin', p.id || p.name)}>
                      {busyItem === 'plugin:' + (p.id || p.name) ? '...' : 'Uninstall'}
                    </button>
                  {/if}
                </div>
              </div>
            {/each}
          </div>
        {/if}
      </section>

      <!-- MCP Servers -->
      <section class="card ext-section">
        <div class="card-header">
          <span class="card-title">MCP Servers</span>
          <span class="badge badge-default">{mcpServers.length}</span>
        </div>
        {#if mcpServers.length === 0}
          <div class="empty-state"><p>No MCP servers configured.</p></div>
        {:else}
          <div class="ext-list">
            {#each mcpServers as m}
              <div class="ext-item">
                <div class="ext-item-info">
                  <strong>{m.name}</strong>
                  <div class="ext-meta">
                    {#if m.transport}<span class="badge badge-default">{m.transport}</span>{/if}
                    {#if m.source}<span class="ext-meta-tag">{m.source}</span>{/if}
                    {#if m.tools_count}<span class="ext-meta-tag">{m.tools_count} tools</span>{/if}
                  </div>
                </div>
                <div class="ext-item-actions">
                  <button
                    class="toggle-switch"
                    class:on={!isDisabledExt('mcp', m.name)}
                    disabled={togglingItem === 'mcp:' + m.name}
                    title={isDisabledExt('mcp', m.name) ? 'Enable' : 'Disable'}
                    onclick={() => handleToggle('mcp', m.name)}
                  >{isDisabledExt('mcp', m.name) ? 'OFF' : 'ON'}</button>
                  {#if hasUpdate('mcp', m.name)}
                    <button class="btn btn-warning btn-sm" disabled={busyItem === 'mcp:' + m.name} onclick={() => handleInstall('mcp', m.name)} title="Update to v{registryVersion('mcp', m.name)}">
                      {busyItem === 'mcp:' + m.name ? '...' : 'Update'}
                    </button>
                  {/if}
                  {#if isHubInstalled('mcp', m.name)}
                    <button class="btn btn-danger btn-sm" disabled={busyItem === 'mcp:' + m.name} onclick={() => handleUninstall('mcp', m.name)}>
                      {busyItem === 'mcp:' + m.name ? '...' : 'Uninstall'}
                    </button>
                  {/if}
                </div>
              </div>
            {/each}
          </div>
        {/if}
      </section>
    {/if}

  {:else}
    <!-- Hub (Registry Browser) -->
    {#if hubLoading}
      <div class="ext-loading">Fetching registry...</div>
    {:else if !registry}
      <div class="ext-loading">Loading...</div>
    {:else}
      <!-- Hub Skills -->
      <section class="card ext-section">
        <div class="card-header">
          <span class="card-title">Skills</span>
          <span class="badge badge-default">{registry.skills.length} available</span>
        </div>
        <div class="ext-list">
          {#each registry.skills as entry}
            <div class="ext-item-wrapper">
              <div class="ext-item">
                <div class="ext-item-info">
                  <div class="ext-item-top">
                    <button class="ext-name-btn" onclick={() => toggleDetail('skill', entry.name, 'hub')}>
                      <strong>{entry.name}</strong>
                      <span class="detail-chevron" class:open={isDetailOpen('skill', entry.name, 'hub')}>{'\u25b8'}</span>
                    </button>
                    <span class="ext-version">v{entry.version}</span>
                    {#if entry.author}<span class="ext-meta-tag">by {entry.author}</span>{/if}
                  </div>
                  <span class="ext-desc">{entry.description}</span>
                  {#if entry.tags?.length}
                    <div class="ext-tags">{#each entry.tags as tag}<span class="ext-tag">{tag}</span>{/each}</div>
                  {/if}
                </div>
                {#if hasUpdate('skill', entry.name)}
                  <button class="btn btn-warning btn-sm" disabled={busyItem === 'skill:' + entry.name} onclick={() => handleInstall('skill', entry.name)}>
                    {busyItem === 'skill:' + entry.name ? 'Updating...' : 'Update'}
                  </button>
                {:else if isInstalled('skill', entry.name)}
                  <span class="badge badge-success">Installed</span>
                {:else}
                  <button class="btn btn-primary btn-sm" disabled={busyItem === 'skill:' + entry.name} onclick={() => handleInstall('skill', entry.name)}>{busyItem === 'skill:' + entry.name ? 'Installing...' : 'Install'}</button>
                {/if}
              </div>
              {#if isDetailOpen('skill', entry.name, 'hub')}
                <div class="ext-detail">
                  {#if detailLoading}<div class="ext-detail-loading">Loading...</div>
                  {:else}<div class="ext-detail-content ext-md">{@html renderMarkdown(detailContent)}</div>{/if}
                </div>
              {/if}
            </div>
          {/each}
        </div>
      </section>

      <!-- Hub Plugins -->
      <section class="card ext-section">
        <div class="card-header">
          <span class="card-title">Plugins</span>
          <span class="badge badge-default">{registry.plugins.length} available</span>
        </div>
        <div class="ext-list">
          {#each registry.plugins as entry}
            <div class="ext-item">
              <div class="ext-item-info">
                <div class="ext-item-top">
                  <strong>{entry.name}</strong>
                  <span class="ext-version">v{entry.version}</span>
                </div>
                <span class="ext-desc">{entry.description}</span>
                {#if entry.tags?.length}
                  <div class="ext-tags">
                    {#each entry.tags as tag}<span class="ext-tag">{tag}</span>{/each}
                  </div>
                {/if}
              </div>
              {#if hasUpdate('plugin', entry.name)}
                <button class="btn btn-warning btn-sm" disabled={busyItem === 'plugin:' + entry.name} onclick={() => handleInstall('plugin', entry.name)}>
                  {busyItem === 'plugin:' + entry.name ? 'Updating...' : 'Update'}
                </button>
              {:else if isInstalled('plugin', entry.name)}
                <span class="badge badge-success">Installed</span>
              {:else}
                <button class="btn btn-primary btn-sm" disabled={busyItem === 'plugin:' + entry.name} onclick={() => handleInstall('plugin', entry.name)}>
                  {busyItem === 'plugin:' + entry.name ? 'Installing...' : 'Install'}
                </button>
              {/if}
            </div>
          {/each}
        </div>
      </section>

      <!-- Hub MCP Servers -->
      <section class="card ext-section">
        <div class="card-header">
          <span class="card-title">MCP Servers</span>
          <span class="badge badge-default">{registry.mcp_servers.length} available</span>
        </div>
        <div class="ext-list">
          {#each registry.mcp_servers as entry}
            <div class="ext-item">
              <div class="ext-item-info">
                <div class="ext-item-top">
                  <strong>{entry.name}</strong>
                  <span class="ext-version">v{entry.version}</span>
                </div>
                <span class="ext-desc">{entry.description}</span>
                {#if entry.tags?.length}
                  <div class="ext-tags">
                    {#each entry.tags as tag}<span class="ext-tag">{tag}</span>{/each}
                  </div>
                {/if}
              </div>
              {#if hasUpdate('mcp', entry.name)}
                <button class="btn btn-warning btn-sm" disabled={busyItem === 'mcp:' + entry.name} onclick={() => handleInstall('mcp', entry.name)}>
                  {busyItem === 'mcp:' + entry.name ? 'Updating...' : 'Update'}
                </button>
              {:else if isInstalled('mcp', entry.name)}
                <span class="badge badge-success">Installed</span>
              {:else}
                <button class="btn btn-primary btn-sm" disabled={busyItem === 'mcp:' + entry.name} onclick={() => handleInstall('mcp', entry.name)}>
                  {busyItem === 'mcp:' + entry.name ? 'Installing...' : 'Install'}
                </button>
              {/if}
            </div>
          {/each}
        </div>
      </section>
    {/if}
  {/if}
</div>

<style>
  .ext-page {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    animation: fadeIn var(--duration-normal) var(--ease-out);
  }

  @keyframes fadeIn {
    from { opacity: 0; transform: translateY(8px); }
    to { opacity: 1; transform: translateY(0); }
  }

  .page-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-4);
  }
  .page-header h2 { font-size: var(--text-2xl); margin-bottom: var(--space-1); }
  .page-subtitle { color: var(--text-tertiary); font-size: var(--text-sm); }
  .page-actions { display: flex; gap: var(--space-2); flex-shrink: 0; }

  .view-toggle { display: flex; background: var(--bg-elevated); border-radius: var(--radius-md); padding: 2px; gap: 2px; }
  .toggle-btn {
    padding: var(--space-1) var(--space-3);
    border: none; border-radius: var(--radius-sm);
    background: transparent; color: var(--text-secondary);
    font-family: var(--font-display); font-size: var(--text-sm); font-weight: 500;
    cursor: pointer; transition: all var(--duration-fast) var(--ease-out);
  }
  .toggle-btn:hover { color: var(--text-primary); }
  .toggle-btn.active { background: var(--accent); color: #fff; }

  .ext-toolbar { display: flex; gap: var(--space-2); }
  .ext-loading { padding: var(--space-10); text-align: center; color: var(--text-tertiary); }

  .message { font-size: var(--text-sm); padding: var(--space-2) var(--space-3); border-radius: var(--radius-md); }
  .message-error { background: rgba(220, 60, 60, 0.15); color: var(--red); border: 1px solid rgba(220, 60, 60, 0.3); }
  .message-success { background: rgba(60, 180, 100, 0.15); color: var(--green); border: 1px solid rgba(60, 180, 100, 0.3); }

  .ext-section { margin-bottom: var(--space-2); }
  .ext-list { display: flex; flex-direction: column; }

  .ext-item {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    padding: var(--space-3) var(--space-4);
    border-bottom: 1px solid var(--border-subtle);
  }
  .ext-item:last-child { border-bottom: none; }
  .ext-item:hover { background: rgba(255, 255, 255, 0.015); }

  .ext-item-info { display: flex; flex-direction: column; gap: 2px; min-width: 0; flex: 1; }
  .ext-item-info strong {
    font-family: var(--font-display); font-size: var(--text-sm); font-weight: 500; color: var(--text-primary);
  }
  .ext-item-top { display: flex; align-items: center; gap: var(--space-2); }

  .ext-desc { font-size: var(--text-xs); color: var(--text-secondary); line-height: 1.4; }
  .ext-version { font-family: var(--font-mono); font-size: 10px; color: var(--text-ghost); }
  .ext-meta { display: flex; gap: var(--space-1); flex-wrap: wrap; margin-top: 2px; }
  .ext-meta-tag { font-family: var(--font-mono); font-size: 10px; color: var(--text-ghost); }

  .ext-item-actions { display: flex; align-items: center; gap: var(--space-2); flex-shrink: 0; }

  /* ── Toggle switch ─────────────────────── */
  .toggle-switch {
    padding: 3px var(--space-2);
    border-radius: var(--radius-sm);
    border: 1px solid var(--border-subtle);
    font-family: var(--font-display);
    font-size: var(--text-xs);
    font-weight: 600;
    letter-spacing: 0.04em;
    cursor: pointer;
    transition: all var(--duration-fast) var(--ease-out);
    background: rgba(255, 255, 255, 0.04);
    color: var(--text-ghost);
    min-width: 38px;
  }
  .toggle-switch.on {
    background: rgba(60, 180, 100, 0.15);
    color: var(--green);
    border-color: rgba(60, 180, 100, 0.3);
  }
  .toggle-switch:hover { transform: scale(1.05); }
  .toggle-switch:disabled { opacity: 0.5; cursor: default; transform: none; }

  .ext-item-wrapper { border-bottom: 1px solid var(--border-subtle); }
  .ext-item-wrapper:last-child { border-bottom: none; }
  .ext-item-wrapper .ext-item { border-bottom: none; }
  .ext-name-btn { display: flex; align-items: center; gap: var(--space-1); background: none; border: none; cursor: pointer; padding: 0; }
  .ext-name-btn strong { color: var(--text-primary); font-family: var(--font-display); font-size: var(--text-sm); font-weight: 500; }
  .ext-name-btn:hover strong { color: var(--accent); }
  .detail-chevron { font-size: 10px; color: var(--text-ghost); transition: transform var(--duration-fast) var(--ease-out); display: inline-block; }
  .detail-chevron.open { transform: rotate(90deg); }
  .ext-detail { padding: var(--space-3) var(--space-4); background: var(--bg-base); border-top: 1px solid var(--border-subtle); }
  .ext-detail-loading { color: var(--text-tertiary); font-size: var(--text-xs); }
  .ext-detail-meta { display: flex; flex-wrap: wrap; gap: var(--space-3); margin-bottom: var(--space-3); padding-bottom: var(--space-2); border-bottom: 1px solid var(--border-subtle); font-size: var(--text-xs); color: var(--text-secondary); }
  .ext-detail-meta strong { color: var(--text-tertiary); margin-right: 2px; }
  .ext-detail-content { font-size: var(--text-sm); line-height: 1.6; color: var(--text-secondary); max-height: 400px; overflow-y: auto; }
  .ext-md :global(h1), .ext-md :global(h2), .ext-md :global(h3) { font-family: var(--font-display); font-weight: 600; color: var(--text-primary); margin: var(--space-3) 0 var(--space-1); }
  .ext-md :global(h1) { font-size: var(--text-base); }
  .ext-md :global(h2) { font-size: var(--text-sm); }
  .ext-md :global(p) { margin: 0 0 var(--space-2); }
  .ext-md :global(ul), .ext-md :global(ol) { margin: var(--space-1) 0; padding-left: var(--space-5); }
  .ext-md :global(li) { margin-bottom: var(--space-1); font-size: var(--text-sm); }
  .ext-md :global(code) { font-family: var(--font-mono); font-size: 0.9em; background: rgba(255,255,255,0.06); padding: 1px 5px; border-radius: 3px; }
  .ext-md :global(pre) { background: var(--bg-surface); border: 1px solid var(--border-subtle); border-radius: var(--radius-sm); padding: var(--space-2); overflow-x: auto; margin: var(--space-2) 0; font-family: var(--font-mono); font-size: var(--text-xs); }
  .ext-md :global(pre code) { background: none; padding: 0; }
  .ext-md :global(strong) { font-weight: 600; color: var(--text-primary); }

  .ext-tags { display: flex; gap: var(--space-1); flex-wrap: wrap; margin-top: 2px; }
  .ext-tag {
    padding: 1px var(--space-1); border-radius: var(--radius-sm);
    background: var(--bg-elevated); font-size: 10px; color: var(--text-tertiary);
  }
</style>
