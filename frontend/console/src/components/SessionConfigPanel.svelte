<script lang="ts">
  import { listChatTools, getSessionConfig, updateSessionConfig, type ChatToolInfo, type SessionToolConfig } from '../lib/api'

  interface Props {
    sessionId: string
    onClose?: () => void
    onChange?: () => void
  }

  let { sessionId, onClose, onChange }: Props = $props()

  let tools: ChatToolInfo[] = $state([])
  let skills: string[] = $state([])
  let config: SessionToolConfig = $state({})
  let loading = $state(true)
  let filterText = $state('')
  let activeTab: 'tools' | 'skills' | 'mcp' = $state('tools')

  let enabledSet: Set<string> = $state(new Set())
  let disabledSet: Set<string> = $state(new Set())
  let skillsEnabledSet: Set<string> = $state(new Set())
  let useCustomConfig = $state(false)
  let useCustomSkills = $state(false)

  async function load() {
    loading = true
    try {
      const [toolsResp, configResp] = await Promise.all([
        listChatTools(),
        sessionId ? getSessionConfig(sessionId) : Promise.resolve({} as SessionToolConfig),
      ])
      tools = toolsResp.tools
      skills = toolsResp.skills ?? []
      config = configResp

      // Initialize sets from config
      if (config.tools_custom || Array.isArray(config.tools_enabled)) {
        useCustomConfig = true
        enabledSet = new Set(config.tools_enabled ?? [])
      } else {
        useCustomConfig = false
        enabledSet = new Set(tools.map((t) => t.name))
      }
      disabledSet = new Set(config.tools_disabled ?? [])

      if (config.skills_custom || Array.isArray(config.skills_enabled)) {
        useCustomSkills = true
        skillsEnabledSet = new Set(config.skills_enabled ?? [])
      } else {
        useCustomSkills = false
        skillsEnabledSet = new Set(skills)
      }
    } catch {
      // ignore
    }
    loading = false
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
    void saveConfig()
  }

  function isSkillEnabled(name: string): boolean {
    if (!useCustomSkills) return true
    return skillsEnabledSet.has(name)
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
    void saveConfig()
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
    void saveConfig()
  }

  function toggleAllSkills() {
    if (useCustomSkills) {
      useCustomSkills = false
      skillsEnabledSet = new Set(skills)
    } else {
      useCustomSkills = true
      skillsEnabledSet = new Set()
    }
    void saveConfig()
  }

  async function saveConfig() {
    if (!sessionId) return
    const newConfig: SessionToolConfig = {}
    if (useCustomConfig) {
      newConfig.tools_custom = true
      newConfig.tools_enabled = [...enabledSet]
    }
    if (disabledSet.size > 0) {
      newConfig.tools_disabled = [...disabledSet]
    }
    if (useCustomSkills) {
      newConfig.skills_custom = true
      newConfig.skills_enabled = [...skillsEnabledSet]
    }
    await updateSessionConfig(sessionId, newConfig)
      .then(() => onChange?.())
      .catch(() => {})
  }

  let filteredTools = $derived(
    tools.filter((t) => !filterText || t.name.toLowerCase().includes(filterText.toLowerCase()))
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
    </div>

    <div class="config-filter">
      <input type="text" bind:value={filterText} placeholder="Filter..." class="config-filter-input" />
    </div>

    {#if activeTab === 'tools'}
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
    {/if}
  {/if}
</div>

<style>
  .config-panel {
    display: flex;
    flex-direction: column;
    height: 100%;
    overflow: hidden;
    background: var(--bg-surface);
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
    color: var(--accent);
    border-bottom-color: var(--accent);
  }
  .config-tab:hover { color: var(--text-primary); }

  .config-filter {
    padding: var(--space-2) var(--space-3);
  }

  .config-filter-input {
    width: 100%;
    padding: var(--space-1) var(--space-2);
    background: var(--bg-base);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    color: var(--text-primary);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
  }

  .config-actions {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: var(--space-1) var(--space-3);
    border-bottom: 1px solid var(--border-subtle);
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
</style>
