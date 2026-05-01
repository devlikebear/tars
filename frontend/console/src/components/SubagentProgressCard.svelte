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
  let title = $derived(progress.mode === 'consensus' ? 'Consensus subagents' : progress.mode === 'compare' ? 'Compare subagents' : 'Parallel subagents')
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

  {#if progress.comparison}
    <div class="compare-summary">
      <section>
        <h4>Common</h4>
        {#if progress.comparison.commonFindings.length > 0}
          <ul>
            {#each progress.comparison.commonFindings as finding}
              <li>{finding}</li>
            {/each}
          </ul>
        {:else}
          <p>No shared findings detected.</p>
        {/if}
      </section>
      <section>
        <h4>Conflicts</h4>
        {#if progress.comparison.conflicts.length > 0}
          <ul>
            {#each progress.comparison.conflicts as conflict}
              <li>{conflict}</li>
            {/each}
          </ul>
        {:else}
          <p>No obvious conflicts detected.</p>
        {/if}
      </section>
      <section>
        <h4>Evidence</h4>
        {#if progress.comparison.evidence.length > 0}
          <ul>
            {#each progress.comparison.evidence as item}
              <li>
                {#if item.href && item.runId}
                  <a href={item.href}>{item.title || item.agent || shortRunID(item.runId)}</a>
                {:else}
                  <span>{item.title || item.agent || 'source'}</span>
                {/if}
                <span>{item.text}</span>
              </li>
            {/each}
          </ul>
        {:else}
          <p>No evidence snippets available.</p>
        {/if}
      </section>
    </div>

    {#if progress.comparison.sideBySide.length > 0}
      <div class="compare-side-by-side">
        {#each progress.comparison.sideBySide as item}
          <article>
            <div class="compare-output-head">
              <strong>{item.title || item.agent || 'Subagent'}</strong>
              {#if item.href && item.runId}
                <a href={item.href}>Run {shortRunID(item.runId)}</a>
              {/if}
            </div>
            <pre>{item.response || item.error || '(waiting)'}</pre>
          </article>
        {/each}
      </div>
    {/if}
  {/if}
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

  .compare-summary {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: var(--space-2);
    margin-top: var(--space-2);
  }

  .compare-summary section,
  .compare-side-by-side article {
    min-width: 0;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface-inset);
    padding: var(--space-2);
  }

  .compare-summary h4 {
    margin: 0 0 var(--space-1);
    color: var(--text-primary);
    font-size: var(--text-xs);
  }

  .compare-summary ul {
    display: grid;
    gap: var(--space-1);
    margin: 0;
    padding-left: var(--space-3);
    color: var(--text-secondary);
  }

  .compare-summary li {
    overflow-wrap: anywhere;
  }

  .compare-summary li a,
  .compare-output-head a {
    color: var(--primary);
    text-decoration: none;
  }

  .compare-summary li a:hover,
  .compare-output-head a:hover {
    text-decoration: underline;
  }

  .compare-summary li span + span {
    display: block;
    margin-top: 2px;
  }

  .compare-summary p {
    margin: 0;
    color: var(--text-tertiary);
  }

  .compare-side-by-side {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
    gap: var(--space-2);
    margin-top: var(--space-2);
  }

  .compare-output-head {
    display: flex;
    gap: var(--space-2);
    align-items: center;
    justify-content: space-between;
    min-width: 0;
    margin-bottom: var(--space-1);
  }

  .compare-output-head strong {
    min-width: 0;
    overflow: hidden;
    color: var(--text-primary);
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .compare-output-head a {
    flex-shrink: 0;
    font-family: var(--font-mono);
    font-size: 10px;
  }

  .compare-side-by-side pre {
    max-height: 220px;
    margin: 0;
    overflow: auto;
    color: var(--text-secondary);
    white-space: pre-wrap;
    overflow-wrap: anywhere;
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

    .compare-summary {
      grid-template-columns: 1fr;
    }
  }
</style>
