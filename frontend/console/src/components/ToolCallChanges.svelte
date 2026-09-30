<script lang="ts">
  // Files one tool call changed, inside its tool card (#1032). Native
  // providers report each change as the call finishes; each file expands to
  // the same DiffView the turn change card uses.
  import { t } from '../i18n'
  import { parseUnifiedDiff } from '../lib/diff'
  import { fileChangePatch, fileChangeStatus, type ToolFileChange } from '../lib/toolFileChanges'
  import DiffView from './DiffView.svelte'

  interface Props {
    changes: ToolFileChange[]
  }

  let { changes }: Props = $props()

  function statusText(op: string): string {
    const status = fileChangeStatus(op)
    return $t.changes.fileStatus[status] ?? status
  }
</script>

<div class="tool-changes">
  <span class="tool-changes-label">{$t.chatThread.tool.changes(changes.length)}</span>
  {#each changes as change (change.path)}
    {@const patch = fileChangePatch(change)}
    <details class="tool-change">
      <summary class="tool-change-head">
        <code class="tool-change-path" title={change.path}>{change.path}</code>
        <span class="tool-change-status">{statusText(change.op)}</span>
        <span class="tool-change-counts"><span class="plus">+{change.additions}</span> <span class="minus">−{change.deletions}</span></span>
      </summary>
      <div class="tool-change-body">
        {#if change.binary}
          <div class="tool-change-note">{$t.changes.binary}</div>
        {:else if patch}
          <DiffView lines={parseUnifiedDiff(patch)} />
          {#if change.truncated}
            <div class="tool-change-note">{$t.changes.truncated}</div>
          {/if}
        {:else if change.truncated}
          <div class="tool-change-note">{$t.changes.truncated}</div>
        {:else}
          <div class="tool-change-note">{$t.changes.noTextChanges}</div>
        {/if}
      </div>
    </details>
  {/each}
</div>

<style>
  .tool-changes {
    display: grid;
    gap: var(--space-1);
    min-width: 0;
  }

  .tool-changes-label {
    font-family: var(--font-mono);
    color: var(--text-ghost);
  }

  .tool-change {
    min-width: 0;
  }

  .tool-change-head {
    display: flex;
    align-items: baseline;
    gap: var(--space-2);
    min-width: 0;
    font-size: var(--text-xs);
    cursor: pointer;
    list-style: none;
  }

  .tool-change-head::-webkit-details-marker {
    display: none;
  }

  .tool-change-head::before {
    content: '▸';
    color: var(--text-tertiary);
  }

  .tool-change[open] > .tool-change-head::before {
    content: '▾';
  }

  .tool-change-path {
    color: var(--text-primary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    min-width: 0;
  }

  .tool-change-status {
    color: var(--text-tertiary);
  }

  .tool-change-counts {
    margin-left: auto;
    font-family: var(--font-mono);
    white-space: nowrap;
  }

  .tool-change-body {
    display: grid;
    gap: var(--space-1);
    padding-top: var(--space-1);
    max-height: 360px;
    overflow-y: auto;
    min-width: 0;
  }

  .tool-change-note {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
  }

  .plus {
    color: var(--success);
  }

  .minus {
    color: var(--error);
  }
</style>
