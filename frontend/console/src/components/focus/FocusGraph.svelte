<script lang="ts">
  // The pipeline graph (ADR §9 P5), opened from the stage bar: stages left
  // to right, loop edges with their iteration counts, plan tasks inside the
  // build node. Read-only — gates and cards stay on the deck.
  import { Background, Controls, SvelteFlow, type NodeTypes } from '@xyflow/svelte'
  import '@xyflow/svelte/dist/style.css'
  import { t } from '../../i18n'
  import { buildFocusGraph, focusGraphColumns, GRAPH_MIN_ZOOM, GRAPH_PADDING } from '../../lib/focusGraph'
  import type { FocusPipeline } from '../../lib/types'
  import FocusGraphStageNode from './FocusGraphStageNode.svelte'
  import FocusGraphTaskNode from './FocusGraphTaskNode.svelte'

  interface Props {
    pipeline: FocusPipeline
  }

  let { pipeline }: Props = $props()

  const nodeTypes: NodeTypes = { focusStage: FocusGraphStageNode, focusTask: FocusGraphTaskNode }

  // The pane's width decides how many stages share a row, so the fitted
  // zoom never drops below GRAPH_MIN_ZOOM (text stays >= 11px on screen).
  let paneWidth = $state(0)
  let labels = $derived({ stages: $t.focus.stages })
  let columns = $derived(paneWidth > 0 ? focusGraphColumns(pipeline, labels, paneWidth) : 0)
  let graph = $derived(buildFocusGraph(pipeline, labels, { columns: columns || undefined }))
  // Tall enough for the content at zoom 1 (fitView zooms at most to 1).
  let canvasHeight = $derived(Math.ceil(graph.height * (1 + 2 * GRAPH_PADDING)) + 8)
</script>

<section class="focus-graph" aria-label={$t.focus.graph.label} data-testid="focus-graph" data-columns={columns} bind:clientWidth={paneWidth} style:height="{canvasHeight}px">
  {#if columns > 0}
    <!-- A new row layout fits the view again. -->
    {#key columns}
      <SvelteFlow
        nodes={graph.nodes}
        edges={graph.edges}
        {nodeTypes}
        colorMode="dark"
        fitView
        fitViewOptions={{ padding: GRAPH_PADDING, minZoom: GRAPH_MIN_ZOOM, maxZoom: 1 }}
        minZoom={GRAPH_MIN_ZOOM}
        maxZoom={1.5}
        nodesDraggable={false}
        nodesConnectable={false}
        elementsSelectable={false}
        proOptions={{ hideAttribution: true }}
      >
        <Controls showLock={false} orientation="horizontal" position="bottom-left" />
        <Background />
      </SvelteFlow>
    {/key}
  {/if}
</section>

<style>
  .focus-graph {
    min-height: 160px;
    overflow: hidden;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--surface-inset);
  }

  /* Graphite Signal over xyflow's dark theme: controls, background dots and
     edges take the console's tokens (app.css), not xyflow's greys. */
  .focus-graph :global(.svelte-flow) {
    --xy-background-color: transparent;
    --xy-background-pattern-color: var(--border-default);
    --xy-edge-stroke: var(--border-strong);
    --xy-edge-label-background-color: var(--surface-elevated);
    --xy-edge-label-color: var(--primary-text);
    --xy-controls-box-shadow: none;
    --xy-controls-button-background-color: var(--surface-elevated);
    --xy-controls-button-background-color-hover: var(--surface-hover);
    --xy-controls-button-color: var(--text-secondary);
    --xy-controls-button-color-hover: var(--text-primary);
    --xy-controls-button-border-color: var(--border-default);
    background: transparent;
    color: var(--text-primary);
  }

  .focus-graph :global(.svelte-flow__controls) {
    border: 1px solid var(--border-default);
    border-radius: var(--radius-sm);
    overflow: hidden;
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
    font-size: 13px;
  }
</style>
