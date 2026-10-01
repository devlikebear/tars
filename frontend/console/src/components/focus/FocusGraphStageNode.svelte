<script lang="ts">
  // One stage of the pipeline graph (lib/focusGraph): left/right handles
  // for the forward edges, two bottom handles so a loop edge returns to its
  // own stage as a U under it, and wrap handles for snaking rows.
  import { Handle, Position, type NodeProps } from '@xyflow/svelte'
  import { t } from '../../i18n'
  import type { FocusGraphNode } from '../../lib/focusGraph'

  let { data }: NodeProps<FocusGraphNode> = $props()
</script>

<div class="stage-node" data-testid="focus-graph-stage" data-status={data.status}>
  <Handle type="target" position={Position.Left} id="in" />
  <span class="stage-label">{data.label}</span>
  {#if data.status}<span class="stage-status mono">{$t.focus.status[data.status]}</span>{/if}
  <Handle type="source" position={Position.Right} id="out" />
  <!-- Right-to-left rows (lib/focusGraph snakes long pipelines). -->
  <Handle type="source" position={Position.Left} id="out-left" />
  <Handle type="target" position={Position.Right} id="in-right" />
  <!-- A row wrap drops from the bottom of one stage to the top of the next. -->
  <Handle type="source" position={Position.Bottom} id="wrap-out" style="left: 50%" />
  <Handle type="target" position={Position.Top} id="wrap-in" style="left: 50%" />
  <Handle type="source" position={Position.Bottom} id="loop-out" style="left: 80%" />
  <Handle type="target" position={Position.Bottom} id="loop-in" style="left: 20%" />
</div>

<style>
  .stage-node {
    display: flex;
    flex-direction: column;
    gap: 2px;
    width: 100%;
    height: 100%;
    padding: var(--space-2) var(--space-3);
  }

  /* 13px = GRAPH_NODE_FONT_PX (lib/focusGraph): >= 11px at the minimum zoom. */
  .stage-label {
    font-family: var(--font-mono);
    font-size: 13px;
    font-weight: 600;
  }

  .stage-status {
    font-size: 13px;
    color: var(--text-tertiary);
  }

  .stage-node :global(.svelte-flow__handle) {
    opacity: 0;
  }
</style>
