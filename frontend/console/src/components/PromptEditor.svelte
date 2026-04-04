<script lang="ts">
  import { getChatContext, updateSessionPrompt } from '../lib/api'

  interface Props {
    sessionId: string
    onClose?: () => void
  }

  let { sessionId, onClose }: Props = $props()

  let systemPrompt = $state('')
  let promptOverride = $state('')
  let originalOverride = $state('')
  let systemPromptTokens = $state(0)
  let loading = $state(true)
  let saving = $state(false)
  let showBase = $state(false)

  async function load() {
    if (!sessionId) return
    loading = true
    try {
      const ctx = await getChatContext(sessionId)
      systemPrompt = ctx.system_prompt
      systemPromptTokens = ctx.system_prompt_tokens
      promptOverride = ctx.prompt_override ?? ''
      originalOverride = promptOverride
    } catch {
      // ignore
    }
    loading = false
  }

  async function save() {
    if (!sessionId) return
    saving = true
    try {
      await updateSessionPrompt(sessionId, promptOverride)
      originalOverride = promptOverride
    } catch {
      // ignore
    }
    saving = false
  }

  async function clearOverride() {
    promptOverride = ''
    await save()
  }

  let isDirty = $derived(promptOverride !== originalOverride)
  let overrideTokens = $derived(Math.max(1, Math.floor(promptOverride.length / 4)))

  $effect(() => {
    if (sessionId) void load()
  })
</script>

<div class="editor-panel">
  <div class="editor-header">
    <span class="editor-title">Prompt Editor</span>
    {#if onClose}
      <button class="editor-close" onclick={onClose}>&times;</button>
    {/if}
  </div>

  {#if loading}
    <div class="editor-loading">Loading...</div>
  {:else}
    <div class="editor-section">
      <button class="section-toggle" onclick={() => showBase = !showBase}>
        {showBase ? '\u25BC' : '\u25B6'} Base System Prompt ({systemPromptTokens.toLocaleString()} tokens)
      </button>
      {#if showBase}
        <pre class="prompt-readonly">{systemPrompt}</pre>
      {/if}
    </div>

    <div class="editor-section">
      <div class="override-header">
        <span class="override-label">Session Override</span>
        {#if promptOverride}
          <span class="override-tokens">~{overrideTokens} tokens</span>
        {/if}
      </div>
      <textarea
        class="override-textarea"
        bind:value={promptOverride}
        rows="8"
        placeholder="Add custom instructions for this session..."
      ></textarea>
      <div class="editor-actions">
        <button class="btn btn-primary btn-sm" onclick={save} disabled={saving || !isDirty}>
          {saving ? 'Saving...' : 'Apply'}
        </button>
        {#if promptOverride}
          <button class="btn btn-ghost btn-sm" onclick={clearOverride}>Clear</button>
        {/if}
        <button class="btn btn-ghost btn-sm" onclick={load}>Refresh</button>
      </div>
    </div>
  {/if}
</div>

<style>
  .editor-panel {
    display: flex;
    flex-direction: column;
    height: 100%;
    overflow: hidden;
    background: var(--bg-surface);
    border-left: 1px solid var(--border-subtle);
  }

  .editor-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--border-subtle);
  }

  .editor-title {
    font-family: var(--font-display);
    font-weight: 600;
    font-size: var(--text-sm);
    color: var(--text-primary);
  }

  .editor-close {
    background: none;
    border: none;
    color: var(--text-ghost);
    cursor: pointer;
    font-size: 18px;
    padding: 0;
    line-height: 1;
  }
  .editor-close:hover { color: var(--text-primary); }

  .editor-loading {
    padding: var(--space-4);
    text-align: center;
    color: var(--text-ghost);
    font-size: var(--text-sm);
  }

  .editor-section {
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--border-subtle);
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

  .prompt-readonly {
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

  .override-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: var(--space-1);
  }

  .override-label {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-secondary);
    font-weight: 500;
  }

  .override-tokens {
    font-family: var(--font-mono);
    font-size: 10px;
    color: var(--text-ghost);
  }

  .override-textarea {
    width: 100%;
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--text-primary);
    background: var(--bg-base);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    padding: var(--space-2);
    resize: vertical;
    min-height: 120px;
  }
  .override-textarea:focus {
    outline: none;
    border-color: var(--accent);
  }

  .editor-actions {
    display: flex;
    gap: var(--space-2);
    margin-top: var(--space-2);
  }
</style>
