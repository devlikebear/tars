<script lang="ts">
  import { getChatContext, type ChatContextInfo } from '../lib/api'

  interface Props {
    sessionId: string
    contextInfo?: {
      system_prompt_tokens?: number
      history_tokens?: number
      history_messages?: number
      tool_count?: number
      tool_names?: string[]
      skill_count?: number
      skill_names?: string[]
      memory_count?: number
      memory_tokens?: number
      used_tool_names?: string[]
      selected_skill_name?: string
      selected_skill_reason?: string
    }
    refreshVersion?: number
    onClose?: () => void
  }

  let { sessionId, contextInfo, refreshVersion = 0, onClose }: Props = $props()

  let fullContext = $state<ChatContextInfo | null>(null)
  let loading = $state(false)
  let showPrompt = $state(false)

  async function loadFullContext() {
    if (!sessionId) return
    loading = true
    try {
      fullContext = await getChatContext(sessionId)
    } catch {
      // ignore
    }
    loading = false
  }

  function valOr(a: number | undefined, b: number | undefined): number {
    return a ?? b ?? 0
  }

  function listOr(primary: string[] | undefined, secondary: string[] | undefined): string[] {
    return primary ?? secondary ?? []
  }

  let totalTokens = $derived(
    valOr(contextInfo?.system_prompt_tokens, fullContext?.system_prompt_tokens) +
    valOr(contextInfo?.history_tokens, fullContext?.history_tokens) +
    valOr(contextInfo?.memory_tokens, fullContext?.memory_tokens)
  )
  let injectedTools = $derived(listOr(contextInfo?.tool_names, fullContext?.tool_names))
  let availableSkills = $derived(listOr(contextInfo?.skill_names, fullContext?.skill_names))
  let usedTools = $derived(listOr(contextInfo?.used_tool_names, fullContext?.used_tool_names))
  let selectedSkillName = $derived(contextInfo?.selected_skill_name ?? fullContext?.selected_skill_name ?? '')
  let selectedSkillReason = $derived(contextInfo?.selected_skill_reason ?? fullContext?.selected_skill_reason ?? '')

  let contextLimit = 200000
  let usagePercent = $derived(Math.min(100, (totalTokens / contextLimit) * 100))
  let usageColor = $derived(
    usagePercent > 80 ? 'var(--error)' : usagePercent > 50 ? 'var(--warning, #f59e0b)' : 'var(--success, #22c55e)'
  )

  $effect(() => {
    refreshVersion
    if (sessionId) void loadFullContext()
  })
</script>

