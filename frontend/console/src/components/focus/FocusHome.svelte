<script lang="ts">
  // The focus home (ADR §3): running pipelines, one row per task, and the
  // "New task" entry — a reduced session board. The list item has no
  // finished flag, so each pipeline is read to tell finished and stopped
  // ones apart (lib/focus pipelinePhase, mirroring Pipeline.Active).
  import { onDestroy, onMount } from 'svelte'
  import { t } from '../../i18n'
  import { getFocusPipeline, listFocusPipelines } from '../../lib/api'
  import { pipelinePhase, stageLabel, type PipelinePhase } from '../../lib/focus'
  import type { FocusListItem } from '../../lib/types'
  import FocusNewTask from './FocusNewTask.svelte'

  interface Props {
    onNavigate: (path: string) => void
  }

  let { onNavigate }: Props = $props()

  // Pipelines read per refresh to learn their phase; beyond this the list
  // still shows, without the finished/stopped mark.
  const phaseLimit = 30

  let items = $state<FocusListItem[]>([])
  let phases = $state<Record<string, PipelinePhase>>({})
  let loaded = $state(false)
  let error = $state('')
  let creating = $state(false)
  let timer: ReturnType<typeof setInterval> | null = null

  async function refresh() {
    try {
      const list = await listFocusPipelines()
      list.sort((a, b) => (a.updated_at < b.updated_at ? 1 : -1))
      items = list
      error = ''
      const read = await Promise.all(list.slice(0, phaseLimit).map((item) => getFocusPipeline(item.session_id).then((p) => [item.session_id, pipelinePhase(p)] as const).catch(() => null)))
      phases = Object.fromEntries(read.filter((r): r is readonly [string, PipelinePhase] => !!r))
    } catch (err) {
      error = err instanceof Error ? err.message : String(err)
    } finally {
      loaded = true
    }
  }

  onMount(() => {
    void refresh()
    timer = setInterval(() => void refresh(), 15_000)
  })

  onDestroy(() => {
    if (timer) clearInterval(timer)
  })

  function open(id: string) {
    onNavigate(`/console/focus/${encodeURIComponent(id)}`)
  }
</script>

<div class="focus-home" data-testid="focus-home">
  <header class="home-header">
    <div>
      <h2>{$t.focus.home.title}</h2>
      <p class="subtitle">{$t.focus.home.subtitle}</p>
    </div>
    <div class="home-actions">
      <button type="button" class="btn btn-ghost btn-sm" onclick={() => onNavigate('/console')}>{$t.focus.home.advanced}</button>
      <button type="button" class="btn btn-ghost btn-sm" onclick={() => onNavigate('/console/focus/release')} data-testid="focus-release-open">{$t.focus.release.open}</button>
      <button type="button" class="btn btn-ghost btn-sm" onclick={() => onNavigate('/console/focus/templates')} data-testid="focus-templates-open">{$t.focus.templateEditor.open}</button>
      {#if !creating}
        <button type="button" class="btn btn-primary" onclick={() => { creating = true }} data-testid="focus-new-task-open">{$t.focus.home.newTask}</button>
      {/if}
    </div>
  </header>

  {#if creating}
    <FocusNewTask onCreated={open} onCancel={() => { creating = false }} />
  {/if}

  {#if error}
    <p class="banner error">{$t.focus.home.loadFailed(error)}</p>
  {/if}

  {#if loaded && items.length === 0 && !error}
    <p class="empty">{$t.focus.home.empty}</p>
  {:else}
    <ul class="task-list">
      {#each items as item (item.session_id)}
        {@const phase = phases[item.session_id]}
        <li>
          <button type="button" class="task-row" class:waiting={!!item.open_gate || item.needs_input > 0} onclick={() => open(item.session_id)} data-testid="focus-task">
            <span class="task-main">
              <span class="task-title" data-content>{item.title || item.goal}</span>
              {#if item.goal && item.goal !== item.title}<span class="task-goal" data-content>{item.goal}</span>{/if}
            </span>
            <span class="task-facts">
              <span class="badge badge-default">{stageLabel({ template: item.template, stages: [{ id: item.current, label: item.current_label }] }, item.current, $t.focus)}</span>
              {#if item.goal_mode}<span class="badge badge-accent" data-testid="focus-task-goal">{$t.focus.screen.goal}</span>{/if}
              {#if phase === 'finished'}
                <span class="badge badge-success">{$t.focus.home.finished}</span>
              {:else if phase === 'stopped'}
                <span class="badge badge-default">{$t.focus.status.blocked}</span>
              {:else if item.open_gate}
                <span class="badge badge-warning">{$t.focus.home.gateOpen}</span>
              {/if}
              {#if item.needs_input > 0}
                <span class="mono needs">{$t.focus.home.needsInput(item.needs_input)}</span>
              {/if}
            </span>
          </button>
        </li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .focus-home {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    max-width: 880px;
    margin: 0 auto;
    padding: var(--space-6) var(--space-6) var(--space-10);
  }

  .home-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-4);
  }

  h2 {
    margin: 0;
  }

  .subtitle {
    margin: var(--space-1) 0 0;
    color: var(--text-secondary);
    font-size: var(--text-sm);
  }

  .home-actions {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .task-list {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .task-row {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    width: 100%;
    padding: var(--space-3) var(--space-4);
    background: var(--surface);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg);
    color: var(--text-primary);
    text-align: left;
    cursor: pointer;
  }

  .task-row:hover {
    border-color: var(--border-strong);
  }

  .task-row.waiting {
    border-color: var(--warning);
  }

  .task-main {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .task-title {
    font-weight: 600;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .task-goal {
    font-size: var(--text-xs);
    color: var(--text-secondary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .task-facts {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
  }

  .needs {
    font-size: var(--text-xs);
    color: var(--warning);
  }

  .empty {
    margin: 0;
    padding: var(--space-8) var(--space-4);
    text-align: center;
    color: var(--text-tertiary);
    border: 1px dashed var(--border-default);
    border-radius: var(--radius-lg);
  }

  .banner.error {
    margin: 0;
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-md);
    background: var(--error-muted);
    color: var(--error);
    font-size: var(--text-sm);
  }
</style>
