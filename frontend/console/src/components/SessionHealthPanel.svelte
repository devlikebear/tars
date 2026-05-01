<script lang="ts">
  import type { SessionHealthAction, SessionHealthReport } from '../lib/sessionHealth'

  interface Props {
    report: SessionHealthReport
    loading?: boolean
    onRefresh?: () => void
    onAction?: (action: SessionHealthAction) => void
  }

  let { report, loading = false, onRefresh, onAction }: Props = $props()

  const statusLabel: Record<SessionHealthReport['status'], string> = {
    healthy: 'Healthy',
    watch: 'Watch',
    attention: 'Needs attention',
    critical: 'Critical',
  }

  const severityLabel: Record<string, string> = {
    info: 'Info',
    warning: 'Warning',
    error: 'Attention',
    critical: 'Critical',
  }

  const actionFallbackLabel: Record<SessionHealthAction, string> = {
    compact: 'Compact',
    review_fork_points: 'Review Chat',
    open_tasks: 'Open Tasks',
    open_config: 'Open Config',
    open_prior: 'Open Prior',
    open_skill_extraction: 'Extract Skill',
  }
</script>

<div class="session-health-panel">
  <header class="health-header">
    <div>
      <span class="health-eyebrow">Session Health</span>
      <h3>{statusLabel[report.status]}</h3>
    </div>
    <button type="button" class="btn btn-ghost btn-sm" disabled={loading} onclick={() => onRefresh?.()}>
      {loading ? 'Checking...' : 'Refresh'}
    </button>
  </header>

  <p class="health-summary">{report.summary}</p>

  <section class="health-metrics" aria-label="Session health metrics">
    <div>
      <strong>{report.metrics.messageCount}</strong>
      <span>Messages</span>
    </div>
    <div>
      <strong>{report.metrics.openTaskCount}</strong>
      <span>Open tasks</span>
    </div>
    <div>
      <strong>{report.metrics.highRiskToolCount}</strong>
      <span>Risk tools</span>
    </div>
    <div>
      <strong>{report.metrics.memoryCount}</strong>
      <span>Memory</span>
    </div>
  </section>

  {#if report.signals.length === 0}
    <div class="health-empty">No active session warnings.</div>
  {:else}
    <section class="health-section" aria-label="Session health signals">
      <h4>Signals</h4>
      <div class="health-list">
        {#each report.signals as signal}
          <article class={`health-row severity-${signal.severity}`}>
            <div class="health-row-title">
              <strong>{signal.title}</strong>
              <span>{severityLabel[signal.severity] ?? signal.severity}</span>
            </div>
            <p>{signal.detail}</p>
          </article>
        {/each}
      </div>
    </section>
  {/if}

  <section class="health-section" aria-label="Session health recommendations">
    <h4>Recommendations</h4>
    {#if report.recommendations.length === 0}
      <div class="health-empty compact">No action needed.</div>
    {:else}
      <div class="health-list">
        {#each report.recommendations as recommendation}
          <article class={`health-row severity-${recommendation.severity}`}>
            <div class="health-row-title">
              <strong>{recommendation.title}</strong>
              <span>{severityLabel[recommendation.severity] ?? recommendation.severity}</span>
            </div>
            <p>{recommendation.detail}</p>
            <button type="button" class="btn btn-ghost btn-sm" onclick={() => onAction?.(recommendation.action)}>
              {recommendation.actionLabel || actionFallbackLabel[recommendation.action]}
            </button>
          </article>
        {/each}
      </div>
    {/if}
  </section>

  <footer class="health-footer">
    Checked {new Date(report.checkedAt).toLocaleTimeString()}
  </footer>
</div>

<style>
  .session-health-panel {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    height: 100%;
    padding: var(--space-4);
    overflow: auto;
    color: var(--text-primary);
  }

  .health-header,
  .health-row-title {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
  }

  .health-eyebrow {
    display: block;
    color: var(--text-tertiary);
    font-family: var(--font-display);
    font-size: var(--text-xs);
    font-weight: 600;
    text-transform: uppercase;
  }

  .health-header h3,
  .health-section h4 {
    margin: 0;
    color: var(--text-primary);
    font-family: var(--font-display);
    font-weight: 600;
  }

  .health-header h3 {
    font-size: var(--text-lg);
  }

  .health-summary {
    margin: 0;
    color: var(--text-secondary);
    font-size: var(--text-sm);
    line-height: 1.5;
  }

  .health-metrics {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: var(--space-2);
  }

  .health-metrics div {
    min-width: 0;
    padding: var(--space-3);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--surface-base);
  }

  .health-metrics strong {
    display: block;
    color: var(--text-primary);
    font-family: var(--font-mono);
    font-size: var(--text-lg);
  }

  .health-metrics span,
  .health-footer {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
  }

  .health-section {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .health-section h4 {
    font-size: var(--text-sm);
  }

  .health-list {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .health-row {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    padding: var(--space-3);
    border: 1px solid var(--border-subtle);
    border-left-width: 3px;
    border-radius: var(--radius-md);
    background: var(--surface-base);
  }

  .health-row.severity-info {
    border-left-color: var(--info);
  }

  .health-row.severity-warning {
    border-left-color: var(--warning);
  }

  .health-row.severity-error,
  .health-row.severity-critical {
    border-left-color: var(--error);
  }

  .health-row-title strong {
    min-width: 0;
    overflow: hidden;
    color: var(--text-primary);
    font-size: var(--text-sm);
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .health-row-title span {
    flex-shrink: 0;
    color: var(--text-tertiary);
    font-family: var(--font-mono);
    font-size: 10px;
    text-transform: uppercase;
  }

  .health-row p {
    margin: 0;
    color: var(--text-secondary);
    font-size: var(--text-sm);
    line-height: 1.45;
  }

  .health-row button {
    align-self: flex-start;
  }

  .health-empty {
    padding: var(--space-4);
    border: 1px dashed var(--border-subtle);
    border-radius: var(--radius-md);
    color: var(--text-tertiary);
    font-size: var(--text-sm);
    text-align: center;
  }

  .health-empty.compact {
    padding: var(--space-3);
  }

  .health-footer {
    margin-top: auto;
    padding-top: var(--space-2);
    border-top: 1px solid var(--border-subtle);
  }
</style>
