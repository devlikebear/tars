<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { getPulseStatus, runPulseOnce, getPulseConfig } from '../lib/api'
  import type { PulseSnapshot, PulseTickOutcome, PulseConfigView } from '../lib/types'

  type SeverityGuideRow = {
    kind: string
    label: string
    info: string
    warn: string
    error: string
  }

  type RecentTickSummary = {
    total: number
    allClear: number
    signalTicks: PulseTickOutcome[]
    warningCount: number
    errorCount: number
    autofixCount: number
  }

  let snapshot: PulseSnapshot | null = $state(null)
  let config: PulseConfigView | null = $state(null)
  let loading = $state(true)
  let error = $state('')

  let running = $state(false)
  let runResult: PulseTickOutcome | null = $state(null)

  let refreshInterval: ReturnType<typeof setInterval> | null = null

  async function loadStatus() {
    try {
      snapshot = await getPulseStatus()
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load status'
    }
  }

  async function loadConfig() {
    try {
      config = await getPulseConfig()
    } catch { /* optional */ }
  }

  async function loadAll() {
    loading = true
    error = ''
    try {
      await Promise.all([loadStatus(), loadConfig()])
    } finally {
      loading = false
    }
  }

  async function handleRun() {
    running = true
    runResult = null
    try {
      runResult = await runPulseOnce()
      await loadStatus()
    } catch (e) {
      runResult = { at: '', err: e instanceof Error ? e.message : 'Run failed' }
    } finally {
      running = false
    }
  }

  function fmtTime(value?: string): string {
    if (!value?.trim()) return '\u2014'
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return value
    return new Intl.DateTimeFormat('en', { dateStyle: 'medium', timeStyle: 'short' }).format(date)
  }

  function relativeTime(value?: string): string {
    if (!value?.trim()) return 'never'
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return value
    if (date.getFullYear() <= 1) return 'never'
    const seconds = Math.floor((Date.now() - date.getTime()) / 1000)
    if (seconds < 60) return `${seconds}s ago`
    if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`
    if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`
    return `${Math.floor(seconds / 86400)}d ago`
  }

  function fmtPercent(value?: number): string {
    if (typeof value !== 'number' || !Number.isFinite(value)) return 'configured'
    return `${value.toFixed(value % 1 === 0 ? 0 : 1)}%`
  }

  function severityGuideRows(cfg: PulseConfigView | null): SeverityGuideRow[] {
    const cronThreshold = cfg?.cron_failure_threshold || 3
    const stuckMinutes = cfg?.stuck_run_minutes || 60
    const diskWarn = fmtPercent(cfg?.disk_warn_percent)
    const diskCritical = fmtPercent(cfg?.disk_critical_percent)

    return [
      {
        kind: 'cron_failures',
        label: 'Cron failures',
        info: 'Individual failures are counted on cron jobs.',
        warn: `${cronThreshold}+ consecutive failures on any job.`,
        error: `${cronThreshold * 2}+ consecutive failures on the worst job.`,
      },
      {
        kind: 'disk_usage',
        label: 'Disk pressure',
        info: 'Below the warn threshold no signal is emitted.',
        warn: `Disk usage reaches ${diskWarn}.`,
        error: `Disk usage reaches ${diskCritical}.`,
      },
      {
        kind: 'stuck_agentruntime_run',
        label: 'AgentRuntime stuck run',
        info: 'Running jobs younger than the stuck window are ignored.',
        warn: `Any run stays running for ${stuckMinutes}m+.`,
        error: 'Three or more runs are stuck at once.',
      },
      {
        kind: 'delivery_failures',
        label: 'Telegram delivery',
        info: 'Recent delivery failures are counted in the configured window.',
        warn: 'Failures reach the delivery threshold.',
        error: 'Failures reach 2x the delivery threshold.',
      },
      {
        kind: 'reflection_failure',
        label: 'Reflection health',
        info: 'Successful nightly runs reset the consecutive-failure count.',
        warn: 'Consecutive nightly failures reach the reflection threshold.',
        error: 'Failures reach 2x the reflection threshold.',
      },
    ]
  }

  function lastSignalSeen(kind: string): string {
    const recent = snapshot?.recent ?? []
    for (let i = recent.length - 1; i >= 0; i -= 1) {
      const tick = recent[i]
      const signal = tick.signals?.find((item) => item.kind === kind)
      if (signal) return relativeTime(signal.at || tick.at)
    }
    return 'never'
  }

  function isWarningSeverity(severity?: string): boolean {
    return severity === 'warn'
  }

  function isErrorSeverity(severity?: string): boolean {
    return severity === 'error' || severity === 'critical'
  }

  function signalCount(tick: PulseTickOutcome): number {
    return tick.signals?.length ?? 0
  }

  function tickHasSignal(tick: PulseTickOutcome): boolean {
    return signalCount(tick) > 0 ||
      Boolean(tick.decision || tick.err || tick.autofix_attempt || tick.autofix_ok || tick.autofix_err || tick.notify_delivered)
  }

  function tickHasWarning(tick: PulseTickOutcome): boolean {
    return isWarningSeverity(tick.decision?.severity) || Boolean(tick.signals?.some((signal) => isWarningSeverity(signal.severity)))
  }

  function tickHasError(tick: PulseTickOutcome): boolean {
    return Boolean(tick.err || tick.autofix_err) ||
      isErrorSeverity(tick.decision?.severity) ||
      Boolean(tick.signals?.some((signal) => isErrorSeverity(signal.severity)))
  }

  function tickBadgeLabel(tick: PulseTickOutcome): string {
    if (tick.err) return 'error'
    if (tick.autofix_attempt || tick.autofix_ok || tick.autofix_err || tick.decision?.action === 'autofix') return 'autofix'
    if (tick.notify_delivered) return 'notified'
    if (tick.decision) return tick.decision.action
    if (signalCount(tick) > 0) return `${signalCount(tick)} signal${signalCount(tick) > 1 ? 's' : ''}`
    if (tick.skipped) return 'skipped'
    return 'no signals'
  }

  function severityBadgeClass(severity?: string): string {
    if (isErrorSeverity(severity)) return 'badge-error'
    if (isWarningSeverity(severity)) return 'badge-warning'
    return 'badge-info'
  }

  function tickBadgeClass(tick: PulseTickOutcome): string {
    if (tickHasError(tick)) return 'badge-error'
    if (tickHasWarning(tick)) return 'badge-warning'
    if (tickHasSignal(tick)) return 'badge-info'
    return 'badge-default'
  }

  function recentTickSummary(ticks: PulseTickOutcome[] = []): RecentTickSummary {
    const summary: RecentTickSummary = {
      total: ticks.length,
      allClear: 0,
      signalTicks: [],
      warningCount: 0,
      errorCount: 0,
      autofixCount: 0,
    }

    for (const tick of [...ticks].reverse()) {
      if (tickHasSignal(tick)) {
        summary.signalTicks.push(tick)
      }
      if (tickHasWarning(tick)) {
        summary.warningCount += 1
      }
      if (tickHasError(tick)) {
        summary.errorCount += 1
      }
      if (tick.autofix_attempt || tick.autofix_ok || tick.autofix_err || tick.decision?.action === 'autofix') {
        summary.autofixCount += 1
      }
    }

    summary.allClear = summary.total - summary.signalTicks.length
    return summary
  }

  onMount(() => {
    void loadAll()
    refreshInterval = setInterval(loadStatus, 30000)
  })

  onDestroy(() => {
    if (refreshInterval) clearInterval(refreshInterval)
  })
