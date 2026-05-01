<script lang="ts">
  import { onMount } from 'svelte'
  import {
    extractSkillsFromSession,
    listSkillExtractions,
    reviewSkillExtractionCandidate,
  } from '../lib/api'
  import type { SkillExtractionCandidate, SkillExtractionCandidateAction } from '../lib/types'

  interface Props {
    sessionId: string
    onClose?: () => void
    onApproved?: (path: string) => void
  }

  let { sessionId, onClose, onApproved }: Props = $props()

  let candidates: SkillExtractionCandidate[] = $state([])
  let loading = $state(false)
  let extracting = $state(false)
  let reviewing = $state('')
  let error = $state('')
  let success = $state('')

  let pendingCount = $derived(candidates.filter((candidate) => candidate.status === 'pending').length)

  function fmtDate(value?: string): string {
    if (!value) return ''
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return value
    return date.toLocaleString()
  }

  function tools(candidate: SkillExtractionCandidate): string {
    return (candidate.recommended_tools ?? []).join(', ')
  }

  async function load() {
    loading = true
    error = ''
    try {
      const res = await listSkillExtractions('all')
      candidates = res.candidates ?? []
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load skill extraction inbox'
    } finally {
      loading = false
    }
  }

  async function extract() {
    if (!sessionId || extracting) return
    extracting = true
    error = ''
    success = ''
    try {
      const res = await extractSkillsFromSession(sessionId, 5)
      candidates = res.candidates ?? []
      success = res.count > 0 ? `Queued ${res.count} candidate${res.count === 1 ? '' : 's'}` : 'No reusable skill candidates found'
    } catch (err) {
      error = err instanceof Error ? err.message : 'Skill extraction failed'
    } finally {
      extracting = false
    }
  }

  async function review(candidate: SkillExtractionCandidate, action: SkillExtractionCandidateAction) {
    if (!candidate.id || reviewing) return
    reviewing = candidate.id
    error = ''
    success = ''
    try {
      const res = await reviewSkillExtractionCandidate(candidate.id, action)
      if (action === 'approve') {
        success = `Saved skill draft: ${res.saved?.path ?? res.candidate.draft_path ?? res.candidate.name}`
        onApproved?.(res.saved?.path ?? res.candidate.draft_path ?? '')
      } else {
        success = `Rejected ${candidate.name}`
      }
      await load()
    } catch (err) {
      error = err instanceof Error ? err.message : 'Skill candidate review failed'
    } finally {
      reviewing = ''
    }
  }

  onMount(() => {
    void load()
  })
</script>

