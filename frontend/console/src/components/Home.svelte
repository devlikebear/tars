<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import {
    getEventsHistory,
    listApprovals,
    listCronJobs,
    listProjects,
    streamEvents,
  } from '../lib/api'
  import type {
    Approval,
    CronJob,
    NotificationMessage,
    Project,
  } from '../lib/types'
  import ChatPanel from './ChatPanel.svelte'

  interface Props {
    onNavigate: (path: string) => void
    initialPrompt?: string
  }

  let { onNavigate, initialPrompt }: Props = $props()

  let projects: Project[] = $state([])
  let approvals: Approval[] = $state([])
  let cronJobs: CronJob[] = $state([])
  let notifications: NotificationMessage[] = $state([])
  let unreadCount = $state(0)

  let loading = $state(true)
  let error = $state('')
  let stopStream: (() => void) | null = null

  function fmt(value?: string): string {
    const text = value?.trim()
    if (!text) return '\u2014'
    const date = new Date(text)
    if (Number.isNaN(date.getTime())) return text
    return new Intl.DateTimeFormat('en', { dateStyle: 'medium', timeStyle: 'short' }).format(date)
  }

  function compact(value?: string, max = 120): string {
    const text = value?.trim()
    if (!text) return '\u2014'
    return text.length <= max ? text : `${text.slice(0, max - 1)}\u2026`
  }

  function pendingApprovals(list: Approval[]): Approval[] {
    return list.filter((a) => a.status === 'pending')
  }

  function failedCronJobs(list: CronJob[]): CronJob[] {
    return list.filter((j) => j.last_run_error?.trim())
  }

  function goToProject(projectId: string) {
    onNavigate(`/console/projects/${encodeURIComponent(projectId)}`)
  }

  async function load() {
    loading = true
    error = ''
    try {
      const [p, a, c, e] = await Promise.allSettled([
        listProjects(),
        listApprovals(),
        listCronJobs(),
        getEventsHistory(20),
      ])
      projects = p.status === 'fulfilled' ? p.value : []
      approvals = a.status === 'fulfilled' ? a.value : []
      cronJobs = c.status === 'fulfilled' ? c.value : []
      if (e.status === 'fulfilled') {
        notifications = (e.value.items ?? []).slice(0, 10)
        unreadCount = e.value.unread_count ?? 0
      }
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load overview'
    } finally {
      loading = false
    }
  }

  function startEventStream() {
    stopStream?.()
    stopStream = streamEvents(
      undefined,
      (event) => {
        notifications = [event, ...notifications.filter((n) => n.id !== event.id)].slice(0, 10)
        unreadCount++

        if (event.category === 'cron' || event.category === 'watchdog') {
          void listCronJobs().then((jobs) => { cronJobs = jobs })
        }
        if (event.category === 'ops') {
          void listApprovals().then((list) => { approvals = list })
        }
        if (event.category === 'project') {
          void listProjects().then((list) => { projects = list })
        }
      },
    )
  }

  onMount(() => {
    void load()
    startEventStream()
  })

  onDestroy(() => {
    stopStream?.()
  })
</script>

