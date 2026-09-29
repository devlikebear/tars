<script lang="ts">
  // Changes dock panel (#969): the session's turns that changed files, and
  // one turn's diff, alone or with every turn before it. A turn's hunks,
  // files, or whole turn can be reverted after a preview, and the last
  // revert undone.
  import { untrack } from 'svelte'
  import { t } from '../i18n'
  import type { CheckpointDiff, CheckpointEntry, RevertFile, RevertFileResult, RevertResult, RevertScope } from '../lib/api/checkpoints'
  import { chatSession } from '../lib/stores/chatSession'
  import type { PendingRevert } from '../lib/stores/changes.svelte'
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
  // The hunk or file a review note is being written for.
  let commentTarget = $state<{ turnId: string; path: string; hunkId?: string } | null>(null)
  let commentText = $state('')

  let selectedFile = $derived(diff?.files.find((file) => file.path === selectedPath) ?? diff?.files[0])
  let selectedLines = $derived(parseUnifiedDiff(selectedFile?.patch))
  // Reverts wait while a turn streams, or while another revert is in flight.
  let revertBusy = $derived(
    chatSession.streaming ||
      changes.pending?.stage === 'checking' ||
      changes.pending?.stage === 'applying' ||
      changes.last?.stage === 'undoing',
  )
  // Hunk numbers mean something only within one turn's own diff.
  let canRevertHunks = $derived(
    changes.scope === 'turn' &&
      selectedFile?.status === 'modified' &&
      !selectedFile.binary &&
      !selectedFile.truncated &&
      (selectedFile.hunks?.length ?? 0) > 0,
  )

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

  function revert(scope: RevertScope, files: RevertFile[] = []) {
    if (selected) void changes.requestRevert(selected.turn_id, scope, files)
  }

  function hunkId(index: number): string {
    return selectedFile?.hunks?.[index]?.id ?? `h${index}`
  }

  // Files the revert would write, merge, or delete.
  function touched(result: RevertResult): number {
    return result.files.filter((f) => f.outcome === 'write' || f.outcome === 'merge').length
  }

  function confirmText(pending: PendingRevert, result: RevertResult): string {
    const count = touched(result)
    if (pending.scope === 'since') return $t.changes.revert.confirmSince(count)
    const [only] = pending.files
    if (pending.files.length === 1 && only.hunk_ids?.length) return $t.changes.revert.confirmHunks(only.hunk_ids.length, only.path)
    if (pending.files.length === 1) return $t.changes.revert.confirmFile(only.path)
    return $t.changes.revert.confirmTurn(count)
  }

  function outcomeText(file: RevertFileResult): string {
    if (file.delete && file.outcome === 'write') return $t.changes.revert.delete
    return $t.changes.revert.outcome[file.outcome] ?? file.outcome
  }

  function errorText(code: string, message: string, failed: (message: string) => string): string {
    return code === 'turn_in_progress' ? $t.changes.revert.busy : failed(message)
  }

  function doneText(result: RevertResult): string {
    const count = touched(result)
    const head = count > 0 ? $t.changes.revert.done(count) : $t.changes.revert.doneNothing
    return result.failed > 0 ? `${head} ${$t.changes.revert.someFailed(result.failed)}` : head
  }

  function startComment(path: string, hunkId?: string) {
    if (!selected) return
    commentTarget = { turnId: selected.turn_id, path, hunkId }
    commentText = ''
  }

  function addComment() {
    const target = commentTarget
    const comment = commentText.trim()
    if (!target || !comment) return
    changes.addNote({ turn_id: target.turnId, path: target.path, hunk_id: target.hunkId, comment, kind: 'comment' })
    commentTarget = null
    commentText = ''
  }

  function noteWhere(path: string, hunkId?: string): string {
    return hunkId ? `${path} (${hunkId})` : path
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

  {#if changes.notes.length > 0}
    <div class="changes-note notes-pending">{$t.changes.notes.pending(changes.notes.length)}</div>
  {/if}

  {#if changes.pending}
    {@const pending = changes.pending}
    <div class="revert-bar" role="alertdialog" aria-label={$t.changes.revert.actionsLabel}>
      {#if pending.stage === 'checking'}
        <p>{$t.changes.revert.checking}</p>
      {:else if pending.stage === 'error'}
        <p class="revert-error">{errorText(pending.errorCode, pending.error, $t.changes.revert.failed)}</p>
      {:else if pending.result}
        {@const result = pending.result}
        {#if pending.stage === 'conflict'}
          <p class="revert-warn">{$t.changes.revert.conflictTitle(result.conflicts)}</p>
        {:else if touched(result) === 0}
          <p>{$t.changes.revert.nothingToDo}</p>
        {:else}
          <p>{confirmText(pending, result)}</p>
        {/if}
        <ul class="revert-files">
          {#each result.files as file (file.path)}
            <li>
              <code title={file.path}>{file.path}</code>
              <span class="revert-outcome outcome-{file.outcome}">{outcomeText(file)}</span>
              {#if file.merged}
                <details class="revert-merge">
                  <summary>{$t.changes.revert.showMerge}</summary>
                  <pre>{file.merged}</pre>
                </details>
              {/if}
            </li>
          {/each}
        </ul>
      {/if}
      <div class="revert-buttons">
        {#if pending.stage === 'conflict'}
          <button type="button" class="btn btn-danger btn-sm" onclick={() => void changes.confirmRevert(true)}>{$t.changes.revert.force}</button>
        {:else if (pending.stage === 'confirm' || pending.stage === 'applying') && pending.result && touched(pending.result) > 0}
          <button type="button" class="btn btn-primary btn-sm" disabled={pending.stage === 'applying'} onclick={() => void changes.confirmRevert()}>
            {pending.stage === 'applying' ? $t.changes.revert.applying : $t.changes.revert.apply}
          </button>
        {/if}
        <button type="button" class="btn btn-ghost btn-sm" disabled={pending.stage === 'applying'} onclick={() => changes.cancelRevert()}>{$t.changes.revert.cancel}</button>
      </div>
    </div>
  {:else if changes.last}
    {@const last = changes.last}
    <div class="revert-bar revert-done" role="status">
      {#if last.stage === 'undone'}
        <p>{$t.changes.revert.undone}</p>
      {:else if last.stage === 'undoConflict'}
        <p class="revert-warn">{$t.changes.revert.undoConflict}</p>
      {:else if last.stage === 'error'}
        <p class="revert-error">{errorText(last.errorCode, last.error, $t.changes.revert.undoFailed)}</p>
      {:else}
        <p>{doneText(last.result)}</p>
      {/if}
      <div class="revert-buttons">
        {#if last.revertId && (last.stage === 'done' || last.stage === 'undoing' || last.stage === 'error')}
          <button type="button" class="btn btn-secondary btn-sm" disabled={last.stage === 'undoing'} onclick={() => void changes.undo()}>
            {last.stage === 'undoing' ? $t.changes.revert.undoing : $t.changes.revert.undo}
          </button>
        {:else if last.stage === 'undoConflict'}
          <button type="button" class="btn btn-danger btn-sm" onclick={() => void changes.undo(true)}>{$t.changes.revert.undoForce}</button>
        {/if}
        <button type="button" class="btn btn-ghost btn-sm" onclick={() => changes.dismissLast()}>{$t.changes.revert.dismiss}</button>
      </div>
    </div>
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
      <div class="turn-actions" role="group" aria-label={$t.changes.revert.actionsLabel}>
        <button type="button" class="btn btn-secondary btn-sm" disabled={revertBusy} onclick={() => revert('turn')}>{$t.changes.revert.turn}</button>
        <button type="button" class="btn btn-ghost btn-sm" disabled={revertBusy} title={$t.changes.revert.sinceTitle} onclick={() => revert('since')}>{$t.changes.revert.since}</button>
      </div>
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
            <span class="file-status">
              {statusText(file.status)}
              {#if changes.scope === 'turn' && selected && changes.isReverted(selected.turn_id, file.path)}
                <span class="reverted-badge">{$t.changes.revert.reverted}</span>
              {/if}
            </span>
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
            <div class="diff-tools">
              {#if changes.scope === 'turn'}
                <button type="button" class="btn btn-ghost btn-sm" onclick={() => startComment(selectedFile.path)}>{$t.changes.notes.commentFile}</button>
                <button type="button" class="btn btn-ghost btn-sm" disabled={revertBusy} onclick={() => revert('turn', [{ path: selectedFile.path }])}>{$t.changes.revert.file}</button>
              {/if}
              <div class="seg" role="group" aria-label={$t.changes.diff.layoutLabel}>
                <button type="button" class="btn btn-ghost btn-sm" class:active={diffMode === 'unified'} onclick={() => (diffMode = 'unified')}>{$t.changes.diff.unified}</button>
                <button type="button" class="btn btn-ghost btn-sm" class:active={diffMode === 'split'} onclick={() => (diffMode = 'split')}>{$t.changes.diff.split}</button>
              </div>
            </div>
          </div>
          {#snippet hunkActions(index: number)}
            {#if selected && selectedFile && changes.isReverted(selected.turn_id, selectedFile.path, hunkId(index))}
              <span class="reverted-badge">{$t.changes.revert.reverted}</span>
            {:else if selectedFile}
              {@const path = selectedFile.path}
              <button type="button" class="hunk-revert" onclick={() => startComment(path, hunkId(index))}>{$t.changes.notes.comment}</button>
              <button type="button" class="hunk-revert" disabled={revertBusy} onclick={() => revert('turn', [{ path, hunk_ids: [hunkId(index)] }])}>{$t.changes.revert.hunk}</button>
            {/if}
          {/snippet}
          {#if commentTarget && commentTarget.path === selectedFile.path}
            <form class="note-form" onsubmit={(event) => { event.preventDefault(); addComment() }}>
              <label for="review-note-text">{$t.changes.notes.title(noteWhere(commentTarget.path, commentTarget.hunkId))}</label>
              <textarea id="review-note-text" rows="3" bind:value={commentText} placeholder={$t.changes.notes.placeholder}></textarea>
              <div class="revert-buttons">
                <button type="submit" class="btn btn-secondary btn-sm" disabled={!commentText.trim()}>{$t.changes.notes.add}</button>
                <button type="button" class="btn btn-ghost btn-sm" onclick={() => (commentTarget = null)}>{$t.changes.notes.cancel}</button>
              </div>
            </form>
          {/if}
          {#if selectedFile.binary}
            <div class="empty-state compact">{$t.changes.binary}</div>
          {:else if selectedLines.length === 0}
            <div class="empty-state compact">{$t.changes.noTextChanges}</div>
          {:else}
            <DiffView lines={selectedLines} mode={diffMode} label={$t.changes.diff.sideBySideLabel} hunkActions={canRevertHunks ? hunkActions : undefined} />
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

  .note-form {
    display: grid;
    gap: var(--space-2);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    background: var(--surface-elevated);
    padding: var(--space-2);
    font-size: var(--text-xs);
  }

  .note-form label {
    color: var(--text-secondary);
  }

  .note-form textarea {
    width: 100%;
    resize: vertical;
    font-size: var(--text-sm);
  }

  .notes-pending {
    color: var(--primary-text);
  }

  .diff-tools {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .turn-actions,
  .revert-buttons {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }

  .revert-bar {
    display: grid;
    gap: var(--space-2);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    background: var(--surface-elevated);
    padding: var(--space-3);
    font-size: var(--text-sm);
    flex: 0 0 auto;
  }

  .revert-bar p {
    margin: 0;
    color: var(--text-primary);
  }

  .revert-bar.revert-done {
    border-color: color-mix(in srgb, var(--success) 40%, var(--border-default));
  }

  .revert-bar p.revert-warn {
    color: var(--warning);
  }

  .revert-bar p.revert-error {
    color: var(--error);
  }

  .revert-files {
    display: grid;
    gap: var(--space-1);
    margin: 0;
    padding: 0;
    list-style: none;
    max-height: 200px;
    overflow-y: auto;
    font-size: var(--text-xs);
  }

  .revert-files li {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: var(--space-2);
    min-width: 0;
  }

  .revert-files code {
    color: var(--text-primary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    min-width: 0;
  }

  .revert-outcome {
    color: var(--text-tertiary);
  }

  .revert-outcome.outcome-conflict,
  .revert-outcome.outcome-failed {
    color: var(--warning);
  }

  .revert-merge {
    flex-basis: 100%;
  }

  .revert-merge pre {
    max-height: 200px;
    overflow: auto;
    margin: var(--space-1) 0 0;
    padding: var(--space-2);
    background: var(--surface-inset);
    border-radius: var(--radius-sm);
    font-size: var(--text-xs);
  }

  .reverted-badge {
    margin-left: var(--space-1);
    padding: 0 var(--space-1);
    border-radius: var(--radius-sm);
    background: var(--surface-active);
    color: var(--text-secondary);
    font-family: var(--font-mono);
    font-size: 10px;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }

  .hunk-revert {
    border: 1px solid var(--border-default);
    border-radius: var(--radius-sm);
    background: var(--surface);
    color: var(--text-secondary);
    font-size: var(--text-xs);
    padding: 0 var(--space-2);
    cursor: pointer;
  }

  .hunk-revert:hover:not(:disabled) {
    color: var(--text-primary);
    border-color: var(--primary);
  }

  .hunk-revert:disabled {
    opacity: 0.5;
    cursor: default;
  }

  .diff-head-title code {
    color: var(--text-primary);
    font-size: var(--text-sm);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
