<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { createCronJob, deleteCronJob, listCronJobs, listCronRuns, runCronJob, streamEvents, updateCronJob } from '../lib/api'
  import type { CronJob, CronRunRecord } from '../lib/types'
  import { t } from '../i18n'

  type DeliveryTarget = 'daily_log' | 'main' | 'both'
  type JobBucket = 'active' | 'paused' | 'done'

  let jobs: CronJob[] = $state([])
  let cronRuns: Record<string, CronRunRecord[]> = $state({})
  let loading = $state(true)
  let error = $state('')
  let newJobName = $state('')
  let newJobPrompt = $state('')
  let newJobSchedule = $state('every:1h')
  let deliveryTarget: DeliveryTarget = $state('daily_log')
  let newJobSaving = $state(false)
  let newJobError = $state('')
  let expandedJob = $state('')
  let runsLoading = $state('')
  let runningJobId = $state('')
  let togglingJobId = $state('')
  let deletingJobId = $state('')
  let deleteConfirmId = $state('')
  let stopStream: (() => void) | null = null

  let activeJobs = $derived.by(() => jobs.filter((job) => job.enabled && !isCompleted(job)))
  let pausedJobs = $derived.by(() => jobs.filter((job) => !job.enabled && !isCompleted(job)))
  let completedJobs = $derived.by(() => jobs.filter((job) => isCompleted(job)))
  let totalRuns = $derived.by(() => Object.values(cronRuns).reduce((sum, runs) => sum + runs.length, 0))
  let hasJobs = $derived(jobs.length > 0)

  export async function load() {
    loading = true
    error = ''
    try {
      jobs = await listCronJobs()
      if (expandedJob) {
        await loadRuns(expandedJob)
      }
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load cron jobs'
    } finally {
      loading = false
    }
  }

  function deliveryPayload() {
    switch (deliveryTarget) {
      case 'main':
        return { session_target: 'main' as const, delivery_mode: 'session' as const }
      case 'both':
        return { session_target: 'main' as const, delivery_mode: 'both' as const }
      default:
        return { session_target: 'isolated' as const, delivery_mode: 'daily_log' as const }
    }
  }

  function deliveryLabel(job: CronJob): string {
    const mode = job.delivery_mode || ''
    if (mode === 'both') return $t.cron.deliveryBoth
    if (mode === 'session' || job.session_target === 'main' || job.session_id) return job.session_id ? $t.cron.deliveryBound : $t.cron.deliveryMain
    if (mode === 'none') return $t.cron.deliveryNone
    return $t.cron.deliveryDailyLog
  }

  function statusLabel(job: CronJob): string {
    if (job.last_run_error) return $t.cron.statusFailed
    if (isCompleted(job)) return $t.cron.statusDone
    return job.enabled ? $t.cron.statusActive : $t.cron.statusPaused
  }

  function statusClass(job: CronJob): string {
    if (job.last_run_error) return 'badge-error'
    if (isCompleted(job)) return 'badge-info'
    return job.enabled ? 'badge-success' : 'badge-default'
  }

  function isCompleted(job: CronJob): boolean {
    return isOneShot(job) && Boolean(job.last_run_at)
  }

  function isOneShot(job: CronJob): boolean {
    const schedule = job.schedule.trim().toLowerCase()
    return job.delete_after_run === true || schedule.startsWith('at:')
  }

  function fmt(value?: string): string {
    const text = value?.trim()
    if (!text) return '-'
    const date = new Date(text)
    if (Number.isNaN(date.getTime())) return text
    return new Intl.DateTimeFormat('en', { dateStyle: 'medium', timeStyle: 'short' }).format(date)
  }

  function relativeTime(value?: string): string {
    const text = value?.trim()
    if (!text) return $t.cron.never
    const date = new Date(text)
    if (Number.isNaN(date.getTime())) return text
    if (date.getFullYear() <= 1) return $t.cron.never
    const seconds = Math.floor((Date.now() - date.getTime()) / 1000)
    if (seconds < 60) return $t.cron.secondsAgo(seconds)
    if (seconds < 3600) return $t.cron.minutesAgo(Math.floor(seconds / 60))
    if (seconds < 86400) return $t.cron.hoursAgo(Math.floor(seconds / 3600))
    return $t.cron.daysAgo(Math.floor(seconds / 86400))
  }

  function compact(value?: string, max = 140): string {
    const text = value?.trim()
    if (!text) return '-'
    return text.length <= max ? text : `${text.slice(0, max - 1)}...`
  }

  function nextRunLabel(job: CronJob): string {
    if (isCompleted(job)) return $t.cron.nextCompleted
    if (!job.enabled) return $t.cron.nextPaused
    const schedule = job.schedule.trim()
    if (schedule.toLowerCase().startsWith('at:')) return fmt(schedule.slice(3))
    if (schedule.toLowerCase().startsWith('every:')) return job.last_run_at ? $t.cron.nextAfter(relativeTime(job.last_run_at)) : $t.cron.nextTick
    return $t.cron.nextSchedule
  }

  function bucketLabel(bucket: JobBucket): string {
    switch (bucket) {
      case 'paused':
        return $t.cron.bucketPaused
      case 'done':
        return $t.cron.bucketDone
      default:
        return $t.cron.bucketActive
    }
  }

  function bucketJobs(bucket: JobBucket): CronJob[] {
    switch (bucket) {
      case 'paused':
        return pausedJobs
      case 'done':
        return completedJobs
      default:
        return activeJobs
    }
  }

  async function handleCreateJob() {
    if (!newJobPrompt.trim()) return
    newJobSaving = true
    newJobError = ''
    try {
      await createCronJob({
        name: newJobName.trim() || undefined,
        prompt: newJobPrompt.trim(),
        schedule: newJobSchedule.trim() || undefined,
        enabled: true,
        wake_mode: 'agent_loop',
        ...deliveryPayload(),
      })
      await load()
      newJobName = ''
      newJobPrompt = ''
      newJobSchedule = 'every:1h'
      deliveryTarget = 'daily_log'
    } catch (err) {
      newJobError = err instanceof Error ? err.message : 'Failed to create cron job'
    } finally {
      newJobSaving = false
    }
  }

  async function handleToggleJob(job: CronJob) {
    togglingJobId = job.id
    error = ''
    try {
      await updateCronJob(job.id, { enabled: !job.enabled })
      await load()
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to update cron job'
    } finally {
      togglingJobId = ''
    }
  }

  async function handleRunJob(jobId: string) {
    runningJobId = jobId
    error = ''
    try {
      await runCronJob(jobId)
      await load()
      await loadRuns(jobId)
      expandedJob = jobId
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to run cron job'
    } finally {
      runningJobId = ''
    }
  }

  async function handleDeleteJob(jobId: string) {
    if (deleteConfirmId !== jobId) {
      deleteConfirmId = jobId
      return
    }
    deletingJobId = jobId
    error = ''
    try {
      await deleteCronJob(jobId)
      jobs = jobs.filter((job) => job.id !== jobId)
      deleteConfirmId = ''
      if (expandedJob === jobId) expandedJob = ''
      delete cronRuns[jobId]
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to delete cron job'
    } finally {
      deletingJobId = ''
    }
  }

  async function loadRuns(jobId: string) {
    if (!jobId) return
    runsLoading = jobId
    try {
      cronRuns[jobId] = await listCronRuns(jobId, 50)
    } catch {
      cronRuns[jobId] = []
    } finally {
      runsLoading = ''
    }
  }

  async function toggleRunHistory(jobId: string) {
    deleteConfirmId = ''
    if (expandedJob === jobId) {
      expandedJob = ''
      return
    }
    expandedJob = jobId
    await loadRuns(jobId)
  }

  onMount(() => {
    void load()
    stopStream = streamEvents(
      (event) => {
        if (event.category === 'cron') {
          void load()
        }
      },
      () => {},
      () => {},
    )
  })

  onDestroy(() => {
    stopStream?.()
  })
