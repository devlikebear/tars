<script lang="ts">
  // One diff as a unified or side-by-side table. Callers parse the patch
  // with lib/diff.ts and may put actions on each hunk's header row.
  import type { Snippet } from 'svelte'
  import { pairDiffLines, type DiffLine } from '../lib/diff'

  interface Props {
    lines: DiffLine[]
    mode?: 'unified' | 'split'
    // Accessible name for the side-by-side table.
    label?: string
    // Rendered on each hunk header row with the hunk's index in its file.
    hunkActions?: Snippet<[number]>
  }

  let { lines, mode = 'unified', label, hunkActions }: Props = $props()

  let pairs = $derived(mode === 'split' ? pairDiffLines(lines) : [])

  function sign(line: DiffLine | undefined, side: 'left' | 'right' | 'both'): string {
    if (!line) return ''
    switch (line.kind) {
      case 'add':
        return side === 'left' ? ' ' : '+'
      case 'del':
        return side === 'right' ? ' ' : '−'
      case 'hunk':
        return '@'
      case 'meta':
        return '\\'
      default:
        return ' '
    }
  }
</script>

{#if mode === 'split'}
  <div class="diff-table diff-split" aria-label={label}>
    {#each pairs as pair, idx (idx)}
      {@const leftKind = pair.left?.kind ?? 'empty'}
      {@const rightKind = pair.right?.kind ?? 'empty'}
      <div class="dline dline-{leftKind}">
        <span class="dline-num">{pair.left?.oldLine ?? ''}</span>
        <span class="dline-sign">{sign(pair.left, 'left')}</span>
        <span class="dline-text">{pair.left?.text ?? ''}</span>
      </div>
      <div class="dline dline-{rightKind}">
        <span class="dline-num">{pair.right?.newLine ?? ''}</span>
        <span class="dline-sign">{sign(pair.right, 'right')}</span>
        <span class="dline-text">
          {#if pair.right?.kind === 'hunk' && hunkActions && pair.right.hunk !== undefined}
            <span class="dline-hunk-text">{pair.right.text}</span>
            <span class="dline-actions">{@render hunkActions(pair.right.hunk)}</span>
          {:else}
            {pair.right?.text ?? ''}
          {/if}
        </span>
      </div>
    {/each}
  </div>
{:else}
  <div class="diff-table diff-unified" aria-label={label}>
    {#each lines as line, idx (idx)}
      <div class="dline dline-{line.kind}">
        <span class="dline-num">{line.oldLine ?? ''}</span>
        <span class="dline-num">{line.newLine ?? ''}</span>
        <span class="dline-sign">{sign(line, 'both')}</span>
        <span class="dline-text">
          {#if line.kind === 'hunk' && hunkActions && line.hunk !== undefined}
            <span class="dline-hunk-text">{line.text}</span>
            <span class="dline-actions">{@render hunkActions(line.hunk)}</span>
          {:else}
            {line.text}
          {/if}
        </span>
      </div>
    {/each}
  </div>
{/if}

<style>
  .diff-table {
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface-inset);
    overflow: auto;
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    line-height: 1.5;
    min-width: 0;
    min-height: 0;
    flex: 1 1 auto;
  }

  .diff-unified {
    display: grid;
    grid-template-columns: max-content max-content max-content minmax(0, 1fr);
  }

  .diff-split {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
    gap: 1px;
    background: var(--border-subtle);
  }

  .diff-split > .dline {
    display: grid;
    grid-template-columns: max-content max-content minmax(0, 1fr);
    background: var(--surface-inset);
  }

  .dline {
    display: contents;
  }

  .diff-split .dline {
    display: grid;
  }

  .dline-num,
  .dline-sign,
  .dline-text {
    padding: 0 var(--space-2);
    white-space: pre;
  }

  .dline-num {
    color: var(--text-ghost);
    text-align: right;
    min-width: 2.5ch;
    user-select: none;
    background: var(--surface);
    border-right: 1px solid var(--border-subtle);
  }

  .dline-sign {
    color: var(--text-tertiary);
    user-select: none;
    text-align: center;
    padding: 0 var(--space-1);
  }

  .dline-text {
    color: var(--text-secondary);
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .dline-hunk .dline-text {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .dline-hunk-text {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .dline-actions {
    display: inline-flex;
    flex-shrink: 0;
    gap: var(--space-1);
    font-family: var(--font-body);
    white-space: nowrap;
  }

  .dline-add .dline-num,
  .dline-add .dline-sign,
  .dline-add .dline-text {
    background: rgba(34, 197, 94, 0.10);
  }

  .dline-add .dline-text,
  .dline-add .dline-sign {
    color: var(--green, #22c55e);
  }

  .dline-del .dline-num,
  .dline-del .dline-sign,
  .dline-del .dline-text {
    background: rgba(239, 68, 68, 0.12);
  }

  .dline-del .dline-text,
  .dline-del .dline-sign {
    color: var(--error, #ef4444);
  }

  .dline-hunk .dline-num,
  .dline-hunk .dline-sign,
  .dline-hunk .dline-text {
    background: var(--surface-elevated);
    color: var(--primary-text);
    font-weight: 500;
  }

  .dline-meta .dline-num,
  .dline-meta .dline-sign,
  .dline-meta .dline-text {
    color: var(--text-tertiary);
    font-style: italic;
  }

  .dline-empty .dline-num,
  .dline-empty .dline-sign,
  .dline-empty .dline-text {
    background: var(--surface-base);
  }

  @media (max-width: 600px) {
    .diff-split {
      grid-template-columns: minmax(0, 1fr);
    }
  }
</style>
