<script lang="ts">
  // The pipeline graph (ADR §9 P5), opened from the stage bar: stages left
  // to right, loop edges with their iteration counts, plan tasks inside the
  // build node. Read-only — gates and cards stay on the deck.
  import { Background, Controls, SvelteFlow, type NodeTypes } from '@xyflow/svelte'
  import '@xyflow/svelte/dist/style.css'
  import { t } from '../../i18n'
  import { buildFocusGraph } from '../../lib/focusGraph'
  import type { FocusPipeline } from '../../lib/types'
  import FocusGraphStageNode from './FocusGraphStageNode.svelte'
  import FocusGraphTaskNode from './FocusGraphTaskNode.svelte'

  interface Props {
    pipeline: FocusPipeline
  }

  let { pipeline }: Props = $props()

  const nodeTypes: NodeTypes = { focusStage: FocusGraphStageNode, focusTask: FocusGraphTaskNode }

  let graph = $derived(buildFocusGraph(pipeline, { stages: $t.focus.stages }))
</script>

<section class="focus-graph" aria-label={$t.focus.graph.label} data-testid="focus-graph">
  <SvelteFlow
    nodes={graph.nodes}
    edges={graph.edges}
    {nodeTypes}
    fitView
    minZoom={0.3}
    maxZoom={1.5}
    nodesDraggable={false}
    nodesConnectable={false}
    elementsSelectable={false}
    proOptions={{ hideAttribution: true }}
  >
    <Controls showLock={false} />
    <Background />
  </SvelteFlow>
</section>

<style>
  .focus-graph {
    height: 320px;
    overflow: hidden;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--surface-inset);
  }

  .focus-graph :global(.svelte-flow) {
    background: transparent;
    color: var(--text-primary);
  }

  .focus-graph :global(.svelte-flow__node.focus-graph-stage) {
    padding: 0;
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    background: var(--surface-elevated);
    color: var(--text-secondary);
    box-shadow: none;
  }

  .focus-graph :global(.svelte-flow__node.focus-graph-done) {
    border-color: rgba(var(--primary-rgb), 0.5);
    color: var(--text-primary);
  }

  .focus-graph :global(.svelte-flow__node.focus-graph-current) {
    border-color: var(--primary);
    background: var(--primary-muted);
    color: var(--primary-text);
  }

  .focus-graph :global(.svelte-flow__node.focus-graph-blocked) {
    border-color: var(--warning);
    color: var(--warning);
  }

  .focus-graph :global(.svelte-flow__node.focus-graph-skipped) {
    border-style: dashed;
    color: var(--text-tertiary);
    opacity: 0.6;
  }

  .focus-graph :global(.svelte-flow__node.focus-graph-task) {
    padding: 0;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface);
    color: var(--text-primary);
    box-shadow: none;
  }

  .focus-graph :global(.focus-graph-edge .svelte-flow__edge-path) {
    stroke: var(--border-strong);
  }

  .focus-graph :global(.focus-graph-edge-skipped .svelte-flow__edge-path) {
    stroke-dasharray: 4 4;
    opacity: 0.5;
  }

  .focus-graph :global(.focus-graph-loop .svelte-flow__edge-path) {
    stroke: var(--text-tertiary);
  }

  .focus-graph :global(.focus-graph-loop-repeated .svelte-flow__edge-path) {
    stroke: var(--primary);
  }

  .focus-graph :global(.svelte-flow__edge-label) {
    background: var(--surface-elevated);
    color: var(--primary-text);
    font-family: var(--font-mono);
    font-size: 10px;
  }
</style>
