<script lang="ts">
  import { onDestroy } from 'svelte'
  import { getPriorContextPreview, type PriorContextPreview, type PriorContextPreviewItem } from '../lib/api'

  interface Props {
    sessionId: string
    draftQuery?: string
    onClose?: () => void
  }

  let { sessionId, draftQuery = '', onClose }: Props = $props()

  // Debounce window after typing stops before we re-fetch the preview. Long
  // enough to avoid hammering the embedding/keyword search on every keystroke,
  // short enough to feel live. The 5-min MemoryCache absorbs repeats.
  const DEBOUNCE_MS = 700

  let preview = $state<PriorContextPreview | null>(null)
  let loading = $state(false)
  let error = $state('')
  let loadedSessionId = $state('')
  let lastPreviewedQuery = $state('')
  let pendingTimer: ReturnType<typeof setTimeout> | null = null

  let trimmedDraft = $derived((draftQuery ?? '').trim())
  let isStale = $derived(preview !== null && trimmedDraft !== lastPreviewedQuery && !loading)
  let mode = $derived(preview?.mode ?? 'default')
  let isRecentMode = $derived(mode === 'recent')
  let displayItems = $derived<PriorContextPreviewItem[]>(
    isRecentMode ? (preview?.recent_fallback_items ?? []) : (preview?.items ?? []),
  )
  let belowThreshold = $derived<PriorContextPreviewItem[]>(preview?.below_threshold_items ?? [])
  let budgetText = $derived(preview && !isRecentMode
    ? `${preview.relevant_tokens.toLocaleString()} / ${preview.relevant_budget_tokens.toLocaleString()} tokens (${preview.budget_percent}%)`
    : isRecentMode
      ? 'Recent fallback (no active query)'
      : '0 / 0 tokens (0%)'
  )

  function sourceClass(source_tag: string): string {
    return source_tag.trim().toLowerCase().replace(/[^a-z0-9_-]/g, '-') || 'context'
  }

  async function load(query = trimmedDraft) {
    if (!sessionId) return
    loading = true
    error = ''
    try {
      preview = await getPriorContextPreview(sessionId, query)
      lastPreviewedQuery = query
    } catch (e) {
      error = e instanceof Error ? e.message : 'Preview failed'
    }
    loading = false
  }

  function scheduleAutoRefresh() {
    if (pendingTimer) clearTimeout(pendingTimer)
    pendingTimer = setTimeout(() => {
      pendingTimer = null
      void load(trimmedDraft)
    }, DEBOUNCE_MS)
  }

  onDestroy(() => {
    if (pendingTimer) {
      clearTimeout(pendingTimer)
      pendingTimer = null
    }
  })

  $effect(() => {
    const sid = sessionId
    if (sid && sid !== loadedSessionId) {
      loadedSessionId = sid
      preview = null
      lastPreviewedQuery = ''
      void load(trimmedDraft)
    }
  })

  // Auto-refresh on draft change (debounced). The first load for a session is
  // handled by the session-change effect above; subsequent draft edits flow
  // through here.
  $effect(() => {
    const draft = trimmedDraft
    if (!loadedSessionId) return
    if (draft === lastPreviewedQuery) return
    scheduleAutoRefresh()
  })
</script>

