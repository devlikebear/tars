<script lang="ts">
  import { onMount } from 'svelte'
  import { getForkPromotions, getSessionHistory, listSessions, promoteForkInsights } from '../lib/api'
  import {
    buildSessionLineageRows,
    forkPreviewFromHistory,
    type ForkPreview,
    type SessionLineageRow,
  } from '../lib/sessionLineage'
  import type { ForkPromotionCandidate, Session } from '../lib/types'

  interface Props {
    onNavigate: (path: string) => void
  }

  let { onNavigate }: Props = $props()

  let rows: SessionLineageRow[] = $state([])
  let loading = $state(true)
  let error = $state('')
  let promotionSessionId = $state('')
  let promotionSessionTitle = $state('')
  let promotionCandidates: ForkPromotionCandidate[] = $state([])
  let selectedPromotionIds: string[] = $state([])
  let promotionLoading = $state(false)
  let promotionSaving = $state(false)
  let promotionError = $state('')
  let promotionSuccess = $state('')

  let rootCount = $derived(rows.filter((row) => row.kind === 'root').length)
  let forkCount = $derived(rows.filter((row) => row.kind === 'fork').length)
  let selectedPromotionCount = $derived(selectedPromotionIds.length)

  async function load() {
    loading = true
    error = ''
    try {
      const sessions = await listSessions(true)
      const previews = await loadForkPreviews(sessions)
      rows = buildSessionLineageRows(sessions, previews)
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load session lineage'
      rows = []
    } finally {
      loading = false
    }
  }

  async function loadForkPreviews(sessions: Session[]): Promise<Record<string, ForkPreview>> {
    const previews: Record<string, ForkPreview> = {}
    await Promise.all(sessions.map(async (session) => {
      const parentId = session.parent_session_id?.trim()
      if (!parentId || (!session.forked_from_message_id && session.forked_from_index === undefined)) return
      try {
        const history = await getSessionHistory(parentId)
        const preview = forkPreviewFromHistory(session, history)
        if (preview) previews[session.id] = preview
      } catch {
        // Missing parent history should not hide the graph itself.
      }
    }))
    return previews
  }

  function openSession(row: SessionLineageRow) {
    onNavigate(`/console/chat/${encodeURIComponent(row.session.id)}`)
  }

  async function reviewForkInsights(row: SessionLineageRow) {
    promotionSessionId = row.session.id
    promotionSessionTitle = row.session.title || row.session.id
    promotionCandidates = []
    selectedPromotionIds = []
    promotionError = ''
    promotionSuccess = ''
    promotionLoading = true
    try {
      const res = await getForkPromotions(row.session.id)
      promotionCandidates = res.candidates || []
      selectedPromotionIds = promotionCandidates.map((candidate) => candidate.id)
    } catch (err) {
      promotionError = err instanceof Error ? err.message : 'Failed to load fork insights'
    } finally {
      promotionLoading = false
    }
  }

  function isPromotionSelected(id: string): boolean {
    return selectedPromotionIds.includes(id)
  }

  function togglePromotionCandidate(id: string) {
    if (isPromotionSelected(id)) {
      selectedPromotionIds = selectedPromotionIds.filter((value) => value !== id)
    } else {
      selectedPromotionIds = [...selectedPromotionIds, id]
    }
  }

  async function queueSelectedPromotions() {
    if (!promotionSessionId || selectedPromotionIds.length === 0 || promotionSaving) return
    promotionError = ''
    promotionSuccess = ''
    promotionSaving = true
    try {
      const res = await promoteForkInsights(promotionSessionId, selectedPromotionIds)
      promotionSuccess = `Queued ${res.promoted_count}; skipped ${res.skipped_count}.`
      selectedPromotionIds = []
    } catch (err) {
      promotionError = err instanceof Error ? err.message : 'Failed to queue fork insights'
    } finally {
      promotionSaving = false
    }
  }

  function formatTime(value: string): string {
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return value || 'unknown'
    return date.toLocaleString()
  }

  onMount(() => {
    void load()
  })
</script>

