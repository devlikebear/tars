<script lang="ts">
  // One plan task, drawn inside the build stage node (lib/focusGraph).
  import type { NodeProps } from '@xyflow/svelte'
  import type { FocusGraphNode } from '../../lib/focusGraph'

  let { data }: NodeProps<FocusGraphNode> = $props()
</script>

<div class="task-node" title={data.label} data-testid="focus-graph-task" data-content>{data.label}</div>

<style>
  .task-node {
    width: 100%;
    height: 100%;
    padding: 0 var(--space-2);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: var(--text-xs);
    line-height: 30px;
  }
</style>