<div class="home">
  <div class="home-header">
    <div>
      <h2>Welcome back</h2>
      <p class="home-subtitle">Here's what needs your attention.</p>
    </div>
  </div>

  {#if error}
    <div class="error-banner">{error}</div>
  {/if}

  {#if loading}
    <div class="home-loading">Loading overview...</div>
  {:else}
    <!-- Pulse strip -->
    <div class="pulse-strip">
      <div class="pulse-item">
        <span class="pulse-value">{projects.length}</span>
        <span class="pulse-label">Projects</span>
      </div>
      <div class="pulse-divider"></div>
      <div class="pulse-item">
        <span class="pulse-value" class:has-attention={pendingApprovals(approvals).length > 0}>
          {pendingApprovals(approvals).length}
        </span>
        <span class="pulse-label">Pending approvals</span>
      </div>
      <div class="pulse-divider"></div>
      <div class="pulse-item">
        <span class="pulse-value" class:has-warning={failedCronJobs(cronJobs).length > 0}>
          {failedCronJobs(cronJobs).length}
        </span>
        <span class="pulse-label">Failed cron jobs</span>
      </div>
      <div class="pulse-divider"></div>
      <div class="pulse-item">
        <span class="pulse-value">{unreadCount}</span>
        <span class="pulse-label">Unread notifications</span>
      </div>
    </div>

    <!-- Chat (main feature) -->
    <section class="chat-main card">
      <ChatPanel {initialPrompt} />
    </section>

    <div class="home-grid">
      <!-- Projects -->
      <section class="card home-section">
        <div class="card-header">
          <span class="card-title">Active projects</span>
          <button type="button" class="btn btn-ghost btn-sm" onclick={() => onNavigate('/console/projects')}>
            View all
          </button>
        </div>
        {#if projects.length === 0}
          <div class="empty-state">
            <p>No projects yet. Go to <strong>Projects</strong> to create one.</p>
          </div>
        {:else}
          <div class="project-cards">
            {#each projects.slice(0, 6) as project}
              <button type="button" class="project-card" onclick={() => goToProject(project.id)}>
                <div class="project-card-top">
                  <strong class="project-card-name">{project.name}</strong>
                  <span class="badge badge-default">{project.status || 'active'}</span>
                </div>
                {#if project.objective}
                  <p class="project-card-desc">{compact(project.objective, 100)}</p>
                {/if}
                <span class="project-card-time">{fmt(project.updated_at)}</span>
              </button>
            {/each}
          </div>
        {/if}
      </section>

      <!-- Approvals -->
      <section class="card home-section">
        <div class="card-header">
          <span class="card-title">Pending approvals</span>
          {#if pendingApprovals(approvals).length > 0}
            <span class="badge badge-warning">{pendingApprovals(approvals).length}</span>
          {/if}
        </div>
        {#if pendingApprovals(approvals).length === 0}
          <div class="empty-state">
            <p>No pending approvals right now.</p>
          </div>
        {:else}
          <div class="list-items">
            {#each pendingApprovals(approvals) as approval}
              <div class="list-item">
                <div class="list-item-top">
                  <strong>{approval.type}</strong>
                  <span class="text-tertiary">{fmt(approval.requested_at)}</span>
                </div>
                <p class="list-item-detail">
                  {approval.plan.candidates.length} candidates, {approval.plan.total_bytes} bytes
                </p>
              </div>
            {/each}
          </div>
        {/if}
      </section>

      <!-- Cron health -->
      <section class="card home-section">
        <div class="card-header">
          <span class="card-title">Cron jobs</span>
          <span class="badge badge-default">{cronJobs.length} total</span>
        </div>
        {#if cronJobs.length === 0}
          <div class="empty-state">
            <p>No cron jobs configured.</p>
          </div>
        {:else}
          <div class="list-items">
            {#each cronJobs.slice(0, 5) as job}
              <div class="list-item">
                <div class="list-item-top">
                  <strong>{job.name}</strong>
                  <span class="badge" class:badge-success={job.enabled && !job.last_run_error}
                    class:badge-error={!!job.last_run_error}
                    class:badge-default={!job.enabled}>
                    {job.last_run_error ? 'failed' : job.enabled ? job.schedule : 'disabled'}
                  </span>
                </div>
                <p class="list-item-detail">{compact(job.prompt, 100)}</p>
                {#if job.last_run_at}
                  <span class="list-item-time">Last run {fmt(job.last_run_at)}</span>
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
  .home {
    animation: fadeIn var(--duration-normal) var(--ease-out);
  }

  @keyframes fadeIn {
    from { opacity: 0; transform: translateY(8px); }
    to { opacity: 1; transform: translateY(0); }
  }

  .home-header {
    margin-bottom: var(--space-6);
  }

  .home-header h2 {
    font-size: var(--text-2xl);
    margin-bottom: var(--space-1);
  }

  .home-subtitle {
    color: var(--text-tertiary);
    font-size: var(--text-base);
  }

  .home-loading {
    padding: var(--space-10);
    text-align: center;
    color: var(--text-tertiary);
  }

  /* ── Pulse strip ──────────────────────────────── */
  .pulse-strip {
    display: flex;
    align-items: center;
    gap: var(--space-5);
    padding: var(--space-5) var(--space-6);
    background: var(--bg-surface);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg);
    margin-bottom: var(--space-6);
  }

  .pulse-item {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }

  .pulse-value {
    font-family: var(--font-display);
    font-size: var(--text-xl);
    font-weight: 600;
    color: var(--text-primary);
    line-height: 1;
  }

  .pulse-value.has-attention {
    color: var(--warning);
  }

  .pulse-value.has-warning {
    color: var(--error);
  }

  .pulse-label {
    font-size: var(--text-xs);
    color: var(--text-tertiary);
  }

  .pulse-divider {
    width: 1px;
    height: 32px;
    background: var(--border-subtle);
    flex-shrink: 0;
  }

  /* ── Chat main ────────────────────────────────── */
  .chat-main {
    margin-bottom: var(--space-5);
  }

  /* ── Home grid ────────────────────────────────── */
  .home-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: var(--space-4);
  }

  .home-section {
    min-height: 200px;
  }

  /* ── Project cards ────────────────────────────── */
  .project-cards {
    display: grid;
    gap: var(--space-2);
  }

  .project-card {
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

  .project-card:hover {
    border-color: var(--border-default);
    background: var(--bg-elevated);
  }

  .project-card-top {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
    margin-bottom: var(--space-1);
  }

  .project-card-name {
    font-family: var(--font-display);
    font-size: var(--text-sm);
    font-weight: 500;
    color: var(--text-primary);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    min-width: 0;
  }

  .project-card-desc {
    font-size: var(--text-sm);
    color: var(--text-secondary);
    margin-bottom: var(--space-2);
    display: -webkit-box;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  .project-card-time {
    font-size: var(--text-xs);
    color: var(--text-ghost);
  }

  /* ── List items ───────────────────────────────── */
  .list-items {
    display: grid;
    gap: var(--space-2);
  }

  .list-item {
    padding: var(--space-3) var(--space-4);
    border-radius: var(--radius-md);
    background: var(--bg-base);
  }

  .list-item-top {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
    margin-bottom: var(--space-1);
  }

  .list-item-top strong {
    font-family: var(--font-display);
    font-size: var(--text-sm);
    font-weight: 500;
    color: var(--text-primary);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    min-width: 0;
  }

  .list-item-detail {
    font-size: var(--text-sm);
    color: var(--text-secondary);
    margin-bottom: var(--space-1);
  }

  .list-item-time {
    font-size: var(--text-xs);
    color: var(--text-ghost);
  }

  .text-tertiary {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
  }

  /* ── Responsive ───────────────────────────────── */
  @media (max-width: 900px) {
    .home-grid {
      grid-template-columns: 1fr;
    }
  }

  @media (max-width: 600px) {
    .pulse-strip {
      flex-wrap: wrap;
      gap: var(--space-4);
    }

    .pulse-divider {
      display: none;
    }

    .pulse-item {
      flex: 1;
      min-width: 80px;
    }
  }
</style>