</script>

<div class="cron-page">
  <section class="cron-header">
    <div>
      <p class="eyebrow">{$t.cron.eyebrow}</p>
      <h1>{$t.cron.title}</h1>
    </div>
    <button class="btn btn-ghost btn-sm" type="button" onclick={load} disabled={loading} title={$t.cron.refresh}>
      {$t.cron.refresh}
    </button>
  </section>

  <section class="cron-metrics" aria-label={$t.cron.summaryAriaLabel}>
    <div class="metric">
      <span>{$t.cron.metricActive}</span>
      <strong>{activeJobs.length}</strong>
    </div>
    <div class="metric">
      <span>{$t.cron.metricPaused}</span>
      <strong>{pausedJobs.length}</strong>
    </div>
    <div class="metric">
      <span>{$t.cron.metricDone}</span>
      <strong>{completedJobs.length}</strong>
    </div>
    <div class="metric">
      <span>{$t.cron.metricLoadedRuns}</span>
      <strong>{totalRuns}</strong>
    </div>
  </section>

  {#if error}
    <div class="error-banner">{error}</div>
  {/if}

  <section class="create-panel" aria-label={$t.cron.createPanelAriaLabel}>
    <div class="section-heading">
      <h2>{$t.cron.newJob}</h2>
      <span>{deliveryTarget === 'daily_log' ? $t.cron.deliveryDailyLog : deliveryTarget === 'both' ? $t.cron.deliveryBoth : $t.cron.deliveryMain}</span>
    </div>

    {#if newJobError}
      <div class="form-error">{newJobError}</div>
    {/if}

    <div class="form-grid">
      <label>
        <span>{$t.cron.nameLabel}</span>
        <input type="text" placeholder={$t.cron.namePlaceholder} bind:value={newJobName} class="form-input" />
      </label>
      <label>
        <span>{$t.cron.scheduleLabel}</span>
        <input type="text" placeholder={$t.cron.schedulePlaceholder} bind:value={newJobSchedule} class="form-input" />
      </label>
      <label>
        <span>{$t.cron.deliveryLabel}</span>
        <select bind:value={deliveryTarget} class="form-input">
          <option value="daily_log">{$t.cron.deliveryDailyLog}</option>
          <option value="main">{$t.cron.deliveryMain}</option>
          <option value="both">{$t.cron.deliveryBoth}</option>
        </select>
      </label>
    </div>

    <label class="prompt-field">
      <span>{$t.cron.promptLabel}</span>
      <textarea placeholder={$t.cron.promptPlaceholder} bind:value={newJobPrompt} class="form-input form-textarea" rows="4"></textarea>
    </label>

    <div class="create-actions">
      <button class="btn btn-primary btn-sm" type="button" disabled={!newJobPrompt.trim() || newJobSaving} onclick={handleCreateJob}>
        {newJobSaving ? $t.cron.creating : $t.cron.createButton}
      </button>
    </div>
  </section>

  <section class="jobs-panel" aria-label={$t.cron.jobsAriaLabel}>
    <div class="section-heading">
      <h2>{$t.cron.jobsTitle}</h2>
      <span>{$t.cron.totalSuffix(jobs.length)}</span>
    </div>

    {#if loading}
      <div class="empty-state">{$t.cron.loadingJobs}</div>
    {:else if !hasJobs}
      <div class="empty-state">{$t.cron.noJobs}</div>
    {:else}
      {#each ['active', 'paused', 'done'] as bucket}
        {@const bucketList = bucketJobs(bucket as JobBucket)}
        {#if bucketList.length > 0}
          <div class="job-bucket">
            <div class="bucket-title">
              <span>{bucketLabel(bucket as JobBucket)}</span>
              <span>{bucketList.length}</span>
            </div>

            <div class="job-list">
              {#each bucketList as job}
                <article class="job-row">
                  <button type="button" class="job-summary" class:open={expandedJob === job.id} onclick={() => { void toggleRunHistory(job.id) }}>
                    <span class={`badge ${statusClass(job)}`}>{statusLabel(job)}</span>
                    <span class="job-main">
                      <strong>{job.name || $t.cron.untitled}</strong>
                      <small>{compact(job.prompt)}</small>
                    </span>
                    <span class="job-meta">
                      <span>{job.schedule}</span>
                      <span>{deliveryLabel(job)}</span>
                      <span>{nextRunLabel(job)}</span>
                    </span>
                  </button>

                  <div class="job-actions">
                    <button class="btn btn-ghost btn-sm" type="button" disabled={isCompleted(job) || togglingJobId === job.id} onclick={() => { void handleToggleJob(job) }}>
                      {job.enabled ? $t.cron.pause : $t.cron.resume}
                    </button>
                    <button class="btn btn-ghost btn-sm" type="button" disabled={runningJobId === job.id} onclick={() => { void handleRunJob(job.id) }}>
                      {runningJobId === job.id ? $t.cron.running : $t.cron.runNow}
                    </button>
                    <button class="btn btn-danger btn-sm" type="button" disabled={deletingJobId === job.id} onclick={() => { void handleDeleteJob(job.id) }}>
                      {deleteConfirmId === job.id ? $t.cron.confirm : $t.cron.delete}
                    </button>
                  </div>

                  {#if expandedJob === job.id}
                    <div class="run-history">
                      <div class="run-history-head">
                        <span>{$t.cron.runHistory}</span>
                        {#if job.last_run_at}
                          <span>{$t.cron.lastRun(relativeTime(job.last_run_at))}</span>
                        {/if}
                      </div>
                      {#if runsLoading === job.id}
                        <div class="runs-empty">{$t.cron.loadingRuns}</div>
                      {:else if !cronRuns[job.id] || cronRuns[job.id].length === 0}
                        <div class="runs-empty">{$t.cron.noRuns}</div>
                      {:else}
                        <div class="run-list">
                          {#each cronRuns[job.id] as run}
                            <div class="run-item" class:run-error={!!run.error}>
                              <span class="run-time">{fmt(run.ran_at)}</span>
                              <span class={`badge ${run.error ? 'badge-error' : 'badge-success'}`}>{run.error ? $t.cron.runError : $t.cron.runOk}</span>
                              <p>{compact(run.error || run.response, 220)}</p>
                            </div>
                          {/each}
                        </div>
                      {/if}
                    </div>
                  {/if}
                </article>
              {/each}
            </div>
          </div>
        {/if}
      {/each}
    {/if}
  </section>
</div>

<style>
  .cron-page {
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
    overflow-y: auto;
  }

  .cron-header,
  .section-heading,
  .cron-metrics,
  .create-actions,
  .job-summary,
  .job-actions,
  .bucket-title,
  .run-history-head,
  .run-item {
    display: flex;
    align-items: center;
  }

  .cron-header,
  .section-heading,
  .job-summary,
  .bucket-title,
  .run-history-head {
    justify-content: space-between;
  }

  .cron-header h1,
  .section-heading h2 {
    margin: 0;
    color: var(--text-primary);
    font-family: var(--font-display);
    letter-spacing: 0;
  }

  .cron-header h1 {
    font-size: var(--text-3xl);
  }

  .section-heading h2 {
    font-size: var(--text-lg);
  }

  .eyebrow,
  .section-heading span,
  .metric span,
  .bucket-title,
  .job-meta,
  .job-main small,
  .run-history-head,
  .run-time,
  .empty-state,
  label span {
    color: var(--text-secondary);
    font-size: var(--text-sm);
  }

  .eyebrow {
    margin: 0 0 var(--space-1);
    text-transform: uppercase;
    font-weight: 700;
    letter-spacing: 0;
  }

  .cron-metrics {
    gap: var(--space-3);
    flex-wrap: wrap;
  }

  .metric {
    min-width: 140px;
    padding: var(--space-3) var(--space-4);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--surface);
  }

  .metric strong {
    display: block;
    margin-top: var(--space-1);
    color: var(--text-primary);
    font-family: var(--font-display);
    font-size: var(--text-xl);
  }

  .create-panel,
  .jobs-panel,
  .job-row {
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--surface);
  }

  .create-panel,
  .jobs-panel {
    padding: var(--space-4);
  }

  .create-panel,
  .jobs-panel,
  .job-bucket,
  .job-list,
  .run-list {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .form-grid {
    display: grid;
    grid-template-columns: minmax(180px, 1fr) minmax(220px, 1fr) minmax(180px, 0.75fr);
    gap: var(--space-3);
  }

  label,
  .prompt-field {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .form-textarea {
    min-height: 96px;
    resize: vertical;
  }

  .create-actions {
    justify-content: flex-end;
  }

  .job-row {
    overflow: hidden;
  }

  .job-summary {
    width: 100%;
    gap: var(--space-3);
    min-height: 76px;
    padding: var(--space-3);
    border: 0;
    background: transparent;
    color: inherit;
    text-align: left;
    cursor: pointer;
  }

  .job-summary:hover,
  .job-summary.open {
    background: var(--surface-elevated);
  }

  .job-main {
    display: flex;
    min-width: 0;
    flex: 1;
    flex-direction: column;
    gap: var(--space-1);
  }

  .job-main strong,
  .job-main small,
  .job-meta span {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .job-main strong {
    color: var(--text-primary);
    font-size: var(--text-sm);
  }

  .job-meta {
    display: grid;
    width: min(40%, 420px);
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: var(--space-2);
  }

  .job-actions {
    justify-content: flex-end;
    gap: var(--space-2);
    padding: 0 var(--space-3) var(--space-3);
  }

  .run-history {
    border-top: 1px solid var(--border-subtle);
    padding: var(--space-3);
    background: var(--surface-inset);
  }

  .run-history-head {
    margin-bottom: var(--space-2);
  }

  .run-item {
    gap: var(--space-2);
    align-items: flex-start;
    padding: var(--space-2);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--surface);
  }

  .run-item p {
    flex: 1;
    min-width: 0;
    margin: 0;
    color: var(--text-secondary);
    font-size: var(--text-sm);
    line-height: 1.45;
    word-break: break-word;
  }

  .run-error {
    border-color: color-mix(in srgb, var(--error) 35%, var(--border-subtle));
  }

  .runs-empty,
  .empty-state {
    padding: var(--space-4);
    text-align: center;
  }

  .form-error,
  .error-banner {
    color: var(--error);
  }

  .error-banner {
    padding: var(--space-2) var(--space-3);
    border: 1px solid color-mix(in srgb, var(--error) 30%, var(--border-subtle));
    border-radius: var(--radius-md);
    background: color-mix(in srgb, var(--error) 8%, var(--surface));
    font-size: var(--text-sm);
  }

  .badge {
    flex: 0 0 auto;
  }

  @media (max-width: 900px) {
    .form-grid {
      grid-template-columns: 1fr;
    }

    .job-summary {
      align-items: flex-start;
      flex-direction: column;
    }

    .job-meta {
      width: 100%;
      grid-template-columns: 1fr;
    }

    .job-actions {
      flex-wrap: wrap;
    }
  }
</style>
