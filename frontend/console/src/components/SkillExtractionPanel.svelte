<script lang="ts">
  import { onMount } from 'svelte'
  import {
    extractSkillsFromSession,
    listSessionLocalSkills,
    listSkillExtractions,
    promoteSessionLocalSkills,
    reviewSkillExtractionCandidate,
  } from '../lib/api'
  import type {
    SessionLocalSkillItem,
    SessionLocalSkillPromoteConflict,
    SessionLocalSkillPromoteMode,
    SkillExtractionCandidate,
    SkillExtractionCandidateAction,
  } from '../lib/types'

  interface Props {
    sessionId: string
    onClose?: () => void
    onApproved?: (path: string) => void
  }

  let { sessionId, onClose, onApproved }: Props = $props()

  type InboxTab = 'extracted' | 'session'

  let activeTab: InboxTab = $state('extracted')
  let candidates: SkillExtractionCandidate[] = $state([])
  let localSkills: SessionLocalSkillItem[] = $state([])
  let cwd = $state('')
  let loading = $state(false)
  let loadingLocal = $state(false)
  let extracting = $state(false)
  let reviewing = $state('')
  let promoting = $state(false)
  let error = $state('')
  let success = $state('')

  let mode: SessionLocalSkillPromoteMode = $state('copy')
  let selected: Set<string> = $state(new Set())

  let conflictDialogOpen = $state(false)
  let conflictNames: string[] = $state([])
  let conflictChoice: SessionLocalSkillPromoteConflict = $state('rename')
  let pendingPromoteNames: string[] = $state([])

  let pendingExtracted = $derived(candidates.filter((candidate) => candidate.status === 'pending').length)
  let pendingLocal = $derived(localSkills.filter((item) => item.kind === 'skill').length)

  function fmtDate(value?: string): string {
    if (!value) return ''
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return value
    return date.toLocaleString()
  }

  function tools(candidate: SkillExtractionCandidate): string {
    return (candidate.recommended_tools ?? []).join(', ')
  }

  async function loadExtracted() {
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

  async function loadLocal() {
    if (!sessionId) {
      localSkills = []
      cwd = ''
      return
    }
    loadingLocal = true
    error = ''
    try {
      const res = await listSessionLocalSkills(sessionId)
      localSkills = res.items ?? []
      cwd = res.cwd ?? ''
      // Drop any stale selections that no longer exist.
      const live = new Set(localSkills.filter((s) => s.kind === 'skill').map((s) => s.name))
      selected = new Set([...selected].filter((name) => live.has(name)))
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load session-local skills'
    } finally {
      loadingLocal = false
    }
  }

  async function reload() {
    if (activeTab === 'extracted') {
      await loadExtracted()
    } else {
      await loadLocal()
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
      await loadExtracted()
    } catch (err) {
      error = err instanceof Error ? err.message : 'Skill candidate review failed'
    } finally {
      reviewing = ''
    }
  }

  function setTab(next: InboxTab) {
    activeTab = next
    error = ''
    success = ''
    if (next === 'session' && localSkills.length === 0 && !loadingLocal) {
      void loadLocal()
    }
  }

  function toggleSelect(name: string) {
    const next = new Set(selected)
    if (next.has(name)) next.delete(name)
    else next.add(name)
    selected = next
  }

  function selectAll() {
    selected = new Set(localSkills.filter((item) => item.kind === 'skill').map((item) => item.name))
  }

  function clearSelection() {
    selected = new Set()
  }

  function attemptPromote(names: string[]) {
    if (names.length === 0) return
    const collisionSet = new Set(
      localSkills.filter((item) => item.has_workspace_collision).map((item) => item.name),
    )
    const collisions = names.filter((name) => collisionSet.has(name))
    if (collisions.length > 0) {
      pendingPromoteNames = names
      conflictNames = collisions
      conflictChoice = 'rename'
      conflictDialogOpen = true
      return
    }
    void doPromote(names, 'rename')
  }

  function confirmConflictDialog() {
    if (conflictChoice === 'abort') {
      conflictDialogOpen = false
      pendingPromoteNames = []
      conflictNames = []
      return
    }
    const names = pendingPromoteNames
    const choice = conflictChoice
    conflictDialogOpen = false
    pendingPromoteNames = []
    conflictNames = []
    void doPromote(names, choice)
  }

  function cancelConflictDialog() {
    conflictDialogOpen = false
    pendingPromoteNames = []
    conflictNames = []
  }

  async function doPromote(names: string[], onConflict: SessionLocalSkillPromoteConflict) {
    if (!sessionId || promoting || names.length === 0) return
    promoting = true
    error = ''
    success = ''
    try {
      const res = await promoteSessionLocalSkills(sessionId, {
        items: names.map((name) => ({ name })),
        mode,
        on_conflict: onConflict,
      })
      const promotedCount = res.promoted?.length ?? 0
      const failedCount = res.failed?.length ?? 0
      if (promotedCount > 0 && failedCount === 0) {
        success = `Promoted ${promotedCount} skill${promotedCount === 1 ? '' : 's'} to shared workspace`
      } else if (promotedCount > 0 && failedCount > 0) {
        success = `Promoted ${promotedCount}; ${failedCount} failed`
      } else if (failedCount > 0) {
        error = res.failed[0]?.error || 'Promotion failed'
      }
      selected = new Set()
      await loadLocal()
    } catch (err) {
      error = err instanceof Error ? err.message : 'Promote failed'
    } finally {
      promoting = false
    }
  }

  function promoteSelected() {
    attemptPromote([...selected])
  }

  function promoteOne(name: string) {
    attemptPromote([name])
  }

  onMount(() => {
    void loadExtracted()
    if (sessionId) void loadLocal()
  })
</script>

<div class="skill-extraction-panel">
  <header class="panel-header">
    <div>
      <strong>Skill Inbox</strong>
      <span>
        {activeTab === 'extracted' ? `${pendingExtracted} pending` : `${pendingLocal} session skill${pendingLocal === 1 ? '' : 's'}`}
      </span>
    </div>
    <button class="btn btn-ghost btn-sm" type="button" onclick={() => onClose?.()}>Close</button>
  </header>

  <nav class="tab-bar" aria-label="Skill inbox tabs">
    <button
      class="tab"
      class:tab-active={activeTab === 'extracted'}
      type="button"
      onclick={() => setTab('extracted')}
    >
      Extracted
      <span class="tab-count">{pendingExtracted}</span>
    </button>
    <button
      class="tab"
      class:tab-active={activeTab === 'session'}
      type="button"
      onclick={() => setTab('session')}
    >
      Session
      <span class="tab-count">{pendingLocal}</span>
    </button>
  </nav>

  {#if error}
    <div class="message message-error">{error}</div>
  {/if}
  {#if success}
    <div class="message message-success">{success}</div>
  {/if}

  {#if activeTab === 'extracted'}
    <div class="panel-actions">
      <button class="btn btn-primary btn-sm" type="button" disabled={extracting || !sessionId} onclick={extract}>
        {extracting ? 'Extracting...' : 'Extract from session'}
      </button>
      <button class="btn btn-ghost btn-sm" type="button" disabled={loading} onclick={loadExtracted}>
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
          <article
            class="candidate-card"
            class:approved={candidate.status === 'approved'}
            class:rejected={candidate.status === 'rejected'}
          >
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
  {:else}
    <div class="panel-actions session-actions">
      <button class="btn btn-ghost btn-sm" type="button" disabled={loadingLocal} onclick={loadLocal}>
        {loadingLocal ? 'Loading...' : 'Reload'}
      </button>
      <div class="mode-toggle" role="group" aria-label="Promote mode">
        <label>
          <input type="radio" name="promote-mode" value="copy" checked={mode === 'copy'} onchange={() => (mode = 'copy')} />
          Copy
        </label>
        <label>
          <input type="radio" name="promote-mode" value="move" checked={mode === 'move'} onchange={() => (mode = 'move')} />
          Move (delete local)
        </label>
      </div>
      <div class="bulk-actions">
        <button class="btn btn-ghost btn-sm" type="button" onclick={selectAll} disabled={pendingLocal === 0}>Select all</button>
        <button class="btn btn-ghost btn-sm" type="button" onclick={clearSelection} disabled={selected.size === 0}>Clear</button>
        <button
          class="btn btn-primary btn-sm"
          type="button"
          onclick={promoteSelected}
          disabled={promoting || selected.size === 0}
        >
          {promoting ? 'Promoting...' : `Promote selected (${selected.size})`}
        </button>
      </div>
    </div>

    {#if cwd}
      <div class="cwd-strip" title={cwd}>cwd: <code>{cwd}</code></div>
    {/if}

    {#if loadingLocal && localSkills.length === 0}
      <div class="empty-state">Loading session skills...</div>
    {:else if localSkills.length === 0}
      <div class="empty-state">No session-local skills found under <code>.tars/skills/</code>.</div>
    {:else}
      <div class="candidate-list">
        {#each localSkills as item (item.kind + ':' + item.name)}
          <article class="candidate-card session-card" class:command={item.kind === 'command'}>
            <div class="candidate-main">
              <div class="session-header">
                {#if item.kind === 'skill'}
                  <input
                    type="checkbox"
                    aria-label={`Select ${item.name}`}
                    checked={selected.has(item.name)}
                    onchange={() => toggleSelect(item.name)}
                  />
                {/if}
                <strong>{item.name}</strong>
                {#if item.slash}<span class="candidate-name">/{item.slash}</span>{/if}
              </div>
              <div class="badges">
                <span class="badge {item.kind === 'skill' ? 'badge-default' : 'badge-soft'}">{item.kind}</span>
                {#if item.has_workspace_collision}
                  <span class="badge badge-warn" title="A workspace skill with this name already exists">collision</span>
                {/if}
              </div>
            </div>
            {#if item.description}<p>{item.description}</p>{/if}
            <div class="candidate-meta">
              <span class="draft-path">{item.file_path}</span>
            </div>
            {#if item.kind === 'skill'}
              <div class="candidate-actions">
                <button
                  class="btn btn-primary btn-sm"
                  type="button"
                  disabled={promoting}
                  onclick={() => promoteOne(item.name)}
                >
                  Promote
                </button>
              </div>
            {/if}
          </article>
        {/each}
      </div>
    {/if}
  {/if}

  {#if conflictDialogOpen}
    <div class="modal-backdrop" role="presentation" onclick={cancelConflictDialog}></div>
    <div class="modal-card" role="dialog" aria-modal="true" aria-labelledby="conflict-title">
      <h3 id="conflict-title">Workspace skill already exists</h3>
      <p>
        {conflictNames.length} of the selected skills collide with workspace skills of the same name. Choose how to apply this batch:
      </p>
      <ul class="conflict-list">
        {#each conflictNames as name}
          <li><code>{name}</code></li>
        {/each}
      </ul>
      <div class="conflict-options" role="radiogroup" aria-label="Conflict resolution">
        <label>
          <input type="radio" name="conflict" value="rename" checked={conflictChoice === 'rename'} onchange={() => (conflictChoice = 'rename')} />
          Rename (auto-suffix)
        </label>
        <label>
          <input type="radio" name="conflict" value="overwrite" checked={conflictChoice === 'overwrite'} onchange={() => (conflictChoice = 'overwrite')} />
          Overwrite existing
        </label>
        <label>
          <input type="radio" name="conflict" value="abort" checked={conflictChoice === 'abort'} onchange={() => (conflictChoice = 'abort')} />
          Cancel batch
        </label>
      </div>
      <p class="apply-to-all">Choice applies to all colliding items in this batch.</p>
      <div class="modal-actions">
        <button class="btn btn-ghost btn-sm" type="button" onclick={cancelConflictDialog}>Cancel</button>
        <button class="btn btn-primary btn-sm" type="button" onclick={confirmConflictDialog}>
          {conflictChoice === 'abort' ? 'Abort' : 'Apply'}
        </button>
      </div>
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
    position: relative;
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

  .tab-bar {
    display: flex;
    gap: var(--space-1);
    border-bottom: 1px solid var(--border-subtle);
  }

  .tab {
    background: transparent;
    border: 0;
    padding: var(--space-2) var(--space-3);
    color: var(--text-secondary);
    font: inherit;
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    border-bottom: 2px solid transparent;
    cursor: pointer;
  }

  .tab:hover {
    color: var(--text-primary);
  }

  .tab-active {
    color: var(--text-primary);
    border-bottom-color: var(--accent-amber);
  }

  .tab-count {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
    background: var(--surface-inset);
    border-radius: 999px;
    padding: 0 var(--space-2);
    min-width: 22px;
    text-align: center;
  }

  .session-actions {
    justify-content: space-between;
  }

  .mode-toggle,
  .bulk-actions {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
  }

  .mode-toggle label {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    font-size: var(--text-xs);
    color: var(--text-secondary);
  }

  .cwd-strip {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
  }

  .cwd-strip code,
  .conflict-list code {
    font-family: var(--font-mono);
    color: var(--text-secondary);
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

  .session-card.command {
    opacity: 0.85;
  }

  .session-header {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    min-width: 0;
  }

  .badges {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
  }

  .badge-warn {
    border-color: rgba(224, 145, 69, 0.4);
    background: rgba(224, 145, 69, 0.12);
    color: var(--accent-amber, #e09145);
  }

  .badge-soft {
    background: var(--surface-inset);
    color: var(--text-tertiary);
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

  .modal-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.5);
    z-index: 50;
  }

  .modal-card {
    position: fixed;
    top: 50%;
    left: 50%;
    transform: translate(-50%, -50%);
    background: var(--surface-raised);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    padding: var(--space-4);
    width: min(420px, 90vw);
    z-index: 51;
    display: grid;
    gap: var(--space-3);
  }

  .modal-card h3 {
    margin: 0;
    font-size: var(--text-md);
  }

  .modal-card p {
    margin: 0;
    color: var(--text-secondary);
    font-size: var(--text-sm);
  }

  .conflict-list {
    margin: 0;
    padding: 0 0 0 var(--space-4);
    color: var(--text-secondary);
    font-size: var(--text-xs);
    max-height: 120px;
    overflow: auto;
  }

  .conflict-options {
    display: grid;
    gap: var(--space-1);
  }

  .conflict-options label {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    font-size: var(--text-sm);
    color: var(--text-primary);
  }

  .apply-to-all {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
  }

  .modal-actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
  }
</style>