</script>

<div class="pulse">
  {#if loading}
    <div class="pulse-loading">Loading pulse...</div>
  {:else}
    {#if error}
      <div class="error-banner" style="margin-bottom:var(--space-4)">{error}</div>
    {/if}

    <!-- Status summary -->
    <section class="card">
      <div class="card-header">
        <span class="card-title">Pulse Status</span>
        {#if config?.enabled}
          <span class="badge badge-success">enabled</span>
        {:else}
          <span class="badge badge-default">disabled</span>
        {/if}
      </div>
      <dl class="pulse-facts">
        <div><dt>Interval</dt><dd>{config ? `${config.interval_seconds}s` : '—'}</dd></div>
        <div><dt>Active Hours</dt><dd>{config?.active_hours || 'always'}</dd></div>
        <div><dt>Timezone</dt><dd>{config?.timezone || 'system'}</dd></div>
        <div>
          <dt>Min Severity</dt>
          <dd>
            <span class="badge badge-warning">{config?.min_severity || '—'}</span>
            <span class="pulse-min-note">Notifications below this floor are dropped after the decider classifies a tick.</span>
          </dd>
        </div>
        <div><dt>Last Tick</dt><dd>{relativeTime(snapshot?.last_tick_at)}</dd></div>
        <div><dt>Total Ticks</dt><dd>{snapshot?.total_ticks ?? 0}</dd></div>
        <div><dt>Decisions</dt><dd>{snapshot?.total_decisions ?? 0}</dd></div>
        <div><dt>Notifies</dt><dd>{snapshot?.total_notifies ?? 0}</dd></div>
        <div><dt>Autofixes</dt><dd>{snapshot?.total_autofixes ?? 0}</dd></div>
      </dl>

      <div class="pulse-severity-guide">
        <div class="pulse-guide-header">
          <strong>Min Severity guide</strong>
          <span>Signal thresholds use pulse_* config fields.</span>
        </div>
        <div class="pulse-guide-grid">
          {#each severityGuideRows(config) as row}
            <div class="pulse-guide-row">
              <div>
                <span class="pulse-guide-label">{row.label}</span>
                <code>{row.kind}</code>
              </div>
              <dl>
                <div><dt>info</dt><dd>{row.info}</dd></div>
                <div><dt>warn</dt><dd>{row.warn}</dd></div>
                <div><dt>error</dt><dd>{row.error}</dd></div>
              </dl>
            </div>
          {/each}
        </div>
        <div class="pulse-last-seen">
          <span class="pulse-last-seen-title">Last seen by signal</span>
          <div>
            {#each severityGuideRows(config) as row}
              <span><code>{row.kind}</code> {lastSignalSeen(row.kind)}</span>
            {/each}
          </div>
        </div>
      </div>
    </section>

    <!-- Last Decision + Run Action -->
    <section class="card">
      <div class="card-header">
        <span class="card-title">Last Decision</span>
        {#if snapshot?.last_decision}
          <span class="badge badge-info">{snapshot.last_decision.action}</span>
          <span class="badge badge-default">{snapshot.last_decision.severity}</span>
        {/if}
      </div>
      {#if snapshot?.last_err}
        <div class="pulse-error">{snapshot.last_err}</div>
      {/if}
      {#if snapshot?.last_decision}
        {#if snapshot.last_decision.title}
          <div class="pulse-title">{snapshot.last_decision.title}</div>
        {/if}
        {#if snapshot.last_decision.summary}
          <div class="pulse-response">{snapshot.last_decision.summary}</div>
        {/if}
      {:else}
        <div class="pulse-empty">No pulse decisions yet.</div>
      {/if}

      <div class="pulse-actions">
        <button class="btn btn-primary btn-sm" disabled={running || !config?.enabled} onclick={handleRun}>
          {running ? 'Running...' : 'Run Tick Now'}
        </button>
        {#if !config?.enabled}
          <span class="pulse-hint">Pulse is disabled in config</span>
        {/if}
      </div>

      {#if runResult}
        <div class="pulse-run-result">
          <div class="pulse-run-result-header">
            <strong>Tick result</strong>
            <div class="pulse-badges">
              {#if runResult.skipped}<span class="badge badge-warning">skipped</span>{/if}
              {#if runResult.decider_invoked}<span class="badge badge-info">decider ran</span>{/if}
              {#if runResult.notify_delivered}<span class="badge badge-success">notified</span>{/if}
              {#if runResult.autofix_ok}<span class="badge badge-success">autofix ok</span>{/if}
              {#if runResult.err}<span class="badge badge-error">error</span>{/if}
            </div>
          </div>
          {#if runResult.skip_reason}
            <div class="pulse-skip-reason">{runResult.skip_reason}</div>
          {/if}
          {#if runResult.err}
            <div class="pulse-error">{runResult.err}</div>
          {/if}
          {#if runResult.decision?.summary}
            <div class="pulse-response">{runResult.decision.summary}</div>
          {/if}
        </div>
      {/if}
    </section>

    <!-- Recent ticks -->
    {#if snapshot?.recent && snapshot.recent.length > 0}
      {@const recentSummary = recentTickSummary(snapshot.recent)}
      <section class="card">
        <div class="card-header">
          <span class="card-title">Recent Ticks</span>
          <span class="badge badge-default">{snapshot.recent.length}</span>
        </div>

        <div class="pulse-recent-summary">
          <div>
            <strong>Last {recentSummary.total} ticks</strong>
            <span>
              {recentSummary.signalTicks.length === 0
                ? 'all clear (no signals)'
                : `${recentSummary.signalTicks.length} signal tick${recentSummary.signalTicks.length > 1 ? 's' : ''}, ${recentSummary.allClear} all-clear`}
            </span>
          </div>
          <div class="pulse-recent-counters">
            <span class="badge badge-warning">{recentSummary.warningCount} warning{recentSummary.warningCount === 1 ? '' : 's'}</span>
            <span class="badge badge-error">{recentSummary.errorCount} error{recentSummary.errorCount === 1 ? '' : 's'}</span>
            <span class="badge badge-success">{recentSummary.autofixCount} autofix{recentSummary.autofixCount === 1 ? '' : 'es'}</span>
          </div>
        </div>

        <div class="pulse-recent-dots" aria-label="Recent tick timeline">
          {#each [...snapshot.recent].reverse() as tick}
            <span
              class:has-signal={tickHasSignal(tick)}
              class:has-warning={tickHasWarning(tick)}
              class:has-error={tickHasError(tick)}
              title={`${fmtTime(tick.at)} - ${tickBadgeLabel(tick)}`}
              aria-label={`${fmtTime(tick.at)} ${tickBadgeLabel(tick)}`}
            ></span>
          {/each}
        </div>

        {#if recentSummary.signalTicks.length > 0}
          <div class="pulse-signal-ticks">
            <div class="pulse-signal-heading">Signal ticks</div>
            <ul class="pulse-recent">
              {#each recentSummary.signalTicks as tick}
                <li>
                  <div class="pulse-recent-row">
                    <span class="pulse-recent-time">{fmtTime(tick.at)}</span>
                    <span class="badge {tickBadgeClass(tick)}">{tickBadgeLabel(tick)}</span>
                    {#if signalCount(tick) > 0}
                      <span class="badge badge-default">{signalCount(tick)} signal{signalCount(tick) > 1 ? 's' : ''}</span>
                    {/if}
                    {#if tick.notify_delivered}
                      <span class="badge badge-success">notified</span>
                    {/if}
                  </div>
                  {#if tick.decision?.title}
                    <div class="pulse-recent-title">{tick.decision.title}</div>
                  {:else if tick.err}
                    <div class="pulse-recent-title">{tick.err}</div>
                  {/if}
                  {#if tick.decision?.summary}
                    <div class="pulse-recent-detail">{tick.decision.summary}</div>
                  {/if}
                  {#if tick.signals?.length}
                    <ul class="pulse-signal-list">
                      {#each tick.signals as signal}
                        <li>
                          <span class="pulse-signal-kind">{signal.kind}</span>
                          <span class="badge {severityBadgeClass(signal.severity)}">{signal.severity}</span>
                          <span class="pulse-signal-summary">{signal.summary}</span>
                        </li>
                      {/each}
                    </ul>
                  {/if}
                </li>
              {/each}
            </ul>
          </div>
        {/if}
      </section>
    {/if}
  {/if}
</div>

<style>
  .pulse {
    display: grid;
    gap: var(--space-4);
    animation: fadeIn var(--duration-normal) var(--ease-out);
  }

  @keyframes fadeIn {
    from { opacity: 0; transform: translateY(8px); }
    to { opacity: 1; transform: translateY(0); }
  }

  .pulse-loading {
    padding: var(--space-10);
    text-align: center;
    color: var(--text-tertiary);
  }

  .pulse-facts {
    display: grid;
    gap: var(--space-2);
    margin-top: var(--space-3);
  }

  .pulse-facts div {
    display: grid;
    grid-template-columns: 140px minmax(0, 1fr);
    gap: var(--space-3);
  }

  .pulse-facts dt {
    color: var(--text-tertiary);
    font-size: var(--text-sm);
  }

  .pulse-facts dd {
    margin: 0;
    font-size: var(--text-sm);
  }

  .pulse-facts dd .badge {
    margin-right: var(--space-2);
  }

  .pulse-min-note {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
  }

  .pulse-severity-guide {
    display: grid;
    gap: var(--space-3);
    margin-top: var(--space-4);
    padding-top: var(--space-3);
    border-top: 1px solid var(--border-subtle);
  }

  .pulse-guide-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    color: var(--text-secondary);
  }

  .pulse-guide-header strong {
    color: var(--text-primary);
    font-family: var(--font-display);
    font-size: var(--text-sm);
    font-weight: 500;
  }

  .pulse-guide-header span {
    color: var(--text-ghost);
    font-size: var(--text-xs);
  }

  .pulse-guide-grid {
    display: grid;
    gap: var(--space-2);
  }

  .pulse-guide-row {
    display: grid;
    grid-template-columns: minmax(150px, 0.7fr) minmax(0, 1.3fr);
    gap: var(--space-3);
    padding-top: var(--space-2);
    border-top: 1px solid var(--border-subtle);
  }

  .pulse-guide-label {
    display: block;
    margin-bottom: var(--space-1);
    color: var(--text-primary);
    font-size: var(--text-sm);
    font-weight: 500;
  }

  .pulse-guide-row code,
  .pulse-last-seen code {
    color: var(--text-tertiary);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
  }

  .pulse-guide-row dl {
    display: grid;
    gap: var(--space-1);
    margin: 0;
  }

  .pulse-guide-row dl div {
    display: grid;
    grid-template-columns: 44px minmax(0, 1fr);
    gap: var(--space-2);
  }

  .pulse-guide-row dt {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
    font-weight: 600;
    text-transform: uppercase;
  }

  .pulse-guide-row dd {
    margin: 0;
    color: var(--text-secondary);
    font-size: var(--text-sm);
    line-height: 1.45;
  }

  .pulse-last-seen {
    display: grid;
    gap: var(--space-2);
    padding-top: var(--space-2);
    border-top: 1px solid var(--border-subtle);
  }

  .pulse-last-seen-title {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
    font-weight: 600;
    text-transform: uppercase;
  }

  .pulse-last-seen div {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }

  .pulse-last-seen span:not(.pulse-last-seen-title) {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    padding: var(--space-1) var(--space-2);
    color: var(--text-secondary);
    background: var(--surface);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    font-size: var(--text-xs);
  }

  .pulse-badges {
    display: flex;
    gap: var(--space-1);
  }

  .pulse-title {
    font-weight: 500;
    margin-top: var(--space-2);
    color: var(--text-primary);
  }

  .pulse-response {
    padding: var(--space-3);
    background: var(--surface-base);
    border-radius: var(--radius-md);
    font-size: var(--text-sm);
    line-height: 1.6;
    white-space: pre-wrap;
    color: var(--text-secondary);
    margin-top: var(--space-2);
  }

  .pulse-error {
    padding: var(--space-2) var(--space-3);
    background: var(--error-muted);
    border-radius: var(--radius-md);
    font-size: var(--text-sm);
    color: var(--error);
    margin-top: var(--space-2);
  }

  .pulse-skip-reason {
    font-size: var(--text-xs);
    color: var(--text-ghost);
    margin-top: var(--space-1);
  }

  .pulse-empty {
    padding: var(--space-3);
    color: var(--text-ghost);
    font-size: var(--text-sm);
  }

  .pulse-actions {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    margin-top: var(--space-3);
  }

  .pulse-hint {
    font-size: var(--text-xs);
    color: var(--text-ghost);
  }

  .pulse-run-result {
    margin-top: var(--space-3);
    padding: var(--space-3);
    background: rgba(224, 145, 69, 0.06);
    border: 1px solid rgba(224, 145, 69, 0.12);
    border-radius: var(--radius-md);
  }

  .pulse-run-result-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: var(--space-2);
  }

  .pulse-run-result-header strong {
    font-family: var(--font-display);
    font-size: var(--text-sm);
    font-weight: 500;
  }

  .pulse-recent {
    list-style: none;
    padding: 0;
    margin: var(--space-3) 0 0;
    display: grid;
    gap: var(--space-2);
  }

  .pulse-recent-summary {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-3);
    margin-top: var(--space-3);
    padding-top: var(--space-3);
    border-top: 1px solid var(--border-subtle);
  }

  .pulse-recent-summary > div:first-child {
    display: grid;
    gap: var(--space-1);
  }

  .pulse-recent-summary strong,
  .pulse-signal-heading {
    color: var(--text-primary);
    font-family: var(--font-display);
    font-size: var(--text-sm);
    font-weight: 500;
  }

  .pulse-recent-summary span:not(.badge) {
    color: var(--text-tertiary);
    font-size: var(--text-sm);
  }

  .pulse-recent-counters {
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: var(--space-1);
  }

  .pulse-recent-dots {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-1);
    margin-top: var(--space-3);
    padding: var(--space-2) 0;
  }

  .pulse-recent-dots span {
    width: 10px;
    height: 10px;
    border-radius: 999px;
    background: var(--border-default);
    opacity: 0.7;
    transform: scale(0.75);
  }

  .pulse-recent-dots span.has-signal {
    background: var(--accent);
    opacity: 1;
    transform: scale(1);
  }

  .pulse-recent-dots span.has-warning {
    background: var(--warning);
  }

  .pulse-recent-dots span.has-error {
    background: var(--error);
  }

  .pulse-signal-ticks {
    display: grid;
    gap: var(--space-1);
    margin-top: var(--space-2);
    padding-top: var(--space-2);
    border-top: 1px solid var(--border-subtle);
  }

  .pulse-recent li {
    padding: var(--space-2) var(--space-3);
    background: var(--surface-base);
    border-radius: var(--radius-sm);
  }

  .pulse-recent-row {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .pulse-recent-time {
    font-size: var(--text-xs);
    color: var(--text-tertiary);
    font-family: var(--font-mono);
  }

  .pulse-recent-title {
    margin-top: var(--space-1);
    font-size: var(--text-sm);
    color: var(--text-secondary);
  }

  .pulse-recent-detail {
    margin-top: var(--space-1);
    color: var(--text-tertiary);
    font-size: var(--text-xs);
    line-height: 1.5;
  }

  .pulse-signal-list {
    display: grid;
    gap: var(--space-1);
    margin: var(--space-2) 0 0;
    padding: 0;
    list-style: none;
  }

  .pulse-signal-list li {
    display: grid;
    grid-template-columns: minmax(130px, 0.5fr) auto minmax(0, 1fr);
    gap: var(--space-2);
    align-items: center;
    padding: var(--space-1) 0 0;
    background: transparent;
    border-top: 1px solid var(--border-subtle);
    border-radius: 0;
  }

  .pulse-signal-kind {
    color: var(--text-secondary);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
  }

  .pulse-signal-summary {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
    line-height: 1.45;
  }
</style>
