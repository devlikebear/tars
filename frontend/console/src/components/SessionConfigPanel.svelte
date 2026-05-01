<script lang="ts">
  import {
    getSessionAutomationConsent,
    listChatTools,
    getSessionConfig,
    updateSessionAutomationConsent,
    updateSessionConfig,
    type ChatToolInfo,
    type SessionToolConfig,
  } from '../lib/api'
  import { buildSessionPermissionPreview, type SessionPermissionPreview } from '../lib/sessionPermissionPreview'
  import type { SessionAutomationConsent } from '../lib/types'

  interface Props {
    sessionId: string
    onClose?: () => void
    onChange?: () => void
  }

  let { sessionId, onClose, onChange }: Props = $props()

  let tools: ChatToolInfo[] = $state([])
  let skills: string[] = $state([])
  let mcpServers: string[] = $state([])
  let config: SessionToolConfig = $state({})
  let pendingConfig: SessionToolConfig | null = $state(null)
  let pendingPreview: SessionPermissionPreview | null = $state(null)
  let automationConsent: SessionAutomationConsent = $state({})
  let loading = $state(true)
  let filterText = $state('')
  let activeTab: 'tools' | 'skills' | 'automation' = $state('tools')
  let automationSaving = $state(false)

  let enabledSet: Set<string> = $state(new Set())
  let disabledSet: Set<string> = $state(new Set())
  let allowGroupsSet: Set<string> = $state(new Set())
  let denyGroupsSet: Set<string> = $state(new Set())
  let skillsEnabledSet: Set<string> = $state(new Set())
  let useCustomConfig = $state(false)
  let useCustomSkills = $state(false)

  type AutomationToggleKey = 'auto_resume' | 'git_mutations' | 'autonomous_mutations'
  const defaultAutoResumeMinutes = 30
  const autoResumeModes = [
    { id: 'record_assumption_and_proceed', label: 'Assume + proceed' },
    { id: 'proceed_with_assumption', label: 'Proceed' },
    { id: 'move_to_next_task', label: 'Next task' },
  ]
  const previewRiskLabel: Record<SessionPermissionPreview['risk'], string> = {
    low: 'Low risk',
    medium: 'Medium risk',
    high: 'High risk',
  }

  async function load() {
    loading = true
    try {
      const [toolsResp, configResp, automationResp] = await Promise.all([
        listChatTools(),
        sessionId ? getSessionConfig(sessionId) : Promise.resolve({} as SessionToolConfig),
        sessionId ? getSessionAutomationConsent(sessionId) : Promise.resolve({} as SessionAutomationConsent),
      ])
      tools = toolsResp.tools
      skills = toolsResp.skills ?? []
      mcpServers = toolsResp.mcp_servers ?? []
      config = configResp
      automationConsent = automationResp

      applyConfigState(config, tools, skills)
      pendingConfig = null
      pendingPreview = null
    } catch {
      // ignore
    }
    loading = false
  }

  function applyConfigState(nextConfig: SessionToolConfig, availableTools = tools, availableSkills = skills) {
    if (nextConfig.tools_custom || Array.isArray(nextConfig.tools_enabled)) {
      useCustomConfig = true
      enabledSet = new Set(nextConfig.tools_enabled ?? [])
    } else {
      useCustomConfig = false
      enabledSet = new Set(availableTools.map((t) => t.name))
    }
    disabledSet = new Set(nextConfig.tools_disabled ?? [])
    allowGroupsSet = new Set(nextConfig.tools_allow_groups ?? [])
    denyGroupsSet = new Set(nextConfig.tools_deny_groups ?? [])

    if (nextConfig.skills_custom || Array.isArray(nextConfig.skills_enabled)) {
      useCustomSkills = true
      skillsEnabledSet = new Set(nextConfig.skills_enabled ?? [])
    } else {
      useCustomSkills = false
      skillsEnabledSet = new Set(availableSkills)
    }
  }

  function isToolEnabled(name: string): boolean {
    if (!useCustomConfig) return !disabledSet.has(name)
    return enabledSet.has(name) && !disabledSet.has(name)
  }

  function toggleTool(name: string) {
    if (isToolEnabled(name)) {
      if (useCustomConfig) {
        enabledSet.delete(name)
        enabledSet = new Set(enabledSet)
      } else {
        disabledSet.add(name)
        disabledSet = new Set(disabledSet)
      }
    } else {
      if (useCustomConfig) {
        enabledSet.add(name)
        enabledSet = new Set(enabledSet)
      }
      disabledSet.delete(name)
      disabledSet = new Set(disabledSet)
    }
    queueConfigPreview()
  }

  function isSkillEnabled(name: string): boolean {
    if (!useCustomSkills) return true
    return skillsEnabledSet.has(name)
  }

  function toggleAllowGroup(name: string) {
    if (allowGroupsSet.has(name)) {
      allowGroupsSet.delete(name)
    } else {
      allowGroupsSet.add(name)
      denyGroupsSet.delete(name)
    }
    allowGroupsSet = new Set(allowGroupsSet)
    denyGroupsSet = new Set(denyGroupsSet)
    queueConfigPreview()
  }

  function toggleDenyGroup(name: string) {
    if (denyGroupsSet.has(name)) {
      denyGroupsSet.delete(name)
    } else {
      denyGroupsSet.add(name)
      allowGroupsSet.delete(name)
    }
    allowGroupsSet = new Set(allowGroupsSet)
    denyGroupsSet = new Set(denyGroupsSet)
    queueConfigPreview()
  }

  function toggleSkill(name: string) {
    if (isSkillEnabled(name)) {
      if (!useCustomSkills) {
        useCustomSkills = true
        skillsEnabledSet = new Set(skills.filter((s) => s !== name))
      } else {
        skillsEnabledSet.delete(name)
        skillsEnabledSet = new Set(skillsEnabledSet)
      }
    } else {
      skillsEnabledSet.add(name)
      skillsEnabledSet = new Set(skillsEnabledSet)
    }
    queueConfigPreview()
  }

  function toggleAllTools() {
    if (useCustomConfig) {
      useCustomConfig = false
      enabledSet = new Set(tools.map((t) => t.name))
      disabledSet = new Set()
    } else {
      useCustomConfig = true
      enabledSet = new Set()
      disabledSet = new Set()
    }
    queueConfigPreview()
  }

  function toggleAllSkills() {
    if (useCustomSkills) {
      useCustomSkills = false
      skillsEnabledSet = new Set(skills)
    } else {
      useCustomSkills = true
      skillsEnabledSet = new Set()
    }
    queueConfigPreview()
  }

  function draftConfigFromState(): SessionToolConfig {
    const newConfig: SessionToolConfig = {}
    if (useCustomConfig) {
      newConfig.tools_custom = true
      newConfig.tools_enabled = [...enabledSet]
    }
    if (disabledSet.size > 0) {
      newConfig.tools_disabled = [...disabledSet]
    }
    if (allowGroupsSet.size > 0) {
      newConfig.tools_allow_groups = [...allowGroupsSet]
    }
    if (denyGroupsSet.size > 0) {
      newConfig.tools_deny_groups = [...denyGroupsSet]
    }
    if (useCustomSkills) {
      newConfig.skills_custom = true
      newConfig.skills_enabled = [...skillsEnabledSet]
    }
    if (Array.isArray(config.mcp_enabled)) {
      newConfig.mcp_enabled = [...config.mcp_enabled]
    }
    return newConfig
  }

  function queueConfigPreview() {
    if (!sessionId) return
    const nextConfig = draftConfigFromState()
    const preview = buildSessionPermissionPreview(config, nextConfig, {
      tools,
      skills,
      mcpServers,
    })
    if (!hasPermissionPreviewChanges(preview)) {
      pendingConfig = null
      pendingPreview = null
      return
    }
    pendingConfig = nextConfig
    pendingPreview = preview
  }

  async function applyPendingConfig() {
    if (!sessionId || !pendingConfig) return
    const nextConfig = pendingConfig
    try {
      await updateSessionConfig(sessionId, nextConfig)
      config = nextConfig
      pendingConfig = null
      pendingPreview = null
      onChange?.()
    } catch {
      void load()
    }
  }

  function cancelPendingConfig() {
    pendingConfig = null
    pendingPreview = null
    applyConfigState(config)
  }

  function hasPermissionPreviewChanges(preview: SessionPermissionPreview): boolean {
    return [
      preview.gainedTools,
      preview.lostTools,
      preview.gainedGroups,
      preview.lostGroups,
      preview.gainedSkills,
      preview.lostSkills,
      preview.gainedMCPServers,
      preview.lostMCPServers,
    ].some((items) => items.length > 0)
  }

  function previewRows(preview: SessionPermissionPreview) {
    return [
      { label: 'Tools enabled', items: preview.gainedTools },
      { label: 'Tools disabled', items: preview.lostTools },
      { label: 'Groups enabled', items: preview.gainedGroups },
      { label: 'Groups disabled', items: preview.lostGroups },
      { label: 'Skills enabled', items: preview.gainedSkills },
      { label: 'Skills disabled', items: preview.lostSkills },
      { label: 'MCP enabled', items: preview.gainedMCPServers },
      { label: 'MCP disabled', items: preview.lostMCPServers },
    ].filter((row) => row.items.length > 0)
  }

  async function saveAutomationConsent(next: SessionAutomationConsent) {
    if (!sessionId || automationSaving) return
    automationConsent = next
    automationSaving = true
    try {
      automationConsent = await updateSessionAutomationConsent(sessionId, next)
      onChange?.()
    } catch {
      void load()
    } finally {
      automationSaving = false
    }
  }

  async function toggleAutomationConsent(key: AutomationToggleKey) {
    if (!sessionId || automationSaving) return
    const next: SessionAutomationConsent = {
      ...automationConsent,
      [key]: !automationConsent[key],
    }
    if (key === 'auto_resume') {
      next.auto_resume_enabled = !!next.auto_resume
      if (next.auto_resume && !next.auto_resume_after_minutes) {
        next.auto_resume_after_minutes = defaultAutoResumeMinutes
      }
      if (next.auto_resume && (!next.allowed_resume_modes || next.allowed_resume_modes.length === 0)) {
        next.allowed_resume_modes = ['record_assumption_and_proceed']
      }
    }
    await saveAutomationConsent(next)
  }

  async function setAutoResumeAfterMinutes(value: string) {
    const parsed = Number.parseInt(value, 10)
    const minutes = Number.isFinite(parsed) && parsed > 0 ? Math.min(parsed, 1440) : defaultAutoResumeMinutes
    await saveAutomationConsent({
      ...automationConsent,
      auto_resume: true,
      auto_resume_enabled: true,
      auto_resume_after_minutes: minutes,
    })
  }

  function effectiveAutoResumeModes(): string[] {
    return automationConsent.allowed_resume_modes?.length
      ? automationConsent.allowed_resume_modes
      : ['record_assumption_and_proceed']
  }

  function isAutoResumeModeEnabled(mode: string): boolean {
    return effectiveAutoResumeModes().includes(mode)
  }

  async function toggleAutoResumeMode(mode: string) {
    const modes = new Set(effectiveAutoResumeModes())
    if (modes.has(mode)) {
      modes.delete(mode)
    } else {
      modes.add(mode)
    }
    if (modes.size === 0) modes.add('record_assumption_and_proceed')
    await saveAutomationConsent({
      ...automationConsent,
      auto_resume: true,
      auto_resume_enabled: true,
      allowed_resume_modes: [...modes],
    })
  }

  let filteredTools = $derived(
    tools.filter((t) => !filterText || t.name.toLowerCase().includes(filterText.toLowerCase()))
  )
  let toolGroups = $derived(
    [...new Set(tools.map((t) => t.group).filter((group): group is string => Boolean(group)))].sort()
  )
  let filteredSkills = $derived(
    skills.filter((s) => !filterText || s.toLowerCase().includes(filterText.toLowerCase()))
  )

  $effect(() => {
    if (sessionId) void load()
  })
