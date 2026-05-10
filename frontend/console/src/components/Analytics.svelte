<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { getAnalytics, getCodexUsage, streamEvents } from '../lib/api'
  import type { AnalyticsDailyRow, AnalyticsModelRow, AnalyticsResponse, AnalyticsSkillRow, CodexUsageTier } from '../lib/types'
  import { formatResetCountdown } from '../lib/formatDuration'
  import { t } from '../i18n'

  const dayOptions = [7, 30, 90]
  const chartWidth = 720
  const chartHeight = 220
  const chartBase = 174
  const chartTop = 24
  const chartPlotHeight = chartBase - chartTop

  let analytics = $state<AnalyticsResponse | null>(null)
  let selectedDays = $state(7)
  let loading = $state(true)
  let error = $state('')
  let codexTiers: CodexUsageTier[] = $state([])
  let codexLoading = $state(false)
  let codexError = $state('')
  let codexTimer: ReturnType<typeof setInterval> | null = null
  let stopCodexStream: (() => void) | null = null

  let daily: AnalyticsDailyRow[] = $derived.by(() => analytics?.daily ?? [])
  let models: AnalyticsModelRow[] = $derived.by(() => analytics?.models ?? [])
  let skills: AnalyticsSkillRow[] = $derived.by(() => analytics?.skills ?? [])
  let hasUsage = $derived((analytics?.totals.total_tokens ?? 0) > 0)
  let maxDailyTokens = $derived.by(() => Math.max(1, ...daily.map((row) => row.total_tokens)))

  export async function loadAnalytics(days = selectedDays) {
    loading = true
    error = ''
    try {
      analytics = await getAnalytics(days)
      selectedDays = analytics.days || days
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load analytics'
    } finally {
      loading = false
    }
  }

  function selectDays(days: number) {
    selectedDays = days
    void loadAnalytics(days)
  }

  function fmtInt(value?: number): string {
    return new Intl.NumberFormat('en').format(Math.round(value ?? 0))
  }

  function fmtCost(value?: number): string {
    return `$${(value ?? 0).toFixed(4)}`
  }

  function fmtAvg(value?: number): string {
    return fmtInt(value ?? 0)
  }

  function shortDay(value: string): string {
    const date = new Date(`${value}T00:00:00Z`)
    if (Number.isNaN(date.getTime())) return value
    return new Intl.DateTimeFormat('en', { month: 'short', day: '2-digit', timeZone: 'UTC' }).format(date)
  }

  function barGap(): number {
    return daily.length > 30 ? 2 : 4
  }

  function barWidth(): number {
    if (daily.length === 0) return 0
    return Math.max(2, (chartWidth - 48 - barGap() * (daily.length - 1)) / daily.length)
  }

  function barX(index: number): number {
    return 24 + index * (barWidth() + barGap())
  }

  function barHeight(tokens: number): number {
    return Math.max(tokens > 0 ? 2 : 0, (tokens / maxDailyTokens) * chartPlotHeight)
  }

  function segmentHeight(tokens: number, total: number, totalHeight: number): number {
    if (tokens <= 0 || total <= 0) return 0
    return Math.max(1, (tokens / total) * totalHeight)
  }

  async function refreshCodexUsage() {
    codexLoading = true
    try {
      const res = await getCodexUsage()
      codexTiers = (res.tiers ?? []).filter((tier) => (tier.provider ?? '').toLowerCase() === 'openai-codex')
      codexError = ''
    } catch (err) {
      codexError = err instanceof Error ? err.message : 'failed to load codex quota'
    } finally {
      codexLoading = false
    }
  }

  function bandClass(usedPercent: number | undefined): string {
    if (usedPercent === undefined) return ''
    if (usedPercent >= 95) return 'codex-row-critical'
    if (usedPercent >= 90) return 'codex-row-warn'
    return ''
  }

  onMount(() => {
    void loadAnalytics()
    void refreshCodexUsage()
    codexTimer = setInterval(() => { void refreshCodexUsage() }, 60_000)
    stopCodexStream = streamEvents((event) => {
      if (event.category === 'codex_quota') {
        void refreshCodexUsage()
      }
    })
  })

  onDestroy(() => {
    stopCodexStream?.()
    if (codexTimer !== null) {
      clearInterval(codexTimer)
      codexTimer = null
    }
  })
</script>

