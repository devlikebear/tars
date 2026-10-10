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
    SyspromptFile,
  } from '../lib/types'
  import { t } from '../i18n'
  import { isStalledPlan } from '../lib/plans'

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
  // A plan nothing has touched for a day is not active work; the tile counts
  // it apart, as the Plans page does.
  let livePlans = $derived(plans.filter((item) => !isStalledPlan(item)))
  let stalledPlanCount = $derived(plans.length - livePlans.length)
  let cronJobs: CronJob[] = $state([])
  let agentRuns: AgentRuntimeRun[] = $state([])
  let serverVersion = $state('')
  let userFile: SyspromptFile | null = $state(null)
  let config: ConfigFile | null = $state(null)
  let unreadCount = $state(0)
  let loading = $state(true)
  let error = $state('')
  let stopStream: (() => void) | null = null
  let pollTimer: ReturnType<typeof setInterval> | null = null

  let mainSessions = $derived(
    sessions.filter((session) => !session.hidden && (session.kind ?? 'main') === 'main'),
  )
  let todaySessions = $derived(mainSessions.filter((session) => isToday(session.updated_at)))
  let activeCronJobs = $derived(cronJobs.filter((job) => job.enabled && !isCronCompleted(job)))
  let failedCronJobs = $derived(cronJobs.filter((job) => !!job.last_run_error))
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
        title: $t.home.recommendations.userMdTitle,
        detail: $t.home.recommendations.userMdDetail,
        path: '/console/sysprompt',
        action: $t.home.recommendations.userMdAction,
      })
    }
    if (needsAnthropicSetup(config)) {
      actions.push({
        id: 'anthropic-key',
        title: $t.home.recommendations.anthropicKeyTitle,
        detail: $t.home.recommendations.anthropicKeyDetail,
        path: '/console/config',
        action: $t.home.recommendations.anthropicKeyAction,
      })
    }
    if (todaySessions.length === 0) {
      actions.push({
        id: 'new-chat',
        title: $t.home.recommendations.newChatTitle,
        detail: $t.home.recommendations.newChatDetail,
        path: '/console/chat',
        action: $t.home.recommendations.newChatAction,
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

  function notificationTimestamp(item: NotificationMessage): string {
    return item.last_seen?.trim() || item.timestamp
  }

  function compact(value?: string, max = 110): string {
    const text = value?.trim()
    if (!text) return '-'
    return text.length <= max ? text : `${text.slice(0, max - 1)}...`
  }

  function relativeTime(value?: string): string {
    if (!value?.trim()) return $t.home.relativeTime.never
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return value
    if (date.getFullYear() <= 1) return $t.home.relativeTime.never
    const seconds = Math.floor((Date.now() - date.getTime()) / 1000)
    if (seconds < 60) return $t.home.relativeTime.secondsAgo(seconds)
    if (seconds < 3600) return $t.home.relativeTime.minutesAgo(Math.floor(seconds / 60))
    if (seconds < 86400) return $t.home.relativeTime.hoursAgo(Math.floor(seconds / 3600))
    return $t.home.relativeTime.daysAgo(Math.floor(seconds / 86400))
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
    if (!ops) return $t.home.disk.unknown
    return `${Math.round(ops.disk_used_percent)}% ${$t.home.disk.usedSuffix}`
  }

  function diskClass(): string {
    const used = ops?.disk_used_percent ?? 0
    if (used >= 90) return 'danger'
    if (used >= 80) return 'warn'
    return 'ok'
  }

  function pulseLabel(): string {
    if (pulse?.last_err) return $t.home.pulseStates.error
    if ((pulse?.total_ticks ?? 0) > 0) return $t.home.pulseStates.active
    return $t.home.pulseStates.idle
  }

  function reflectionLabel(): string {
    if ((reflection?.consecutive_failures ?? 0) > 0) return $t.home.reflectionStates.failing
    if (hasRealTimestamp(reflection?.last_successful_run_at)) return $t.home.reflectionStates.healthy
    return $t.home.reflectionStates.idle
  }

  function isCronCompleted(job: CronJob): boolean {
    return isOneShotCron(job) && Boolean(job.last_run_at)
  }

  function isOneShotCron(job: CronJob): boolean {
    const schedule = job.schedule.trim().toLowerCase()
    return job.delete_after_run === true || schedule.startsWith('at:')
  }

  function isAgentRunActive(run: AgentRuntimeRun): boolean {
    const status = run.status.trim().toLowerCase()
    return status === 'running' || status === 'queued' || status === 'pending' || status === 'in_progress'
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
    } catch (err) {
      error = err instanceof Error ? err.message : $t.home.errorLoad
    } finally {
      if (showLoading) loading = false
    }
  }

  function startEventStream() {
    stopStream?.()
    stopStream = streamEvents((event) => {
      // A category "companion" event (#1192) is CASE's own face/bubble,
      // never a general notification — the server never stores it either
      // (notify.go's Emit), so it must not join this "recent
      // notifications" list or its unread count.
      if ((event.category || '').trim().toLowerCase() === 'companion') return
      notifications = [event, ...notifications.filter((item) => item.id !== event.id)].slice(0, 10)
      if (!event.coalesced) unreadCount++
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
      <h2>{$t.home.title}</h2>
      <p class="home-subtitle">{$t.home.subtitle}</p>
    </div>
    <button type="button" class="btn btn-primary" onclick={() => onNavigate('/console/chat')}>{$t.home.newChat}</button>
  </div>

  {#if error}
    <div class="error-banner">{error}</div>
  {/if}

  {#if loading}
    <div class="home-loading">{$t.home.loading}</div>
  {:else}
    <section class="status-strip" aria-label="Home status strip">
      <button type="button" class="status-tile" onclick={() => onNavigate('/console/pulse')}>
        <span class="status-label">{$t.home.statusStrip.pulse}</span>
        <strong class:status-ok={pulseLabel() === $t.home.pulseStates.active} class:status-danger={pulseLabel() === $t.home.pulseStates.error}>{pulseLabel()}</strong>
        <span>{pulse?.last_tick_at ? relativeTime(pulse.last_tick_at) : $t.home.statusStrip.never}</span>
      </button>
      <button type="button" class="status-tile" onclick={() => onNavigate('/console/reflection')}>
        <span class="status-label">{$t.home.statusStrip.reflection}</span>
        <strong class:status-ok={reflectionLabel() === $t.home.reflectionStates.healthy} class:status-danger={reflectionLabel() === $t.home.reflectionStates.failing}>{reflectionLabel()}</strong>
        <span>{reflection?.last_successful_run_at ? relativeTime(reflection.last_successful_run_at) : $t.home.statusStrip.never}</span>
      </button>
      <button type="button" class="status-tile" onclick={() => onNavigate('/console/tasks')}>
        <span class="status-label">{$t.home.statusStrip.activePlans}</span>
        <strong>{livePlans.length}</strong>
        <span>{$t.home.statusStrip.taskActive(livePlans.reduce((total, item) => total + (item.summary?.in_progress ?? 0), 0))}{stalledPlanCount > 0 ? ` · ${$t.home.statusStrip.plansStalled(stalledPlanCount)}` : ''}</span>
      </button>
      <button type="button" class="status-tile" onclick={() => onNavigate('/console/agentruntime')}>
        <span class="status-label">{$t.home.statusStrip.agentRuns}</span>
        <strong>{activeAgentRuns.length}</strong>
        <span>{agentRuns.length} {$t.home.statusStrip.recent}</span>
      </button>
      <button type="button" class="status-tile" onclick={() => onNavigate('/console/cron')}>
        <span class="status-label">{$t.home.statusStrip.cronJobs}</span>
        <strong class:status-danger={failedCronJobs.length > 0}>{activeCronJobs.length}</strong>
        <span>{failedCronJobs.length > 0 ? $t.home.statusStrip.failed(failedCronJobs.length) : $t.home.statusStrip.total(cronJobs.length)}</span>
      </button>
      <button type="button" class="status-tile" onclick={() => onNavigate('/console/approvals')}>
        <span class="status-label">{$t.home.statusStrip.diskPressure}</span>
        <strong class:status-ok={diskClass() === 'ok'} class:status-warn={diskClass() === 'warn'} class:status-danger={diskClass() === 'danger'}>{diskUsedLabel()}</strong>
        <span>{ops ? $t.home.statusStrip.gbFree(Math.round(ops.disk_free_bytes / 1024 / 1024 / 1024)) : $t.home.statusStrip.opsUnavailable}</span>
      </button>
      <button type="button" class="status-tile" onclick={() => onNavigate('/console/chat')}>
        <span class="status-label">{$t.home.statusStrip.activeSessions}</span>
        <strong>{mainSessions.length}</strong>
        <span>{$t.home.statusStrip.touchedToday(todaySessions.length)}</span>
      </button>
    </section>

    <div class="dashboard-grid">
      <section class="dashboard-section">
        <div class="section-heading">
          <div>
            <h3>{$t.home.notifications.title}</h3>
            <p>{$t.home.notifications.unreadSuffix(unreadCount)}</p>
          </div>
        </div>
        {#if notifications.length === 0}
          <div class="empty-state"><p>{$t.home.notifications.empty}</p></div>
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
                  {#if (item.occurrences ?? 0) > 1}
                    <span class="badge badge-default">x{item.occurrences}</span>
                  {/if}
                </span>
                <span class="notification-message">{compact(item.message, 140)}</span>
                <span class="session-meta">{fmt(notificationTimestamp(item))}</span>
              </button>
            {/each}
          </div>
        {/if}
      </section>

      <section class="dashboard-section">
        <div class="section-heading">
          <div>
            <h3>{$t.home.recommendations.title}</h3>
            <p>{$t.home.recommendations.subtitle}</p>
          </div>
        </div>
        {#if recommendedActions.length === 0}
          <div class="empty-state"><p>{$t.home.recommendations.empty}</p></div>
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
            <h3>{$t.home.delivery.title}</h3>
            <p>{$t.home.delivery.subtitle}</p>
          </div>
        </div>
        <div class="action-grid">
          {#if releaseHref}
            <a class="action-card" href={releaseHref} target="_blank" rel="noreferrer">
              <strong>{serverVersion}</strong>
              <span>{$t.home.delivery.release}</span>
              <em>Open Release</em>
            </a>
          {:else}
            <div class="action-card action-card-static">
              <strong>{serverVersion || 'dev'}</strong>
              <span>{$t.home.delivery.devBuild}</span>
              <em>{$t.home.delivery.localBuild}</em>
            </div>
          {/if}
          <a class="action-card" href="https://github.com/devlikebear/tars/pulls" target="_blank" rel="noreferrer">
            <strong>{$t.home.delivery.pullRequests}</strong>
            <span>{$t.home.delivery.prsDetail}</span>
            <em>{$t.home.delivery.openPRs}</em>
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
  .notification-row,
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
  .notification-message,
  .action-card span {
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

  .action-grid,
  .notification-list {
    display: grid;
    gap: var(--space-3);
  }

  .notification-row,
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

  .notification-row:hover:not(:disabled),
  .action-card:hover {
    background: var(--surface-elevated);
    border-color: var(--border-strong);
  }

  .notification-row:disabled {
    cursor: default;
  }

  .notification-top strong,
  .action-card strong {
    color: var(--text-primary);
    font-family: var(--font-display);
    font-size: var(--text-sm);
    font-weight: 600;
  }

  .notification-message {
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .action-card em {
    color: var(--primary);
    font-style: normal;
    font-size: var(--text-xs);
    font-weight: 600;
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
    .dashboard-grid {
      grid-template-columns: 1fr;
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
