<script lang="ts">
  import { onMount } from 'svelte'
  import { getSessionTasks } from '../lib/api'
  import type { SessionTasks } from '../lib/types'

  interface Props {
    sessionId: string
    onClose: () => void
  }

  let { sessionId, onClose }: Props = $props()

  let data: SessionTasks = $state({ tasks: [] })
  let loading = $state(true)
  let error = $state('')
  let planExpanded = $state(true)
  let taskList = $derived(Array.isArray(data.tasks) ? data.tasks : [])

  export async function load() {
    loading = true
    error = ''
    try {
      data = await getSessionTasks(sessionId)
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load tasks'
    } finally {
      loading = false
    }
  }

  function statusIcon(status: string): string {
    switch (status) {
      case 'completed': return '\u2714'
      case 'in_progress': return '\u25b6'
      case 'cancelled': return '\u2716'
      default: return '\u25cb'
    }
  }

  function statusClass(status: string): string {
    switch (status) {
      case 'completed': return 'badge-success'
      case 'in_progress': return 'badge-accent'
      case 'cancelled': return 'badge-error'
      default: return 'badge-default'
    }
  }

  let summary = $derived({
    total: taskList.length,
    pending: taskList.filter(t => t.status === 'pending').length,
    in_progress: taskList.filter(t => t.status === 'in_progress').length,
    completed: taskList.filter(t => t.status === 'completed').length,
    cancelled: taskList.filter(t => t.status === 'cancelled').length,
  })

  let progress = $derived(
    summary.total > 0 ? Math.round((summary.completed / summary.total) * 100) : 0
  )

  onMount(() => { void load() })

  // Reload when sessionId changes
  $effect(() => {
    void sessionId
    void load()
  })
</script>