<div class="analytics-page">
  <section class="analytics-header">
    <div>
      <span class="analytics-kicker">{$t.analytics.kicker}</span>
      <h2>{$t.analytics.title}</h2>
    </div>
    <div class="period-toggle" aria-label={$t.analytics.periodAriaLabel}>
      {#each dayOptions as days}
        <button
          type="button"
          class:active={selectedDays === days}
          onclick={() => selectDays(days)}
        >
          {days}{$t.analytics.periodSuffix}
        </button>
      {/each}
    </div>
  </section>

  {#if error}
    <div class="error-banner">{error}</div>
  {/if}

  <section class="summary-grid">
    <div class="summary-card card">
      <span>{$t.analytics.summary.totalTokens}</span>
      <strong>{fmtInt(analytics?.totals.total_tokens)}</strong>
      <small>{$t.analytics.summary.tokensInOut(fmtInt(analytics?.totals.input_tokens), fmtInt(analytics?.totals.output_tokens))}</small>
    </div>
    <div class="summary-card card">
      <span>{$t.analytics.summary.sessions}</span>
      <strong>{fmtInt(analytics?.totals.sessions)}</strong>
      <small>{$t.analytics.summary.callsSuffix(fmtInt(analytics?.totals.calls))}</small>
    </div>
    <div class="summary-card card">
      <span>{$t.analytics.summary.avgPerSession}</span>
      <strong>{fmtAvg(analytics?.totals.avg_tokens_per_session)}</strong>
      <small>{$t.analytics.summary.tokensSuffix}</small>
    </div>
    <div class="summary-card card">
      <span>{$t.analytics.summary.estimatedCost}</span>
      <strong>{fmtCost(analytics?.totals.cost_usd)}</strong>
      <small>{$t.analytics.summary.daysSuffix(selectedDays)}</small>
    </div>
  </section>

  {#if codexTiers.length > 0 || codexLoading || codexError}
    <section class="card codex-quota-section">
      <div class="card-header">
        <span class="card-title">Codex subscription quota</span>
        <div class="card-header-actions">
          <button type="button" class="btn btn-ghost btn-sm" disabled={codexLoading} onclick={() => { void refreshCodexUsage() }}>
            {codexLoading ? 'Refreshing…' : 'Refresh'}
          </button>
        </div>
      </div>
      {#if codexError}
        <div class="error-banner">{codexError}</div>
      {/if}
      {#if codexTiers.length === 0 && !codexLoading && !codexError}
        <div class="empty-state">No openai-codex tiers configured.</div>
      {:else}
        <div class="codex-list">
          {#each codexTiers as tier}
            <div class="codex-tier {bandClass(tier.snapshot?.primary?.used_percent)}">
              <div class="codex-tier-head">
                <span class="codex-tier-label">{tier.tier}</span>
                <span class="codex-tier-meta">{tier.provider ?? ''}{tier.model ? ` · ${tier.model}` : ''}</span>
              </div>
              {#if !tier.snapshot}
                <div class="codex-tier-empty">Awaiting first request…</div>
              {:else}
                <div class="codex-windows">
                  {#if tier.snapshot.primary}
                    <div class="codex-window {bandClass(tier.snapshot.primary.used_percent)}">
                      <span class="codex-window-label">5h window</span>
                      <span class="codex-window-pct">{tier.snapshot.primary.used_percent.toFixed(1)}%</span>
                      {#if tier.snapshot.primary.reset_after_seconds !== undefined}
                        <span class="codex-window-reset">resets in {formatResetCountdown(tier.snapshot.primary.reset_after_seconds)}</span>
                      {/if}
                    </div>
                  {/if}
                  {#if tier.snapshot.secondary}
                    <div class="codex-window {bandClass(tier.snapshot.secondary.used_percent)}">
                      <span class="codex-window-label">weekly</span>
                      <span class="codex-window-pct">{tier.snapshot.secondary.used_percent.toFixed(1)}%</span>
                      {#if tier.snapshot.secondary.reset_after_seconds !== undefined}
                        <span class="codex-window-reset">resets in {formatResetCountdown(tier.snapshot.secondary.reset_after_seconds)}</span>
                      {/if}
                    </div>
                  {/if}
                </div>
              {/if}
            </div>
          {/each}
        </div>
      {/if}
    </section>
  {/if}

  <section class="analytics-chart card">
    <div class="card-header">
      <span class="card-title">{$t.analytics.chart.title}</span>
      <div class="chart-legend">
        <span><i class="legend-input"></i>{$t.analytics.chart.legendInput}</span>
        <span><i class="legend-output"></i>{$t.analytics.chart.legendOutput}</span>
      </div>
    </div>

    {#if loading && !analytics}
      <div class="empty-state">{$t.analytics.chart.loading}</div>
    {:else if !hasUsage}
      <div class="empty-state">
        <strong>{$t.analytics.chart.emptyTitle}</strong>
        <p>{$t.analytics.chart.emptyBody}</p>
      </div>
    {:else}
      <svg viewBox={`0 0 ${chartWidth} ${chartHeight}`} role="img" aria-label={$t.analytics.chart.ariaLabel}>
        <line x1="24" y1={chartBase} x2={chartWidth - 24} y2={chartBase} class="axis" />
        {#each daily as row, index}
          {@const totalHeight = barHeight(row.total_tokens)}
          {@const inputHeight = segmentHeight(row.input_tokens, row.total_tokens, totalHeight)}
          {@const outputHeight = segmentHeight(row.output_tokens, row.total_tokens, totalHeight)}
          {@const x = barX(index)}
          {@const w = barWidth()}
          <g>
            <title>{$t.analytics.chart.barTitle(row.day, fmtInt(row.input_tokens), fmtInt(row.output_tokens))}</title>
            <rect
              class="input-bar"
              x={x}
              y={chartBase - inputHeight}
              width={w}
              height={inputHeight}
              rx="3"
            />
            <rect
              class="output-bar"
              x={x}
              y={chartBase - inputHeight - outputHeight}
              width={w}
              height={outputHeight}
              rx="3"
            />
          </g>
          {#if selectedDays === 7 || index === 0 || index === daily.length - 1}
            <text x={x + w / 2} y="202" text-anchor="middle">{shortDay(row.day)}</text>
          {/if}
        {/each}
      </svg>
    {/if}
  </section>

  <div class="analytics-tables">
    <section class="card">
      <div class="card-header">
        <span class="card-title">{$t.analytics.models.title}</span>
        <span class="badge badge-default">{models.length}</span>
      </div>
      {#if models.length === 0}
        <div class="empty-state">{$t.analytics.models.empty}</div>
      {:else}
        <div class="table-wrap">
          <table>
            <thead>
              <tr>
                <th>{$t.analytics.models.thModel}</th>
                <th>{$t.analytics.models.thSessions}</th>
                <th>{$t.analytics.models.thInput}</th>
                <th>{$t.analytics.models.thOutput}</th>
                <th>{$t.analytics.models.thCost}</th>
              </tr>
            </thead>
            <tbody>
              {#each models as row}
                <tr>
                  <td>
                    <strong>{row.model}</strong>
                    <span>{row.provider}</span>
                  </td>
                  <td>{fmtInt(row.sessions)}</td>
                  <td>{fmtInt(row.input_tokens)}</td>
                  <td>{fmtInt(row.output_tokens)}</td>
                  <td>{fmtCost(row.cost_usd)}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </section>

    <section class="card">
      <div class="card-header">
        <span class="card-title">{$t.analytics.skills.title}</span>
        <span class="badge badge-default">{skills.length}</span>
      </div>
      {#if skills.length === 0}
        <div class="empty-state">{$t.analytics.skills.empty}</div>
      {:else}
        <div class="skill-list">
          {#each skills as row}
            <div class="skill-row">
              <span>
                <strong>{row.name}</strong>
                {#if row.source}<small>{row.source}</small>{/if}
              </span>
              <span class="badge badge-info">{$t.analytics.skills.callsSuffix(fmtInt(row.calls))}</span>
            </div>
          {/each}
        </div>
      {/if}
    </section>
  </div>
</div>

<style>
  .analytics-page {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .analytics-header {
    display: flex;
    align-items: flex-end;
    justify-content: space-between;
    gap: var(--space-4);
  }

  .analytics-kicker {
    display: block;
    margin-bottom: var(--space-1);
    font-family: var(--font-display);
    font-size: var(--text-xs);
    color: var(--text-tertiary);
    letter-spacing: 0;
    text-transform: uppercase;
  }

  .period-toggle {
    display: inline-flex;
    gap: 2px;
    padding: 3px;
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    background: var(--surface-inset);
  }

  .period-toggle button {
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-secondary);
    padding: 6px 10px;
    cursor: pointer;
    font-family: var(--font-display);
    font-size: var(--text-xs);
  }

  .period-toggle button.active {
    background: var(--primary-muted);
    color: var(--primary-text);
  }

  .summary-grid {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: var(--space-4);
  }

  .summary-card {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }

  .summary-card span,
  .summary-card small {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
  }

  .summary-card strong {
    font-family: var(--font-display);
    font-size: var(--text-xl);
    font-weight: 500;
  }

  .chart-legend {
    display: flex;
    gap: var(--space-3);
    color: var(--text-secondary);
    font-size: var(--text-xs);
  }

  .chart-legend span {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
  }

  .chart-legend i {
    width: 10px;
    height: 10px;
    border-radius: var(--radius-sm);
  }

  .legend-input {
    background: var(--primary);
  }

  .legend-output {
    background: var(--success);
  }

  svg {
    display: block;
    width: 100%;
    min-height: 220px;
  }

  .axis {
    stroke: var(--border-default);
    stroke-width: 1;
  }

  .input-bar {
    fill: var(--primary);
  }

  .output-bar {
    fill: var(--success);
  }

  text {
    fill: var(--text-tertiary);
    font-family: var(--font-mono);
    font-size: 10px;
  }

  .analytics-tables {
    display: grid;
    grid-template-columns: minmax(0, 1.35fr) minmax(260px, 0.65fr);
    gap: var(--space-4);
    align-items: start;
  }

  .table-wrap {
    overflow-x: auto;
  }

  table {
    width: 100%;
    border-collapse: collapse;
  }

  th,
  td {
    padding: var(--space-2) 0;
    border-bottom: 1px solid var(--border-subtle);
    text-align: right;
    font-size: var(--text-sm);
  }

  th {
    color: var(--text-tertiary);
    font-family: var(--font-display);
    font-size: var(--text-xs);
    font-weight: 500;
  }

  th:first-child,
  td:first-child {
    text-align: left;
  }

  td:first-child {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  td span,
  .skill-row small {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
  }

  .skill-list {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .skill-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    padding: var(--space-3);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--surface-inset);
  }

  .skill-row > span:first-child {
    display: flex;
    min-width: 0;
    flex-direction: column;
    gap: 2px;
  }

  .skill-row strong {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  @media (max-width: 980px) {
    .analytics-header {
      align-items: flex-start;
      flex-direction: column;
    }

    .summary-grid,
    .analytics-tables {
      grid-template-columns: 1fr;
    }
  }

  .codex-quota-section {
    margin-bottom: var(--space-6);
  }

  .codex-list {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .codex-tier {
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    padding: var(--space-3) var(--space-4);
    background: var(--surface);
    transition: border-color var(--duration-fast) var(--ease-out), background var(--duration-fast) var(--ease-out);
  }

  .codex-tier.codex-row-warn {
    border-color: var(--warning);
    background: var(--warning-muted);
  }

  .codex-tier.codex-row-critical {
    border-color: var(--danger, #ef4444);
    background: rgba(239, 68, 68, 0.1);
  }

  .codex-tier-head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: var(--space-3);
    margin-bottom: var(--space-2);
  }

  .codex-tier-label {
    font-weight: 600;
    text-transform: uppercase;
    font-size: var(--text-xs);
    letter-spacing: 0.04em;
    color: var(--primary-text);
  }

  .codex-tier-meta {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
    font-family: var(--font-mono);
  }

  .codex-tier-empty {
    color: var(--text-tertiary);
    font-style: italic;
    font-size: var(--text-sm);
  }

  .codex-windows {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
    gap: var(--space-3);
  }

  .codex-window {
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-sm);
    background: var(--surface-subtle, rgba(255, 255, 255, 0.03));
  }

  .codex-window.codex-row-warn {
    background: var(--warning-muted);
  }

  .codex-window.codex-row-critical {
    background: rgba(239, 68, 68, 0.18);
  }

  .codex-window-label {
    font-size: var(--text-xs);
    color: var(--text-tertiary);
    text-transform: uppercase;
    letter-spacing: 0.03em;
  }

  .codex-window-pct {
    font-size: var(--text-lg);
    font-weight: 600;
    font-family: var(--font-mono);
  }

  .codex-window-reset {
    font-size: var(--text-xs);
    color: var(--text-secondary);
  }
</style>
