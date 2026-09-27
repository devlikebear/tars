<script lang="ts">
  // Changes dock panel (#969): the session's turns that changed files, and
  // one turn's diff, alone or with every turn before it. Read-only for now;
  // reverting arrives with the revert API.
  import { untrack } from 'svelte'
  import { t } from '../i18n'
  import type { CheckpointDiff, CheckpointEntry } from '../lib/api/checkpoints'
  import { parseUnifiedDiff } from '../lib/diff'
  import { changes } from '../lib/stores/changesStore'
  import type { ChangesScope } from '../lib/stores/changes.svelte'
  import DiffView from './DiffView.svelte'

  interface Props {
    sessionId: string
    onClose?: () => void
  }

  let { sessionId }: Props = $props()

  let listed = $derived(changes.turns.filter((turn) => turn.files > 0 || !!turn.skipped).reverse())
  let selected = $derived(changes.selectedTurn)
  let diff = $state<CheckpointDiff | null>(null)
  let diffKey = $state('')
  let diffLoading = $state(false)
  let diffFailed = $state(false)
  let selectedPath = $state('')
  let diffMode = $state<'unified' | 'split'>('unified')

  let selectedFile = $derived(diff?.files.find((file) => file.path === selectedPath) ?? diff?.files[0])
  let selectedLines = $derived(parseUnifiedDiff(selectedFile?.patch))

  $effect(() => {
    const id = sessionId
    untrack(() => void changes.load(id))
  })

  // Load the selected turn's diff in the chosen scope.
  $effect(() => {
    void changes.version
    const turn = selected
    const scope = changes.scope
    if (!turn || turn.skipped || turn.files === 0) {
      diff = null
      diffKey = ''
      return
    }
    const key = `${turn.turn_id}:${scope}`
    let current = true
    diffLoading = true
    diffFailed = false
    changes.diff(turn.turn_id, scope)
      .then((result) => {
        if (!current) return
        if (key !== diffKey || !result.files.some((file) => file.path === selectedPath)) {
          selectedPath = result.files[0]?.path ?? ''
        }
        diff = result
        diffKey = key
      })
      .catch(() => { if (current) diffFailed = true })
      .finally(() => { if (current) diffLoading = false })
    return () => { current = false }
  })

  function setScope(scope: ChangesScope) {
    changes.setScope(scope)
  }

  function skipText(reason: string): string {
    return $t.changes.skipped[reason] ?? $t.changes.skippedOther(reason)
  }

  function statusText(status: string): string {
    return $t.changes.fileStatus[status] ?? status
  }

  function turnTime(turn: CheckpointEntry): string {
    const at = new Date(turn.ended_at || turn.started_at)
    return Number.isNaN(at.getTime()) ? '' : at.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  }
</script>

