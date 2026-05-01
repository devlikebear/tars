<script lang="ts">
  import type { SubagentProgress, SubagentProgressStatus } from '../lib/subagentProgress'
  import { shortRunID } from '../lib/subagentProgress'

  interface Props {
    progress: SubagentProgress
    tone: 'running' | 'done' | 'error'
    elapsedLabel?: string
  }

  let { progress, tone, elapsedLabel = '' }: Props = $props()

  let doneCount = $derived(progress.completed + progress.failed)
  let progressPct = $derived(progress.count > 0 ? Math.min(100, Math.max(0, (doneCount / progress.count) * 100)) : 0)
  let title = $derived(progress.mode === 'consensus' ? 'Consensus subagents' : 'Parallel subagents')
  let badgeClass = $derived(tone === 'error' || progress.failed > 0 ? 'badge-error' : tone === 'running' ? 'badge-accent' : 'badge-default')
  let summaryLabel = $derived(`${progress.completed}/${progress.count} done${progress.failed > 0 ? `, ${progress.failed} failed` : ''}`)

  function statusLabel(status: SubagentProgressStatus): string {
    switch (status) {
      case 'completed': return 'done'
      case 'failed': return 'failed'
      case 'running': return 'running'
      default: return 'pending'
    }
  }
</script>

<details class="chat-msg chat-tool subagent-progress-card chat-tool-{tone}" open={!progress.complete || progress.failed > 0}>
  <summary class="subagent-header">
    <span class="subagent-icon">{tone === 'error' || progress.failed > 0 ? '!' : progress.complete ? '\u2713' : '\u27F3'}</span>
    <span class="subagent-title">{title}</span>
    <span class="subagent-summary">{summaryLabel}</span>
    {#if elapsedLabel}
      <span class="subagent-elapsed">{elapsedLabel}</span>
    {/if}
    <span class="badge {badgeClass} subagent-badge">{progress.running > 0 ? `${progress.running} running` : tone}</span>
  </summary>

  <div class="subagent-meter" aria-hidden="true">
    <span style={`width: ${progressPct}%`}></span>
  </div>

  <div class="subagent-list">
    {#each progress.tasks as task, index}
      <div class="subagent-row subagent-row-{task.status}">
        <span class="status-pill">{statusLabel(task.status)}</span>
        <div class="subagent-task-main">
          <span class="subagent-task-title">{task.title || `Subagent ${index + 1}`}</span>
          <span class="subagent-task-meta">
            {[task.agent, task.tier, task.error || task.summary].filter(Boolean).join(' · ')}
          </span>
        </div>
        {#if task.href && task.runId}
          <a class="subagent-run-link" href={task.href}>Run {shortRunID(task.runId)}</a>
        {/if}
      </div>
    {/each}
  </div>
</details>

<style>
  .subagent-progress-card {
    background: var(--surface-base);
    border: 1px solid var(--border-subtle);
    padding: var(--space-2) var(--space-3);
    font-size: var(--text-xs);
  }

  .subagent-progress-card.chat-tool-running {
    background: var(--primary-muted);
    border-color: color-mix(in srgb, var(--primary) 35%, var(--border-subtle));
  }

  .subagent-progress-card.chat-tool-error {
    background: var(--error-muted);
    border-color: color-mix(in srgb, var(--error) 35%, var(--border-subtle));
  }

  .subagent-header {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    cursor: pointer;
    list-style: none;
    min-width: 0;
  }

  .subagent-header::-webkit-details-marker {
    display: none;
  }

  .subagent-icon {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 18px;
    height: 18px;
    border-radius: 999px;
    background: var(--surface-elevated);
    color: var(--text-secondary);
    font-size: 11px;
    flex-shrink: 0;
  }

  .chat-tool-running .subagent-icon {
    background: var(--primary-muted);
    color: var(--primary-text);
  }

  .chat-tool-error .subagent-icon {
    background: var(--error-muted);
    color: var(--error);
  }

  .subagent-title {
    font-family: var(--font-mono);
    font-weight: 700;
    color: var(--text-primary);
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .subagent-summary {
    color: var(--text-secondary);
    white-space: nowrap;
  }

  .subagent-elapsed {
    margin-left: auto;
    font-family: var(--font-mono);
    color: var(--text-tertiary);
    white-space: nowrap;
  }

  .subagent-badge {
    font-size: 10px;
    padding: 1px 6px;
    white-space: nowrap;
  }

  .subagent-meter {
    height: 4px;
    margin-top: var(--space-2);
    overflow: hidden;
    border-radius: 999px;
    background: var(--surface-inset);
  }

  .subagent-meter span {
    display: block;
    height: 100%;
    background: var(--primary);
    transition: width var(--duration-normal);
  }

  .subagent-list {
    display: grid;
    gap: var(--space-2);
    margin-top: var(--space-2);
  }

  .subagent-row {
    display: grid;
    grid-template-columns: 64px minmax(0, 1fr) auto;
    gap: var(--space-2);
    align-items: center;
    padding: var(--space-2);
    border-radius: var(--radius-sm);
    background: var(--surface-inset);
  }

  .status-pill {
    font-family: var(--font-mono);
    font-size: 10px;
    color: var(--text-tertiary);
    text-transform: uppercase;
  }

  .subagent-row-completed .status-pill {
    color: var(--success);
  }

  .subagent-row-failed .status-pill {
    color: var(--error);
  }

  .subagent-row-running .status-pill {
    color: var(--primary);
  }

  .subagent-task-main {
    display: grid;
    gap: 2px;
    min-width: 0;
  }

  .subagent-task-title {
    color: var(--text-primary);
    font-weight: 600;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .subagent-task-meta {
    color: var(--text-ghost);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .subagent-run-link {
    color: var(--primary);
    font-family: var(--font-mono);
    font-size: 10px;
    text-decoration: none;
    white-space: nowrap;
  }

  .subagent-run-link:hover {
    text-decoration: underline;
  }

  @media (max-width: 720px) {
    .subagent-header {
      flex-wrap: wrap;
    }

    .subagent-elapsed {
      margin-left: 0;
    }

    .subagent-row {
      grid-template-columns: 1fr;
      align-items: start;
    }
  }
</style>
