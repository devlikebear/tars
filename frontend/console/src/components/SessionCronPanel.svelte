<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { createCronJob, deleteCronJob, listCronJobs, listCronRuns, runCronJob, streamEvents, updateCronJob } from '../lib/api'
  import type { CronJob, CronRunRecord } from '../lib/types'

  interface Props {
    sessionId: string
    sessionKind?: string
    onClose: () => void
  }

  let { sessionId, sessionKind = '', onClose }: Props = $props()

  let jobs: CronJob[] = $state([])
  let cronRuns: Record<string, CronRunRecord[]> = $state({})
  let loading = $state(true)
  let error = $state('')
  let showNewJob = $state(false)
  let newJobName = $state('')
  let newJobPrompt = $state('')
  let newJobSchedule = $state('')
  let newJobSaving = $state(false)
  let newJobError = $state('')
  let expandedJob = $state('')
  let runsLoading = $state('')
  let runningJobId = $state('')
  let deletingJobId = $state('')
  let deleteConfirmId: string | null = $state(null)
  let editingJobId: string | null = $state(null)
  let editJobName = $state('')
  let editJobPrompt = $state('')
  let editJobSchedule = $state('')
  let editJobEnabled = $state(true)
  let editJobSaving = $state(false)
  let editJobError = $state('')
  let stopStream: (() => void) | null = null

  let isMainSession = $derived(sessionKind === 'main')
  let scopeLabel = $derived(isMainSession ? 'Global cron jobs' : 'Session cron jobs')
  let scopeHint = $derived(isMainSession
    ? 'Runs created here stay global, but target the main chat session by default.'
    : 'Runs created here are bound to this chat session and stay out of Telegram notifications.')
  let scopedJobs = $derived.by(() => {
    const currentSessionId = sessionId?.trim()
    return jobs.filter((job) => {
      const boundSessionId = job.session_id?.trim() ?? ''
      return isMainSession ? boundSessionId === '' : boundSessionId === currentSessionId
    })
  })

  export async function load() {
    loading = true
    error = ''
    try {
      jobs = await listCronJobs()
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load cron jobs'
    } finally {
      loading = false
    }
  }

  function compact(value?: string, max = 120): string {
    const text = value?.trim()
    if (!text) return '\u2014'
    return text.length <= max ? text : `${text.slice(0, max - 1)}\u2026`
  }

  function fmt(value?: string): string {
    const text = value?.trim()
    if (!text) return '\u2014'
    const date = new Date(text)
    if (Number.isNaN(date.getTime())) return text
    return new Intl.DateTimeFormat('en', { dateStyle: 'medium', timeStyle: 'short' }).format(date)
  }

  function scopePayload() {
    return isMainSession
      ? { session_target: 'main' as const, session_id: undefined }
      : { session_target: undefined, session_id: sessionId }
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
        ...scopePayload(),
      })
      await load()
      showNewJob = false
      newJobName = ''
      newJobPrompt = ''
      newJobSchedule = ''
    } catch (err) {
      newJobError = err instanceof Error ? err.message : 'Failed to create cron job'
    } finally {
      newJobSaving = false
    }
  }

  function enterEditJob(job: CronJob) {
    editingJobId = job.id
    editJobName = job.name
    editJobPrompt = job.prompt
    editJobSchedule = job.schedule
    editJobEnabled = job.enabled
    editJobError = ''
  }

  function cancelEditJob() {
    editingJobId = null
    editJobError = ''
  }

  async function handleSaveEditJob() {
    if (!editingJobId || !editJobPrompt.trim()) return
    editJobSaving = true
    editJobError = ''
    try {
      await updateCronJob(editingJobId, {
        name: editJobName.trim() || undefined,
        prompt: editJobPrompt.trim(),
        schedule: editJobSchedule.trim() || undefined,
        enabled: editJobEnabled,
        ...scopePayload(),
      })
      await load()
      editingJobId = null
    } catch (err) {
      editJobError = err instanceof Error ? err.message : 'Failed to update cron job'
    } finally {
      editJobSaving = false
    }
  }

  async function handleRunJob(jobId: string) {
    runningJobId = jobId
    error = ''
    try {
      await runCronJob(jobId)
      await load()
      await loadRuns(jobId)
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
      deleteConfirmId = null
      if (expandedJob === jobId) expandedJob = ''
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to delete cron job'
    } finally {
      deletingJobId = ''
    }
  }

  async function loadRuns(jobId: string) {
    runsLoading = jobId
    try {
      cronRuns[jobId] = await listCronRuns(jobId)
    } catch {
      cronRuns[jobId] = []
    } finally {
      runsLoading = ''
    }
  }

  async function toggleJobRuns(jobId: string) {
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

  $effect(() => {
    void sessionId
    void sessionKind
    void load()
  })
</script>

<div class="session-cron-panel">
  <div class="panel-header">
    <div class="panel-title-row">
      <span class="card-title">Cron</span>
      <span class="badge badge-info">{isMainSession ? 'global' : 'session'}</span>
      <span class="badge badge-default">{scopedJobs.length}</span>
    </div>
    <div class="panel-actions">
      <button class="btn btn-ghost btn-sm" type="button" onclick={load} title="Refresh">&#x21bb;</button>
      <button class="btn btn-ghost btn-sm" type="button" onclick={onClose} title="Close">&times;</button>
    </div>
  </div>

  <div class="scope-card">
    <div class="scope-title">{scopeLabel}</div>
    <p class="scope-hint">{scopeHint}</p>
  </div>

  {#if error}
    <div class="error-banner">{error}</div>
  {/if}

  <div class="create-actions">
    <button class="btn btn-primary btn-sm" type="button" onclick={() => { showNewJob = !showNewJob }}>
      {showNewJob ? 'Cancel' : '+ New Job'}
    </button>
  </div>

  {#if showNewJob}
    <div class="inline-form">
      {#if newJobError}
        <div class="form-error">{newJobError}</div>
      {/if}
      <input type="text" placeholder="Job name (optional)" bind:value={newJobName} class="form-input" />
      <textarea placeholder="Prompt *" bind:value={newJobPrompt} class="form-input form-textarea" rows="3"></textarea>
      <input type="text" placeholder="Schedule (e.g. every:10m or in 1 minute)" bind:value={newJobSchedule} class="form-input" />
      <button class="btn btn-primary btn-sm" disabled={!newJobPrompt.trim() || newJobSaving} onclick={handleCreateJob}>
        {newJobSaving ? 'Creating...' : 'Create Job'}
      </button>
    </div>
  {/if}

  {#if loading}
    <div class="empty-state">Loading cron jobs...</div>
  {:else if scopedJobs.length === 0}
    <div class="empty-state">
      <p>No cron jobs in this scope yet.</p>
      <p class="hint">Create one here or manage all jobs from Operations.</p>
    </div>
  {:else}
    <div class="cron-list">
      {#each scopedJobs as job}
        <div class="cron-item">
          {#if editingJobId === job.id}
            <div class="inline-form">
              {#if editJobError}
                <div class="form-error">{editJobError}</div>
              {/if}
              <input type="text" placeholder="Job name" bind:value={editJobName} class="form-input" />
              <textarea placeholder="Prompt *" bind:value={editJobPrompt} class="form-input form-textarea" rows="3"></textarea>
              <input type="text" placeholder="Schedule" bind:value={editJobSchedule} class="form-input" />
              <label class="form-checkbox">
                <input type="checkbox" bind:checked={editJobEnabled} />
                Enabled
              </label>
              <div class="edit-actions">
                <button class="btn btn-primary btn-sm" disabled={!editJobPrompt.trim() || editJobSaving} onclick={handleSaveEditJob}>
                  {editJobSaving ? 'Saving...' : 'Save'}
                </button>
                <button class="btn btn-ghost btn-sm" onclick={cancelEditJob}>Cancel</button>
              </div>
            </div>
          {:else}
            <button type="button" class="cron-item-btn" class:active={expandedJob === job.id} onclick={() => { void toggleJobRuns(job.id) }}>
              <div class="cron-item-top">
                <strong class="cron-name">{job.name}</strong>
                <div class="cron-badges">
                  <span class="badge" class:badge-success={job.enabled && !job.last_run_error} class:badge-error={!!job.last_run_error} class:badge-default={!job.enabled}>
                    {job.last_run_error ? 'failed' : job.enabled ? 'active' : 'disabled'}
                  </span>
                  <span class="badge badge-default">{job.schedule}</span>
                </div>
              </div>
              <p class="cron-prompt">{compact(job.prompt, 120)}</p>
              <div class="cron-meta">
                <span>{isMainSession ? 'Global / main-target' : 'Bound to this session'}</span>
                {#if job.last_run_at}
                  <span>Last run: {fmt(job.last_run_at)}</span>
                {/if}
              </div>
            </button>

            <div class="cron-actions">
              <button class="btn btn-ghost btn-sm" disabled={runningJobId === job.id} onclick={(e: MouseEvent) => { e.stopPropagation(); void handleRunJob(job.id) }}>
                {runningJobId === job.id ? 'Running...' : 'Run'}
              </button>
              <button class="btn btn-ghost btn-sm" onclick={(e: MouseEvent) => { e.stopPropagation(); enterEditJob(job) }}>Edit</button>
              <button class="btn btn-danger btn-sm" disabled={deletingJobId === job.id} onclick={(e: MouseEvent) => { e.stopPropagation(); void handleDeleteJob(job.id) }}>
                {deleteConfirmId === job.id ? 'Confirm?' : 'Delete'}
              </button>
            </div>
          {/if}

          {#if expandedJob === job.id && editingJobId !== job.id}
            <div class="cron-runs">
              {#if runsLoading === job.id}
                <div class="runs-loading">Loading runs...</div>
              {:else if !cronRuns[job.id] || cronRuns[job.id].length === 0}
                <div class="runs-empty">No run history.</div>
              {:else}
                {#each cronRuns[job.id] as run}
                  <div class="run-item" class:run-error={!!run.error}>
                    <div class="run-top">
                      <span class="badge" class:badge-success={!run.error} class:badge-error={!!run.error}>
                        {run.error ? 'error' : 'ok'}
                      </span>
                      <span class="run-time">{fmt(run.ran_at)}</span>
                    </div>
                    {#if run.error}
                      <p class="run-detail run-error-text">{compact(run.error, 200)}</p>
                    {:else if run.response}
                      <p class="run-detail">{compact(run.response, 200)}</p>
                    {/if}
                  </div>
                {/each}
              {/if}
            </div>
          {/if}
        </div>
      {/each}
    </div>
  {/if}
</div>

<style>
  .session-cron-panel {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    height: 100%;
    overflow-y: auto;
  }

  .panel-header,
  .panel-title-row,
  .panel-actions,
  .create-actions,
  .cron-item-top,
  .cron-badges,
  .cron-meta,
  .cron-actions,
  .run-top,
  .edit-actions {
    display: flex;
    align-items: center;
  }

  .panel-header,
  .cron-item-top {
    justify-content: space-between;
  }

  .panel-title-row,
  .panel-actions,
  .cron-badges,
  .cron-meta,
  .cron-actions,
  .run-top,
  .edit-actions {
    gap: var(--space-2);
  }

  .scope-card,
  .cron-item,
  .run-item {
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--bg-inset);
  }

  .scope-card {
    padding: var(--space-3);
  }

  .scope-title {
    font-family: var(--font-display);
    font-size: var(--text-sm);
    color: var(--text-primary);
  }

  .scope-hint,
  .hint,
  .cron-prompt,
  .cron-meta,
  .run-detail,
  .empty-state {
    color: var(--text-secondary);
    font-size: var(--text-sm);
  }

  .scope-hint,
  .cron-prompt,
  .run-detail {
    margin: 0;
    line-height: 1.5;
  }

  .inline-form {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    padding: var(--space-3);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--bg-surface);
  }

  .cron-list {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .cron-item {
    overflow: hidden;
  }

  .cron-item-btn {
    width: 100%;
    padding: var(--space-3);
    border: none;
    background: none;
    color: inherit;
    text-align: left;
    cursor: pointer;
  }

  .cron-item-btn.active,
  .cron-item-btn:hover {
    background: var(--bg-surface);
  }

  .cron-name {
    font-size: var(--text-sm);
    color: var(--text-primary);
  }

  .cron-actions,
  .cron-runs {
    padding: 0 var(--space-3) var(--space-3);
  }

  .cron-meta {
    flex-wrap: wrap;
    margin-top: var(--space-2);
  }

  .cron-runs {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .run-item {
    padding: var(--space-2) var(--space-3);
  }

  .run-error {
    border-color: color-mix(in srgb, var(--accent-danger) 35%, var(--border-subtle));
  }

  .run-error-text,
  .form-error,
  .error-banner {
    color: var(--accent-danger);
  }

  .empty-state {
    padding: var(--space-6) var(--space-4);
    text-align: center;
  }

  .error-banner {
    font-size: var(--text-sm);
    padding: var(--space-2) var(--space-3);
    border: 1px solid color-mix(in srgb, var(--accent-danger) 30%, var(--border-subtle));
    border-radius: var(--radius-md);
    background: color-mix(in srgb, var(--accent-danger) 8%, var(--bg-surface));
  }
</style>
