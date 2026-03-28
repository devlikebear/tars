<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import {
    getOpsStatus,
    listApprovals,
    listCronJobs,
    listCronRuns,
    reviewApproval,
    createCleanupPlan,
    streamEvents,
  } from '../lib/api'
  import type {
    Approval,
    CronJob,
    CronRunRecord,
    OpsStatus,
  } from '../lib/types'

  let status: OpsStatus | null = $state(null)
  let approvals: Approval[] = $state([])
  let cronJobs: CronJob[] = $state([])
  let cronRuns: Record<string, CronRunRecord[]> = $state({})

  let loading = $state(true)
  let error = $state('')
  let reviewingId = $state('')
  let planCreating = $state(false)
  let expandedJob: string | null = $state(null)
  let runsLoading = $state('')

  let stopStream: (() => void) | null = null

  function fmt(value?: string): string {
    const text = value?.trim()
    if (!text) return '\u2014'
    const date = new Date(text)
    if (Number.isNaN(date.getTime())) return text
    return new Intl.DateTimeFormat('en', { dateStyle: 'medium', timeStyle: 'short' }).format(date)
  }

  function fmtBytes(bytes: number): string {
    if (bytes < 1024) return `${bytes} B`
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
    if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
    return `${(bytes / (1024 * 1024 * 1024)).toFixed(1)} GB`
  }

  function compact(value?: string, max = 120): string {
    const text = value?.trim()
    if (!text) return '\u2014'
    return text.length <= max ? text : `${text.slice(0, max - 1)}\u2026`
  }

  function statusBadge(s: string): string {
    switch (s) {
      case 'pending': return 'badge-warning'
      case 'approved': return 'badge-success'
      case 'rejected': return 'badge-error'
      case 'applied': return 'badge-info'
      default: return 'badge-default'
    }
  }

  function diskLevel(percent: number): string {
    if (percent >= 90) return 'disk-critical'
    if (percent >= 75) return 'disk-warning'
    return 'disk-ok'
  }

  async function load() {
    loading = true
    error = ''
    try {
      const [s, a, c] = await Promise.allSettled([
        getOpsStatus(),
        listApprovals(),
        listCronJobs(),
      ])
      status = s.status === 'fulfilled' ? s.value : null
      approvals = a.status === 'fulfilled' ? a.value : []
      cronJobs = c.status === 'fulfilled' ? c.value : []
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load ops data'
    } finally {
      loading = false
    }
  }

  async function handleReview(approvalId: string, action: 'approve' | 'reject') {
    reviewingId = approvalId
    try {
      await reviewApproval(approvalId, action)
      approvals = await listApprovals()
    } catch (err) {
      error = err instanceof Error ? err.message : 'Review failed'
    } finally {
      reviewingId = ''
    }
  }

  async function handleCreatePlan() {
    planCreating = true
    error = ''
    try {
      await createCleanupPlan()
      approvals = await listApprovals()
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to create cleanup plan'
    } finally {
      planCreating = false
    }
  }

  async function toggleJobRuns(jobId: string) {
    if (expandedJob === jobId) {
      expandedJob = null
      return
    }
    expandedJob = jobId
    if (cronRuns[jobId]) return
    runsLoading = jobId
    try {
      cronRuns[jobId] = await listCronRuns(jobId)
    } catch {
      cronRuns[jobId] = []
    } finally {
      runsLoading = ''
    }
  }

  onMount(() => {
    void load()
    stopStream = streamEvents(
      undefined,
      (event) => {
        if (event.category === 'ops') {
          void listApprovals().then((list) => { approvals = list })
          void getOpsStatus().then((s) => { status = s }).catch(() => {})
        }
        if (event.category === 'cron') {
          void listCronJobs().then((jobs) => { cronJobs = jobs })
        }
      },
    )
  })

  onDestroy(() => {
    stopStream?.()
  })
</script>