</script>

<div class="config-panel">
  <div class="config-header">
    <span class="config-title">Session Config</span>
    {#if onClose}
      <button class="config-close" onclick={onClose}>&times;</button>
    {/if}
  </div>

  {#if loading}
    <div class="config-loading">Loading...</div>
  {:else}
    <div class="config-tabs">
      <button class="config-tab" class:active={activeTab === 'tools'} onclick={() => activeTab = 'tools'}>
        Tools ({tools.length})
      </button>
      <button class="config-tab" class:active={activeTab === 'skills'} onclick={() => activeTab = 'skills'}>
        Skills ({skills.length})
      </button>
      <button class="config-tab" class:active={activeTab === 'automation'} onclick={() => activeTab = 'automation'}>
        Automation
      </button>
    </div>

    {#if activeTab !== 'automation'}
      <div class="config-filter">
        <input type="text" bind:value={filterText} placeholder="Filter..." class="config-filter-input" />
      </div>
    {/if}

    {#if pendingPreview}
      <div
        class="permission-preview"
        class:risk-medium={pendingPreview.risk === 'medium'}
        class:risk-high={pendingPreview.risk === 'high'}
      >
        <div class="permission-preview-head">
          <strong>Permission change preview</strong>
          <span>{previewRiskLabel[pendingPreview.risk]}</span>
        </div>
        <p>{pendingPreview.summary}</p>
        {#if pendingPreview.capabilities.length > 0}
          <div class="permission-preview-chips" aria-label="Affected capabilities">
            {#each pendingPreview.capabilities as capability}
              <span class="permission-preview-chip">{capability}</span>
            {/each}
          </div>
        {/if}
        <div class="permission-preview-list">
          {#each previewRows(pendingPreview) as row}
            <div class="permission-preview-row">
              <span>{row.label}</span>
              <em>{row.items.slice(0, 4).join(', ')}{row.items.length > 4 ? ` +${row.items.length - 4}` : ''}</em>
            </div>
          {/each}
        </div>
        <div class="permission-preview-actions">
          <button type="button" class="preview-apply" onclick={() => { void applyPendingConfig() }}>
            Apply
          </button>
          <button type="button" class="preview-cancel" onclick={cancelPendingConfig}>
            Cancel
          </button>
        </div>
      </div>
    {/if}

    {#if activeTab === 'tools'}
      {#if toolGroups.length > 0}
        <div class="config-groups">
          <div class="group-section">
            <div class="group-heading">Allow groups</div>
            <div class="group-list">
              {#each toolGroups as group}
                <label class="group-chip" class:active={allowGroupsSet.has(group)}>
                  <input type="checkbox" checked={allowGroupsSet.has(group)} onchange={() => toggleAllowGroup(group)} />
                  <span>{group}</span>
                </label>
              {/each}
            </div>
          </div>
          <div class="group-section">
            <div class="group-heading">Deny groups</div>
            <div class="group-list">
              {#each toolGroups as group}
                <label class="group-chip group-chip-warning" class:active={denyGroupsSet.has(group)}>
                  <input type="checkbox" checked={denyGroupsSet.has(group)} onchange={() => toggleDenyGroup(group)} />
                  <span>{group}</span>
                </label>
              {/each}
            </div>
          </div>
        </div>
      {/if}
      <div class="config-actions">
        <label class="config-toggle">
          <input type="checkbox" checked={!useCustomConfig} onchange={toggleAllTools} />
          <span>All tools</span>
        </label>
        <span class="config-count">{useCustomConfig ? enabledSet.size : tools.length - disabledSet.size} active</span>
      </div>
      <div class="config-list">
        {#each filteredTools as t}
          <label class="config-item" class:high-risk={t.high_risk}>
            <input type="checkbox" checked={isToolEnabled(t.name)} onchange={() => toggleTool(t.name)} />
            <span class="item-name">{t.name}</span>
            {#if t.group}
              <span class="badge badge-neutral" style="font-size:9px;padding:0 4px;">{t.group}</span>
            {/if}
            {#if t.high_risk}
              <span class="badge badge-warning" style="font-size:9px;padding:0 4px;">risk</span>
            {/if}
          </label>
        {/each}
      </div>
    {:else if activeTab === 'skills'}
      <div class="config-actions">
        <label class="config-toggle">
          <input type="checkbox" checked={!useCustomSkills} onchange={toggleAllSkills} />
          <span>All skills</span>
        </label>
        <span class="config-count">{useCustomSkills ? skillsEnabledSet.size : skills.length} active</span>
      </div>
      <div class="config-list">
        {#each filteredSkills as s}
          <label class="config-item">
            <input type="checkbox" checked={isSkillEnabled(s)} onchange={() => toggleSkill(s)} />
            <span class="item-name">{s}</span>
          </label>
        {/each}
      </div>
    {:else if activeTab === 'automation'}
      <div class="automation-list">
        <label class="automation-item">
          <input
            type="checkbox"
            checked={!!(automationConsent.auto_resume || automationConsent.auto_resume_enabled)}
            disabled={automationSaving}
            onchange={() => { void toggleAutomationConsent('auto_resume') }}
          />
          <span>
            <strong>Auto-resume stalled chats</strong>
            <small>Pulse</small>
          </span>
        </label>
        {#if automationConsent.auto_resume || automationConsent.auto_resume_enabled}
          <div class="automation-subgrid">
            <label class="automation-field">
              <span>After</span>
              <input
                type="number"
                min="1"
                max="1440"
                value={automationConsent.auto_resume_after_minutes ?? defaultAutoResumeMinutes}
                disabled={automationSaving}
                onchange={(event) => { void setAutoResumeAfterMinutes((event.currentTarget as HTMLInputElement).value) }}
              />
              <small>minutes</small>
            </label>
            <div class="automation-modes">
              {#each autoResumeModes as mode}
                <label class="automation-mode">
                  <input
                    type="checkbox"
                    checked={isAutoResumeModeEnabled(mode.id)}
                    disabled={automationSaving}
                    onchange={() => { void toggleAutoResumeMode(mode.id) }}
                  />
                  <span>{mode.label}</span>
                </label>
              {/each}
            </div>
          </div>
        {/if}
        <label class="automation-item">
          <input
            type="checkbox"
            checked={!!automationConsent.git_mutations}
            disabled={automationSaving}
            onchange={() => { void toggleAutomationConsent('git_mutations') }}
          />
          <span>
            <strong>Approved git mutations</strong>
            <small>Git</small>
          </span>
        </label>
        <label class="automation-item automation-item-danger">
          <input
            type="checkbox"
            checked={!!automationConsent.autonomous_mutations}
            disabled={automationSaving}
            onchange={() => { void toggleAutomationConsent('autonomous_mutations') }}
          />
          <span>
            <strong>Autonomous workspace mutations</strong>
            <small>High autonomy</small>
          </span>
        </label>
        {#if automationConsent.updated_at}
          <div class="automation-updated">Updated {new Date(automationConsent.updated_at).toLocaleString()}</div>
        {/if}
      </div>
    {/if}
  {/if}
</div>

<style>
  .config-panel {
    display: flex;
    flex-direction: column;
    height: 100%;
    overflow: hidden;
    background: var(--surface);
    border-left: 1px solid var(--border-subtle);
  }

  .config-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--border-subtle);
  }

  .config-title {
    font-family: var(--font-display);
    font-weight: 600;
    font-size: var(--text-sm);
    color: var(--text-primary);
  }

  .config-close {
    background: none;
    border: none;
    color: var(--text-ghost);
    cursor: pointer;
    font-size: 18px;
    padding: 0;
    line-height: 1;
  }
  .config-close:hover { color: var(--text-primary); }

  .config-loading {
    padding: var(--space-4);
    text-align: center;
    color: var(--text-ghost);
    font-size: var(--text-sm);
  }

  .config-tabs {
    display: flex;
    border-bottom: 1px solid var(--border-subtle);
  }

  .config-tab {
    flex: 1;
    padding: var(--space-2);
    background: none;
    border: none;
    border-bottom: 2px solid transparent;
    color: var(--text-ghost);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    cursor: pointer;
    transition: all var(--duration-fast);
  }
  .config-tab.active {
    color: var(--primary);
    border-bottom-color: var(--primary);
  }
  .config-tab:hover { color: var(--text-primary); }

  .config-filter {
    padding: var(--space-2) var(--space-3);
  }

  .config-filter-input {
    width: 100%;
    padding: var(--space-1) var(--space-2);
    background: var(--surface-base);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    color: var(--text-primary);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
  }

  .permission-preview {
    display: grid;
    gap: var(--space-2);
    margin: 0 var(--space-3) var(--space-2);
    padding: var(--space-2);
    border: 1px solid rgba(245, 158, 11, 0.35);
    border-left: 2px solid rgba(245, 158, 11, 0.8);
    border-radius: var(--radius-sm);
    background: rgba(245, 158, 11, 0.08);
  }

  .permission-preview.risk-medium {
    border-color: rgba(245, 158, 11, 0.45);
    border-left-color: rgba(245, 158, 11, 0.9);
  }

  .permission-preview.risk-high {
    border-color: rgba(248, 113, 113, 0.45);
    border-left-color: rgba(248, 113, 113, 0.9);
    background: rgba(248, 113, 113, 0.08);
  }

  .permission-preview-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
  }

  .permission-preview-head strong {
    color: var(--text-primary);
    font-size: var(--text-xs);
  }

  .permission-preview-head span {
    color: var(--text-secondary);
    font-family: var(--font-mono);
    font-size: 10px;
    white-space: nowrap;
  }

  .permission-preview p {
    margin: 0;
    color: var(--text-secondary);
    font-family: var(--font-mono);
    font-size: 10px;
    line-height: 1.4;
  }

  .permission-preview-chips {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-1);
  }

  .permission-preview-chip {
    padding: 2px 6px;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    color: var(--text-primary);
    background: var(--surface-base);
    font-family: var(--font-mono);
    font-size: 10px;
  }

  .permission-preview-list {
    display: grid;
    gap: 3px;
  }

  .permission-preview-row {
    display: grid;
    grid-template-columns: minmax(92px, auto) minmax(0, 1fr);
    gap: var(--space-2);
    align-items: baseline;
    min-width: 0;
  }

  .permission-preview-row span {
    color: var(--text-ghost);
    font-family: var(--font-mono);
    font-size: 10px;
  }

  .permission-preview-row em {
    color: var(--text-secondary);
    font-family: var(--font-mono);
    font-size: 10px;
    font-style: normal;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .permission-preview-actions {
    display: flex;
    gap: var(--space-1);
  }

  .permission-preview-actions button {
    padding: 3px 8px;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    font-family: var(--font-mono);
    font-size: 10px;
    cursor: pointer;
  }

  .preview-apply {
    color: var(--surface);
    background: var(--primary);
    border-color: var(--primary);
  }

  .preview-cancel {
    color: var(--text-secondary);
    background: var(--surface-base);
  }

  .config-actions {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: var(--space-1) var(--space-3);
    border-bottom: 1px solid var(--border-subtle);
  }

  .config-groups {
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--border-subtle);
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .group-section {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }

  .group-heading {
    font-family: var(--font-mono);
    font-size: 10px;
    color: var(--text-ghost);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }

  .group-list {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-1);
  }

  .group-chip {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 2px 6px;
    border: 1px solid var(--border-subtle);
    border-radius: 999px;
    font-family: var(--font-mono);
    font-size: 10px;
    color: var(--text-secondary);
    cursor: pointer;
    background: var(--surface-base);
  }

  .group-chip.active {
    border-color: var(--primary);
    color: var(--primary);
  }

  .group-chip-warning.active {
    border-color: rgba(248, 113, 113, 0.7);
    color: rgb(248, 113, 113);
  }

  .group-chip input {
    margin: 0;
  }

  .config-toggle {
    display: flex;
    align-items: center;
    gap: var(--space-1);
    font-size: var(--text-xs);
    color: var(--text-secondary);
    cursor: pointer;
  }

  .config-count {
    font-family: var(--font-mono);
    font-size: 10px;
    color: var(--text-ghost);
  }

  .config-list {
    flex: 1;
    overflow-y: auto;
    padding: var(--space-1) 0;
  }

  .config-item {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: 3px var(--space-3);
    cursor: pointer;
    transition: background var(--duration-fast);
  }
  .config-item:hover { background: rgba(255, 255, 255, 0.03); }

  .config-item.high-risk { border-left: 2px solid rgba(248, 113, 113, 0.3); }

  .item-name {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-secondary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .automation-list {
    display: grid;
    gap: var(--space-2);
    padding: var(--space-3);
  }

  .automation-item {
    display: grid;
    grid-template-columns: 18px minmax(0, 1fr);
    gap: var(--space-2);
    align-items: flex-start;
    padding: var(--space-2);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface-base);
    cursor: pointer;
  }

  .automation-item input {
    margin-top: 3px;
  }

  .automation-item span {
    display: grid;
    gap: 2px;
    min-width: 0;
  }

  .automation-item strong {
    color: var(--text-primary);
    font-size: var(--text-xs);
  }

  .automation-item small {
    color: var(--text-tertiary);
    font-family: var(--font-mono);
    font-size: 10px;
  }

  .automation-item-danger {
    border-left: 2px solid rgba(248, 113, 113, 0.35);
  }

  .automation-subgrid {
    display: grid;
    gap: var(--space-2);
    padding: var(--space-2);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: rgba(255, 255, 255, 0.02);
  }

  .automation-field {
    display: grid;
    grid-template-columns: minmax(0, 1fr) 76px auto;
    align-items: center;
    gap: var(--space-2);
    color: var(--text-secondary);
    font-family: var(--font-mono);
    font-size: 10px;
  }

  .automation-field input {
    min-width: 0;
    padding: 3px 6px;
    background: var(--surface-base);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    color: var(--text-primary);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
  }

  .automation-modes {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-1);
  }

  .automation-mode {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 3px 6px;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    color: var(--text-secondary);
    font-family: var(--font-mono);
    font-size: 10px;
    background: var(--surface-base);
  }

  .automation-mode input {
    margin: 0;
  }

  .automation-updated {
    color: var(--text-tertiary);
    font-family: var(--font-mono);
    font-size: 10px;
  }
</style>