<div class="skill-extraction-panel">
  <header class="panel-header">
    <div>
      <strong>Skill Extraction Inbox</strong>
      <span>{pendingCount} pending</span>
    </div>
    <button class="btn btn-ghost btn-sm" type="button" onclick={() => onClose?.()}>Close</button>
  </header>

  {#if error}
    <div class="message message-error">{error}</div>
  {/if}
  {#if success}
    <div class="message message-success">{success}</div>
  {/if}

  <div class="panel-actions">
    <button class="btn btn-primary btn-sm" type="button" disabled={extracting || !sessionId} onclick={extract}>
      {extracting ? 'Extracting...' : 'Extract from session'}
    </button>
    <button class="btn btn-ghost btn-sm" type="button" disabled={loading} onclick={load}>
      {loading ? 'Loading...' : 'Reload'}
    </button>
  </div>

  {#if loading && candidates.length === 0}
    <div class="empty-state">Loading candidates...</div>
  {:else if candidates.length === 0}
    <div class="empty-state">No skill candidates yet.</div>
  {:else}
    <div class="candidate-list">
      {#each candidates as candidate}
        <article class="candidate-card" class:approved={candidate.status === 'approved'} class:rejected={candidate.status === 'rejected'}>
          <div class="candidate-main">
            <div>
              <strong>{candidate.title || candidate.name}</strong>
              <span class="candidate-name">{candidate.name}</span>
            </div>
            <span class="badge {candidate.status === 'approved' ? 'badge-success' : candidate.status === 'rejected' ? 'badge-error' : 'badge-default'}">{candidate.status}</span>
          </div>
          <p>{candidate.summary}</p>
          <div class="candidate-meta">
            {#if candidate.trigger}<span>{candidate.trigger}</span>{/if}
            {#if tools(candidate)}<span>tools: {tools(candidate)}</span>{/if}
            {#if candidate.repeated_count}<span>{candidate.repeated_count} evidence</span>{/if}
            {#if candidate.message_range}<span>{candidate.message_range}</span>{/if}
            {#if candidate.updated_at}<span>{fmtDate(candidate.updated_at)}</span>{/if}
          </div>
          {#if candidate.evidence?.length}
            <details class="evidence">
              <summary>Evidence</summary>
              <div class="evidence-list">
                {#each candidate.evidence as evidence}
                  <div class="evidence-row">
                    <span>{evidence.role}</span>
                    <p>{evidence.snippet}</p>
                  </div>
                {/each}
              </div>
            </details>
          {/if}
          {#if candidate.draft_path}
            <div class="draft-path">{candidate.draft_path}</div>
          {/if}
          {#if candidate.status === 'pending'}
            <div class="candidate-actions">
              <button class="btn btn-primary btn-sm" type="button" disabled={reviewing === candidate.id} onclick={() => review(candidate, 'approve')}>
                {reviewing === candidate.id ? 'Saving...' : 'Approve draft'}
              </button>
              <button class="btn btn-ghost btn-sm" type="button" disabled={reviewing === candidate.id} onclick={() => review(candidate, 'reject')}>Reject</button>
            </div>
          {/if}
        </article>
      {/each}
    </div>
  {/if}
</div>

<style>
  .skill-extraction-panel {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    height: 100%;
    padding: var(--space-3);
    overflow: auto;
  }

  .panel-header,
  .panel-actions,
  .candidate-main,
  .candidate-actions,
  .candidate-meta {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex-wrap: wrap;
  }

  .panel-header {
    justify-content: space-between;
    border-bottom: 1px solid var(--border-subtle);
    padding-bottom: var(--space-2);
  }

  .panel-header > div {
    display: grid;
    gap: 2px;
  }

  .panel-header span,
  .candidate-name,
  .candidate-meta,
  .draft-path {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
  }

  .message {
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-sm);
    font-size: var(--text-xs);
  }

  .message-error {
    border: 1px solid rgba(239, 68, 68, 0.25);
    background: rgba(239, 68, 68, 0.08);
    color: var(--error);
  }

  .message-success {
    border: 1px solid rgba(34, 197, 94, 0.25);
    background: rgba(34, 197, 94, 0.08);
    color: var(--green);
  }

  .candidate-list {
    display: grid;
    gap: var(--space-3);
  }

  .candidate-card {
    display: grid;
    gap: var(--space-2);
    padding: var(--space-3);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface-raised);
  }

  .candidate-card.approved {
    border-color: rgba(34, 197, 94, 0.28);
  }

  .candidate-card.rejected {
    opacity: 0.72;
  }

  .candidate-main {
    justify-content: space-between;
  }

  .candidate-main > div {
    display: grid;
    gap: 2px;
    min-width: 0;
  }

  .candidate-card p {
    margin: 0;
    color: var(--text-secondary);
    font-size: var(--text-sm);
    line-height: 1.45;
  }

  .evidence {
    color: var(--text-secondary);
    font-size: var(--text-xs);
  }

  .evidence-list {
    display: grid;
    gap: var(--space-2);
    margin-top: var(--space-2);
  }

  .evidence-row {
    display: grid;
    gap: 2px;
    padding: var(--space-2);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface-inset);
  }

  .evidence-row span {
    color: var(--text-tertiary);
    font-family: var(--font-mono);
  }

  .draft-path {
    font-family: var(--font-mono);
    overflow-wrap: anywhere;
  }
</style>