<div class="changes-panel">
  <div class="changes-header">
    <span class="section-title">{$t.changes.heading}</span>
    <div class="changes-actions">
      <div class="seg" role="group" aria-label={$t.changes.scopeLabel}>
        <button type="button" class="btn btn-ghost btn-sm" class:active={changes.scope === 'turn'} aria-pressed={changes.scope === 'turn'} onclick={() => setScope('turn')}>{$t.changes.scope.turn}</button>
        <button type="button" class="btn btn-ghost btn-sm" class:active={changes.scope === 'session'} aria-pressed={changes.scope === 'session'} onclick={() => setScope('session')}>{$t.changes.scope.session}</button>
      </div>
      <button type="button" class="btn btn-ghost btn-sm" disabled={changes.loading} onclick={() => void changes.refresh()}>{$t.changes.refresh}</button>
    </div>
  </div>

  {#if changes.error}
    <div class="error-banner">{$t.changes.loadFailed}</div>
  {/if}

  {#if changes.loading && changes.turns.length === 0}
    <div class="empty-state compact">{$t.changes.loading}</div>
  {:else if listed.length === 0}
    <div class="empty-state compact">
      <p>{$t.changes.empty}</p>
      <p>{$t.changes.emptyHint}</p>
    </div>
  {:else}
    <div class="turn-list" role="listbox" aria-label={$t.changes.turnsLabel}>
      {#each listed as turn (turn.turn_id)}
        <button
          type="button"
          role="option"
          class="turn-row"
          class:active={selected?.turn_id === turn.turn_id}
          aria-selected={selected?.turn_id === turn.turn_id}
          onclick={() => changes.select(turn.turn_id)}
        >
          <span class="turn-preview" data-content>{turn.preview || $t.changes.untitledTurn}</span>
          <span class="turn-meta">
            {#if turn.skipped}
              <span class="turn-skipped">{skipText(turn.skipped)}</span>
            {:else}
              <span class="turn-counts">{$t.changes.summary(turn.files, turn.additions, turn.deletions)}</span>
            {/if}
            <span class="turn-time">{turnTime(turn)}</span>
          </span>
        </button>
      {/each}
    </div>

    {#if selected?.skipped}
      <div class="empty-state compact">{skipText(selected.skipped)}{selected.skip_detail ? ` — ${selected.skip_detail}` : ''}</div>
    {:else if diffLoading && !diff}
      <div class="empty-state compact">{$t.changes.diff.loading}</div>
    {:else if diffFailed}
      <div class="error-banner">{$t.changes.loadFailed}</div>
    {:else if diff}
      {#if diff.root}
        <div class="changes-root"><span class="section-title">{$t.changes.folder}</span> <code title={diff.root}>{diff.root}</code></div>
      {/if}
      <div class="file-list" role="listbox" aria-label={$t.changes.filesLabel}>
        {#each diff.files as file (file.path)}
          <button
            type="button"
            role="option"
            class="file-row"
            class:active={selectedFile?.path === file.path}
            aria-selected={selectedFile?.path === file.path}
            onclick={() => (selectedPath = file.path)}
          >
            <code class="file-path" title={file.path}>{file.path}</code>
            <span class="file-status">{statusText(file.status)}</span>
            <span class="file-counts"><span class="plus">+{file.additions}</span> <span class="minus">−{file.deletions}</span></span>
          </button>
        {/each}
      </div>
      {#if diff.unknown?.length}
        <div class="changes-note">{$t.changes.unknownPaths(diff.unknown.length)}</div>
      {/if}

      {#if selectedFile}
        <div class="diff-section">
          <div class="diff-head">
            <div class="diff-head-title">
              <code title={selectedFile.path}>{selectedFile.path}</code>
              {#if selectedFile.old_path}
                <span class="changes-note" data-content>{$t.changes.renamedFrom(selectedFile.old_path)}</span>
              {/if}
            </div>
            <div class="seg" role="group" aria-label={$t.changes.diff.layoutLabel}>
              <button type="button" class="btn btn-ghost btn-sm" class:active={diffMode === 'unified'} onclick={() => (diffMode = 'unified')}>{$t.changes.diff.unified}</button>
              <button type="button" class="btn btn-ghost btn-sm" class:active={diffMode === 'split'} onclick={() => (diffMode = 'split')}>{$t.changes.diff.split}</button>
            </div>
          </div>
          {#if selectedFile.binary}
            <div class="empty-state compact">{$t.changes.binary}</div>
          {:else if selectedLines.length === 0}
            <div class="empty-state compact">{$t.changes.noTextChanges}</div>
          {:else}
            <DiffView lines={selectedLines} mode={diffMode} label={$t.changes.diff.sideBySideLabel} />
            {#if selectedFile.truncated}
              <div class="changes-note">{$t.changes.truncated}</div>
            {/if}
          {/if}
        </div>
      {:else}
        <div class="empty-state compact">{$t.changes.diff.selectFile}</div>
      {/if}
    {/if}
  {/if}
</div>

<style>
  .changes-panel {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    min-width: 0;
    min-height: 0;
    height: 100%;
    overflow-y: auto;
  }

  .changes-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
    flex-wrap: wrap;
  }

  .changes-actions {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .section-title {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }

  .seg {
    display: inline-flex;
    gap: 2px;
  }

  .seg .active {
    color: var(--primary-text);
    border-color: var(--primary);
  }

  .turn-list,
  .file-list {
    display: grid;
    gap: var(--space-1);
    max-height: 220px;
    overflow-y: auto;
    padding-right: 2px;
    flex: 0 0 auto;
  }

  .turn-row,
  .file-row {
    display: grid;
    gap: 2px;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface);
    color: inherit;
    padding: var(--space-2);
    min-width: 0;
    text-align: left;
    cursor: pointer;
  }

  .turn-row:hover,
  .file-row:hover {
    background: var(--surface-hover);
  }

  .turn-row.active,
  .file-row.active {
    border-color: var(--primary);
    background: var(--surface-elevated);
  }

  .turn-preview {
    color: var(--text-primary);
    font-size: var(--text-sm);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .turn-meta {
    display: flex;
    justify-content: space-between;
    gap: var(--space-2);
    color: var(--text-tertiary);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
  }

  .turn-skipped {
    color: var(--warning);
  }

  .file-row {
    grid-template-columns: minmax(0, 1fr) auto auto;
    align-items: baseline;
    gap: var(--space-2);
  }

  .file-path {
    color: var(--text-primary);
    font-size: var(--text-xs);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .file-status {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
  }

  .file-counts {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    white-space: nowrap;
  }

  .plus {
    color: var(--success);
  }

  .minus {
    color: var(--error);
  }

  .changes-root {
    display: flex;
    align-items: baseline;
    gap: var(--space-2);
    min-width: 0;
    font-size: var(--text-xs);
  }

  .changes-root code {
    color: var(--text-secondary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .changes-note {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
  }

  .diff-section {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    min-width: 0;
  }

  .diff-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
    flex-wrap: wrap;
  }

  .diff-head-title {
    display: flex;
    align-items: baseline;
    gap: var(--space-2);
    min-width: 0;
  }

  .diff-head-title code {
    color: var(--text-primary);
    font-size: var(--text-sm);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
