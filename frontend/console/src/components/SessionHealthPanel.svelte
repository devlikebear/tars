<script lang="ts">
  import { t } from '../i18n'
  import type { SessionHealthAction, SessionHealthReport } from '../lib/sessionHealth'

  interface Props {
    report: SessionHealthReport
    loading?: boolean
    onRefresh?: () => void
    onAction?: (action: SessionHealthAction) => void
  }

  let { report, loading = false, onRefresh, onAction }: Props = $props()

  // Severity is typed, but an unknown value still renders as itself.
  function severityLabel(severity: string): string {
    const labels: Record<string, string> = $t.sessionHealth.severity
    return labels[severity] ?? severity
  }
</script>

<div class="session-health-panel">
  <header class="health-header">
    <div>
      <span class="health-eyebrow">{$t.sessionHealth.panel.eyebrow}</span>
      <h3>{$t.sessionHealth.status[report.status]}</h3>
    </div>
    <button type="button" class="btn btn-ghost btn-sm" disabled={loading} onclick={() => onRefresh?.()}>
      {loading ? $t.sessionHealth.panel.checking : $t.sessionHealth.panel.refresh}
    </button>
  </header>

  <p class="health-summary">{report.summary}</p>

  <section class="health-metrics" aria-label={$t.sessionHealth.panel.metricsAriaLabel}>
    <div>
      <strong>{report.metrics.messageCount}</strong>
      <span>{$t.sessionHealth.panel.metrics.messages}</span>
    </div>
    <div>
      <strong>{report.metrics.openTaskCount}</strong>
      <span>{$t.sessionHealth.panel.metrics.openTasks}</span>
    </div>
    <div>
      <strong>{report.metrics.highRiskToolCount}</strong>
      <span>{$t.sessionHealth.panel.metrics.riskTools}</span>
    </div>
    <div>
      <strong>{report.metrics.memoryCount}</strong>
      <span>{$t.sessionHealth.panel.metrics.memory}</span>
    </div>
  </section>

  {#if report.signals.length === 0}
    <div class="health-empty">{$t.sessionHealth.panel.noWarnings}</div>
  {:else}
    <section class="health-section" aria-label={$t.sessionHealth.panel.signalsAriaLabel}>
      <h4>{$t.sessionHealth.panel.signals}</h4>
      <div class="health-list">
        {#each report.signals as signal}
          <article class={`health-row severity-${signal.severity}`}>
            <div class="health-row-title">
              <strong>{signal.title}</strong>
              <span>{severityLabel(signal.severity)}</span>
            </div>
            <p>{signal.detail}</p>
          </article>
        {/each}
      </div>
    </section>
  {/if}

  <section class="health-section" aria-label={$t.sessionHealth.panel.recommendationsAriaLabel}>
    <h4>{$t.sessionHealth.panel.recommendations}</h4>
    {#if report.recommendations.length === 0}
      <div class="health-empty compact">{$t.sessionHealth.panel.noActionNeeded}</div>
    {:else}
      <div class="health-list">
        {#each report.recommendations as recommendation}
          <article class={`health-row severity-${recommendation.severity}`}>
            <div class="health-row-title">
              <strong>{recommendation.title}</strong>
              <span>{severityLabel(recommendation.severity)}</span>
            </div>
            <p>{recommendation.detail}</p>
            <button type="button" class="btn btn-ghost btn-sm" onclick={() => onAction?.(recommendation.action)}>
              {recommendation.actionLabel || $t.sessionHealth.actions[recommendation.action]}
            </button>
          </article>
        {/each}
      </div>
    {/if}
  </section>

  <footer class="health-footer">
    {$t.sessionHealth.panel.checkedAt(new Date(report.checkedAt).toLocaleTimeString())}
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
