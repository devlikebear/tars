<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import {
    getConfig,
    getEventsHistory,
    getGlobalPlans,
    getOpsStatus,
    getPulseStatus,
    getReflectionStatus,
    getServerStatus,
    getSessionTasks,
    getSyspromptFile,
    listAgentRuntimeRuns,
    listCronJobs,
    listSessions,
    streamEvents,
  } from '../lib/api'
  import type {
    AgentRuntimeRun,
    ConfigFile,
    CronJob,
    GlobalPlanItem,
    NotificationMessage,
    OpsStatus,
    PulseSnapshot,
    ReflectionSnapshot,
    Session,
    SessionTasks,
    SyspromptFile,
  } from '../lib/types'

  interface Props {
    onNavigate: (path: string) => void
  }

  type Recommendation = {
    id: string
    title: string
    detail: string
    path: string
    action: string
  }

  let { onNavigate }: Props = $props()

  let pulse: PulseSnapshot | null = $state(null)
  let reflection: ReflectionSnapshot | null = $state(null)
  let ops: OpsStatus | null = $state(null)
  let notifications: NotificationMessage[] = $state([])
  let sessions: Session[] = $state([])
  let plans: GlobalPlanItem[] = $state([])
  let cronJobs: CronJob[] = $state([])
  let agentRuns: AgentRuntimeRun[] = $state([])
  let serverVersion = $state('')
  let userFile: SyspromptFile | null = $state(null)
  let config: ConfigFile | null = $state(null)
  let continueSession: { session: Session; tasks: SessionTasks } | null = $state(null)
  let unreadCount = $state(0)
  let loading = $state(true)
  let error = $state('')
  let stopStream: (() => void) | null = null
  let pollTimer: ReturnType<typeof setInterval> | null = null

  let mainSessions = $derived(
    sessions
      .filter((session) => !session.hidden && (session.kind ?? 'main') === 'main')
      .sort((a, b) => Date.parse(b.updated_at || '') - Date.parse(a.updated_at || '')),
  )
  let recentMainSessions = $derived(mainSessions.slice(0, 6))
  let todaySessions = $derived(mainSessions.filter((session) => isToday(session.updated_at)))
  let recentPlans = $derived(plans.slice(0, 4))
  let activeCronJobs = $derived(cronJobs.filter((job) => job.enabled && !isCronCompleted(job)))
  let failedCronJobs = $derived(cronJobs.filter((job) => !!job.last_run_error))
  let recentAgentRuns = $derived(agentRuns.slice(0, 5))
  let activeAgentRuns = $derived(agentRuns.filter((run) => isAgentRunActive(run)))
  let releaseHref = $derived.by(() => {
    const version = serverVersion.trim().replace(/^v/, '')
    if (!/^\d+\.\d+\.\d+/.test(version)) return ''
    return `https://github.com/devlikebear/tars/releases/tag/v${version}`
  })
  let recommendedActions = $derived.by<Recommendation[]>(() => {
    const actions: Recommendation[] = []
    if (isUserFileBlank(userFile)) {
      actions.push({
        id: 'user-md',
        title: 'USER.md is empty',
        detail: 'Add your identity and preferences.',
        path: '/console/sysprompt',
        action: 'Open System Prompt',
      })
    }
    if (needsAnthropicSetup(config)) {
      actions.push({
        id: 'anthropic-key',
        title: 'ANTHROPIC_API_KEY is not configured',
        detail: 'Add provider credentials before model calls.',
        path: '/console/config',
        action: 'Open Settings',
      })
    }
    if (todaySessions.length === 0) {
      actions.push({
        id: 'new-chat',
        title: 'No activity today',
        detail: 'Start a fresh main chat.',
        path: '/console/chat',
        action: 'New Chat',
      })
    }
    return actions.slice(0, 3)
  })

  function fmt(value?: string): string {
    const text = value?.trim()
    if (!text) return '-'
    const date = new Date(text)
    if (Number.isNaN(date.getTime())) return text
    return new Intl.DateTimeFormat('en', { dateStyle: 'medium', timeStyle: 'short' }).format(date)
  }

  function compact(value?: string, max = 110): string {
    const text = value?.trim()
    if (!text) return '-'
    return text.length <= max ? text : `${text.slice(0, max - 1)}...`
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

  function isToday(value?: string): boolean {
    if (!value?.trim()) return false
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return false
    const now = new Date()
    return date.getFullYear() === now.getFullYear()
      && date.getMonth() === now.getMonth()
      && date.getDate() === now.getDate()
  }

  function hasRealTimestamp(value?: string): boolean {
    if (!value?.trim()) return false
    const date = new Date(value)
    return !Number.isNaN(date.getTime()) && date.getFullYear() > 1
  }

  function isUserFileBlank(file: SyspromptFile | null): boolean {
    if (!file?.exists) return true
    const content = file.content?.trim() ?? ''
    if (!content) return true
    return !!file.starter_content?.trim() && content === file.starter_content.trim()
  }

  function needsAnthropicSetup(file: ConfigFile | null): boolean {
    const content = file?.content ?? ''
    if (!/kind:\s*anthropic/i.test(content)) return false
    return !/ANTHROPIC_API_KEY/.test(content) && !/api_key:\s*["']?[^"'\s#]/i.test(content)
  }

  function diskUsedLabel(): string {
    if (!ops) return 'unknown'
    return `${Math.round(ops.disk_used_percent)}% used`
  }

  function diskClass(): string {
    const used = ops?.disk_used_percent ?? 0
    if (used >= 90) return 'danger'
    if (used >= 80) return 'warn'
    return 'ok'
  }

  function pulseLabel(): string {
    if (pulse?.last_err) return 'error'
    if ((pulse?.total_ticks ?? 0) > 0) return 'active'
    return 'idle'
  }

  function reflectionLabel(): string {
    if ((reflection?.consecutive_failures ?? 0) > 0) return 'failing'
    if (hasRealTimestamp(reflection?.last_successful_run_at)) return 'healthy'
    return 'idle'
  }

  function planProgressPercent(item: GlobalPlanItem): number {
    const total = Math.max(0, item.summary?.total ?? item.tasks.length)
    if (total === 0) return 0
    return Math.max(0, Math.min(100, Math.round((planFinishedCount(item) / total) * 100)))
  }

  function planFinishedCount(item: GlobalPlanItem): number {
    return (item.summary?.completed ?? 0) + (item.summary?.cancelled ?? 0)
  }

  function isCronCompleted(job: CronJob): boolean {
    return isOneShotCron(job) && Boolean(job.last_run_at)
  }

  function isOneShotCron(job: CronJob): boolean {
    const schedule = job.schedule.trim().toLowerCase()
    return job.delete_after_run === true || schedule.startsWith('at:')
  }

  function cronStatusLabel(job: CronJob): string {
    if (job.last_run_error) return 'failed'
    if (isCronCompleted(job)) return 'done'
    return job.enabled ? 'active' : 'paused'
  }

  function nextCronRunLabel(job: CronJob): string {
    if (isCronCompleted(job)) return 'Completed'
    if (!job.enabled) return 'Paused'
    const schedule = job.schedule.trim()
    if (schedule.toLowerCase().startsWith('at:')) return fmt(schedule.slice(3))
    if (schedule.toLowerCase().startsWith('every:')) return job.last_run_at ? `After ${relativeTime(job.last_run_at)}` : 'Next tick'
    return 'Cron schedule'
  }

  function isAgentRunActive(run: AgentRuntimeRun): boolean {
    const status = run.status.trim().toLowerCase()
    return status === 'running' || status === 'queued' || status === 'pending' || status === 'in_progress'
  }

  function agentRunTime(run: AgentRuntimeRun): string {
    return run.updated_at || run.completed_at || run.started_at || run.created_at || ''
  }

  function agentRunTitle(run: AgentRuntimeRun): string {
    return run.prompt?.trim() || run.agent?.trim() || run.run_id
  }

  function planSummary(tasks: SessionTasks): string {
    if (tasks.plan?.goal?.trim()) return tasks.plan.goal.trim()
    const active = tasks.tasks.find((task) => task.status === 'in_progress') ?? tasks.tasks[0]
    return active?.title?.trim() || 'Open session plan'
  }

  function openContinueSession() {
    if (!continueSession) return
    onNavigate(`/console/chat/${encodeURIComponent(continueSession.session.id)}`)
  }

  function openPlanSession(sessionId: string) {
    onNavigate(`/console/chat?session=${encodeURIComponent(sessionId)}`)
  }

  async function loadContinueSession(candidates: Session[]) {
    const results = await Promise.allSettled(
      candidates.slice(0, 5).map(async (session) => ({
        session,
        tasks: await getSessionTasks(session.id),
      })),
    )
    continueSession = results
      .filter((result): result is PromiseFulfilledResult<{ session: Session; tasks: SessionTasks }> => result.status === 'fulfilled')
      .map((result) => result.value)
      .find((item) => !!item.tasks.plan || item.tasks.tasks.length > 0) ?? null
  }

  async function load(showLoading = true) {
    if (showLoading) loading = true
    error = ''
    try {
      const [
        statusResult,
        pulseResult,
        reflectionResult,
        opsResult,
        eventsResult,
        sessionsResult,
        plansResult,
        cronResult,
        runsResult,
        userResult,
        configResult,
      ] = await Promise.allSettled([
        getServerStatus(),
        getPulseStatus(),
        getReflectionStatus(),
        getOpsStatus(),
        getEventsHistory(10),
        listSessions(false),
        getGlobalPlans(true),
        listCronJobs(),
        listAgentRuntimeRuns({ limit: 8 }),
        getSyspromptFile('workspace', 'USER.md'),
        getConfig(),
      ])

      serverVersion = statusResult.status === 'fulfilled' ? statusResult.value.version : ''
      pulse = pulseResult.status === 'fulfilled' ? pulseResult.value : null
      reflection = reflectionResult.status === 'fulfilled' ? reflectionResult.value : null
      ops = opsResult.status === 'fulfilled' ? opsResult.value : null
      if (eventsResult.status === 'fulfilled') {
        notifications = (eventsResult.value.items ?? []).slice(0, 10)
        unreadCount = eventsResult.value.unread_count ?? 0
      }
      sessions = sessionsResult.status === 'fulfilled' ? sessionsResult.value : []
      plans = plansResult.status === 'fulfilled' ? plansResult.value.items : []
      cronJobs = cronResult.status === 'fulfilled' ? cronResult.value : []
      agentRuns = runsResult.status === 'fulfilled' ? runsResult.value : []
      userFile = userResult.status === 'fulfilled' ? userResult.value : null
      config = configResult.status === 'fulfilled' ? configResult.value : null
      await loadContinueSession(
        (sessionsResult.status === 'fulfilled' ? sessionsResult.value : [])
          .filter((session) => !session.hidden && (session.kind ?? 'main') === 'main')
          .sort((a, b) => Date.parse(b.updated_at || '') - Date.parse(a.updated_at || '')),
      )
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load home dashboard'
    } finally {
      if (showLoading) loading = false
    }
  }

  function startEventStream() {
    stopStream?.()
    stopStream = streamEvents((event) => {
      notifications = [event, ...notifications.filter((item) => item.id !== event.id)].slice(0, 10)
      unreadCount++
      if (event.category === 'pulse') {
        void getPulseStatus().then((snapshot) => { pulse = snapshot }).catch(() => {})
        void getReflectionStatus().then((snapshot) => { reflection = snapshot }).catch(() => {})
      }
    })
  }

  onMount(() => {
    void load()
    pollTimer = setInterval(() => {
      void load(false)
    }, 30_000)
    startEventStream()
  })

  onDestroy(() => {
    stopStream?.()
    if (pollTimer) clearInterval(pollTimer)
  })
</script>

<div class="home">
  <div class="home-header">
    <div>
      <h2>Mission Control</h2>
      <p class="home-subtitle">Live work, automation, health, and delivery state.</p>
    </div>
    <button type="button" class="btn btn-primary" onclick={() => onNavigate('/console/chat')}>New Chat</button>
  </div>

  {#if error}
    <div class="error-banner">{error}</div>
  {/if}

  {#if loading}
    <div class="home-loading">Loading Mission Control...</div>
  {:else}
    <section class="status-strip" aria-label="Home status strip">
      <button type="button" class="status-tile" onclick={() => onNavigate('/console/pulse')}>
        <span class="status-label">Pulse</span>
        <strong class:status-ok={pulseLabel() === 'active'} class:status-danger={pulseLabel() === 'error'}>{pulseLabel()}</strong>
        <span>{pulse?.last_tick_at ? relativeTime(pulse.last_tick_at) : 'never'}</span>
      </button>
      <button type="button" class="status-tile" onclick={() => onNavigate('/console/reflection')}>
        <span class="status-label">Reflection</span>
        <strong class:status-ok={reflectionLabel() === 'healthy'} class:status-danger={reflectionLabel() === 'failing'}>{reflectionLabel()}</strong>
        <span>{reflection?.last_successful_run_at ? relativeTime(reflection.last_successful_run_at) : 'never'}</span>
      </button>
      <button type="button" class="status-tile" onclick={() => onNavigate('/console/tasks')}>
        <span class="status-label">Active plans</span>
        <strong>{plans.length}</strong>
        <span>{plans.reduce((total, item) => total + (item.summary?.in_progress ?? 0), 0)} task active</span>
      </button>
      <button type="button" class="status-tile" onclick={() => onNavigate('/console/agentruntime')}>
        <span class="status-label">Agent runs</span>
        <strong>{activeAgentRuns.length}</strong>
        <span>{agentRuns.length} recent</span>
      </button>
      <button type="button" class="status-tile" onclick={() => onNavigate('/console/cron')}>
        <span class="status-label">Cron jobs</span>
        <strong class:status-danger={failedCronJobs.length > 0}>{activeCronJobs.length}</strong>
        <span>{failedCronJobs.length > 0 ? `${failedCronJobs.length} failed` : `${cronJobs.length} total`}</span>
      </button>
      <button type="button" class="status-tile" onclick={() => onNavigate('/console/approvals')}>
        <span class="status-label">Disk pressure</span>
        <strong class:status-ok={diskClass() === 'ok'} class:status-warn={diskClass() === 'warn'} class:status-danger={diskClass() === 'danger'}>{diskUsedLabel()}</strong>
        <span>{ops ? `${Math.round(ops.disk_free_bytes / 1024 / 1024 / 1024)} GB free` : 'ops unavailable'}</span>
      </button>
      <button type="button" class="status-tile" onclick={() => onNavigate('/console/chat')}>
        <span class="status-label">Active sessions</span>
        <strong>{mainSessions.length}</strong>
        <span>{todaySessions.length} touched today</span>
      </button>
    </section>

    <div class="dashboard-grid">
      <section class="dashboard-section plans-section">
        <div class="section-heading">
          <div>
            <h3>Active plans</h3>
            <p>Current task contracts across sessions.</p>
          </div>
          <button type="button" class="btn btn-ghost btn-sm" onclick={() => onNavigate('/console/tasks')}>Open Plans</button>
        </div>
        {#if recentPlans.length === 0}
          <div class="empty-state"><p>No active plans.</p></div>
        {:else}
          <div class="work-list">
            {#each recentPlans as item}
              {@const percent = planProgressPercent(item)}
              <button type="button" class="work-row" onclick={() => openPlanSession(item.session.id)}>
                <span class="work-topline">
                  <strong>{compact(item.plan.goal, 120)}</strong>
                  <span class="badge badge-default">{item.plan.status ?? 'executing'}</span>
                </span>
                <span class="mini-progress" aria-label={`${percent}% complete`}><span style={`width: ${percent}%`}></span></span>
                <span class="work-meta">{planFinishedCount(item)}/{item.summary?.total ?? item.tasks.length} done · {item.summary?.in_progress ?? 0} active · updated {relativeTime(item.updated_at)}</span>
              </button>
            {/each}
          </div>
        {/if}
      </section>

      <section class="dashboard-section">
        <div class="section-heading">
          <div>
            <h3>Agent runs</h3>
            <p>Latest runtime work and active delegated agents.</p>
          </div>
          <button type="button" class="btn btn-ghost btn-sm" onclick={() => onNavigate('/console/agentruntime')}>Open Runtime</button>
        </div>
        {#if recentAgentRuns.length === 0}
          <div class="empty-state"><p>No agent runs recorded.</p></div>
        {:else}
          <div class="work-list">
            {#each recentAgentRuns as run}
              <button type="button" class="work-row" onclick={() => onNavigate(`/console/agentruntime/runs/${encodeURIComponent(run.run_id)}`)}>
                <span class="work-topline">
                  <strong>{compact(agentRunTitle(run), 120)}</strong>
                  <span class="badge" class:badge-accent={isAgentRunActive(run)} class:badge-default={!isAgentRunActive(run)}>{run.status}</span>
                </span>
                <span class="work-meta">{run.agent || 'agent'} · {run.tier || 'tier'} · {relativeTime(agentRunTime(run))}</span>
              </button>
            {/each}
          </div>
        {/if}
      </section>

      <section class="dashboard-section">
        <div class="section-heading">
          <div>
            <h3>Cron jobs</h3>
            <p>Scheduled automation, paused jobs, and recent failures.</p>
          </div>
          <button type="button" class="btn btn-ghost btn-sm" onclick={() => onNavigate('/console/cron')}>Open Cron</button>
        </div>
        {#if cronJobs.length === 0}
          <div class="empty-state"><p>No cron jobs configured.</p></div>
        {:else}
          <div class="work-list">
            {#each cronJobs.slice(0, 5) as job}
              <button type="button" class="work-row" onclick={() => onNavigate('/console/cron')}>
                <span class="work-topline">
                  <strong>{job.name || compact(job.prompt, 80)}</strong>
                  <span class="badge" class:badge-error={!!job.last_run_error} class:badge-success={job.enabled && !job.last_run_error} class:badge-default={!job.enabled && !job.last_run_error}>{cronStatusLabel(job)}</span>
                </span>
                <span class="work-meta">{job.schedule} · {nextCronRunLabel(job)}</span>
              </button>
            {/each}
          </div>
        {/if}
      </section>

      <section class="dashboard-section sessions-section">
        <div class="section-heading">
          <div>
            <h3>Active sessions</h3>
            <p>Main sessions, most recently updated first.</p>
          </div>
          <button type="button" class="btn btn-ghost btn-sm" onclick={() => onNavigate('/console/chat')}>Open Chat</button>
        </div>

        {#if recentMainSessions.length === 0}
          <div class="empty-state"><p>No main sessions yet.</p></div>
        {:else}
          <div class="session-grid">
            {#each recentMainSessions as session}
              <button type="button" class="session-card" onclick={() => onNavigate(`/console/chat/${encodeURIComponent(session.id)}`)}>
                <span class="session-title">{session.title || 'Untitled session'}</span>
                <span class="session-meta">{relativeTime(session.updated_at)}</span>
                <span class="session-id">{session.id}</span>
              </button>
            {/each}
          </div>
        {/if}
      </section>

      <section class="dashboard-section continue-section">
        <div class="section-heading">
          <div>
            <h3>Continue working on...</h3>
            <p>Most recent session with a plan or task list.</p>
          </div>
        </div>
        {#if continueSession}
          <button type="button" class="focus-card" onclick={openContinueSession}>
            <span class="focus-kicker">{continueSession.session.title || 'Untitled session'}</span>
            <strong>{compact(planSummary(continueSession.tasks), 180)}</strong>
            <span>{continueSession.tasks.tasks.length} tasks tracked</span>
          </button>
        {:else}
          <div class="empty-state"><p>No active plan found.</p></div>
        {/if}
      </section>

      <section class="dashboard-section">
        <div class="section-heading">
          <div>
            <h3>Recent notifications</h3>
            <p>{unreadCount} unread in the event stream.</p>
          </div>
        </div>
        {#if notifications.length === 0}
          <div class="empty-state"><p>No recent notifications.</p></div>
        {:else}
          <div class="notification-list">
            {#each notifications.slice(0, 10) as item}
              <button
                type="button"
                class="notification-row"
                disabled={!item.open_path}
                onclick={() => item.open_path && onNavigate(item.open_path)}
              >
                <span class="notification-top">
                  <strong>{item.title}</strong>
                  <span class="badge badge-default">{item.severity || item.category}</span>
                </span>
                <span class="notification-message">{compact(item.message, 140)}</span>
                <span class="session-meta">{fmt(item.timestamp)}</span>
              </button>
            {/each}
          </div>
        {/if}
      </section>

      <section class="dashboard-section">
        <div class="section-heading">
          <div>
            <h3>Recommended actions</h3>
            <p>Small checks that reduce setup friction.</p>
          </div>
        </div>
        {#if recommendedActions.length === 0}
          <div class="empty-state"><p>No recommendations right now.</p></div>
        {:else}
          <div class="action-grid">
            {#each recommendedActions as action}
              <button type="button" class="action-card" onclick={() => onNavigate(action.path)}>
                <strong>{action.title}</strong>
                <span>{action.detail}</span>
                <em>{action.action}</em>
              </button>
            {/each}
          </div>
        {/if}
      </section>

      <section class="dashboard-section">
        <div class="section-heading">
          <div>
            <h3>Delivery</h3>
            <p>Current installed version and release shortcuts.</p>
          </div>
        </div>
        <div class="action-grid">
          {#if releaseHref}
            <a class="action-card" href={releaseHref} target="_blank" rel="noreferrer">
              <strong>{serverVersion}</strong>
              <span>Latest known release for this running server.</span>
              <em>Open Release</em>
            </a>
          {:else}
            <div class="action-card action-card-static">
              <strong>{serverVersion || 'dev'}</strong>
              <span>Development build or unreleased local server.</span>
              <em>Local build</em>
            </div>
          {/if}
          <a class="action-card" href="https://github.com/devlikebear/tars/pulls" target="_blank" rel="noreferrer">
            <strong>Pull requests</strong>
            <span>Review recent delivery activity on GitHub.</span>
            <em>Open PRs</em>
          </a>
        </div>
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

  .home-header,
  .section-heading,
  .notification-top {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-3);
  }

  .home-header {
    margin-bottom: var(--space-5);
  }

  .home-header h2,
  .section-heading h3 {
    margin: 0;
    font-family: var(--font-display);
    color: var(--text-primary);
  }

  .home-header h2 {
    font-size: var(--text-2xl);
  }

  .home-subtitle,
  .section-heading p {
    margin: var(--space-1) 0 0;
    color: var(--text-tertiary);
  }

  .home-loading {
    padding: var(--space-10);
    text-align: center;
    color: var(--text-tertiary);
  }

  .status-strip {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
    gap: var(--space-3);
    margin-bottom: var(--space-6);
  }

  .status-tile,
  .session-card,
  .focus-card,
  .notification-row,
  .work-row,
  .action-card {
    background: var(--surface);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
  }

  .status-tile {
    display: flex;
    flex-direction: column;
    gap: 3px;
    padding: var(--space-4);
    min-width: 0;
    color: inherit;
    text-align: left;
    cursor: pointer;
    transition:
      border-color var(--duration-fast) var(--ease-out),
      background var(--duration-fast) var(--ease-out);
  }

  .status-tile:hover {
    background: var(--surface-elevated);
    border-color: var(--border-strong);
  }

  .status-label,
  .session-meta,
  .session-id,
  .notification-message,
  .work-meta,
  .action-card span,
  .focus-card span {
    color: var(--text-tertiary);
    font-size: var(--text-xs);
  }

  .status-tile strong {
    font-family: var(--font-display);
    font-size: var(--text-xl);
    color: var(--text-primary);
    line-height: 1.1;
  }

  .status-ok { color: var(--success) !important; }
  .status-warn { color: var(--warning) !important; }
  .status-danger { color: var(--error) !important; }

  .dashboard-grid {
    display: grid;
    grid-template-columns: minmax(0, 1.2fr) minmax(320px, 0.8fr);
    gap: var(--space-6);
  }

  .dashboard-section {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    min-width: 0;
  }

  .sessions-section {
    grid-row: span 2;
  }

  .plans-section {
    grid-column: 1 / -1;
  }

  .session-grid,
  .action-grid,
  .notification-list,
  .work-list {
    display: grid;
    gap: var(--space-3);
  }

  .session-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .session-card,
  .focus-card,
  .notification-row,
  .work-row,
  .action-card {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: var(--space-2);
    padding: var(--space-4);
    color: inherit;
    text-align: left;
    cursor: pointer;
    transition:
      border-color var(--duration-fast) var(--ease-out),
      background var(--duration-fast) var(--ease-out);
  }

  .session-card:hover,
  .focus-card:hover,
  .notification-row:hover:not(:disabled),
  .work-row:hover,
  .action-card:hover {
    background: var(--surface-elevated);
    border-color: var(--border-strong);
  }

  .notification-row:disabled {
    cursor: default;
  }

  .session-title,
  .focus-card strong,
  .notification-top strong,
  .work-topline strong,
  .action-card strong {
    color: var(--text-primary);
    font-family: var(--font-display);
    font-size: var(--text-sm);
    font-weight: 600;
  }

  .session-title,
  .notification-message {
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .session-id {
    max-width: 100%;
    font-family: var(--font-mono);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .focus-kicker,
  .action-card em {
    color: var(--primary);
    font-style: normal;
    font-size: var(--text-xs);
    font-weight: 600;
  }

  .work-topline {
    display: flex;
    width: 100%;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-2);
    min-width: 0;
  }

  .work-topline strong {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .work-meta {
    line-height: 1.45;
  }

  .mini-progress {
    display: block;
    width: 100%;
    height: 4px;
    overflow: hidden;
    border-radius: 999px;
    background: var(--surface-inset);
  }

  .mini-progress span {
    display: block;
    height: 100%;
    background: var(--primary);
  }

  .action-card {
    text-decoration: none;
  }

  .action-card-static {
    cursor: default;
  }

  .action-card-static:hover {
    background: var(--surface);
    border-color: var(--border-subtle);
  }

  .empty-state {
    padding: var(--space-5);
    border: 1px dashed var(--border-subtle);
    border-radius: var(--radius-md);
    color: var(--text-tertiary);
  }

  @media (max-width: 1100px) {
    .status-strip,
    .dashboard-grid,
    .session-grid {
      grid-template-columns: 1fr;
    }

    .sessions-section {
      grid-row: auto;
    }
  }

  @media (max-width: 640px) {
    .home-header,
    .section-heading,
    .notification-top {
      flex-direction: column;
      align-items: stretch;
    }
  }
</style>