<div class="prior-panel">
  <div class="prior-header">
    <div>
      <span class="prior-title">Prior Context</span>
      <span class="prior-subtitle">{budgetText}</span>
    </div>
    {#if onClose}
      <button class="prior-close" onclick={onClose}>&times;</button>
    {/if}
  </div>

  <div class="prior-toolbar">
    <button class="btn btn-ghost btn-sm" onclick={() => load()} disabled={loading || !sessionId}>
      {loading ? 'Loading...' : 'Refresh preview'}
    </button>
    {#if isStale}
      <span class="stale-badge">Updating…</span>
    {/if}
  </div>

  {#if error}
    <div class="prior-error">{error}</div>
  {/if}

  <div class="prior-body">
    {#if !preview && loading}
      <div class="prior-empty">Loading...</div>
    {:else if preview}
      {#if isRecentMode}
        <div class="prior-banner" role="status">
          <strong>No active query.</strong> Showing recent memory the assistant could reach. Type in the chat box to preview the actual recall.
        </div>
      {:else}
        <div class="prior-meter" aria-label="Prior Context token budget">
          <div class="prior-meter-fill" style="width: {Math.min(100, Math.max(0, preview.budget_percent))}%;"></div>
        </div>
      {/if}

      {#if displayItems.length > 0}
        <div class="prior-items">
          {#each displayItems as item}
            <article class="prior-item">
              <div class="prior-item-meta">
                <span class="source-badge tag-{sourceClass(item.source_tag)}">{item.source_tag}</span>
                <span class="source-path">{item.source}</span>
                <span class="item-tokens">{item.tokens.toLocaleString()} tokens</span>
              </div>
              <p>{item.snippet}</p>
            </article>
          {/each}
        </div>
      {:else}
        <div class="prior-empty">
          {isRecentMode ? 'No recent memory available yet.' : 'No matches for this query.'}
        </div>
      {/if}

      {#if belowThreshold.length > 0}
        <details class="prior-section prior-below">
          <summary>Below threshold ({belowThreshold.length}) — not sent to LLM</summary>
          <div class="prior-items prior-items-dim">
            {#each belowThreshold as item}
              <article class="prior-item">
                <div class="prior-item-meta">
                  <span class="source-badge tag-{sourceClass(item.source_tag)}">{item.source_tag}</span>
                  <span class="source-path">{item.source}</span>
                  <span class="item-tokens">{item.tokens.toLocaleString()} tokens</span>
                </div>
                <p>{item.snippet}</p>
              </article>
            {/each}
          </div>
        </details>
      {/if}

      {#if !isRecentMode}
        <details class="prior-section">
          <summary>Prompt Section</summary>
          <pre>{preview.section || '## Prior Context\n\n'}</pre>
        </details>
      {/if}
    {:else}
      <div class="prior-empty">No session selected</div>
    {/if}
  </div>
</div>

<style>
  .prior-panel {
    display: flex;
    flex-direction: column;
    height: 100%;
    overflow: hidden;
    background: var(--surface);
    border-left: 1px solid var(--border-subtle);
  }

  .prior-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-3);
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--border-subtle);
  }

  .prior-title,
  .prior-subtitle {
    display: block;
  }

  .prior-title {
    font-family: var(--font-display);
    font-weight: 600;
    font-size: var(--text-sm);
    color: var(--text-primary);
  }

  .prior-subtitle {
    margin-top: 2px;
    color: var(--text-ghost);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
  }

  .prior-close {
    background: none;
    border: none;
    color: var(--text-ghost);
    cursor: pointer;
    font-size: 18px;
    padding: 0;
    line-height: 1;
  }

  .prior-toolbar {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--border-subtle);
  }

  .stale-badge {
    border: 1px solid rgba(245, 158, 11, 0.3);
    border-radius: var(--radius-sm);
    padding: 2px 6px;
    color: var(--warning, #f59e0b);
    font-size: var(--text-xs);
  }

  .prior-error {
    margin: var(--space-3);
    padding: var(--space-2);
    border: 1px solid rgba(239, 68, 68, 0.3);
    border-radius: var(--radius-sm);
    color: var(--error);
    background: rgba(239, 68, 68, 0.08);
    font-size: var(--text-sm);
  }

  .prior-body {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    min-height: 0;
    overflow: auto;
    padding: var(--space-3);
  }

  .prior-banner {
    border: 1px solid rgba(99, 102, 241, 0.3);
    border-radius: var(--radius-sm);
    background: rgba(99, 102, 241, 0.08);
    color: var(--text-secondary);
    padding: var(--space-2) var(--space-3);
    font-size: var(--text-sm);
    line-height: 1.45;
  }

  .prior-banner strong {
    color: var(--text-primary);
  }

  .prior-meter {
    height: 8px;
    overflow: hidden;
    border-radius: var(--radius-sm);
    background: var(--surface-muted);
    border: 1px solid var(--border-subtle);
  }

  .prior-meter-fill {
    height: 100%;
    background: var(--accent);
  }

  .prior-items {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .prior-items-dim {
    opacity: 0.55;
  }

  .prior-item {
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    padding: var(--space-2);
    background: var(--surface-muted);
  }

  .prior-item-meta {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-width: 0;
    color: var(--text-ghost);
    font-size: var(--text-xs);
  }

  .source-badge {
    flex: 0 0 auto;
    border-radius: var(--radius-sm);
    padding: 2px 6px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0;
    color: var(--text-primary);
    background: rgba(99, 102, 241, 0.14);
    border: 1px solid rgba(99, 102, 241, 0.24);
  }

  .tag-experience {
    background: rgba(34, 197, 94, 0.14);
    border-color: rgba(34, 197, 94, 0.24);
  }

  .tag-project {
    background: rgba(14, 165, 233, 0.14);
    border-color: rgba(14, 165, 233, 0.24);
  }

  .tag-daily {
    background: rgba(245, 158, 11, 0.14);
    border-color: rgba(245, 158, 11, 0.24);
  }

  .tag-conversation {
    background: rgba(168, 85, 247, 0.14);
    border-color: rgba(168, 85, 247, 0.24);
  }

  .source-path {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-family: var(--font-mono);
  }

  .item-tokens {
    margin-left: auto;
    flex: 0 0 auto;
    font-family: var(--font-mono);
  }

  .prior-item p {
    margin: var(--space-2) 0 0;
    color: var(--text-primary);
    font-size: var(--text-sm);
    line-height: 1.45;
    word-break: break-word;
  }

  .prior-section {
    border-top: 1px solid var(--border-subtle);
    padding-top: var(--space-2);
  }

  .prior-section summary {
    cursor: pointer;
    color: var(--text-secondary);
    font-size: var(--text-sm);
    font-weight: 600;
  }

  .prior-below summary {
    color: var(--text-ghost);
  }

  .prior-section pre {
    max-height: 280px;
    overflow: auto;
    margin: var(--space-2) 0 0;
    padding: var(--space-2);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface-muted);
    color: var(--text-primary);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    white-space: pre-wrap;
    word-break: break-word;
  }

  .prior-empty {
    color: var(--text-ghost);
    font-size: var(--text-sm);
    padding: var(--space-4) 0;
    text-align: center;
  }
</style>
