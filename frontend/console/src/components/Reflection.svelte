<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { getReflectionStatus, runReflectionOnce, getReflectionConfig } from '../lib/api'
  import type { ReflectionSnapshot, ReflectionRunSummary, ReflectionConfigView, ReflectionJobResult } from '../lib/types'
  import { t } from '../i18n'

  type RunMetric = {
    label: string
    value: string
    delta: string
  }

  let snapshot: ReflectionSnapshot | null = $state(null)
  let config: ReflectionConfigView | null = $state(null)
  let loading = $state(true)
  let error = $state('')

  let running = $state(false)
  let runResult: ReflectionRunSummary | null = $state(null)
  let lastRunBeforeManualRun: ReflectionRunSummary | null = $state(null)

  let refreshInterval: ReturnType<typeof setInterval> | null = null

  async function loadStatus() {
    try {
      snapshot = await getReflectionStatus()
    } catch (e) {
      error = e instanceof Error ? e.message : $t.reflection.errorLoadStatus
    }
  }

  async function loadConfig() {
    try {
      config = await getReflectionConfig()
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
    lastRunBeforeManualRun = snapshot?.last_run_summary ?? null
    try {
      runResult = await runReflectionOnce()
      await loadStatus()
    } catch (e) {
      runResult = {
        started_at: '',
        finished_at: '',
        results: [],
        success: false,
        err: e instanceof Error ? e.message : $t.reflection.errorRunFailed,
      }
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
    if (!value?.trim()) return $t.reflection.relativeTime.never
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return value
    if (date.getFullYear() <= 1) return $t.reflection.relativeTime.never
    const seconds = Math.floor((Date.now() - date.getTime()) / 1000)
    if (seconds < 60) return $t.reflection.relativeTime.secondsAgo(seconds)
    if (seconds < 3600) return $t.reflection.relativeTime.minutesAgo(Math.floor(seconds / 60))
    if (seconds < 86400) return $t.reflection.relativeTime.hoursAgo(Math.floor(seconds / 3600))
    return $t.reflection.relativeTime.daysAgo(Math.floor(seconds / 86400))
  }

  function fmtDuration(durationNs: number): string {
    if (!durationNs || durationNs < 0) return '\u2014'
    if (durationNs < 1000) return `${Math.round(durationNs / 1e6)}ms`
    const seconds = durationNs / 1e9
    if (seconds < 60) return `${seconds.toFixed(1)}s`
    return `${(seconds / 60).toFixed(1)}m`
  }

  function summaryDuration(summary: ReflectionRunSummary): string {
    const started = Date.parse(summary.started_at)
    const finished = Date.parse(summary.finished_at)
    if (Number.isNaN(started) || Number.isNaN(finished) || finished <= started) return '\u2014'
    return fmtDuration((finished - started) * 1e6)
  }

  function findJob(summary: ReflectionRunSummary | null, name: string): ReflectionJobResult | null {
    return summary?.results?.find((job) => job.name === name) ?? null
  }

  function detailNumber(job: ReflectionJobResult | null, key: string): number | null {
    const value = job?.details?.[key]
    if (typeof value === 'number' && Number.isFinite(value)) return value
    if (typeof value === 'string') {
      const parsed = Number(value)
      if (Number.isFinite(parsed)) return parsed
    }
    return null
  }

  function firstDetailNumber(job: ReflectionJobResult | null, keys: string[]): number | null {
    for (const key of keys) {
      const value = detailNumber(job, key)
      if (value !== null) return value
    }
    return null
  }

  function kbEntriesCompiled(job: ReflectionJobResult | null): number | null {
    if (!job) return null
    return firstDetailNumber(job, ['kb_entries_compiled', 'knowledge_entries_compiled', 'entries_compiled'])
  }

  function metricValue(value: number | null, plus = false): string {
    if (value === null) return $t.reflection.metrics.notReported
    if (plus && value > 0) return `+${value}`
    return `${value}`
  }

  function metricDelta(current: number | null, previous: number | null, hasPrevious: boolean): string {
    if (current === null) return $t.reflection.metrics.notReported
    if (!hasPrevious) return $t.reflection.metrics.firstRun
    if (previous === null) return $t.reflection.metrics.noPreviousValue
    const delta = current - previous
    if (delta === 0) return $t.reflection.metrics.sameAsLastRun
    return `${delta > 0 ? '+' : ''}${delta} vs last run`
  }

  function runMetrics(summary: ReflectionRunSummary, previous: ReflectionRunSummary | null): RunMetric[] {
    const memoryJob = findJob(summary, 'memory')
    const previousMemoryJob = findJob(previous, 'memory')
    const cleanupJob = findJob(summary, 'kb_cleanup')
    const previousCleanupJob = findJob(previous, 'kb_cleanup')
    const hasPrevious = Boolean(previous)

    const experiences = detailNumber(memoryJob, 'experiences_added')
    const previousExperiences = detailNumber(previousMemoryJob, 'experiences_added')
    const removed = detailNumber(cleanupJob, 'removed_count')
    const previousRemoved = detailNumber(previousCleanupJob, 'removed_count')
    const compiled = kbEntriesCompiled(memoryJob)
    const previousCompiled = kbEntriesCompiled(previousMemoryJob)

    return [
      {
        label: $t.reflection.metrics.experiencesLabel,
        value: metricValue(experiences, true),
        delta: metricDelta(experiences, previousExperiences, hasPrevious),
      },
      {
        label: $t.reflection.metrics.removedLabel,
        value: metricValue(removed),
        delta: metricDelta(removed, previousRemoved, hasPrevious),
      },
      {
        label: $t.reflection.metrics.compiledLabel,
        value: metricValue(compiled),
        delta: metricDelta(compiled, previousCompiled, hasPrevious),
      },
    ]
  }

  function jobDetailLine(job: ReflectionJobResult): string {
    if (job.name === 'memory') {
      const sessions = detailNumber(job, 'sessions_scanned')
      const turns = detailNumber(job, 'turns_processed')
      const experiences = detailNumber(job, 'experiences_added')
      const compiled = kbEntriesCompiled(job)
      const parts = [
        sessions !== null ? $t.reflection.jobDetails.sessionsScanned(sessions) : '',
        turns !== null ? $t.reflection.jobDetails.turnsProcessed(turns) : '',
        experiences !== null ? $t.reflection.jobDetails.experiencesExtracted(experiences) : '',
        compiled !== null ? $t.reflection.jobDetails.kbEntriesCompiled(compiled) : '',
      ].filter(Boolean)
      return parts.join(' · ')
    }
    if (job.name === 'kb_cleanup') {
      const removed = detailNumber(job, 'removed_count')
      const skipped = detailNumber(job, 'skipped_count')
      const parts = [
        removed !== null ? $t.reflection.jobDetails.emptySessionsRemoved(removed) : '',
        skipped !== null ? $t.reflection.jobDetails.skipped(skipped) : '',
      ].filter(Boolean)
      return parts.join(' · ')
    }
    return ''
  }

  function hoursFromSeconds(seconds?: number): string {
    if (!seconds || seconds <= 0) return '\u2014'
    const hours = seconds / 3600
    if (hours < 1) return `${Math.round(seconds / 60)}m`
    return `${hours.toFixed(0)}h`
  }

  onMount(() => {
    void loadAll()
    refreshInterval = setInterval(loadStatus, 30000)
  })

  onDestroy(() => {
    if (refreshInterval) clearInterval(refreshInterval)
  })
</script>

<div class="reflection">
  {#if loading}
    <div class="r-loading">{$t.reflection.loading}</div>
  {:else}
    {#if error}
      <div class="error-banner" style="margin-bottom:var(--space-4)">{error}</div>
    {/if}

    <section class="card r-intro-card">
      <div class="card-header">
        <span class="card-title">{$t.reflection.introTitle}</span>
        <span class="badge badge-accent">{$t.reflection.systemSurfaceBadge}</span>
      </div>
      <p class="r-intro-lead">
        {$t.reflection.introLead(config?.sleep_window || '02:00-05:00')}
      </p>
      <div class="r-intro-grid" aria-label="Reflection maintenance jobs">
        <div class="r-intro-job">
          <span class="r-job-name">{$t.reflection.jobs.memoryLabel}</span>
          <span>{$t.reflection.jobs.memoryDesc(config ? `${config.memory_lookback_hours}h` : '24h')}</span>
        </div>
        <div class="r-intro-job">
          <span class="r-job-name">{$t.reflection.jobs.cleanupLabel}</span>
          <span>{$t.reflection.jobs.cleanupDesc(config ? hoursFromSeconds(config.empty_session_age_seconds) : '24h')}</span>
        </div>
      </div>
      <ul class="r-intro-actions">
        <li>{$t.reflection.introAction1}</li>
        <li>{$t.reflection.introAction2}</li>
        <li>{$t.reflection.introAction3}</li>
      </ul>
    </section>

    <!-- Status summary -->
    <section class="card">
      <div class="card-header">
        <span class="card-title">{$t.reflection.statusTitle}</span>
        {#if config?.enabled}
          <span class="badge badge-success">{$t.reflection.enabled}</span>
        {:else}
          <span class="badge badge-default">{$t.reflection.disabled}</span>
        {/if}
        {#if snapshot && snapshot.consecutive_failures > 0}
          <span class="badge badge-error">{$t.reflection.consecutiveFailures(snapshot.consecutive_failures)}</span>
        {/if}
      </div>
      <dl class="r-facts">
        <div><dt>{$t.reflection.facts.sleepWindow}</dt><dd>{config?.sleep_window || '—'}</dd></div>
        <div><dt>{$t.reflection.facts.timezone}</dt><dd>{config?.timezone || 'system'}</dd></div>
        <div><dt>{$t.reflection.facts.tickInterval}</dt><dd>{config ? `${config.tick_interval_seconds}s` : '—'}</dd></div>
        <div><dt>{$t.reflection.facts.emptySessionAge}</dt><dd>{config ? hoursFromSeconds(config.empty_session_age_seconds) : '—'}</dd></div>
        <div><dt>{$t.reflection.facts.memoryLookback}</dt><dd>{config ? `${config.memory_lookback_hours}h` : '—'}</dd></div>
        <div><dt>{$t.reflection.facts.lastRun}</dt><dd>{relativeTime(snapshot?.last_run_at)}</dd></div>
        <div><dt>{$t.reflection.facts.lastSuccess}</dt><dd>{relativeTime(snapshot?.last_successful_run_at)}</dd></div>
        <div><dt>{$t.reflection.facts.totalRuns}</dt><dd>{snapshot?.total_runs ?? 0}</dd></div>
        <div><dt>{$t.reflection.facts.successes}</dt><dd>{snapshot?.total_successes ?? 0}</dd></div>
        <div><dt>{$t.reflection.facts.failures}</dt><dd>{snapshot?.total_failures ?? 0}</dd></div>
      </dl>
    </section>

    <!-- Last Run + Run Action -->
    <section class="card">
      <div class="card-header">
        <span class="card-title">{$t.reflection.lastRunTitle}</span>
        {#if snapshot?.last_run_summary}
          {#if snapshot.last_run_summary.success}
            <span class="badge badge-success">{$t.reflection.successBadge}</span>
          {:else}
            <span class="badge badge-error">{$t.reflection.failedBadge}</span>
          {/if}
        {/if}
      </div>
      {#if snapshot?.last_run_summary}
        <div class="r-run-meta">
          <span>{$t.reflection.startedLabel} {fmtTime(snapshot.last_run_summary.started_at)}</span>
          <span>{$t.reflection.finishedLabel} {fmtTime(snapshot.last_run_summary.finished_at)}</span>
        </div>
        {#if snapshot.last_run_summary.err}
          <div class="r-error">{snapshot.last_run_summary.err}</div>
        {/if}
        {#if snapshot.last_run_summary.results && snapshot.last_run_summary.results.length > 0}
          <ul class="r-jobs">
            {#each snapshot.last_run_summary.results as job}
              <li>
                <div class="r-job-row">
                  <span class="r-job-name">{job.name}</span>
                  {#if job.success}
                    <span class="badge badge-success">{$t.reflection.jobOk}</span>
                  {:else}
                    <span class="badge badge-error">{$t.reflection.jobFail}</span>
                  {/if}
                  {#if job.changed}
                    <span class="badge badge-info">{$t.reflection.jobChanged}</span>
                  {/if}
                  <span class="r-job-duration">{fmtDuration(job.duration_ms)}</span>
                </div>
                {#if job.summary}
                  <div class="r-job-summary">{job.summary}</div>
                {/if}
                {#if jobDetailLine(job)}
                  <div class="r-job-detail">{jobDetailLine(job)}</div>
                {/if}
                {#if job.err}
                  <div class="r-error">{job.err}</div>
                {/if}
              </li>
            {/each}
          </ul>
        {/if}
      {:else}
        <div class="r-empty-state">
          <div class="r-empty">{$t.reflection.noRunsYet}</div>
          <div class="r-run-preview">
            <div class="r-preview-title">{$t.reflection.expectedOutput}</div>
            <ul>
              <li>
                <span class="badge badge-success">{$t.reflection.jobs.memoryLabel}</span>
                <span>{$t.reflection.previewMemory}</span>
              </li>
              <li>
                <span class="badge badge-success">{$t.reflection.jobs.cleanupLabel}</span>
                <span>{$t.reflection.previewCleanup}</span>
              </li>
              <li>
                <span class="badge badge-info">{$t.reflection.failedBadge}</span>
                <span>{$t.reflection.previewFailure}</span>
              </li>
            </ul>
          </div>
        </div>
      {/if}

      <div class="r-actions">
        <button class="btn btn-primary btn-sm" disabled={running || !config?.enabled} onclick={handleRun}>
          {running ? $t.reflection.runningButton : $t.reflection.runNowButton}
        </button>
        {#if !config?.enabled}
          <span class="r-hint">{$t.reflection.disabledHint}</span>
        {:else}
          <span class="r-hint">{$t.reflection.bypassHint}</span>
        {/if}
      </div>

      {#if runResult}
        <div class="r-run-result">
          <div class="r-run-result-header">
            <strong>{$t.reflection.runResultTitle}</strong>
            {#if runResult.success}
              <span class="badge badge-success">{$t.reflection.successBadge}</span>
            {:else}
              <span class="badge badge-error">{$t.reflection.failedBadge}</span>
            {/if}
          </div>
          <div class="r-run-meta">
            <span>{$t.reflection.startedLabel} {fmtTime(runResult.started_at)}</span>
            <span>{$t.reflection.finishedLabel} {fmtTime(runResult.finished_at)}</span>
            <span>{$t.reflection.durationLabel} {summaryDuration(runResult)}</span>
          </div>
          {#if runResult.err}
            <div class="r-error">{runResult.err}</div>
          {/if}
          <div class="r-run-stats" aria-label="Run totals">
            <div class="r-run-stats-title">{$t.reflection.runTotalsTitle}</div>
            {#each runMetrics(runResult, lastRunBeforeManualRun) as metric}
              <div class="r-run-stat">
                <span>{metric.label}</span>
                <strong>{metric.value}</strong>
                <small>{metric.delta}</small>
              </div>
            {/each}
          </div>
          {#if runResult.results && runResult.results.length > 0}
            <ul class="r-jobs">
              {#each runResult.results as job}
                <li>
                  <div class="r-job-row">
                    <span class="r-job-name">{job.name}</span>
                    {#if job.success}
                      <span class="badge badge-success">{$t.reflection.jobOk}</span>
                    {:else}
                      <span class="badge badge-error">{$t.reflection.jobFail}</span>
                    {/if}
                    {#if job.changed}
                      <span class="badge badge-info">{$t.reflection.jobChanged}</span>
                    {/if}
                    <span class="r-job-duration">{fmtDuration(job.duration_ms)}</span>
                  </div>
                  {#if job.summary}
                    <div class="r-job-summary">{job.summary}</div>
                  {/if}
                  {#if jobDetailLine(job)}
                    <div class="r-job-detail">{jobDetailLine(job)}</div>
                  {/if}
                  {#if job.err}
                    <div class="r-error">{job.err}</div>
                  {/if}
                </li>
              {/each}
            </ul>
          {/if}
        </div>
      {/if}
    </section>

    <!-- Recent runs -->
    {#if snapshot?.recent && snapshot.recent.length > 0}
      <section class="card">
        <div class="card-header">
          <span class="card-title">{$t.reflection.recentRunsTitle}</span>
          <span class="badge badge-default">{snapshot.recent.length}</span>
        </div>
        <ul class="r-recent">
          {#each [...snapshot.recent].reverse() as run}
            <li>
              <div class="r-recent-row">
                <span class="r-recent-time">{fmtTime(run.started_at)}</span>
                {#if run.success}
                  <span class="badge badge-success">{$t.reflection.successBadge}</span>
                {:else}
                  <span class="badge badge-error">{$t.reflection.failedBadge}</span>
                {/if}
                <span class="r-recent-count">{$t.reflection.jobsCount(run.results?.length ?? 0)}</span>
              </div>
              {#if run.err}
                <div class="r-recent-err">{run.err}</div>
              {/if}
            </li>
          {/each}
        </ul>
      </section>
    {/if}
  {/if}
</div>

<style>
  .reflection {
    display: grid;
    gap: var(--space-4);
    animation: fadeIn var(--duration-normal) var(--ease-out);
  }

  @keyframes fadeIn {
    from { opacity: 0; transform: translateY(8px); }
    to { opacity: 1; transform: translateY(0); }
  }

  .r-loading {
    padding: var(--space-10);
    text-align: center;
    color: var(--text-tertiary);
  }

  .r-intro-card {
    border-color: rgba(224, 145, 69, 0.18);
  }

  .r-intro-lead {
    margin: var(--space-3) 0 0;
    max-width: 780px;
    color: var(--text-secondary);
    font-size: var(--text-sm);
    line-height: 1.6;
  }

  .r-intro-grid {
    display: grid;
    gap: var(--space-2);
    margin-top: var(--space-4);
  }

  .r-intro-job {
    display: grid;
    grid-template-columns: 112px minmax(0, 1fr);
    gap: var(--space-3);
    padding-top: var(--space-2);
    border-top: 1px solid var(--border-subtle);
    color: var(--text-secondary);
    font-size: var(--text-sm);
    line-height: 1.55;
  }

  .r-intro-actions {
    display: grid;
    gap: var(--space-1);
    margin: var(--space-4) 0 0;
    padding-left: var(--space-4);
    color: var(--text-tertiary);
    font-size: var(--text-sm);
    line-height: 1.55;
  }

  .r-facts {
    display: grid;
    gap: var(--space-2);
    margin-top: var(--space-3);
  }

  .r-facts div {
    display: grid;
    grid-template-columns: 160px minmax(0, 1fr);
    gap: var(--space-3);
  }

  .r-facts dt {
    color: var(--text-tertiary);
    font-size: var(--text-sm);
  }

  .r-facts dd {
    margin: 0;
    font-size: var(--text-sm);
  }

  .r-run-meta {
    display: flex;
    gap: var(--space-4);
    margin-top: var(--space-2);
    font-size: var(--text-xs);
    color: var(--text-tertiary);
  }

  .r-jobs {
    list-style: none;
    padding: 0;
    margin: var(--space-3) 0 0;
    display: grid;
    gap: var(--space-2);
  }

  .r-jobs li {
    padding: var(--space-2) var(--space-3);
    background: var(--surface-base);
    border-radius: var(--radius-md);
  }

  .r-job-row {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .r-job-name {
    font-weight: 500;
    font-family: var(--font-mono);
    font-size: var(--text-sm);
  }

  .r-job-duration {
    margin-left: auto;
    font-size: var(--text-xs);
    color: var(--text-ghost);
    font-family: var(--font-mono);
  }

  .r-job-summary {
    margin-top: var(--space-1);
    font-size: var(--text-sm);
    color: var(--text-secondary);
  }

  .r-job-detail {
    margin-top: var(--space-1);
    font-size: var(--text-xs);
    color: var(--text-tertiary);
  }

  .r-error {
    padding: var(--space-2) var(--space-3);
    background: var(--error-muted);
    border-radius: var(--radius-md);
    font-size: var(--text-sm);
    color: var(--error);
    margin-top: var(--space-2);
  }

  .r-empty {
    padding: var(--space-3);
    color: var(--text-ghost);
    font-size: var(--text-sm);
  }

  .r-empty-state {
    display: grid;
    gap: var(--space-2);
  }

  .r-run-preview {
    padding: var(--space-3);
    background: var(--surface-base);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
  }

  .r-preview-title {
    margin-bottom: var(--space-2);
    color: var(--text-secondary);
    font-size: var(--text-xs);
    font-weight: 600;
    text-transform: uppercase;
  }

  .r-run-preview ul {
    list-style: none;
    padding: 0;
    margin: 0;
    display: grid;
    gap: var(--space-2);
  }

  .r-run-preview li {
    display: grid;
    grid-template-columns: max-content minmax(0, 1fr);
    align-items: start;
    gap: var(--space-2);
    font-size: var(--text-sm);
    color: var(--text-secondary);
  }

  .r-actions {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--space-3);
    margin-top: var(--space-3);
  }

  .r-hint {
    font-size: var(--text-xs);
    color: var(--text-ghost);
  }

  .r-run-result {
    margin-top: var(--space-3);
    padding: var(--space-3);
    background: rgba(224, 145, 69, 0.06);
    border: 1px solid rgba(224, 145, 69, 0.12);
    border-radius: var(--radius-md);
  }

  .r-run-result-header {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin-bottom: var(--space-2);
  }

  .r-run-result-header strong {
    font-family: var(--font-display);
    font-size: var(--text-sm);
    font-weight: 500;
  }

  .r-run-stats {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
    gap: var(--space-2);
    margin-top: var(--space-3);
  }

  .r-run-stats-title {
    grid-column: 1 / -1;
    color: var(--text-tertiary);
    font-size: var(--text-xs);
    font-weight: 600;
    text-transform: uppercase;
  }

  .r-run-stat {
    display: grid;
    gap: 2px;
    min-width: 0;
    padding: var(--space-2);
    background: var(--surface-base);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
  }

  .r-run-stat span {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
  }

  .r-run-stat strong {
    color: var(--text-primary);
    font-family: var(--font-mono);
    font-size: var(--text-sm);
    font-weight: 600;
  }

  .r-run-stat small {
    color: var(--text-ghost);
    font-size: var(--text-xs);
  }

  .r-recent {
    list-style: none;
    padding: 0;
    margin: var(--space-3) 0 0;
    display: grid;
    gap: var(--space-2);
  }

  .r-recent li {
    padding: var(--space-2) var(--space-3);
    background: var(--surface-base);
    border-radius: var(--radius-sm);
  }

  .r-recent-row {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .r-recent-time {
    font-size: var(--text-xs);
    color: var(--text-tertiary);
    font-family: var(--font-mono);
  }

  .r-recent-count {
    margin-left: auto;
    font-size: var(--text-xs);
    color: var(--text-ghost);
  }

  .r-recent-err {
    margin-top: var(--space-1);
    font-size: var(--text-xs);
    color: var(--error);
  }
</style>
