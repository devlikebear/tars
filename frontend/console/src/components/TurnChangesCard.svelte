<script lang="ts">
  // What one chat turn changed on disk, under the turn's last message (#969).
  // Collapsed it is a one-line summary; expanded it shows each file's diff.
  // Only turns that changed files, or whose recording was skipped, show one.
  import { t } from '../i18n'
  import type { CheckpointDiff } from '../lib/api/checkpoints'
  import { parseUnifiedDiff } from '../lib/diff'
  import { changes } from '../lib/stores/changesStore'
  import { chatDock } from '../lib/stores/chatDockStore.svelte'
  import DiffView from './DiffView.svelte'

  interface Props {
    turnId: string
  }

  let { turnId }: Props = $props()

  let entry = $derived(changes.turn(turnId))
  let expanded = $state(false)
  let diff = $state<CheckpointDiff | null>(null)
  let loading = $state(false)
  let failed = $state(false)

  function skipText(reason: string): string {
    return $t.changes.skipped[reason] ?? $t.changes.skippedOther(reason)
  }

  function statusText(status: string): string {
    return $t.changes.fileStatus[status] ?? status
  }

  // Load the diff when first expanded, and again after the store drops it.
  $effect(() => {
    void changes.version
    const files = entry?.files ?? 0
    if (!expanded || files === 0) return
    let current = true
    loading = true
    failed = false
    changes.diff(turnId, 'turn')
      .then((result) => { if (current) diff = result })
      .catch(() => { if (current) failed = true })
      .finally(() => { if (current) loading = false })
    return () => { current = false }
  })

  function openInPanel() {
    changes.select(turnId)
    changes.setScope('turn')
    chatDock.open('changes')
  }
</script>

{#if entry?.skipped}
  <div class="turn-changes skipped" role="note">± {skipText(entry.skipped)}</div>
{:else if entry && entry.files > 0}
  <section class="turn-changes" aria-label={$t.changes.card.label}>
    <div class="turn-changes-head">
      <button
        type="button"
        class="turn-changes-toggle"
        aria-expanded={expanded}
        title={expanded ? $t.changes.card.hide : $t.changes.card.show}
        onclick={() => (expanded = !expanded)}
      >
        <span class="turn-changes-caret" aria-hidden="true">{expanded ? '▾' : '▸'}</span>
        <span class="turn-changes-summary">{$t.changes.summary(entry.files, entry.additions, entry.deletions)}</span>
      </button>
      <button type="button" class="btn btn-ghost btn-sm" onclick={openInPanel}>{$t.changes.card.openPanel}</button>
    </div>
    {#if expanded}
      <div class="turn-changes-body">
        {#if loading && !diff}
          <div class="turn-changes-note">{$t.changes.diff.loading}</div>
        {:else if failed}
          <div class="turn-changes-note error">{$t.changes.loadFailed}</div>
        {:else if diff}
          {#each diff.files as file (file.path)}
            <article class="turn-file">
              <header class="turn-file-head">
                <code class="turn-file-path" title={file.path}>{file.path}</code>
                <span class="turn-file-status">{statusText(file.status)}</span>
                <span class="turn-file-counts"><span class="plus">+{file.additions}</span> <span class="minus">−{file.deletions}</span></span>
              </header>
              {#if file.binary}
                <div class="turn-changes-note">{$t.changes.binary}</div>
              {:else if file.patch}
                <DiffView lines={parseUnifiedDiff(file.patch)} />
                {#if file.truncated}
                  <div class="turn-changes-note">{$t.changes.truncated}</div>
                {/if}
              {:else}
                <div class="turn-changes-note">{$t.changes.noTextChanges}</div>
              {/if}
            </article>
          {/each}
          {#if diff.unknown?.length}
            <div class="turn-changes-note">{$t.changes.unknownPaths(diff.unknown.length)}</div>
          {/if}
        {/if}
      </div>
    {/if}
  </section>
{/if}

<style>
  .turn-changes {
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--surface);
    font-size: var(--text-sm);
    min-width: 0;
  }

  .turn-changes.skipped {
    padding: var(--space-2) var(--space-3);
    color: var(--text-tertiary);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
  }

  .turn-changes-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
    padding: var(--space-1) var(--space-2);
  }

  .turn-changes-toggle {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    min-width: 0;
    border: 0;
    background: transparent;
    color: var(--text-secondary);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    padding: var(--space-1);
    cursor: pointer;
  }

  .turn-changes-toggle:hover,
  .turn-changes-toggle[aria-expanded='true'] {
    color: var(--text-primary);
  }

  .turn-changes-caret {
    color: var(--text-tertiary);
  }

  .turn-changes-body {
    display: grid;
    gap: var(--space-2);
    padding: 0 var(--space-2) var(--space-2);
    max-height: 480px;
    overflow-y: auto;
  }

  .turn-file {
    display: grid;
    gap: var(--space-1);
    min-width: 0;
  }

  .turn-file-head {
    display: flex;
    align-items: baseline;
    gap: var(--space-2);
    min-width: 0;
    font-size: var(--text-xs);
  }

  .turn-file-path {
    color: var(--text-primary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    min-width: 0;
  }

  .turn-file-status {
    color: var(--text-tertiary);
  }

  .turn-file-counts {
    margin-left: auto;
    font-family: var(--font-mono);
    white-space: nowrap;
  }

  .plus {
    color: var(--success);
  }

  .minus {
    color: var(--error);
  }

  .turn-changes-note {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
  }

  .turn-changes-note.error {
    color: var(--error);
  }
</style>
