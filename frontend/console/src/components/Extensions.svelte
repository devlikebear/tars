<script lang="ts">
  import { onMount } from 'svelte'
  import {
    getHubRegistry,
    getHubInstalled,
    getHubSkillContent,
    getSkillDetail,
    getExtensionsHealth,
    getDisabledExtensions,
    setExtensionDisabled,
    repairExtension,
    APIRequestError,
    hubInstall,
    hubUninstall,
    hubUpdate,
    listSkills,
    listMCPServers,
    reloadExtensions,
  } from '../lib/api'
  import { renderMarkdown } from '../lib/markdown'
  import SkillCreator from './SkillCreator.svelte'
  import MCPServerCreator from './MCPServerCreator.svelte'
  import { t } from '../i18n'
  import type {
    HubRegistry,
    HubRegistryEntry,
    HubInstalled,
    SkillDef,
    MCPServerStatus,
    ExtensionHealthItem,
    ExtensionHealthResponse,
    ExtensionHealthStatus,
    SkillCreatorSaveResponse,
    MCPServerCreatorSaveResponse,
    SkillSandboxReport,
  } from '../lib/types'

  type Tab = 'hub' | 'installed'

  let tab: Tab = $state('installed')
  let loading = $state(true)
  let error = $state('')
  let success = $state('')
  let skillSandboxReport: SkillSandboxReport | null = $state(null)

  // Hub tab
  let registry: HubRegistry | null = $state(null)
  let installedNames: Set<string> = $state(new Set())   // all loaded (runtime + hub)
  let hubInstalledNames: Set<string> = $state(new Set()) // hub DB only (can uninstall)
  let hubLoading = $state(false)
  let busyItem = $state('')

  // Installed tab
  let skills: SkillDef[] = $state([])
  let mcpServers: MCPServerStatus[] = $state([])
  let installed: HubInstalled = $state({ skills: [], plugins: [], mcps: [] })
  let disabledSkills: Set<string> = $state(new Set())
  let disabledMCPs: Set<string> = $state(new Set())
  let extensionHealth: ExtensionHealthResponse | null = $state(null)
  let reloading = $state(false)
  let updating = $state(false)
  let diagnosing = $state(false)
  let togglingItem = $state('')
  let repairingItem = $state('')
  let skillCreatorOpen = $state(false)
  let mcpCreatorOpen = $state(false)

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

  function skillSlashLabel(skill: SkillDef): string {
    return '/' + (skill.slash || skill.name).replace(/^\/+/, '')
  }

  function sandboxSummary(report: SkillSandboxReport): string {
    const total = report.checks?.length ?? 0
    const passed = report.checks?.filter((check) => check.status === 'passed').length ?? 0
    return $t.extensions.sandboxChecksSummary(passed, total)
  }

  function sandboxTitle(report: SkillSandboxReport): string {
    const name = report.package_name || report.skill_name || 'package'
    return report.package_type ? `${report.package_type} ${name}` : name
  }

  function healthItem(kind: 'skill' | 'mcp', name: string): ExtensionHealthItem | null {
    const list = kind === 'skill' ? extensionHealth?.skills : extensionHealth?.mcp_servers
    return list?.find((item) => item.name.toLowerCase() === name.toLowerCase()) ?? null
  }

  function healthLabel(status: ExtensionHealthStatus): string {
    if (status === 'pass') return $t.extensions.healthPass
    if (status === 'warn') return $t.extensions.healthWarn
    if (status === 'fail') return $t.extensions.healthFail
    return $t.extensions.healthUnknown
  }

  function healthBadgeClass(item: ExtensionHealthItem | null): string {
    if (!item) return 'badge badge-default'
    if (item.status === 'pass') return 'badge badge-success'
    if (item.status === 'fail') return 'badge badge-error'
    if (item.status === 'warn') return 'badge badge-warning'
    return 'badge badge-default'
  }

  function healthDetailVisible(item: ExtensionHealthItem | null): boolean {
    return Boolean(item?.checks?.length && item.status !== 'pass')
  }

  type QualitySignal = { label: string; value: string; title?: string }

  function qualityScoreLabel(entry: HubRegistryEntry): string {
    if (!entry.quality) return ''
    const score = Math.max(0, Math.min(100, Math.round(entry.quality.score)))
    return $t.extensions.qualityScore(score)
  }

  function qualityScoreClass(entry: HubRegistryEntry): string {
    const score = entry.quality?.score ?? 0
    if (score >= 85) return 'quality-score strong'
    if (score >= 65) return 'quality-score steady'
    return 'quality-score watch'
  }

  function qualitySignals(entry: HubRegistryEntry): QualitySignal[] {
    const quality = entry.quality
    if (!quality) return []
    const signals: QualitySignal[] = []
    const labels = $t.extensions.qualityLabels
    if (quality.last_updated) {
      signals.push({ label: labels.lastUpdated, value: quality.last_updated })
    }
    if (quality.tests_passing !== undefined) {
      signals.push({ label: labels.testsPassing, value: quality.tests_passing ? labels.yes : labels.no })
    }
    if (quality.required_tools?.length) {
      signals.push({ label: labels.requiredTools, value: quality.required_tools.join(', ') })
    }
    if (quality.permissions?.length) {
      signals.push({ label: labels.permissions, value: quality.permissions.join(', ') })
    }
    if (quality.companion_cli !== undefined) {
      signals.push({ label: labels.companionCli, value: quality.companion_cli ? labels.yes : labels.no })
    }
    if (quality.install_count !== undefined) {
      signals.push({ label: labels.installs, value: quality.install_count.toLocaleString() })
    }
    return signals
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
        detailContent = detail.content || $t.extensions.detailNoContent
        detailMeta = {
          [$t.extensions.detailMetaSource]: detail.source || '',
          [$t.extensions.detailMetaInvocable]: detail.user_invocable ? $t.extensions.invocableYes(name) : $t.extensions.invocableNo,
        }
      } else if (kind === 'skill' && source === 'hub') {
        const result = await getHubSkillContent(name)
        detailContent = result.content || $t.extensions.detailNoContent
        detailMeta = { [$t.extensions.detailMetaVersion]: result.version }
      } else {
        detailContent = $t.extensions.detailFallback
      }
    } catch { detailContent = $t.extensions.detailLoadFailed }
    finally { detailLoading = false }
  }

  function isDetailOpen(kind: string, name: string, source: 'installed' | 'hub'): boolean {
    return detailKey === `${source}-${kind}:${name}`
  }

  async function loadInstalled() {
    loading = true
    error = ''
    try {
      const [s, m, inst, dis] = await Promise.all([listSkills(), listMCPServers(), getHubInstalled(), getDisabledExtensions()])
      skills = s
      mcpServers = m
      installed = inst
      // Hub DB items (can be uninstalled via hub)
      const hubNames = new Set<string>()
      for (const i of inst.skills) hubNames.add('skill:' + i.name)
      for (const i of inst.mcps) hubNames.add('mcp:' + i.name)
      hubInstalledNames = hubNames

      // All loaded names (hub + runtime) for "Installed" badge in Hub tab
      const names = new Set(hubNames)
      for (const sk of s) names.add('skill:' + sk.name)
      for (const mc of m) names.add('mcp:' + mc.name)
      installedNames = names
      disabledSkills = new Set((dis.skills ?? []).map((n: string) => n.toLowerCase()))
      disabledMCPs = new Set((dis.mcp_servers ?? []).map((n: string) => n.toLowerCase()))

      // Track installed versions for update detection
      const versions = new Map<string, string>()
      for (const i of inst.skills) versions.set('skill:' + i.name, i.version || '')
      for (const i of inst.mcps) versions.set('mcp:' + i.name, i.version || '')
      installedVersions = versions
    } catch (e) {
      error = e instanceof Error ? e.message : $t.extensions.failedLoadExtensions
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
        plugins: [],
        mcp_servers: raw.mcp_servers ?? [],
      }
      // Track registry versions for update detection
      const regVers = new Map<string, string>()
      for (const e of registry.skills) regVers.set('skill:' + e.name, e.version || '')
      for (const e of registry.mcp_servers) regVers.set('mcp:' + e.name, e.version || '')
      registryVersions = regVers
    } catch (e) {
      error = e instanceof Error ? e.message : $t.extensions.failedFetchRegistry
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
    skillSandboxReport = null
    try {
      const result = await hubInstall(type, name)
      skillSandboxReport = result.sandbox_report ?? null
      installedNames = new Set([...installedNames, type + ':' + name])
      const sandboxText = result.sandbox_report ? $t.extensions.sandboxSuffix(sandboxSummary(result.sandbox_report)) : ''
      const pluginText = result.requires_plugin ? $t.extensions.requiresPluginSuffix(result.requires_plugin) : ''
      success = $t.extensions.installSuccess(type, name, sandboxText, pluginText)
      await loadInstalled()
    } catch (e) {
      if (e instanceof APIRequestError && e.payload?.sandbox_report) {
        skillSandboxReport = e.payload.sandbox_report
      }
      error = e instanceof Error ? e.message : $t.extensions.installFailed
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
      success = $t.extensions.uninstallSuccess(type, name)
      await loadInstalled()
    } catch (e) {
      error = e instanceof Error ? e.message : $t.extensions.uninstallFailed
    } finally {
      busyItem = ''
    }
  }

  function isDisabledExt(kind: string, name: string): boolean {
    const key = name.toLowerCase()
    if (kind === 'skill') return disabledSkills.has(key)
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
      success = $t.extensions.toggledSuccess(name, currently ? $t.extensions.enabledLabel : $t.extensions.disabledLabel)
      await loadInstalled()
    } catch (e) {
      error = e instanceof Error ? e.message : $t.extensions.toggleFailed
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
      success = $t.extensions.reloadSuccess(result.skills, result.plugins, result.mcp_count)
      await loadInstalled()
    } catch (e) {
      error = e instanceof Error ? e.message : $t.extensions.reloadFailed
    } finally {
      reloading = false
    }
  }

  async function handleRunDiagnostics() {
    diagnosing = true
    error = ''
    success = ''
    try {
      extensionHealth = await getExtensionsHealth()
      success = $t.extensions.diagnosticsSuccess(extensionHealth.skills.length, extensionHealth.mcp_servers.length)
    } catch (e) {
      error = e instanceof Error ? e.message : $t.extensions.diagnosticsFailed
    } finally {
      diagnosing = false
    }
  }

  async function handleRepair(kind: 'skill' | 'mcp', name: string) {
    repairingItem = kind + ':' + name
    error = ''
    success = ''
    try {
      await repairExtension(kind, name)
      success = $t.extensions.repairSuccess(name)
      await loadInstalled()
      extensionHealth = await getExtensionsHealth()
    } catch (e) {
      error = e instanceof Error ? e.message : $t.extensions.repairFailed
    } finally {
      repairingItem = ''
    }
  }

  async function handleSkillCreated(result: SkillCreatorSaveResponse) {
    success = $t.extensions.skillCreatedSuccess(result.path)
    skillCreatorOpen = false
    await loadInstalled()
  }

  async function handleMCPServerCreated(result: MCPServerCreatorSaveResponse) {
    success = $t.extensions.mcpCreatedSuccess(result.path)
    mcpCreatorOpen = false
    await reloadExtensions()
    await loadInstalled()
  }

  async function handleUpdateAll() {
    updating = true
    error = ''
    success = ''
    try {
      const result = await hubUpdate()
      const total = result.updated_skills?.length ?? 0
      success = total > 0 ? $t.extensions.updatedTotal(total) : $t.extensions.everythingUpToDate
      await loadInstalled()
    } catch (e) {
      error = e instanceof Error ? e.message : $t.extensions.updateFailed
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

{#snippet renderQuality(entry: HubRegistryEntry)}
  {#if entry.quality}
    <div class="quality-row">
      <span class={qualityScoreClass(entry)}>{qualityScoreLabel(entry)}</span>
      {#if qualitySignals(entry).length}
        <div class="quality-signals">
          {#each qualitySignals(entry) as signal}
            <span class="quality-signal" title={signal.title || `${signal.label}: ${signal.value}`}>
              <strong>{signal.label}</strong>
              <span>{signal.value}</span>
            </span>
          {/each}
        </div>
      {/if}
    </div>
  {/if}
{/snippet}

<div class="ext-page">
  <div class="page-header">
    <div>
      <h2>{$t.extensions.title}</h2>
      <p class="page-subtitle">{$t.extensions.subtitle}</p>
    </div>
    <div class="page-actions">
      <div class="view-toggle">
        <button class="toggle-btn" class:active={tab === 'installed'} onclick={() => switchTab('installed')}>{$t.extensions.tabInstalled}</button>
        <button class="toggle-btn" class:active={tab === 'hub'} onclick={() => switchTab('hub')}>{$t.extensions.tabHub}</button>
      </div>
    </div>
  </div>

  {#if error}
    <div class="message message-error">{error}</div>
  {/if}
  {#if success}
    <div class="message message-success">{success}</div>
  {/if}
  {#if skillSandboxReport}
    <div class="sandbox-report" class:failed={!skillSandboxReport.passed}>
      <div class="sandbox-report-header">
        <strong>{sandboxTitle(skillSandboxReport)}</strong>
        <span class={skillSandboxReport.passed ? 'badge badge-success' : 'badge badge-error'}>
          {skillSandboxReport.passed ? $t.extensions.sandboxPassed : $t.extensions.sandboxFailed}
        </span>
        <span class="sandbox-summary">{sandboxSummary(skillSandboxReport)}</span>
      </div>
      <div class="sandbox-checks">
        {#each skillSandboxReport.checks as check}
          <div class="sandbox-check" class:failed={check.status === 'failed'}>
            <span class={check.status === 'passed' ? 'badge badge-success' : 'badge badge-error'}>{check.status}</span>
            <div class="sandbox-check-body">
              <div class="sandbox-check-title">
                <strong>{check.name}</strong>
                {#if check.duration_ms !== undefined}<span>{check.duration_ms}ms</span>{/if}
              </div>
              {#if check.command}<code>{check.command}</code>{/if}
              {#if check.error}<span class="sandbox-error">{check.error}</span>{/if}
              {#if check.output}<pre>{check.output}</pre>{/if}
            </div>
          </div>
        {/each}
      </div>
    </div>
  {/if}

  {#if tab === 'installed'}
    <!-- Installed Extensions -->
    <div class="ext-toolbar">
      <button class="btn btn-primary btn-sm" onclick={() => { skillCreatorOpen = true }}>
        {$t.extensions.createSkill}
      </button>
      <button class="btn btn-ghost btn-sm" onclick={() => { mcpCreatorOpen = true }}>
        {$t.extensions.createMCP}
      </button>
      <button class="btn btn-ghost btn-sm" disabled={diagnosing} onclick={handleRunDiagnostics}>
        {diagnosing ? $t.extensions.diagnosing : $t.extensions.diagnose}
      </button>
      <button class="btn btn-ghost btn-sm" disabled={reloading} onclick={handleReload}>
        {reloading ? $t.extensions.reloading : $t.extensions.reload}
      </button>
      <button class="btn btn-ghost btn-sm" disabled={updating || updateCount === 0} onclick={handleUpdateAll}>
        {updating ? $t.extensions.updating : $t.extensions.updateAll}
      </button>
      {#if updateCount > 0}
        <span class="badge badge-warning">{$t.extensions.updatesAvailable(updateCount)}</span>
      {/if}
    </div>

    {#if loading}
      <div class="ext-loading">{$t.extensions.loadingExtensions}</div>
    {:else}
      <!-- Skills -->
      <section class="card ext-section">
        <div class="card-header">
          <div class="section-heading">
            <span class="card-title">{$t.extensions.skillsTitle}</span>
            <span class="section-definition">{$t.extensions.skillsDefinition}</span>
          </div>
          <span class="badge badge-default">{skills.length}</span>
        </div>
        {#if skills.length === 0}
          <div class="empty-state"><p>{$t.extensions.noSkills}</p></div>
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
                      {#if s.user_invocable}<span class="badge badge-accent" title="User can invoke this skill from chat">{skillSlashLabel(s)}</span>{/if}
                      {#if healthItem('skill', s.name)}
                        <span class={healthBadgeClass(healthItem('skill', s.name))} title={healthItem('skill', s.name)?.summary || ''}>
                          {healthLabel(healthItem('skill', s.name)?.status || 'unknown')}
                        </span>
                      {/if}
                    </div>
                  </div>
                  <div class="ext-item-actions">
                    {#if healthItem('skill', s.name)?.repairable}
                      <button class="btn btn-warning btn-sm" disabled={repairingItem === 'skill:' + s.name} onclick={() => handleRepair('skill', s.name)}>
                        {repairingItem === 'skill:' + s.name ? $t.extensions.repairing : $t.extensions.repair}
                      </button>
                    {/if}
                    <button class="toggle-switch" class:on={!isDisabledExt('skill', s.name)} disabled={togglingItem === 'skill:' + s.name} onclick={() => handleToggle('skill', s.name)}>{isDisabledExt('skill', s.name) ? $t.extensions.off : $t.extensions.on}</button>
                    {#if hasUpdate('skill', s.name)}
                      <button class="btn btn-warning btn-sm" disabled={busyItem === 'skill:' + s.name} onclick={() => handleInstall('skill', s.name)} title={$t.extensions.updateTooltip(registryVersion('skill', s.name))}>
                        {busyItem === 'skill:' + s.name ? $t.extensions.updateBusy : $t.extensions.update}
                      </button>
                    {/if}
                    {#if isHubInstalled('skill', s.name)}
                      <button class="btn btn-danger btn-sm" disabled={busyItem === 'skill:' + s.name} onclick={() => handleUninstall('skill', s.name)}>{busyItem === 'skill:' + s.name ? $t.extensions.updateBusy : $t.extensions.uninstall}</button>
                    {/if}
                  </div>
                </div>
                {#if healthDetailVisible(healthItem('skill', s.name))}
                  <div class="health-detail">
                    {#each healthItem('skill', s.name)?.checks ?? [] as check}
                      <div class="health-check" class:failed={check.status === 'fail'} class:warn={check.status === 'warn' || check.status === 'unknown'}>
                        <span class={check.status === 'pass' ? 'badge badge-success' : check.status === 'fail' ? 'badge badge-error' : 'badge badge-warning'}>{healthLabel(check.status)}</span>
                        <div class="health-check-body">
                          <strong>{check.name}</strong>
                          {#if check.message}<span>{check.message}</span>{/if}
                          {#if check.detail}<code>{check.detail}</code>{/if}
                        </div>
                      </div>
                    {/each}
                  </div>
                {/if}
                {#if isDetailOpen('skill', s.name, 'installed')}
                  <div class="ext-detail">
                    {#if detailLoading}<div class="ext-detail-loading">{$t.extensions.detailLoading}</div>
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

      <!-- MCP Servers -->
      <section class="card ext-section">
        <div class="card-header">
          <div class="section-heading">
            <span class="card-title">{$t.extensions.mcpTitle}</span>
            <span class="section-definition">{$t.extensions.mcpDefinition}</span>
          </div>
          <span class="badge badge-default">{mcpServers.length}</span>
        </div>
        {#if mcpServers.length === 0}
          <div class="empty-state"><p>{$t.extensions.noMCP}</p></div>
        {:else}
          <div class="ext-list">
            {#each mcpServers as m}
              <div class="ext-item-wrapper">
                <div class="ext-item">
                  <div class="ext-item-info">
                    <strong>{m.name}</strong>
                    <div class="ext-meta">
                      {#if m.transport}<span class="badge badge-default">{m.transport}</span>{/if}
                      {#if m.source}<span class="ext-meta-tag">{m.source}</span>{/if}
                      {#if m.connected !== undefined}
                        <span class={m.connected ? 'badge badge-success' : 'badge badge-error'}>{m.connected ? $t.extensions.connected : $t.extensions.disconnected}</span>
                      {/if}
                      {#if m.tool_count}<span class="ext-meta-tag">{$t.extensions.toolsCount(m.tool_count)}</span>{/if}
                      {#if healthItem('mcp', m.name)}
                        <span class={healthBadgeClass(healthItem('mcp', m.name))} title={healthItem('mcp', m.name)?.summary || ''}>
                          {healthLabel(healthItem('mcp', m.name)?.status || 'unknown')}
                        </span>
                      {/if}
                      {#if m.error}<span class="ext-error">{m.error}</span>{/if}
                    </div>
                  </div>
                  <div class="ext-item-actions">
                    {#if healthItem('mcp', m.name)?.repairable}
                      <button class="btn btn-warning btn-sm" disabled={repairingItem === 'mcp:' + m.name} onclick={() => handleRepair('mcp', m.name)}>
                        {repairingItem === 'mcp:' + m.name ? $t.extensions.repairing : $t.extensions.repair}
                      </button>
                    {/if}
                    <button
                      class="toggle-switch"
                      class:on={!isDisabledExt('mcp', m.name)}
                      disabled={togglingItem === 'mcp:' + m.name}
                      title={isDisabledExt('mcp', m.name) ? $t.extensions.enable : $t.extensions.disable}
                      onclick={() => handleToggle('mcp', m.name)}
                    >{isDisabledExt('mcp', m.name) ? $t.extensions.off : $t.extensions.on}</button>
                    {#if hasUpdate('mcp', m.name)}
                      <button class="btn btn-warning btn-sm" disabled={busyItem === 'mcp:' + m.name} onclick={() => handleInstall('mcp', m.name)} title={$t.extensions.updateTooltip(registryVersion('mcp', m.name))}>
                        {busyItem === 'mcp:' + m.name ? $t.extensions.updateBusy : $t.extensions.update}
                      </button>
                    {/if}
                    {#if isHubInstalled('mcp', m.name)}
                      <button class="btn btn-danger btn-sm" disabled={busyItem === 'mcp:' + m.name} onclick={() => handleUninstall('mcp', m.name)}>
                        {busyItem === 'mcp:' + m.name ? $t.extensions.updateBusy : $t.extensions.uninstall}
                      </button>
                    {/if}
                  </div>
                </div>
                {#if healthDetailVisible(healthItem('mcp', m.name))}
                  <div class="health-detail">
                    {#each healthItem('mcp', m.name)?.checks ?? [] as check}
                      <div class="health-check" class:failed={check.status === 'fail'} class:warn={check.status === 'warn' || check.status === 'unknown'}>
                        <span class={check.status === 'pass' ? 'badge badge-success' : check.status === 'fail' ? 'badge badge-error' : 'badge badge-warning'}>{healthLabel(check.status)}</span>
                        <div class="health-check-body">
                          <strong>{check.name}</strong>
                          {#if check.message}<span>{check.message}</span>{/if}
                          {#if check.detail}<code>{check.detail}</code>{/if}
                        </div>
                      </div>
                    {/each}
                  </div>
                {/if}
              </div>
            {/each}
          </div>
        {/if}
      </section>
    {/if}

  {:else}
    <!-- Hub (Registry Browser) -->
    {#if hubLoading}
      <div class="ext-loading">{$t.extensions.fetchingRegistry}</div>
    {:else if !registry}
      <div class="ext-loading">{$t.extensions.loading}</div>
    {:else}
      <!-- Hub Skills -->
      <section class="card ext-section">
        <div class="card-header">
          <div class="section-heading">
            <span class="card-title">{$t.extensions.skillsTitle}</span>
            <span class="section-definition">{$t.extensions.skillsDefinition}</span>
          </div>
          <span class="badge badge-default">{$t.extensions.available(registry.skills.length)}</span>
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
                    <span class="ext-version">{$t.extensions.versionPrefix(entry.version)}</span>
                    {#if entry.author}<span class="ext-meta-tag">{$t.extensions.byAuthor(entry.author)}</span>{/if}
                  </div>
                  <span class="ext-desc">{entry.description}</span>
                  {#if entry.tags?.length}
                    <div class="ext-tags">{#each entry.tags as tag}<span class="ext-tag">{tag}</span>{/each}</div>
                  {/if}
                  {@render renderQuality(entry)}
                </div>
                {#if hasUpdate('skill', entry.name)}
                  <button class="btn btn-warning btn-sm" disabled={busyItem === 'skill:' + entry.name} onclick={() => handleInstall('skill', entry.name)}>
                    {busyItem === 'skill:' + entry.name ? $t.extensions.updating : $t.extensions.update}
                  </button>
                {:else if isInstalled('skill', entry.name)}
                  <span class="badge badge-success">{$t.extensions.installed}</span>
                {:else}
                  <button class="btn btn-primary btn-sm" disabled={busyItem === 'skill:' + entry.name} onclick={() => handleInstall('skill', entry.name)}>{busyItem === 'skill:' + entry.name ? $t.extensions.installing : $t.extensions.install}</button>
                {/if}
              </div>
              {#if isDetailOpen('skill', entry.name, 'hub')}
                <div class="ext-detail">
                  {#if detailLoading}<div class="ext-detail-loading">{$t.extensions.detailLoading}</div>
                  {:else}<div class="ext-detail-content ext-md">{@html renderMarkdown(detailContent)}</div>{/if}
                </div>
              {/if}
            </div>
          {/each}
        </div>
      </section>

      <!-- Hub MCP Servers -->
      <section class="card ext-section">
        <div class="card-header">
          <div class="section-heading">
            <span class="card-title">{$t.extensions.mcpTitle}</span>
            <span class="section-definition">{$t.extensions.mcpDefinition}</span>
          </div>
          <span class="badge badge-default">{$t.extensions.available(registry.mcp_servers.length)}</span>
        </div>
        <div class="ext-list">
          {#each registry.mcp_servers as entry}
            <div class="ext-item">
              <div class="ext-item-info">
                <div class="ext-item-top">
                  <strong>{entry.name}</strong>
                  <span class="ext-version">{$t.extensions.versionPrefix(entry.version)}</span>
                </div>
                <span class="ext-desc">{entry.description}</span>
                {#if entry.tags?.length}
                  <div class="ext-tags">
                    {#each entry.tags as tag}<span class="ext-tag">{tag}</span>{/each}
                  </div>
                {/if}
                {@render renderQuality(entry)}
              </div>
              {#if hasUpdate('mcp', entry.name)}
                <button class="btn btn-warning btn-sm" disabled={busyItem === 'mcp:' + entry.name} onclick={() => handleInstall('mcp', entry.name)}>
                  {busyItem === 'mcp:' + entry.name ? $t.extensions.updating : $t.extensions.update}
                </button>
              {:else if isInstalled('mcp', entry.name)}
                <span class="badge badge-success">{$t.extensions.installed}</span>
              {:else}
                <button class="btn btn-primary btn-sm" disabled={busyItem === 'mcp:' + entry.name} onclick={() => handleInstall('mcp', entry.name)}>
                  {busyItem === 'mcp:' + entry.name ? $t.extensions.installing : $t.extensions.install}
                </button>
              {/if}
            </div>
          {/each}
        </div>
      </section>
    {/if}
  {/if}

  {#if skillCreatorOpen}
    <SkillCreator onclose={() => { skillCreatorOpen = false }} onsaved={handleSkillCreated} />
  {/if}
  {#if mcpCreatorOpen}
    <MCPServerCreator onclose={() => { mcpCreatorOpen = false }} onsaved={handleMCPServerCreated} />
  {/if}
</div>

<style>
  .ext-page {
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    overflow-y: auto;
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

  .view-toggle { display: flex; background: var(--surface-elevated); border-radius: var(--radius-md); padding: 2px; gap: 2px; }
  .toggle-btn {
    padding: var(--space-1) var(--space-3);
    border: none; border-radius: var(--radius-sm);
    background: transparent; color: var(--text-secondary);
    font-family: var(--font-display); font-size: var(--text-sm); font-weight: 500;
    cursor: pointer; transition: all var(--duration-fast) var(--ease-out);
  }
  .toggle-btn:hover { color: var(--text-primary); }
  .toggle-btn.active { background: var(--primary); color: #fff; }

  .ext-toolbar { display: flex; gap: var(--space-2); }
  .ext-loading { padding: var(--space-10); text-align: center; color: var(--text-tertiary); }

  .message { font-size: var(--text-sm); padding: var(--space-2) var(--space-3); border-radius: var(--radius-md); }
  .message-error { background: rgba(220, 60, 60, 0.15); color: var(--red); border: 1px solid rgba(220, 60, 60, 0.3); }
  .message-success { background: rgba(60, 180, 100, 0.15); color: var(--green); border: 1px solid rgba(60, 180, 100, 0.3); }

  .sandbox-report {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    padding: var(--space-3);
    border: 1px solid rgba(60, 180, 100, 0.3);
    border-radius: var(--radius-md);
    background: rgba(60, 180, 100, 0.08);
  }
  .sandbox-report.failed {
    border-color: rgba(220, 60, 60, 0.32);
    background: rgba(220, 60, 60, 0.08);
  }
  .sandbox-report-header {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-width: 0;
    flex-wrap: wrap;
  }
  .sandbox-report-header strong {
    color: var(--text-primary);
    font-family: var(--font-display);
    font-size: var(--text-sm);
  }
  .sandbox-summary { color: var(--text-secondary); font-size: var(--text-xs); }
  .sandbox-checks { display: flex; flex-direction: column; gap: var(--space-2); }
  .sandbox-check {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr);
    gap: var(--space-2);
    padding: var(--space-2);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: rgba(255, 255, 255, 0.025);
  }
  .sandbox-check.failed { border-color: rgba(220, 60, 60, 0.28); }
  .sandbox-check-body { display: flex; flex-direction: column; gap: 4px; min-width: 0; }
  .sandbox-check-title { display: flex; align-items: center; gap: var(--space-2); flex-wrap: wrap; }
  .sandbox-check-title strong { color: var(--text-primary); font-size: var(--text-xs); }
  .sandbox-check-title span { color: var(--text-tertiary); font-size: 10px; font-family: var(--font-mono); }
  .sandbox-check code {
    width: fit-content;
    max-width: 100%;
    overflow-wrap: anywhere;
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-secondary);
    background: rgba(255, 255, 255, 0.06);
    padding: 2px 5px;
    border-radius: var(--radius-sm);
  }
  .sandbox-error { color: var(--red); font-size: var(--text-xs); }
  .sandbox-check pre {
    max-height: 120px;
    margin: 0;
    overflow: auto;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-secondary);
  }

  .section-heading { display: flex; flex-direction: column; align-items: flex-start; gap: 2px; min-width: 0; }
  .section-definition { color: var(--text-secondary); font-size: var(--text-xs); font-weight: 400; line-height: 1.4; }
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
  .ext-error {
    max-width: 520px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--red);
    font-family: var(--font-mono);
    font-size: 10px;
  }

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
  .ext-name-btn:hover strong { color: var(--primary); }
  .detail-chevron { font-size: 10px; color: var(--text-ghost); transition: transform var(--duration-fast) var(--ease-out); display: inline-block; }
  .detail-chevron.open { transform: rotate(90deg); }
  .ext-detail { padding: var(--space-3) var(--space-4); background: var(--surface-base); border-top: 1px solid var(--border-subtle); }
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
  .ext-md :global(pre) { background: var(--surface); border: 1px solid var(--border-subtle); border-radius: var(--radius-sm); padding: var(--space-2); overflow-x: auto; margin: var(--space-2) 0; font-family: var(--font-mono); font-size: var(--text-xs); }
  .ext-md :global(pre code) { background: none; padding: 0; }
  .ext-md :global(strong) { font-weight: 600; color: var(--text-primary); }

  .health-detail {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-4) var(--space-3);
    background: var(--surface-base);
    border-top: 1px solid var(--border-subtle);
  }
  .health-check {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr);
    gap: var(--space-2);
    align-items: flex-start;
    padding: var(--space-2);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: rgba(255, 255, 255, 0.025);
  }
  .health-check.failed { border-color: rgba(248, 113, 113, 0.28); }
  .health-check.warn { border-color: rgba(251, 191, 36, 0.24); }
  .health-check-body {
    display: flex;
    flex-direction: column;
    gap: 4px;
    min-width: 0;
    color: var(--text-secondary);
    font-size: var(--text-xs);
  }
  .health-check-body strong {
    color: var(--text-primary);
    font-family: var(--font-display);
    font-size: var(--text-xs);
  }
  .health-check-body code {
    width: fit-content;
    max-width: 100%;
    overflow-wrap: anywhere;
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-secondary);
    background: rgba(255, 255, 255, 0.06);
    padding: 2px 5px;
    border-radius: var(--radius-sm);
  }

  .ext-tags { display: flex; gap: var(--space-1); flex-wrap: wrap; margin-top: 2px; }
  .ext-tag {
    padding: 1px var(--space-1); border-radius: var(--radius-sm);
    background: var(--surface-elevated); font-size: 10px; color: var(--text-tertiary);
  }

  .quality-row {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex-wrap: wrap;
    margin-top: var(--space-1);
  }
  .quality-score {
    display: inline-flex;
    align-items: center;
    min-height: 20px;
    padding: 2px var(--space-2);
    border-radius: var(--radius-sm);
    border: 1px solid var(--border-subtle);
    font-family: var(--font-mono);
    font-size: 10px;
    font-weight: 700;
    color: var(--text-secondary);
    background: rgba(255, 255, 255, 0.04);
  }
  .quality-score.strong {
    color: var(--green);
    border-color: rgba(60, 180, 100, 0.28);
    background: rgba(60, 180, 100, 0.1);
  }
  .quality-score.steady {
    color: var(--primary);
    border-color: rgba(224, 145, 69, 0.28);
    background: rgba(224, 145, 69, 0.1);
  }
  .quality-score.watch {
    color: var(--text-tertiary);
    border-color: var(--border-subtle);
  }
  .quality-signals {
    display: flex;
    align-items: center;
    gap: var(--space-1);
    flex-wrap: wrap;
    min-width: 0;
  }
  .quality-signal {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    max-width: 260px;
    min-height: 20px;
    padding: 2px var(--space-2);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    color: var(--text-secondary);
    background: rgba(255, 255, 255, 0.025);
    font-size: 10px;
  }
  .quality-signal strong {
    flex-shrink: 0;
    color: var(--text-tertiary);
    font-family: var(--font-display);
    font-size: 10px;
  }
  .quality-signal span {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-family: var(--font-mono);
  }
</style>