<div class="monitor-panel">
  <div class="monitor-header">
    <span class="monitor-title">Context HUD</span>
    {#if onClose}
      <button class="monitor-close" onclick={onClose}>&times;</button>
    {/if}
  </div>

  <div class="monitor-bar-container">
    <div class="monitor-bar" style="width: {usagePercent}%; background: {usageColor};"></div>
  </div>
  <div class="monitor-bar-label">
    {totalTokens.toLocaleString()} / {contextLimit.toLocaleString()} tokens ({usagePercent.toFixed(1)}%)
  </div>

  <div class="monitor-grid">
    <div class="monitor-stat">
      <span class="stat-label">System Prompt</span>
      <span class="stat-value">{(contextInfo?.system_prompt_tokens ?? fullContext?.system_prompt_tokens ?? 0).toLocaleString()}</span>
    </div>
    <div class="monitor-stat">
      <span class="stat-label">History</span>
      <span class="stat-value">{(contextInfo?.history_tokens ?? fullContext?.history_tokens ?? 0).toLocaleString()} ({contextInfo?.history_messages ?? fullContext?.history_messages ?? 0} msgs)</span>
    </div>
    <div class="monitor-stat">
      <span class="stat-label">Tools</span>
      <span class="stat-value">{contextInfo?.tool_count ?? fullContext?.tool_count ?? 0}</span>
    </div>
    <div class="monitor-stat">
      <span class="stat-label">Skills</span>
      <span class="stat-value">{contextInfo?.skill_count ?? fullContext?.skill_count ?? 0}</span>
    </div>
    <div class="monitor-stat">
      <span class="stat-label">Memory</span>
      <span class="stat-value">{(contextInfo?.memory_count ?? fullContext?.memory_count ?? 0)} ({(contextInfo?.memory_tokens ?? fullContext?.memory_tokens ?? 0).toLocaleString()} tokens)</span>
    </div>
    <div class="monitor-stat">
      <span class="stat-label">Used This Turn</span>
      <span class="stat-value">{usedTools.length}</span>
    </div>
  </div>

  {#if selectedSkillName}
    <div class="monitor-section">
      <span class="section-title">Selected Skill</span>
      <div class="tool-chips">
        <span class="tool-chip">{selectedSkillName}{#if selectedSkillReason} · {selectedSkillReason}{/if}</span>
      </div>
    </div>
  {/if}

  {#if injectedTools.length > 0}
    <div class="monitor-section">
      <span class="section-title">Injected Tools</span>
      <div class="tool-chips">
        {#each injectedTools as name}
          <span class="tool-chip">{name}</span>
        {/each}
      </div>
    </div>
  {/if}

  {#if availableSkills.length > 0}
    <div class="monitor-section">
      <span class="section-title">Available Skills</span>
      <div class="tool-chips">
        {#each availableSkills as name}
          <span class="tool-chip">{name}</span>
        {/each}
      </div>
    </div>
  {/if}

  {#if usedTools.length > 0}
    <div class="monitor-section">
      <span class="section-title">Used Tools</span>
      <div class="tool-chips">
        {#each usedTools as name}
          <span class="tool-chip">{name}</span>
        {/each}
      </div>
    </div>
  {/if}

  {#if fullContext?.system_prompt}
    <div class="monitor-section">
      <button class="section-toggle" onclick={() => showPrompt = !showPrompt}>
        {showPrompt ? '\u25BC' : '\u25B6'} System Prompt
      </button>
      {#if showPrompt}
        <pre class="prompt-preview">{fullContext.system_prompt}</pre>
      {/if}
    </div>
  {/if}

  <div class="monitor-actions">
    <button class="btn btn-ghost btn-sm" onclick={loadFullContext} disabled={loading}>
      {loading ? 'Loading...' : 'Refresh'}
    </button>
  </div>
</div>

<style>
  .monitor-panel {
    display: flex;
    flex-direction: column;
    height: 100%;
    overflow: hidden;
    background: var(--bg-surface);
    border-left: 1px solid var(--border-subtle);
  }

  .monitor-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--border-subtle);
  }

  .monitor-title {
    font-family: var(--font-display);
    font-weight: 600;
    font-size: var(--text-sm);
    color: var(--text-primary);
  }

  .monitor-close {
    background: none;
    border: none;
    color: var(--text-ghost);
    cursor: pointer;
    font-size: 18px;
    padding: 0;
    line-height: 1;
  }
  .monitor-close:hover { color: var(--text-primary); }

  .monitor-bar-container {
    height: 4px;
    background: var(--bg-base);
    margin: var(--space-2) var(--space-3) 0;
    border-radius: 2px;
    overflow: hidden;
  }

  .monitor-bar {
    height: 100%;
    border-radius: 2px;
    transition: width 0.3s ease, background 0.3s ease;
  }

  .monitor-bar-label {
    font-family: var(--font-mono);
    font-size: 10px;
    color: var(--text-ghost);
    text-align: center;
    padding: 2px var(--space-3) var(--space-2);
  }

  .monitor-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 1px;
    padding: 0 var(--space-3);
    margin-bottom: var(--space-2);
  }

  .monitor-stat {
    display: flex;
    flex-direction: column;
    padding: var(--space-2);
    background: var(--bg-base);
    border-radius: var(--radius-sm);
  }

  .stat-label {
    font-family: var(--font-mono);
    font-size: 10px;
    color: var(--text-ghost);
    margin-bottom: 2px;
  }

  .stat-value {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-primary);
    font-weight: 500;
  }

  .monitor-section {
    padding: var(--space-2) var(--space-3);
    border-top: 1px solid var(--border-subtle);
  }

  .section-title {
    font-family: var(--font-mono);
    font-size: 10px;
    color: var(--text-ghost);
    display: block;
    margin-bottom: var(--space-1);
  }

  .section-toggle {
    background: none;
    border: none;
    color: var(--text-secondary);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    cursor: pointer;
    padding: 0;
    display: block;
    width: 100%;
    text-align: left;
  }
  .section-toggle:hover { color: var(--accent); }

  .tool-chips {
    display: flex;
    flex-wrap: wrap;
    gap: 3px;
  }

  .tool-chip {
    font-family: var(--font-mono);
    font-size: 9px;
    color: var(--text-ghost);
    background: var(--bg-base);
    padding: 1px 5px;
    border-radius: 3px;
    border: 1px solid var(--border-subtle);
  }

  .prompt-preview {
    font-family: var(--font-mono);
    font-size: 10px;
    color: var(--text-ghost);
    background: var(--bg-base);
    border-radius: var(--radius-sm);
    padding: var(--space-2);
    margin-top: var(--space-1);
    max-height: 300px;
    overflow-y: auto;
    white-space: pre-wrap;
    word-break: break-word;
  }

  .monitor-actions {
    padding: var(--space-2) var(--space-3);
    border-top: 1px solid var(--border-subtle);
    margin-top: auto;
  }
</style>
