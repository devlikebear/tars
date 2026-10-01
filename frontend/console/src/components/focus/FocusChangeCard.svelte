<script lang="ts">
  // A change card (U1): one turn's checkpoint diff. The card lists the
  // turn's files with their stats and shows one file's diff at a time (ADR
  // §7), drawn with the shared DiffView; the card is acknowledged as a
  // whole.
  import { t } from '../../i18n'
  import { parseUnifiedDiff } from '../../lib/diff'
  import type { FocusChangePayload, FocusChangeTurnPayload } from '../../lib/types'
  import DiffView from '../DiffView.svelte'

  interface Props {
    change: FocusChangeTurnPayload
  }

  let { change }: Props = $props()

  let files = $derived<FocusChangePayload[]>(change?.files ?? [])
  let index = $state(0)
  let file = $derived(files[Math.min(index, Math.max(files.length - 1, 0))])
  let lines = $derived(file ? parseUnifiedDiff(file.patch) : [])

  function show(i: number) {
    if (files.length === 0) return
    index = (i + files.length) % files.length
  }
</script>

<div class="change-card" data-testid="focus-change">
  <p class="mono summary">
    {$t.focus.change.files(files.length)} · <span class="add">+{change?.additions ?? 0}</span> <span class="del">−{change?.deletions ?? 0}</span>
  </p>
  {#if files.length > 1}
    <ul class="files" aria-label={$t.focus.change.fileList}>
      {#each files as f, i (f.path)}
        <li>
          <button type="button" class="file mono" class:current={i === index} aria-current={i === index} onclick={() => show(i)} data-testid="focus-change-file">
            <span class="path" data-content>{f.path}</span>
            <span class="stats"><span class="add">+{f.additions}</span> <span class="del">−{f.deletions}</span></span>
          </button>
        </li>
      {/each}
    </ul>
  {/if}
  {#if file}
    <div class="change-head">
      {#if files.length > 1}
        <button type="button" class="btn btn-ghost btn-sm" aria-label={$t.focus.change.prevFile} title={$t.focus.change.prevFile} onclick={() => show(index - 1)} data-testid="focus-change-prev">‹</button>
        <span class="mono pos">{$t.focus.change.position(index + 1, files.length)}</span>
        <button type="button" class="btn btn-ghost btn-sm" aria-label={$t.focus.change.nextFile} title={$t.focus.change.nextFile} onclick={() => show(index + 1)} data-testid="focus-change-next">›</button>
      {/if}
      <span class="mono path" data-content>{file.path}</span>
      <span class="badge badge-default">{file.status}</span>
      <span class="mono stats"><span class="add">+{file.additions}</span> <span class="del">−{file.deletions}</span></span>
    </div>
    {#if file.binary}
      <p class="muted">{$t.focus.change.binary}</p>
    {:else if lines.length === 0}
      <p class="muted">{$t.focus.change.noPatch}</p>
    {:else}
      {#key file.path}
        <div class="change-diff" data-content>
          <DiffView {lines} label={file.path} />
        </div>
      {/key}
    {/if}
  {/if}
</div>

<style>
  .change-card {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    min-width: 0;
  }

  .summary {
    margin: 0;
    font-size: var(--text-sm);
    color: var(--text-secondary);
  }

  .files {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
    max-height: 180px;
    overflow-y: auto;
  }

  .file {
    width: 100%;
    display: flex;
    justify-content: space-between;
    gap: var(--space-3);
    padding: var(--space-1) var(--space-2);
    background: transparent;
    border: 1px solid transparent;
    border-radius: var(--radius-sm);
    color: var(--text-secondary);
    font-size: var(--text-xs);
    text-align: left;
    cursor: pointer;
  }

  .file:hover {
    background: var(--surface-hover);
  }

  .file.current {
    border-color: var(--primary);
    color: var(--text-primary);
  }

  .file .path {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    direction: rtl;
    text-align: left;
  }

  .change-head {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex-wrap: wrap;
  }

  .pos {
    font-size: var(--text-xs);
    color: var(--text-tertiary);
  }

  .change-head .path {
    color: var(--text-primary);
    word-break: break-all;
  }

  .add {
    color: var(--success);
  }

  .del {
    color: var(--error);
  }

  .change-diff {
    max-height: 420px;
    overflow: auto;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
  }

  .muted {
    margin: 0;
    color: var(--text-tertiary);
    font-size: var(--text-sm);
  }
</style>
