<script lang="ts">
  // A change card: one file of a turn's checkpoint diff, drawn with the
  // shared DiffView.
  import { t } from '../../i18n'
  import { parseUnifiedDiff } from '../../lib/diff'
  import type { FocusChangePayload } from '../../lib/types'
  import DiffView from '../DiffView.svelte'

  interface Props {
    change: FocusChangePayload
  }

  let { change }: Props = $props()

  let lines = $derived(parseUnifiedDiff(change.patch))
</script>

<div class="change-card" data-testid="focus-change">
  <div class="change-head">
    <span class="mono path" data-content>{change.path}</span>
    <span class="badge badge-default">{change.status}</span>
    <span class="mono stats"><span class="add">+{change.additions}</span> <span class="del">−{change.deletions}</span></span>
  </div>
  {#if change.binary}
    <p class="muted">{$t.focus.change.binary}</p>
  {:else if lines.length === 0}
    <p class="muted">{$t.focus.change.noPatch}</p>
  {:else}
    <div class="change-diff" data-content>
      <DiffView {lines} label={change.path} />
    </div>
  {/if}
</div>

<style>
  .change-card {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .change-head {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex-wrap: wrap;
  }

  .path {
    color: var(--text-primary);
    word-break: break-all;
  }

  .stats .add {
    color: var(--success);
  }

  .stats .del {
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