<div class="tasks-panel">
  <div class="panel-header">
    <div class="panel-title-row">
      <span class="card-title">Tasks</span>
      {#if summary.total > 0}
        <span class="badge badge-accent">{summary.completed}/{summary.total}</span>
      {/if}
    </div>
    <div class="panel-actions">
      <button class="btn btn-ghost btn-sm" type="button" onclick={load} title="Refresh">&#x21bb;</button>
      <button class="btn btn-ghost btn-sm" type="button" onclick={onClose} title="Close">&times;</button>
    </div>
  </div>

  {#if loading}
    <div class="empty-state">Loading tasks...</div>
  {:else if error}
    <div class="error-banner">{error}</div>
  {:else if !data.plan && taskList.length === 0}
    <div class="empty-state">
      <p>No tasks yet.</p>
      <p class="hint">Ask TARS to create a plan and tasks for your work.</p>
    </div>
  {:else}
    {#if data.plan}
      <div class="plan-section">
        <button class="plan-header" type="button" onclick={() => planExpanded = !planExpanded}>
          <span class="plan-toggle">{planExpanded ? '\u25be' : '\u25b8'}</span>
          <span class="plan-label">Plan</span>
        </button>
        {#if planExpanded}
          <div class="plan-body">
            <p class="plan-goal">{data.plan.goal}</p>
            {#if data.plan.constraints}
              <p class="plan-constraints">{data.plan.constraints}</p>
            {/if}
          </div>
        {/if}
      </div>
    {/if}

    {#if summary.total > 0}
      <div class="progress-section">
        <div class="progress-bar">
          <div class="progress-fill" style="width: {progress}%"></div>
        </div>
        <div class="progress-stats">
          {#if summary.in_progress > 0}<span class="stat-chip accent">{summary.in_progress} active</span>{/if}
          {#if summary.pending > 0}<span class="stat-chip">{summary.pending} pending</span>{/if}
          {#if summary.completed > 0}<span class="stat-chip success">{summary.completed} done</span>{/if}
          {#if summary.cancelled > 0}<span class="stat-chip error">{summary.cancelled} skipped</span>{/if}
        </div>
      </div>
    {/if}

    <div class="tasks-list">
      {#each taskList as task (task.id)}
        <div class="task-card" class:completed={task.status === 'completed'} class:cancelled={task.status === 'cancelled'} class:active={task.status === 'in_progress'}>
          <span class="task-status-icon">{statusIcon(task.status)}</span>
          <div class="task-content">
            <span class="task-title">{task.title}</span>
            {#if task.description}
              <span class="task-desc">{task.description}</span>
            {/if}
          </div>
          <span class="badge {statusClass(task.status)}">{task.status.replace('_', ' ')}</span>
        </div>
      {/each}
    </div>
  {/if}
</div>

<style>
  .tasks-panel {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    height: 100%;
    overflow-y: auto;
  }

  .panel-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
  }

  .panel-title-row {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .panel-actions {
    display: flex;
    gap: var(--space-1);
  }

  .empty-state {
    padding: var(--space-6) var(--space-4);
    text-align: center;
    color: var(--text-secondary);
    font-size: var(--text-sm);
  }

  .hint {
    margin-top: var(--space-2);
    font-size: var(--text-xs);
    color: var(--text-tertiary);
  }

  .plan-section {
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--bg-inset);
    overflow: hidden;
  }

  .plan-header {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    width: 100%;
    padding: var(--space-3);
    background: none;
    border: none;
    color: var(--text-primary);
    font-family: var(--font-display);
    font-size: var(--text-xs);
    text-transform: uppercase;
    letter-spacing: 0.04em;
    cursor: pointer;
  }

  .plan-toggle {
    font-size: var(--text-xs);
    color: var(--text-tertiary);
  }

  .plan-body {
    padding: 0 var(--space-3) var(--space-3);
  }

  .plan-goal {
    font-size: var(--text-sm);
    color: var(--text-primary);
    line-height: 1.5;
  }

  .plan-constraints {
    margin-top: var(--space-2);
    font-size: var(--text-xs);
    color: var(--text-secondary);
  }

  .progress-section {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .progress-bar {
    height: 4px;
    background: var(--bg-inset);
    border-radius: 2px;
    overflow: hidden;
  }

  .progress-fill {
    height: 100%;
    background: var(--accent);
    border-radius: 2px;
    transition: width 0.3s var(--ease-out);
  }

  .progress-stats {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-1);
  }

  .stat-chip {
    padding: 2px var(--space-2);
    border-radius: var(--radius-sm);
    font-size: var(--text-xs);
    color: var(--text-secondary);
    background: var(--bg-inset);
  }

  .stat-chip.accent { color: var(--accent-text); background: rgba(224, 145, 69, 0.12); }
  .stat-chip.success { color: var(--success); background: rgba(74, 222, 128, 0.12); }
  .stat-chip.error { color: var(--error); background: rgba(239, 68, 68, 0.12); }

  .tasks-list {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .task-card {
    display: flex;
    align-items: flex-start;
    gap: var(--space-2);
    padding: var(--space-3);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--bg-inset);
    transition: border-color var(--duration-fast) var(--ease-out);
  }

  .task-card.active {
    border-color: var(--accent);
    background: rgba(224, 145, 69, 0.06);
  }

  .task-card.completed {
    opacity: 0.6;
  }

  .task-card.cancelled {
    opacity: 0.4;
  }

  .task-status-icon {
    flex-shrink: 0;
    width: 18px;
    text-align: center;
    font-size: var(--text-sm);
    line-height: 1.4;
  }

  .task-card.active .task-status-icon { color: var(--accent); }
  .task-card.completed .task-status-icon { color: var(--success); }
  .task-card.cancelled .task-status-icon { color: var(--text-tertiary); }

  .task-content {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .task-title {
    font-size: var(--text-sm);
    color: var(--text-primary);
    line-height: 1.4;
  }

  .task-desc {
    font-size: var(--text-xs);
    color: var(--text-secondary);
    line-height: 1.4;
  }
</style>