<div class="ops">
  <div class="ops-header">
    <div>
      <h2>Operations</h2>
      <p class="ops-subtitle">System health, approvals, and scheduled jobs.</p>
    </div>
    <button type="button" class="btn btn-ghost btn-sm" onclick={() => { void load() }}>
      Refresh
    </button>
  </div>

  {#if error}
    <div class="error-banner">{error}</div>
  {/if}

  {#if loading}
    <div class="ops-loading">Loading operations data...</div>
  {:else}
    <!-- System Status -->
    <section class="ops-status-strip">
      {#if status}
        <div class="status-gauge">
          <div class="gauge-ring {diskLevel(status.disk_used_percent)}">
            <svg viewBox="0 0 36 36" class="gauge-svg">
              <path
                d="M18 2.0845 a 15.9155 15.9155 0 0 1 0 31.831 a 15.9155 15.9155 0 0 1 0 -31.831"
                fill="none"
                stroke="var(--border-subtle)"
                stroke-width="3"
              />
              <path
                d="M18 2.0845 a 15.9155 15.9155 0 0 1 0 31.831 a 15.9155 15.9155 0 0 1 0 -31.831"
                fill="none"
                stroke="currentColor"
                stroke-width="3"
                stroke-dasharray="{status.disk_used_percent}, 100"
                stroke-linecap="round"
              />
            </svg>
            <span class="gauge-text">{status.disk_used_percent.toFixed(0)}%</span>
          </div>
          <div class="gauge-label">
            <strong>Disk</strong>
            <span>{fmtBytes(status.disk_total_bytes - status.disk_free_bytes)} / {fmtBytes(status.disk_total_bytes)}</span>
          </div>
        </div>
        <div class="status-divider"></div>
        <div class="status-metric">
          <span class="metric-value">{fmtBytes(status.disk_free_bytes)}</span>
          <span class="metric-label">Free space</span>
        </div>
        <div class="status-divider"></div>
        <div class="status-metric">
          <span class="metric-value">{status.process_count}</span>
          <span class="metric-label">Processes</span>
        </div>
        <div class="status-divider"></div>
        <div class="status-metric">
          <span class="metric-value metric-time">{fmt(status.timestamp)}</span>
          <span class="metric-label">Last checked</span>
        </div>
      {:else}
        <div class="status-unavailable">System status unavailable</div>
      {/if}
    </section>

    <div class="ops-grid">
      <!-- Approvals -->
      <section class="card ops-section">
        <div class="card-header">
          <span class="card-title">Approvals</span>
          <div class="card-header-actions">
            {#if approvals.filter((a) => a.status === 'pending').length > 0}
              <span class="badge badge-warning">{approvals.filter((a) => a.status === 'pending').length} pending</span>
            {/if}
            <button
              type="button"
              class="btn btn-ghost btn-sm"
              disabled={planCreating}
              onclick={handleCreatePlan}
            >
              {planCreating ? 'Creating...' : 'New cleanup plan'}
            </button>
          </div>
        </div>

        {#if approvals.length === 0}
          <div class="empty-state"><p>No approvals found.</p></div>
        {:else}
          <div class="approval-list">
            {#each approvals as approval}
              <div class="approval-item" class:approval-pending={approval.status === 'pending'}>
                <div class="approval-top">
                  <div class="approval-info">
                    <strong class="mono">{approval.id}</strong>
                    <span class="badge {statusBadge(approval.status)}">{approval.status}</span>
                  </div>
                  <span class="approval-time">{fmt(approval.requested_at)}</span>
                </div>

                <div class="approval-detail">
                  <span>{approval.plan.candidates.length} candidates</span>
                  <span class="approval-dot"></span>
                  <span>{fmtBytes(approval.plan.total_bytes)}</span>
                  {#if approval.note}
                    <span class="approval-dot"></span>
                    <span class="approval-note">{compact(approval.note, 80)}</span>
                  {/if}
                </div>

                {#if approval.status === 'pending'}
                  <div class="approval-actions">
                    <button
                      type="button"
                      class="btn btn-sm btn-primary"
                      disabled={reviewingId === approval.id}
                      onclick={() => { void handleReview(approval.id, 'approve') }}
                    >
                      Approve
                    </button>
                    <button
                      type="button"
                      class="btn btn-sm btn-danger"
                      disabled={reviewingId === approval.id}
                      onclick={() => { void handleReview(approval.id, 'reject') }}
                    >
                      Reject
                    </button>
                  </div>
                {/if}

                {#if approval.plan.candidates.length > 0}
                  <details class="approval-candidates">
                    <summary>{approval.plan.candidates.length} cleanup candidates</summary>
                    <div class="candidate-list">
                      {#each approval.plan.candidates as candidate}
                        <div class="candidate-row">
                          <span class="mono candidate-path">{candidate.path}</span>
                          <span class="candidate-size">{fmtBytes(candidate.size_bytes)}</span>
                          {#if candidate.reason}
                            <span class="candidate-reason">{candidate.reason}</span>
                          {/if}
                        </div>
                      {/each}
                    </div>
                  </details>
                {/if}
              </div>
            {/each}
          </div>
        {/if}
      </section>

      <!-- Cron Jobs -->
      <section class="card ops-section">
        <div class="card-header">
          <span class="card-title">Cron jobs</span>
          <span class="badge badge-default">{cronJobs.length} total</span>
        </div>

        {#if cronJobs.length === 0}
          <div class="empty-state"><p>No cron jobs configured.</p></div>
        {:else}
          <div class="cron-list">
            {#each cronJobs as job}
              <div class="cron-item">
                <button
                  type="button"
                  class="cron-item-btn"
                  class:active={expandedJob === job.id}
                  onclick={() => { void toggleJobRuns(job.id) }}
                >
                  <div class="cron-item-top">
                    <strong class="cron-name">{job.name}</strong>
                    <div class="cron-badges">
                      <span
                        class="badge"
                        class:badge-success={job.enabled && !job.last_run_error}
                        class:badge-error={!!job.last_run_error}
                        class:badge-default={!job.enabled}
                      >
                        {job.last_run_error ? 'failed' : job.enabled ? 'active' : 'disabled'}
                      </span>
                      <span class="badge badge-default">{job.schedule}</span>
                    </div>
                  </div>
                  <p class="cron-prompt">{compact(job.prompt, 120)}</p>
                  <div class="cron-meta">
                    {#if job.project_id}
                      <span>project: {job.project_id}</span>
                    {/if}
                    {#if job.last_run_at}
                      <span>Last run: {fmt(job.last_run_at)}</span>
                    {/if}
                    {#if job.last_run_error}
                      <span class="cron-error">{compact(job.last_run_error, 80)}</span>
                    {/if}
                  </div>
                </button>

                {#if expandedJob === job.id}
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
      </section>
    </div>
  {/if}
</div>

<style>
  .ops {
    animation: fadeIn var(--duration-normal) var(--ease-out);
  }

  @keyframes fadeIn {
    from { opacity: 0; transform: translateY(8px); }
    to { opacity: 1; transform: translateY(0); }
  }

  .ops-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-4);
    margin-bottom: var(--space-6);
  }

  .ops-header h2 {
    font-size: var(--text-2xl);
    margin-bottom: var(--space-1);
  }

  .ops-subtitle {
    color: var(--text-tertiary);
  }

  .ops-loading {
    padding: var(--space-10);
    text-align: center;
    color: var(--text-tertiary);
  }

  /* ── System Status Strip ──────────────────────── */
  .ops-status-strip {
    display: flex;
    align-items: center;
    gap: var(--space-6);
    padding: var(--space-5) var(--space-6);
    background: var(--bg-surface);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg);
    margin-bottom: var(--space-6);
  }

  .status-gauge {
    display: flex;
    align-items: center;
    gap: var(--space-4);
  }

  .gauge-ring {
    position: relative;
    width: 56px;
    height: 56px;
    flex-shrink: 0;
  }

  .gauge-ring.disk-ok { color: var(--success); }
  .gauge-ring.disk-warning { color: var(--warning); }
  .gauge-ring.disk-critical { color: var(--error); }

  .gauge-svg {
    width: 100%;
    height: 100%;
    transform: rotate(-90deg);
  }

  .gauge-text {
    position: absolute;
    inset: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    font-family: var(--font-display);
    font-size: var(--text-xs);
    font-weight: 600;
    color: var(--text-primary);
  }

  .gauge-label {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .gauge-label strong {
    font-family: var(--font-display);
    font-size: var(--text-sm);
    font-weight: 500;
    color: var(--text-primary);
  }

  .gauge-label span {
    font-size: var(--text-xs);
    color: var(--text-tertiary);
  }

  .status-divider {
    width: 1px;
    height: 32px;
    background: var(--border-subtle);
    flex-shrink: 0;
  }

  .status-metric {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }

  .metric-value {
    font-family: var(--font-display);
    font-size: var(--text-lg);
    font-weight: 600;
    color: var(--text-primary);
    line-height: 1;
  }

  .metric-value.metric-time {
    font-size: var(--text-sm);
    font-weight: 400;
  }

  .metric-label {
    font-size: var(--text-xs);
    color: var(--text-tertiary);
  }

  .status-unavailable {
    color: var(--text-tertiary);
    font-size: var(--text-sm);
  }

  /* ── Grid ─────────────────────────────────────── */
  .ops-grid {
    display: grid;
    grid-template-columns: 1fr;
    gap: var(--space-4);
  }

  .ops-section {
    min-height: 200px;
  }

  .card-header-actions {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  /* ── Approvals ────────────────────────────────── */
  .approval-list {
    display: grid;
    gap: var(--space-2);
  }

  .approval-item {
    padding: var(--space-3) var(--space-4);
    border-radius: var(--radius-md);
    background: var(--bg-base);
    border: 1px solid transparent;
  }

  .approval-pending {
    border-color: rgba(251, 191, 36, 0.2);
    background: rgba(251, 191, 36, 0.04);
  }

  .approval-top {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
    margin-bottom: var(--space-1);
  }

  .approval-info {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-width: 0;
  }

  .approval-info strong {
    font-size: var(--text-xs);
    color: var(--text-primary);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .approval-time {
    font-size: var(--text-xs);
    color: var(--text-ghost);
    flex-shrink: 0;
  }

  .approval-detail {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    font-size: var(--text-sm);
    color: var(--text-secondary);
    margin-bottom: var(--space-2);
  }

  .approval-dot {
    width: 3px;
    height: 3px;
    border-radius: 50%;
    background: var(--text-ghost);
    flex-shrink: 0;
  }

  .approval-note {
    color: var(--text-tertiary);
  }

  .approval-actions {
    display: flex;
    gap: var(--space-2);
    margin-bottom: var(--space-2);
  }

  .approval-candidates {
    margin-top: var(--space-2);
  }

  .approval-candidates summary {
    font-size: var(--text-xs);
    color: var(--text-tertiary);
    cursor: pointer;
    user-select: none;
  }

  .approval-candidates summary:hover {
    color: var(--text-secondary);
  }

  .candidate-list {
    display: grid;
    gap: var(--space-1);
    margin-top: var(--space-2);
    padding: var(--space-2) var(--space-3);
    background: var(--bg-surface);
    border-radius: var(--radius-sm);
    max-height: 200px;
    overflow-y: auto;
  }

  .candidate-row {
    display: flex;
    align-items: baseline;
    gap: var(--space-3);
    font-size: var(--text-xs);
  }

  .candidate-path {
    color: var(--text-secondary);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    min-width: 0;
    flex: 1;
  }

  .candidate-size {
    color: var(--text-tertiary);
    flex-shrink: 0;
  }

  .candidate-reason {
    color: var(--text-ghost);
    flex-shrink: 0;
  }

  /* ── Cron Jobs ────────────────────────────────── */
  .cron-list {
    display: grid;
    gap: var(--space-2);
  }

  .cron-item-btn {
    display: block;
    width: 100%;
    padding: var(--space-3) var(--space-4);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--bg-base);
    text-align: left;
    cursor: pointer;
    transition:
      border-color var(--duration-fast) var(--ease-out),
      background var(--duration-fast) var(--ease-out);
  }

  .cron-item-btn:hover {
    border-color: var(--border-default);
    background: var(--bg-elevated);
  }

  .cron-item-btn.active {
    border-color: var(--accent);
    background: var(--accent-muted);
  }

  .cron-item-top {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
    margin-bottom: var(--space-1);
  }

  .cron-name {
    font-family: var(--font-display);
    font-size: var(--text-sm);
    font-weight: 500;
    color: var(--text-primary);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    min-width: 0;
  }

  .cron-badges {
    display: flex;
    gap: var(--space-1);
    flex-shrink: 0;
  }

  .cron-prompt {
    font-size: var(--text-sm);
    color: var(--text-secondary);
    margin-bottom: var(--space-1);
  }

  .cron-meta {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-1) var(--space-3);
    font-size: var(--text-xs);
    color: var(--text-ghost);
  }

  .cron-error {
    color: var(--error);
  }

  /* ── Run history ──────────────────────────────── */
  .cron-runs {
    padding: var(--space-2) var(--space-4) var(--space-3);
  }

  .runs-loading,
  .runs-empty {
    font-size: var(--text-xs);
    color: var(--text-tertiary);
    padding: var(--space-2) 0;
  }

  .run-item {
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-sm);
    margin-bottom: var(--space-1);
  }

  .run-item.run-error {
    background: var(--error-muted);
  }

  .run-top {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
    margin-bottom: var(--space-1);
  }

  .run-time {
    font-size: var(--text-xs);
    color: var(--text-ghost);
  }

  .run-detail {
    font-size: var(--text-xs);
    color: var(--text-secondary);
    white-space: pre-wrap;
    word-break: break-word;
  }

  .run-error-text {
    color: var(--error);
  }

  /* ── Responsive ───────────────────────────────── */
  @media (max-width: 600px) {
    .ops-status-strip {
      flex-wrap: wrap;
      gap: var(--space-4);
    }

    .status-divider {
      display: none;
    }
  }
</style>