<section class="lineage-page">
  <header class="lineage-header">
    <div>
      <h2>Session Lineage</h2>
      <p>Forked chats, roots, and branch points across this workspace.</p>
    </div>
    <button type="button" class="btn btn-ghost btn-sm" disabled={loading} onclick={() => { void load() }}>Refresh</button>
  </header>

  <div class="lineage-stats" aria-label="Session lineage summary">
    <div><span>{rows.length}</span><small>sessions</small></div>
    <div><span>{rootCount}</span><small>roots</small></div>
    <div><span>{forkCount}</span><small>forks</small></div>
  </div>

  {#if loading}
    <div class="lineage-empty">Loading lineage...</div>
  {:else if error}
    <div class="error-banner">{error}</div>
  {:else if rows.length === 0}
    <div class="lineage-empty">No sessions yet.</div>
  {:else}
    <div class="lineage-graph" role="list" aria-label="Session lineage graph">
      {#each rows as row (row.session.id)}
        <div
          class="lineage-row"
          class:fork={row.kind === 'fork'}
          style={`--depth: ${row.depth}`}
          role="listitem"
        >
          <button type="button" class="lineage-main" onclick={() => openSession(row)}>
            <span class="lineage-gutter" aria-hidden="true">
              <span class="lineage-rail"></span>
              <span class="lineage-branch">{row.branchLabel}</span>
            </span>
            <span class="lineage-node">
              <span class="lineage-node-top">
                <strong>{row.session.title || row.session.id}</strong>
                <span class="badge {row.kind === 'fork' ? 'badge-accent' : 'badge-default'}">{row.kind}</span>
              </span>
              <span class="lineage-meta">
                <span>{row.session.id}</span>
                <span>{formatTime(row.session.updated_at)}</span>
                {#if row.parent}
                  <span>parent: {row.parent.title || row.parent.id}</span>
                {/if}
              </span>
              {#if row.forkPreview}
                <span class="fork-preview">
                  <span>Fork point</span>
                  <strong>{row.forkPreview.role}</strong>
                  <span>{row.forkPreview.content || row.forkPreview.message_id}</span>
                </span>
              {:else if row.kind === 'fork'}
                <span class="fork-preview muted">
                  <span>Fork point</span>
                  <span>{row.session.forked_from_message_id || `index ${row.session.forked_from_index}`}</span>
                </span>
              {/if}
            </span>
          </button>
          {#if row.kind === 'fork'}
            <div class="lineage-actions">
              <button
                type="button"
                class="btn btn-ghost btn-sm"
                class:active-action={promotionSessionId === row.session.id}
                disabled={promotionLoading && promotionSessionId === row.session.id}
                onclick={() => { void reviewForkInsights(row) }}
              >
                {promotionLoading && promotionSessionId === row.session.id ? 'Loading...' : 'Review insights'}
              </button>
            </div>
          {/if}
        </div>
      {/each}
    </div>
  {/if}

  {#if promotionSessionId}
    <section class="promotion-panel" aria-label="Fork insights">
      <header class="promotion-header">
        <div>
          <span class="panel-label">Fork Insights</span>
          <h3>{promotionSessionTitle}</h3>
        </div>
        <button type="button" class="btn btn-primary btn-sm" disabled={promotionSaving || selectedPromotionCount === 0} onclick={() => { void queueSelectedPromotions() }}>
          {promotionSaving ? 'Queuing...' : `Queue selected (${selectedPromotionCount})`}
        </button>
      </header>
      {#if promotionError}
        <div class="error-banner">{promotionError}</div>
      {/if}
      {#if promotionSuccess}
        <div class="success-banner">
          <span>{promotionSuccess}</span>
          <button type="button" class="btn btn-ghost btn-sm" onclick={() => onNavigate('/console/memory?tab=inbox')}>Open Memory Inbox</button>
        </div>
      {/if}
      {#if promotionLoading}
        <div class="lineage-empty">Loading fork insights...</div>
      {:else if promotionCandidates.length === 0}
        <div class="lineage-empty">No reviewable fork insights.</div>
      {:else}
        <div class="promotion-list">
          {#each promotionCandidates as candidate}
            <label class="promotion-candidate">
              <input
                type="checkbox"
                checked={isPromotionSelected(candidate.id)}
                onchange={() => togglePromotionCandidate(candidate.id)}
              />
              <span class="promotion-copy">
                <span>
                  <strong>{candidate.category}</strong>
                  <small>{candidate.role} · message {candidate.message_index + 1}</small>
                </span>
                <span>{candidate.summary}</span>
              </span>
            </label>
          {/each}
        </div>
      {/if}
    </section>
  {/if}
</section>

<style>
  .lineage-page {
    display: grid;
    gap: var(--space-4);
    padding: var(--space-5);
    max-width: 1120px;
    margin: 0 auto;
  }

  .lineage-header {
    display: flex;
    justify-content: space-between;
    gap: var(--space-4);
    align-items: flex-start;
  }

  .lineage-header h2 {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--text-2xl);
    font-weight: 500;
    color: var(--text-primary);
  }

  .lineage-header p {
    margin: var(--space-1) 0 0;
    color: var(--text-secondary);
    font-size: var(--text-sm);
  }

  .lineage-stats {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--surface);
    overflow: hidden;
  }

  .lineage-stats div {
    display: grid;
    gap: 2px;
    padding: var(--space-3) var(--space-4);
    border-right: 1px solid var(--border-subtle);
  }

  .lineage-stats div:last-child {
    border-right: 0;
  }

  .lineage-stats span {
    font-family: var(--font-display);
    color: var(--text-primary);
    font-size: var(--text-xl);
  }

  .lineage-stats small {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
    text-transform: uppercase;
  }

  .lineage-graph {
    display: grid;
    gap: var(--space-2);
  }

  .lineage-row {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    width: 100%;
    min-height: 96px;
    padding: 0;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--surface);
    color: inherit;
    text-align: left;
    transition: border-color var(--duration-fast), background var(--duration-fast);
    overflow: hidden;
  }

  .lineage-row:hover {
    border-color: var(--border-strong);
    background: var(--surface-elevated);
  }

  .lineage-main {
    display: grid;
    grid-template-columns: calc(var(--depth) * 28px + 44px) minmax(0, 1fr);
    min-height: 96px;
    width: 100%;
    padding: 0;
    border: 0;
    background: transparent;
    color: inherit;
    text-align: left;
    cursor: pointer;
  }

  .lineage-actions {
    display: flex;
    align-items: center;
    padding: var(--space-3);
    border-left: 1px solid var(--border-subtle);
    background: var(--surface-inset);
  }

  .lineage-actions .btn {
    white-space: nowrap;
  }

  .lineage-actions .active-action {
    border-color: var(--primary);
    color: var(--primary-text);
  }

  .lineage-gutter {
    position: relative;
    display: flex;
    align-items: center;
    justify-content: flex-end;
    padding-right: var(--space-3);
    color: var(--primary-text);
    font-family: var(--font-mono);
  }

  .lineage-rail {
    position: absolute;
    top: 0;
    bottom: 0;
    right: 25px;
    width: 1px;
    background: var(--border-default);
  }

  .lineage-branch {
    position: relative;
    z-index: 1;
    padding: 2px 6px;
    border-radius: var(--radius-sm);
    background: var(--surface-inset);
  }

  .lineage-node {
    display: grid;
    gap: var(--space-2);
    padding: var(--space-3) var(--space-4);
    min-width: 0;
  }

  .lineage-node-top,
  .lineage-meta,
  .fork-preview {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-width: 0;
  }

  .lineage-node-top strong {
    color: var(--text-primary);
    font-family: var(--font-display);
    font-weight: 500;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .lineage-meta {
    color: var(--text-tertiary);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    flex-wrap: wrap;
  }

  .fork-preview {
    padding: var(--space-2);
    border-radius: var(--radius-sm);
    background: var(--surface-inset);
    color: var(--text-secondary);
    font-size: var(--text-sm);
  }

  .fork-preview > span:first-child {
    color: var(--primary-text);
    font-size: var(--text-xs);
    text-transform: uppercase;
  }

  .fork-preview strong {
    color: var(--text-primary);
    font-size: var(--text-xs);
    text-transform: uppercase;
  }

  .fork-preview span:last-child {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .fork-preview.muted {
    color: var(--text-tertiary);
  }

  .lineage-empty {
    padding: var(--space-6);
    border: 1px dashed var(--border-default);
    border-radius: var(--radius-md);
    color: var(--text-secondary);
    text-align: center;
  }

  .promotion-panel {
    display: grid;
    gap: var(--space-3);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--surface);
    padding: var(--space-4);
  }

  .promotion-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-3);
  }

  .promotion-header h3 {
    margin: 2px 0 0;
    font-size: var(--text-lg);
    font-weight: 500;
    color: var(--text-primary);
  }

  .panel-label {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
    text-transform: uppercase;
  }

  .success-banner {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    padding: var(--space-3);
    border: 1px solid color-mix(in srgb, var(--success) 42%, transparent);
    border-radius: var(--radius-sm);
    background: var(--success-muted);
    color: var(--success);
  }

  .promotion-list {
    display: grid;
    gap: var(--space-2);
  }

  .promotion-candidate {
    display: grid;
    grid-template-columns: 18px minmax(0, 1fr);
    gap: var(--space-3);
    align-items: flex-start;
    padding: var(--space-3);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface-inset);
  }

  .promotion-candidate input {
    margin-top: 3px;
  }

  .promotion-copy {
    display: grid;
    gap: var(--space-1);
    min-width: 0;
  }

  .promotion-copy > span:first-child {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex-wrap: wrap;
  }

  .promotion-copy strong {
    color: var(--text-primary);
    font-size: var(--text-xs);
    text-transform: uppercase;
  }

  .promotion-copy small {
    color: var(--text-tertiary);
  }

  .promotion-copy > span:last-child {
    color: var(--text-secondary);
    line-height: 1.45;
    overflow-wrap: anywhere;
  }

  @media (max-width: 760px) {
    .lineage-page {
      padding: var(--space-3);
    }

    .lineage-header {
      display: grid;
    }

    .lineage-stats {
      grid-template-columns: 1fr;
    }

    .lineage-stats div {
      border-right: 0;
      border-bottom: 1px solid var(--border-subtle);
    }

    .lineage-stats div:last-child {
      border-bottom: 0;
    }

    .lineage-row {
      grid-template-columns: minmax(0, 1fr);
    }

    .lineage-main {
      grid-template-columns: 34px minmax(0, 1fr);
    }

    .lineage-actions {
      border-left: 0;
      border-top: 1px solid var(--border-subtle);
      justify-content: flex-end;
    }

    .promotion-header,
    .success-banner {
      display: grid;
    }
  }
</style>
