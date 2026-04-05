<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { getReflectionStatus, runReflectionOnce, getReflectionConfig } from '../lib/api'
  import type { ReflectionSnapshot, ReflectionRunSummary, ReflectionConfigView } from '../lib/types'

  let snapshot: ReflectionSnapshot | null = $state(null)
  let config: ReflectionConfigView | null = $state(null)
  let loading = $state(true)
  let error = $state('')

  let running = $state(false)
  let runResult: ReflectionRunSummary | null = $state(null)

  let refreshInterval: ReturnType<typeof setInterval> | null = null

  async function loadStatus() {
    try {
      snapshot = await getReflectionStatus()
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load status'
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
    try {
      runResult = await runReflectionOnce()
      await loadStatus()
    } catch (e) {
      runResult = {
        started_at: '',
        finished_at: '',
        results: [],
        success: false,
        err: e instanceof Error ? e.message : 'Run failed',
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
    if (!value?.trim()) return 'never'
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return value
    const seconds = Math.floor((Date.now() - date.getTime()) / 1000)
    if (seconds < 60) return `${seconds}s ago`
    if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`
    if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`
    return `${Math.floor(seconds / 86400)}d ago`
  }

  function fmtDuration(ms: number): string {
    if (!ms || ms < 0) return '\u2014'
    if (ms < 1000) return `${Math.round(ms / 1e6)}ms`
    const seconds = ms / 1e9
    if (seconds < 60) return `${seconds.toFixed(1)}s`
    return `${(seconds / 60).toFixed(1)}m`
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
    <div class="r-loading">Loading reflection...</div>
  {:else}
    {#if error}
      <div class="error-banner" style="margin-bottom:var(--space-4)">{error}</div>
    {/if}

    <!-- Status summary -->
    <section class="card">
      <div class="card-header">
        <span class="card-title">Reflection Status</span>
        {#if config?.enabled}
          <span class="badge badge-success">enabled</span>
        {:else}
          <span class="badge badge-default">disabled</span>
        {/if}
        {#if snapshot && snapshot.consecutive_failures > 0}
          <span class="badge badge-error">{snapshot.consecutive_failures} consecutive failure{snapshot.consecutive_failures > 1 ? 's' : ''}</span>
        {/if}
      </div>
      <dl class="r-facts">
        <div><dt>Sleep Window</dt><dd>{config?.sleep_window || '—'}</dd></div>
        <div><dt>Timezone</dt><dd>{config?.timezone || 'system'}</dd></div>
        <div><dt>Tick Interval</dt><dd>{config ? `${config.tick_interval_seconds}s` : '—'}</dd></div>
        <div><dt>Empty Session Age</dt><dd>{config ? hoursFromSeconds(config.empty_session_age_seconds) : '—'}</dd></div>
        <div><dt>Memory Lookback</dt><dd>{config ? `${config.memory_lookback_hours}h` : '—'}</dd></div>
        <div><dt>Last Run</dt><dd>{relativeTime(snapshot?.last_run_at)}</dd></div>
        <div><dt>Last Success</dt><dd>{relativeTime(snapshot?.last_successful_run_at)}</dd></div>
        <div><dt>Total Runs</dt><dd>{snapshot?.total_runs ?? 0}</dd></div>
        <div><dt>Successes</dt><dd>{snapshot?.total_successes ?? 0}</dd></div>
        <div><dt>Failures</dt><dd>{snapshot?.total_failures ?? 0}</dd></div>
      </dl>
    </section>

    <!-- Last Run + Run Action -->
    <section class="card">
      <div class="card-header">
        <span class="card-title">Last Run</span>
        {#if snapshot?.last_run_summary}
          {#if snapshot.last_run_summary.success}
            <span class="badge badge-success">success</span>
          {:else}
            <span class="badge badge-error">failed</span>
          {/if}
        {/if}
      </div>
      {#if snapshot?.last_run_summary}
        <div class="r-run-meta">
          <span>Started: {fmtTime(snapshot.last_run_summary.started_at)}</span>
          <span>Finished: {fmtTime(snapshot.last_run_summary.finished_at)}</span>
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
                    <span class="badge badge-success">ok</span>
                  {:else}
                    <span class="badge badge-error">fail</span>
                  {/if}
                  {#if job.changed}
                    <span class="badge badge-info">changed</span>
                  {/if}
                  <span class="r-job-duration">{fmtDuration(job.duration_ms)}</span>
                </div>
                {#if job.summary}
                  <div class="r-job-summary">{job.summary}</div>
                {/if}
                {#if job.err}
                  <div class="r-error">{job.err}</div>
                {/if}
              </li>
            {/each}
          </ul>
        {/if}
      {:else}
        <div class="r-empty">No reflection runs yet.</div>
      {/if}

      <div class="r-actions">
        <button class="btn btn-primary btn-sm" disabled={running || !config?.enabled} onclick={handleRun}>
          {running ? 'Running...' : 'Run Reflection Now'}
        </button>
        {#if !config?.enabled}
          <span class="r-hint">Reflection is disabled in config</span>
        {:else}
          <span class="r-hint">Bypasses the sleep-window gate</span>
        {/if}
      </div>

      {#if runResult}
        <div class="r-run-result">
          <div class="r-run-result-header">
            <strong>Run result</strong>
            {#if runResult.success}
              <span class="badge badge-success">success</span>
            {:else}
              <span class="badge badge-error">failed</span>
            {/if}
          </div>
          {#if runResult.err}
            <div class="r-error">{runResult.err}</div>
          {/if}
          {#if runResult.results && runResult.results.length > 0}
            <ul class="r-jobs">
              {#each runResult.results as job}
                <li>
                  <div class="r-job-row">
                    <span class="r-job-name">{job.name}</span>
                    {#if job.success}
                      <span class="badge badge-success">ok</span>
                    {:else}
                      <span class="badge badge-error">fail</span>
                    {/if}
                    {#if job.changed}
                      <span class="badge badge-info">changed</span>
                    {/if}
                    <span class="r-job-duration">{fmtDuration(job.duration_ms)}</span>
                  </div>
                  {#if job.summary}
                    <div class="r-job-summary">{job.summary}</div>
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
          <span class="card-title">Recent Runs</span>
          <span class="badge badge-default">{snapshot.recent.length}</span>
        </div>
        <ul class="r-recent">
          {#each [...snapshot.recent].reverse() as run}
            <li>
              <div class="r-recent-row">
                <span class="r-recent-time">{fmtTime(run.started_at)}</span>
                {#if run.success}
                  <span class="badge badge-success">success</span>
                {:else}
                  <span class="badge badge-error">failed</span>
                {/if}
                <span class="r-recent-count">{run.results?.length ?? 0} job{(run.results?.length ?? 0) === 1 ? '' : 's'}</span>
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
    background: var(--bg-base);
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

  .r-actions {
    display: flex;
    align-items: center;
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

  .r-recent {
    list-style: none;
    padding: 0;
    margin: var(--space-3) 0 0;
    display: grid;
    gap: var(--space-2);
  }

  .r-recent li {
    padding: var(--space-2) var(--space-3);
    background: var(--bg-base);
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
