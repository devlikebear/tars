<script lang="ts">
  import {
    getSessionAutomationConsent,
    getSessionStyle,
    listChatTools,
    listSkills,
    getSessionConfig,
    getSessionEffectiveConfig,
    updateSessionAutomationConsent,
    updateSessionLocalConfig,
    updateSessionStyle,
    type ChatToolInfo,
    type SessionToolConfig,
  } from '../lib/api'
  import { buildSessionPermissionPreview, type SessionPermissionPreview } from '../lib/sessionPermissionPreview'
  import { buildSessionStylePreview, sessionStylePayload } from '../lib/sessionStyle'
  import type { CommandDef, EffectiveConfigSource, SessionAutomationConsent, SessionEffectiveConfig, SessionStyleResponse, SessionStyleValues, SkillDef } from '../lib/types'

  interface Props {
    sessionId: string
    onClose?: () => void
    onChange?: () => void
  }

  let { sessionId, onClose, onChange }: Props = $props()

  let tools: ChatToolInfo[] = $state([])
  let skills: string[] = $state([])
  let skillDefs: SkillDef[] = $state([])
  let commands: string[] = $state([])
  let commandDefs: CommandDef[] = $state([])
  let mcpServers: string[] = $state([])
  let config: SessionToolConfig = $state({})
  let pendingConfig: SessionToolConfig | null = $state(null)
  let pendingPreview: SessionPermissionPreview | null = $state(null)
  let automationConsent: SessionAutomationConsent = $state({})
  let styleResponse: SessionStyleResponse = $state({
    effective: { directness: 70, humor: 20, caution: 60, autonomy: 40 },
    defaults: { directness: 70, humor: 20, caution: 60, autonomy: 40 },
    preview: [],
  })
  let styleDraft: SessionStyleValues = $state({ directness: 70, humor: 20, caution: 60, autonomy: 40 })
  let loading = $state(true)
  let filterText = $state('')
  let activeTab: 'tools' | 'skills' | 'commands' | 'automation' | 'style' = $state('tools')
  let automationSaving = $state(false)
  let styleSaving = $state(false)
  type SkillSourceFilter = 'all' | 'global' | 'session' | 'enabled' | 'disabled'
  let skillSourceFilter: SkillSourceFilter = $state('all')

  let enabledSet: Set<string> = $state(new Set())
  let disabledSet: Set<string> = $state(new Set())
  let allowGroupsSet: Set<string> = $state(new Set())
  let denyGroupsSet: Set<string> = $state(new Set())
  let skillsEnabledSet: Set<string> = $state(new Set())
  let commandsEnabledSet: Set<string> = $state(new Set())
  let useCustomConfig = $state(false)
  let useCustomSkills = $state(false)
  let useCustomCommands = $state(false)

  // Effective-config snapshot (Phase 3 backend) — drives source badges
  // on tools/skills items so the user can see when a value comes from
  // a `.tars/settings*.json` file rather than the session base.
  let effectiveConfig: SessionEffectiveConfig | null = $state(null)
  const sourceBadgeLabel: Record<EffectiveConfigSource, string> = {
    base: 'session',
    shared: 'shared',
    local: 'local',
  }
  const sourceBadgeTitle: Record<EffectiveConfigSource, string> = {
    base: 'sessions.json',
    shared: '.tars/settings.json',
    local: '.tars/settings.local.json',
  }

  function sourceForToolList(name: string): EffectiveConfigSource {
    if (!effectiveConfig) return 'base'
    const tc = effectiveConfig.effective.tool_config
    if (tc.tools_enabled?.includes(name)) {
      return effectiveConfig.sources['tool_config.tools_enabled'] ?? 'base'
    }
    if (tc.tools_disabled?.includes(name)) {
      return effectiveConfig.sources['tool_config.tools_disabled'] ?? 'base'
    }
    return 'base'
  }

  function sourceForSkillList(name: string): EffectiveConfigSource {
    if (!effectiveConfig) return 'base'
    if (effectiveConfig.effective.tool_config.skills_enabled?.includes(name)) {
      return effectiveConfig.sources['tool_config.skills_enabled'] ?? 'base'
    }
    return 'base'
  }

  function sourceForCommandList(name: string): EffectiveConfigSource {
    if (!effectiveConfig) return 'base'
    if (effectiveConfig.effective.tool_config.commands_enabled?.includes(name)) {
      return effectiveConfig.sources['tool_config.commands_enabled'] ?? 'base'
    }
    return 'base'
  }

  type AutomationToggleKey = 'auto_resume' | 'git_mutations' | 'autonomous_mutations'
  const defaultAutoResumeMinutes = 30
  const autoResumeModes = [
    { id: 'record_assumption_and_proceed', label: 'Assume + proceed' },
    { id: 'proceed_with_assumption', label: 'Proceed' },
    { id: 'move_to_next_task', label: 'Next task' },
  ]
  const styleAxes: Array<{ key: keyof SessionStyleValues; label: string }> = [
    { key: 'directness', label: 'Directness' },
    { key: 'humor', label: 'Humor' },
    { key: 'caution', label: 'Caution' },
    { key: 'autonomy', label: 'Autonomy' },
  ]
  const previewRiskLabel: Record<SessionPermissionPreview['risk'], string> = {
    low: 'Low risk',
    medium: 'Medium risk',
    high: 'High risk',
  }
  const skillSourceFilters: Array<{ id: SkillSourceFilter; label: string }> = [
    { id: 'all', label: 'All' },
    { id: 'global', label: 'Global' },
    { id: 'session', label: 'Session only' },
    { id: 'enabled', label: 'Enabled' },
    { id: 'disabled', label: 'Disabled' },
  ]

  function normalizeSkillDefs(defs: SkillDef[], names: string[]): SkillDef[] {
    const out: SkillDef[] = []
    const seen = new Set<string>()
    for (const def of defs) {
      const name = def.name?.trim()
      if (!name) continue
      const key = name.toLowerCase()
      if (seen.has(key)) continue
      seen.add(key)
      out.push({ ...def, name })
    }
    for (const rawName of names) {
      const name = rawName.trim()
      if (!name) continue
      const key = name.toLowerCase()
      if (seen.has(key)) continue
      seen.add(key)
      out.push({ name, description: '', user_invocable: true })
    }
    return out
  }

  function isCommandSkill(skill: SkillDef): boolean {
    const path = skill.file_path ?? ''
    return path.includes('/.tars/commands/') || path.includes('\\.tars\\commands\\')
  }

  function normalizeCommandDefs(defs: CommandDef[]): CommandDef[] {
    const out: CommandDef[] = []
    const seen = new Set<string>()
    for (const def of defs) {
      const name = def.name?.trim()
      if (!name) continue
      const key = name.toLowerCase()
      if (seen.has(key)) continue
      seen.add(key)
      out.push({ ...def, name })
    }
    return out
  }

  function isSessionSkill(skill: SkillDef): boolean {
    return skill.source === 'session_cwd'
  }

  function skillOriginClass(skill: SkillDef): 'global' | 'session' | 'command' {
    if (isCommandSkill(skill)) return 'command'
    if (isSessionSkill(skill)) return 'session'
    return 'global'
  }

  function skillOriginLabel(skill: SkillDef): string {
    switch (skillOriginClass(skill)) {
      case 'command':
        return 'Command'
      case 'session':
        return 'Session'
      default:
        return 'Global'
    }
  }

  function skillOriginTitle(skill: SkillDef): string {
    if (isCommandSkill(skill)) return '.tars/commands'
    if (isSessionSkill(skill)) return '.tars/skills'
    return skill.source || 'global skill source'
  }

  function skillSlashLabel(skill: SkillDef): string {
    const slash = (skill.slash || skill.name || '').replace(/^\/+/, '').trim()
    return slash ? `/${slash}` : ''
  }

  function matchesSkillSourceFilter(skill: SkillDef): boolean {
    switch (skillSourceFilter) {
      case 'global':
        return !isSessionSkill(skill)
      case 'session':
        return isSessionSkill(skill) && !isCommandSkill(skill)
      case 'enabled':
        return isSkillEnabled(skill.name)
      case 'disabled':
        return !isSkillEnabled(skill.name)
      default:
        return true
    }
  }

  async function load() {
    loading = true
    try {
      const [toolsResp, skillResp, configResp, automationResp, styleResp] = await Promise.all([
        listChatTools(sessionId || undefined),
        listSkills(sessionId || undefined),
        sessionId ? getSessionConfig(sessionId) : Promise.resolve({} as SessionToolConfig),
        sessionId ? getSessionAutomationConsent(sessionId) : Promise.resolve({} as SessionAutomationConsent),
        sessionId
          ? getSessionStyle(sessionId)
          : Promise.resolve({
              effective: { directness: 70, humor: 20, caution: 60, autonomy: 40 },
              defaults: { directness: 70, humor: 20, caution: 60, autonomy: 40 },
              preview: [],
            } as SessionStyleResponse),
      ])
      tools = toolsResp.tools
      skillDefs = normalizeSkillDefs(skillResp, toolsResp.skills ?? [])
      skills = skillDefs.map((skill) => skill.name)
      commandDefs = normalizeCommandDefs(toolsResp.commands ?? [])
      commands = commandDefs.map((command) => command.name)
      mcpServers = toolsResp.mcp_servers ?? []
      config = configResp
      automationConsent = automationResp
      styleResponse = styleResp
      styleDraft = { ...styleResp.effective }

      // Best-effort: failure to load effective config (e.g. service not
      // wired in old tests) just disables the source badges.
      if (sessionId) {
        try {
          effectiveConfig = await getSessionEffectiveConfig(sessionId)
          config = effectiveConfig.effective.tool_config
        } catch {
          effectiveConfig = null
          config = configResp
        }
      } else {
        effectiveConfig = null
        config = configResp
      }

      applyConfigState(config, tools, skills, commands)
      pendingConfig = null
      pendingPreview = null
    } catch {
      // ignore
    }
    loading = false
  }

  function applyConfigState(nextConfig: SessionToolConfig, availableTools = tools, availableSkills = skills, availableCommands = commands) {
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

    if (nextConfig.commands_custom || Array.isArray(nextConfig.commands_enabled)) {
      useCustomCommands = true
      commandsEnabledSet = new Set(nextConfig.commands_enabled ?? [])
    } else {
      useCustomCommands = false
      commandsEnabledSet = new Set(availableCommands)
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

  function isCommandEnabled(name: string): boolean {
    if (!useCustomCommands) return true
    return commandsEnabledSet.has(name)
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

  function toggleCommand(name: string) {
    if (isCommandEnabled(name)) {
      if (!useCustomCommands) {
        useCustomCommands = true
        commandsEnabledSet = new Set(commands.filter((command) => command !== name))
      } else {
        commandsEnabledSet.delete(name)
        commandsEnabledSet = new Set(commandsEnabledSet)
      }
    } else {
      commandsEnabledSet.add(name)
      commandsEnabledSet = new Set(commandsEnabledSet)
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

  function toggleAllCommands() {
    if (useCustomCommands) {
      useCustomCommands = false
      commandsEnabledSet = new Set(commands)
    } else {
      useCustomCommands = true
      commandsEnabledSet = new Set()
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
    if (useCustomCommands) {
      newConfig.commands_custom = true
      newConfig.commands_enabled = [...commandsEnabledSet]
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
      commands,
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
      effectiveConfig = await updateSessionLocalConfig(sessionId, nextConfig)
      config = effectiveConfig.effective.tool_config
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
      preview.gainedCommands,
      preview.lostCommands,
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
      { label: 'Commands enabled', items: preview.gainedCommands },
      { label: 'Commands disabled', items: preview.lostCommands },
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

  async function setStyleAxis(key: keyof SessionStyleValues, rawValue: string) {
    if (!sessionId || styleSaving) return
    const parsed = Number.parseInt(rawValue, 10)
    const next = sessionStylePayload({
      ...styleDraft,
      [key]: Number.isFinite(parsed) ? parsed : styleDraft[key],
    })
    styleDraft = {
      directness: next.directness ?? styleDraft.directness,
      humor: next.humor ?? styleDraft.humor,
      caution: next.caution ?? styleDraft.caution,
      autonomy: next.autonomy ?? styleDraft.autonomy,
    }
    styleSaving = true
    try {
      styleResponse = await updateSessionStyle(sessionId, next)
      styleDraft = { ...styleResponse.effective }
      onChange?.()
    } catch {
      void load()
    } finally {
      styleSaving = false
    }
  }

  let filteredTools = $derived(
    tools.filter((t) => !filterText || t.name.toLowerCase().includes(filterText.toLowerCase()))
  )
  let toolGroups = $derived(
    [...new Set(tools.map((t) => t.group).filter((group): group is string => Boolean(group)))].sort()
  )
  let filteredSkillDefs = $derived(
    skillDefs.filter((skill) => {
      const query = filterText.toLowerCase()
      const matchesText = !query ||
        skill.name.toLowerCase().includes(query) ||
        (skill.description ?? '').toLowerCase().includes(query) ||
        (skill.slash ?? '').toLowerCase().includes(query)
      return matchesText && matchesSkillSourceFilter(skill)
    })
  )
  let filteredCommandDefs = $derived(
    commandDefs.filter((command) => {
      const query = filterText.toLowerCase()
      return !query ||
        command.name.toLowerCase().includes(query) ||
        (command.description ?? '').toLowerCase().includes(query) ||
        (command.slash ?? '').toLowerCase().includes(query)
    })
  )
  let stylePreview = $derived(buildSessionStylePreview({ ...styleResponse, effective: styleDraft }))

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
      <button class="config-tab" class:active={activeTab === 'commands'} onclick={() => activeTab = 'commands'}>
        Commands ({commands.length})
      </button>
      <button class="config-tab" class:active={activeTab === 'automation'} onclick={() => activeTab = 'automation'}>
        Automation
      </button>
      <button class="config-tab" class:active={activeTab === 'style'} onclick={() => activeTab = 'style'}>
        Style
      </button>
    </div>

    {#if activeTab === 'tools' || activeTab === 'skills' || activeTab === 'commands'}
      <div class="config-filter">
        <input type="text" bind:value={filterText} placeholder="Filter..." class="config-filter-input" />
        <button class="config-reload" type="button" disabled={loading} onclick={() => { void load() }}>Reload</button>
      </div>
    {/if}

    {#if activeTab === 'skills'}
      <div class="config-source-filters" aria-label="Skill source filters">
        {#each skillSourceFilters as filter}
          <button
            class="source-filter"
            class:active={skillSourceFilter === filter.id}
            type="button"
            onclick={() => skillSourceFilter = filter.id}
          >
            {filter.label}
          </button>
        {/each}
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
          {@const toolSrc = sourceForToolList(t.name)}
          <label class="config-item" class:high-risk={t.high_risk}>
            <input type="checkbox" checked={isToolEnabled(t.name)} onchange={() => toggleTool(t.name)} />
            <span class="item-name">{t.name}</span>
            {#if t.group}
              <span class="badge badge-neutral" style="font-size:9px;padding:0 4px;">{t.group}</span>
            {/if}
            {#if t.high_risk}
              <span class="badge badge-warning" style="font-size:9px;padding:0 4px;">risk</span>
            {/if}
            {#if toolSrc !== 'base'}
              <span class="source-badge source-{toolSrc}" title={sourceBadgeTitle[toolSrc]}>{sourceBadgeLabel[toolSrc]}</span>
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
        {#each filteredSkillDefs as skill}
          {@const s = skill.name}
          {@const skillSrc = sourceForSkillList(s)}
          {@const originClass = skillOriginClass(skill)}
          <label class="config-item">
            <input type="checkbox" checked={isSkillEnabled(s)} onchange={() => toggleSkill(s)} />
            <span class="item-name">{s}</span>
            {#if skillSlashLabel(skill)}
              <span class="skill-slash">{skillSlashLabel(skill)}</span>
            {/if}
            <span class="skill-origin-badge source-{originClass}" title={skillOriginTitle(skill)}>
              {skillOriginLabel(skill)}
            </span>
            {#if skillSrc !== 'base'}
              <span class="source-badge source-{skillSrc}" title={sourceBadgeTitle[skillSrc]}>{sourceBadgeLabel[skillSrc]}</span>
            {/if}
          </label>
        {/each}
      </div>
    {:else if activeTab === 'commands'}
      <div class="config-actions">
        <label class="config-toggle">
          <input type="checkbox" checked={!useCustomCommands} onchange={toggleAllCommands} />
          <span>All commands</span>
        </label>
        <span class="config-count">{useCustomCommands ? commandsEnabledSet.size : commands.length} active</span>
      </div>
      <div class="config-list">
        {#each filteredCommandDefs as command}
          {@const c = command.name}
          {@const commandSrc = sourceForCommandList(c)}
          <label class="config-item">
            <input type="checkbox" checked={isCommandEnabled(c)} onchange={() => toggleCommand(c)} />
            <span class="item-name">{c}</span>
            {#if skillSlashLabel(command)}
              <span class="skill-slash">{skillSlashLabel(command)}</span>
            {/if}
            <span class="skill-origin-badge source-command" title=".tars/commands">Command</span>
            {#if commandSrc !== 'base'}
              <span class="source-badge source-{commandSrc}" title={sourceBadgeTitle[commandSrc]}>{sourceBadgeLabel[commandSrc]}</span>
            {/if}
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
    {:else if activeTab === 'style'}
      <div class="style-list">
        {#each styleAxes as axis}
          <label class="style-slider">
            <span class="style-slider-head">
              <strong>{axis.label}</strong>
              <small>{styleDraft[axis.key]} / 100 · default {styleResponse.defaults[axis.key]}</small>
            </span>
            <input
              type="range"
              min="0"
              max="100"
              step="1"
              value={styleDraft[axis.key]}
              disabled={styleSaving}
              onchange={(event) => { void setStyleAxis(axis.key, (event.currentTarget as HTMLInputElement).value) }}
            />
          </label>
        {/each}
        <div class="style-preview">
          {#each stylePreview as line}
            <p>{line}</p>
          {/each}
          <span>
            {automationConsent.auto_resume || automationConsent.auto_resume_enabled
              ? `Auto-resume ${automationConsent.auto_resume_after_minutes ?? defaultAutoResumeMinutes}m`
              : 'Auto-resume off'}
            · {automationConsent.autonomous_mutations ? 'autonomous mutations allowed' : 'mutations consent off'}
          </span>
        </div>
        {#if styleResponse.style_control?.updated_at}
          <div class="automation-updated">Updated {new Date(styleResponse.style_control.updated_at).toLocaleString()}</div>
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
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    gap: var(--space-2);
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

  .config-reload {
    padding: 0 var(--space-2);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface-base);
    color: var(--text-secondary);
    font-family: var(--font-mono);
    font-size: 10px;
    cursor: pointer;
  }

  .config-reload:hover {
    border-color: var(--primary);
    color: var(--primary);
  }

  .config-source-filters {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-1);
    padding: 0 var(--space-3) var(--space-2);
    border-bottom: 1px solid var(--border-subtle);
  }

  .source-filter {
    padding: 2px 6px;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface-base);
    color: var(--text-ghost);
    font-family: var(--font-mono);
    font-size: 10px;
    cursor: pointer;
  }

  .source-filter.active {
    border-color: var(--primary);
    color: var(--primary);
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

  .skill-slash {
    margin-left: auto;
    color: var(--text-ghost);
    font-family: var(--font-mono);
    font-size: 10px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 80px;
  }

  .skill-origin-badge {
    padding: 0 5px;
    border-radius: var(--radius-sm);
    font-family: var(--font-display);
    font-size: 9px;
    line-height: 14px;
    letter-spacing: 0.04em;
    text-transform: uppercase;
    white-space: nowrap;
  }

  .skill-origin-badge.source-global {
    color: var(--text-ghost);
    border: 1px solid var(--border-subtle);
    background: var(--surface-base);
  }

  .skill-origin-badge.source-session {
    color: var(--primary);
    border: 1px solid color-mix(in srgb, var(--primary) 35%, transparent);
    background: color-mix(in srgb, var(--primary) 12%, transparent);
  }

  .skill-origin-badge.source-command {
    color: rgb(34, 197, 94);
    border: 1px solid rgba(34, 197, 94, 0.35);
    background: rgba(34, 197, 94, 0.1);
  }

  /* Source badge — shows where the effective value came from when it
     was contributed by `.tars/settings.json` (shared) or
     `.tars/settings.local.json` (local). The base / sessions.json case
     renders no badge so the row stays quiet for plain sessions. */
  .source-badge {
    margin-left: auto;
    padding: 0 5px;
    border-radius: var(--radius-sm);
    font-size: 9px;
    font-family: var(--font-display);
    line-height: 14px;
    letter-spacing: 0.04em;
    text-transform: uppercase;
    cursor: help;
  }
  .skill-origin-badge + .source-badge {
    margin-left: 0;
  }
  .source-badge.source-shared {
    color: var(--primary);
    background: color-mix(in srgb, var(--primary) 14%, transparent);
    border: 1px solid color-mix(in srgb, var(--primary) 35%, transparent);
  }
  .source-badge.source-local {
    color: var(--text-primary);
    background: color-mix(in srgb, var(--text-primary) 10%, transparent);
    border: 1px solid color-mix(in srgb, var(--text-primary) 25%, transparent);
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

  .style-list {
    display: grid;
    gap: var(--space-2);
    padding: var(--space-3);
  }

  .style-slider {
    display: grid;
    gap: var(--space-2);
    padding: var(--space-2);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface-base);
  }

  .style-slider-head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: var(--space-2);
    min-width: 0;
  }

  .style-slider-head strong {
    color: var(--text-primary);
    font-size: var(--text-xs);
  }

  .style-slider-head small {
    color: var(--text-tertiary);
    font-family: var(--font-mono);
    font-size: 10px;
    text-align: right;
  }

  .style-slider input[type='range'] {
    width: 100%;
    accent-color: var(--primary);
  }

  .style-preview {
    display: grid;
    gap: var(--space-1);
    padding: var(--space-2);
    border: 1px solid rgba(224, 145, 69, 0.28);
    border-radius: var(--radius-sm);
    background: rgba(224, 145, 69, 0.08);
  }

  .style-preview p {
    margin: 0;
    color: var(--text-secondary);
    font-family: var(--font-mono);
    font-size: 10px;
    line-height: 1.4;
  }

  .style-preview span {
    color: var(--text-tertiary);
    font-family: var(--font-mono);
    font-size: 10px;
  }
</style>
