<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import {
    getEventsHistory,
    getProject,
    getProjectAutopilot,
    listApprovals,
    listCronJobs,
    listCronRuns,
    listProjectActivity,
    reviewApproval,
    streamChat,
    streamEvents,
  } from '../lib/api'
  import type {
    Approval,
    ChatEvent,
    CronJob,
    CronRunRecord,
    NotificationMessage,
    Project,
    ProjectActivity,
    ProjectAutopilotRun,
  } from '../lib/types'

  interface Props {
    projectId: string
  }

  type ChatMessage = {
    id: string
    role: 'user' | 'assistant' | 'system' | 'error'
    text: string
  }

  let { projectId }: Props = $props()

  let project: Project | null = $state(null)
  let autopilot: ProjectAutopilotRun | null = $state(null)
  let activity: ProjectActivity[] = $state([])
  let cronJobs: CronJob[] = $state([])
  let cronRunsByJob: Record<string, CronRunRecord[]> = $state({})
  let notifications: NotificationMessage[] = $state([])
  let approvals: Approval[] = $state([])

  let loadingDetail = $state(true)
  let loadingPanels = $state(false)
  let detailError = $state('')
  let panelError = $state('')

  let chatInput = $state('')
  let chatBusy = $state(false)
  let chatError = $state('')
  let chatSessionId = $state('')
  let chatStatusLine = $state('')
  let approvalBusyId = $state('')

  let chatMessages: ChatMessage[] = $state([])
  let stopEventStream: (() => void) | null = null

  function fmt(value?: string): string {
    const text = value?.trim()
    if (!text) return '\u2014'
    const date = new Date(text)
    if (Number.isNaN(date.getTime())) return text
    return new Intl.DateTimeFormat('en', { dateStyle: 'medium', timeStyle: 'short' }).format(date)
  }

  function compact(value?: string, max = 180): string {
    const text = value?.trim()
    if (!text) return '\u2014'
    return text.length <= max ? text : `${text.slice(0, max - 1)}\u2026`
  }

  function filterProjectNotifications(items: NotificationMessage[], pid: string): NotificationMessage[] {
    return items
      .filter((item) => item.project_id?.trim() === pid)
      .sort((a, b) => new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime())
      .slice(0, 20)
  }

  async function loadDetail() {
    loadingDetail = true
    detailError = ''
    try {
      const [p, a] = await Promise.all([
        getProject(projectId),
        getProjectAutopilot(projectId),
      ])
      project = p
      autopilot = a
    } catch (err) {
      project = null
      autopilot = null
      detailError = err instanceof Error ? err.message : 'Failed to load project'
    } finally {
      loadingDetail = false
    }
  }

  async function loadPanels() {
    loadingPanels = true
    panelError = ''
    const results = await Promise.allSettled([
      listProjectActivity(projectId, 20).then((items) => { activity = items }),
      listCronJobs().then(async (allJobs) => {
        const jobs = allJobs.filter((j) => j.project_id?.trim() === projectId)
        const runsEntries = await Promise.all(
          jobs.map(async (j) => [j.id, await listCronRuns(j.id, 5)] as const),
        )
        cronJobs = jobs
        cronRunsByJob = Object.fromEntries(runsEntries)
      }),
      getEventsHistory(40).then((history) => {
        notifications = filterProjectNotifications(history.items ?? [], projectId)
      }),
      listApprovals().then((list) => { approvals = list }),
    ])
    const failures = results
      .filter((r): r is PromiseRejectedResult => r.status === 'rejected')
      .map((r) => (r.reason instanceof Error ? r.reason.message : 'Panel error'))
    panelError = failures.join(' \u00b7 ')
    loadingPanels = false
  }

  function startEventStream() {
    stopEventStream?.()
    stopEventStream = streamEvents(
      projectId,
      (event) => {
        notifications = filterProjectNotifications(
          [event, ...notifications.filter((n) => n.id !== event.id)],
          projectId,
        )
        if (event.category === 'cron' || event.category === 'watchdog') {
          void loadPanels()
        }
        if (event.category === 'ops') {
          void listApprovals().then((list) => { approvals = list })
        }
      },
    )
  }

  function handleChatEvent(event: ChatEvent, assistantId: string) {
    switch (event.type) {
      case 'status':
        chatStatusLine = [event.phase, event.message, event.tool_name, event.skill_name]
          .filter(Boolean).join(' \u00b7 ')
        break
      case 'delta': {
        const chunk = event.text ?? ''
        if (!chunk) break
        const idx = chatMessages.findIndex((m) => m.id === assistantId)
        if (idx >= 0) {
          chatMessages[idx] = { ...chatMessages[idx], text: chatMessages[idx].text + chunk }
          chatMessages = [...chatMessages]
        }
        break
      }
      case 'done':
        chatSessionId = event.session_id?.trim() || chatSessionId
        chatStatusLine = 'done'
        break
      case 'error':
        chatError = event.error?.trim() || 'Stream failed'
        break
    }
  }

  async function submitChat() {
    const message = chatInput.trim()
    if (!message || chatBusy) return
    chatBusy = true
    chatError = ''
    chatStatusLine = 'connecting'
    chatInput = ''
    const userId = `user-${Date.now()}`
    const assistantId = `assistant-${Date.now()}`
    chatMessages = [
      ...chatMessages,
      { id: userId, role: 'user', text: message },
      { id: assistantId, role: 'assistant', text: '' },
    ]
    try {
      await streamChat(
        { message, session_id: chatSessionId || undefined, project_id: projectId },
        (event) => handleChatEvent(event, assistantId),
      )
    } catch (err) {
      chatError = err instanceof Error ? err.message : 'Failed to send'
      chatMessages = [...chatMessages, { id: `error-${Date.now()}`, role: 'error', text: chatError }]
    } finally {
      chatBusy = false
    }
  }

  async function handleApprovalAction(approvalId: string, action: 'approve' | 'reject') {
    if (!approvalId.trim() || approvalBusyId) return
    approvalBusyId = approvalId
    panelError = ''
    try {
      await reviewApproval(approvalId, action)
      approvals = await listApprovals()
    } catch (err) {
      panelError = err instanceof Error ? err.message : `Failed to ${action}`
    } finally {
      approvalBusyId = ''
    }
  }

  function phaseColor(phase?: string): string {
    switch (phase?.toLowerCase()) {
      case 'executing': return 'badge-success'
      case 'reviewing': return 'badge-info'
      case 'blocked': return 'badge-error'
      case 'done': return 'badge-default'
      default: return 'badge-accent'
    }
  }

  function statusColor(status?: string): string {
    switch (status?.toLowerCase()) {
      case 'running': return 'badge-success'
      case 'blocked': return 'badge-error'
      case 'done': case 'completed': return 'badge-default'
      case 'failed': return 'badge-error'
      default: return 'badge-accent'
    }
  }

  onMount(() => {
    chatMessages = [{ id: 'system-init', role: 'system', text: `Chat scoped to project ${projectId}` }]
    void loadDetail()
    void loadPanels()
    startEventStream()
  })

  onDestroy(() => {
    stopEventStream?.()
  })
</script>

<div class="pv">
  {#if loadingDetail}
    <div class="pv-loading">Loading project...</div>
  {:else if detailError}
    <div class="error-banner">{detailError}</div>
  {:else if project}
    <!-- Project header -->
    <div class="pv-header">
      <div>
        <h2>{project.name}</h2>
        <p class="pv-objective">{project.objective || 'No objective recorded.'}</p>
      </div>
      <div class="pv-header-meta">
        <span class="badge badge-default">{project.status || 'active'}</span>
        {#if loadingPanels}
          <span class="badge badge-default">refreshing</span>
        {/if}
      </div>
    </div>

    {#if panelError}
      <div class="error-banner" style="margin-bottom: var(--space-4)">{panelError}</div>
    {/if}

    <!-- Phase / Run -->
    <div class="pv-phase-strip">
      {#if autopilot}
        <div class="pv-phase-item">
          <span class="label">Phase</span>
          <span class="badge {phaseColor(autopilot.phase)}">{autopilot.phase || '\u2014'}</span>
        </div>
        <div class="pv-phase-item">
          <span class="label">Status</span>
          <span class="badge {phaseColor(autopilot.phase_status)}">{autopilot.phase_status || '\u2014'}</span>
        </div>
        <div class="pv-phase-item">
          <span class="label">Run</span>
          <span class="badge {statusColor(autopilot.status)}">{autopilot.status || '\u2014'}</span>
        </div>
        <div class="pv-phase-item">
          <span class="label">Iterations</span>
          <strong>{autopilot.iterations}</strong>
        </div>
      {:else}
        <div class="pv-phase-empty">
          <span class="label">No autopilot run active</span>
        </div>
      {/if}
    </div>

    {#if autopilot?.next_action}
      <div class="pv-next-action">
        <span class="label">Next action</span>
        <p>{autopilot.next_action}</p>
      </div>
    {/if}

    <div class="pv-grid">
      <!-- Identity -->
      <section class="card">
        <span class="card-title">Identity</span>
        <dl class="pv-facts">
          <div><dt>ID</dt><dd class="mono">{project.id}</dd></div>
          <div><dt>Type</dt><dd>{project.type || '\u2014'}</dd></div>
          <div><dt>Path</dt><dd class="mono">{project.path || '\u2014'}</dd></div>
          <div><dt>Repo</dt><dd class="mono">{project.git_repo || '\u2014'}</dd></div>
        </dl>
      </section>

      <!-- Run detail -->
      {#if autopilot}
        <section class="card">
          <span class="card-title">Run detail</span>
          <dl class="pv-facts">
            <div><dt>Run ID</dt><dd class="mono">{autopilot.run_id || '\u2014'}</dd></div>
            <div><dt>Summary</dt><dd>{autopilot.summary || '\u2014'}</dd></div>
            <div><dt>Message</dt><dd>{autopilot.message || '\u2014'}</dd></div>
            <div><dt>Updated</dt><dd>{fmt(autopilot.updated_at)}</dd></div>
          </dl>
        </section>
      {/if}

      <!-- Chat -->
      <section class="card pv-wide">
        <span class="card-title">Chat</span>
        <div class="pv-chat-status">
          {chatBusy ? 'streaming' : chatStatusLine || 'idle'}
        </div>
        <div class="pv-chat-log">
          {#each chatMessages as msg}
            <div class="pv-chat-msg pv-chat-{msg.role}">
              <span class="pv-chat-role">{msg.role}</span>
              <div class="pv-chat-text">{msg.text || '\u2026'}</div>
            </div>
          {/each}
        </div>
        {#if chatError}
          <div class="error-banner" style="margin-bottom: var(--space-3)">{chatError}</div>
        {/if}
        <form class="pv-chat-form" onsubmit={(e) => { e.preventDefault(); void submitChat() }}>
          <textarea
            bind:value={chatInput}
            rows="3"
            placeholder="Ask TARS about this project..."
          ></textarea>
          <button type="submit" class="btn btn-primary" disabled={chatBusy || !chatInput.trim()}>
            {chatBusy ? 'Streaming...' : 'Send'}
          </button>
        </form>
      </section>

      <!-- Activity -->
      <section class="card pv-wide">
        <div class="card-header">
          <span class="card-title">Activity</span>
          <span class="badge badge-default">{activity.length}</span>
        </div>
        {#if activity.length === 0}
          <div class="empty-state"><p>No activity recorded yet.</p></div>
        {:else}
          <div class="pv-timeline">
            {#each activity as item}
              <div class="pv-timeline-item">
                <div class="pv-timeline-top">
                  <strong>{item.kind}</strong>
                  <span class="badge badge-default">{item.status || item.source}</span>
                </div>
                <p>{item.message || 'No message'}</p>
                <div class="pv-timeline-meta">
                  <span>{fmt(item.timestamp)}</span>
                  {#if item.agent}<span>{item.agent}</span>{/if}
                </div>
              </div>
            {/each}
          </div>
        {/if}
      </section>

      <!-- Cron -->
      <section class="card pv-wide">
        <div class="card-header">
          <span class="card-title">Cron jobs</span>
          <span class="badge badge-default">{cronJobs.length}</span>
        </div>
        {#if cronJobs.length === 0}
          <div class="empty-state"><p>No cron jobs attached.</p></div>
        {:else}
          <div class="pv-timeline">
            {#each cronJobs as job}
              <div class="pv-timeline-item">
                <div class="pv-timeline-top">
                  <strong>{job.name}</strong>
                  <span class="badge" class:badge-success={job.enabled && !job.last_run_error}
                    class:badge-error={!!job.last_run_error}
                    class:badge-default={!job.enabled}>
                    {job.enabled ? job.schedule : 'disabled'}
                  </span>
                </div>
                <p>{compact(job.prompt, 160)}</p>
                {#if (cronRunsByJob[job.id] ?? []).length > 0}
                  <div class="pv-cron-runs">
                    {#each cronRunsByJob[job.id] ?? [] as run}
                      <div class="pv-cron-run">
                        <strong>{fmt(run.ran_at)}</strong>
                        <span>{compact(run.error || run.response || 'No output', 140)}</span>
                      </div>
                    {/each}
                  </div>
                {/if}
              </div>
            {/each}
          </div>
        {/if}
      </section>

      <!-- Notifications -->
      <section class="card pv-wide">
        <div class="card-header">
          <span class="card-title">Notifications</span>
          <span class="badge badge-default">{notifications.length}</span>
        </div>
        {#if notifications.length === 0}
          <div class="empty-state"><p>No project notifications.</p></div>
        {:else}
          <div class="pv-timeline">
            {#each notifications as item}
              <div class="pv-timeline-item">
                <div class="pv-timeline-top">
                  <strong>{item.title}</strong>
                  <span class="badge badge-default">{item.severity}</span>
                </div>
                <p>{item.message}</p>
                <div class="pv-timeline-meta">
                  <span>{fmt(item.timestamp)}</span>
                  <span>{item.category}</span>
                </div>
              </div>
            {/each}
          </div>
        {/if}
      </section>

      <!-- Approvals -->
      <section class="card pv-wide">
        <div class="card-header">
          <span class="card-title">Approvals</span>
          <span class="badge badge-default">{approvals.length}</span>
        </div>
        {#if approvals.length === 0}
          <div class="empty-state"><p>No approvals pending.</p></div>
        {:else}
          <div class="pv-timeline">
            {#each approvals as approval}
              <div class="pv-timeline-item">
                <div class="pv-timeline-top">
                  <strong>{approval.type}</strong>
                  <span class="badge badge-default">{approval.status}</span>
                </div>
                <p>{approval.plan.candidates.length} candidates, {approval.plan.total_bytes} bytes</p>
                {#if approval.status === 'pending'}
                  <div class="pv-approval-actions">
                    <button type="button" class="btn btn-secondary btn-sm"
                      disabled={approvalBusyId === approval.id}
                      onclick={() => { void handleApprovalAction(approval.id, 'approve') }}>
                      Approve
                    </button>
                    <button type="button" class="btn btn-danger btn-sm"
                      disabled={approvalBusyId === approval.id}
                      onclick={() => { void handleApprovalAction(approval.id, 'reject') }}>
                      Reject
                    </button>
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
  .pv {
    animation: fadeIn var(--duration-normal) var(--ease-out);
  }

  @keyframes fadeIn {
    from { opacity: 0; transform: translateY(8px); }
    to { opacity: 1; transform: translateY(0); }
  }

  .pv-loading {
    padding: var(--space-10);
    text-align: center;
    color: var(--text-tertiary);
  }

  .pv-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-4);
    margin-bottom: var(--space-5);
  }

  .pv-header h2 {
    font-size: var(--text-2xl);
    margin-bottom: var(--space-1);
  }

  .pv-objective {
    color: var(--text-secondary);
    max-width: 600px;
  }

  .pv-header-meta {
    display: flex;
    gap: var(--space-2);
    flex-shrink: 0;
  }

  /* ── Phase strip ──────────────────────────────── */
  .pv-phase-strip {
    display: flex;
    gap: var(--space-5);
    padding: var(--space-4) var(--space-5);
    background: var(--bg-surface);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg);
    margin-bottom: var(--space-4);
  }

  .pv-phase-item {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }

  .pv-phase-item strong {
    font-family: var(--font-display);
    font-size: var(--text-lg);
    color: var(--text-primary);
  }

  .pv-phase-empty {
    padding: var(--space-2) 0;
  }

  .pv-next-action {
    padding: var(--space-3) var(--space-4);
    background: var(--accent-muted);
    border: 1px solid rgba(224, 145, 69, 0.2);
    border-radius: var(--radius-md);
    margin-bottom: var(--space-4);
  }

  .pv-next-action p {
    margin-top: var(--space-1);
    color: var(--text-primary);
  }

  /* ── Grid ─────────────────────────────────────── */
  .pv-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: var(--space-4);
  }

  .pv-wide {
    grid-column: 1 / -1;
  }

  /* ── Facts ────────────────────────────────────── */
  .pv-facts {
    display: grid;
    gap: var(--space-3);
    margin-top: var(--space-3);
  }

  .pv-facts div {
    display: grid;
    grid-template-columns: 100px minmax(0, 1fr);
    gap: var(--space-3);
  }

  .pv-facts dt {
    color: var(--text-tertiary);
    font-size: var(--text-sm);
  }

  .pv-facts dd {
    margin: 0;
    word-break: break-word;
    font-size: var(--text-sm);
  }

  /* ── Chat ─────────────────────────────────────── */
  .pv-chat-status {
    font-size: var(--text-xs);
    color: var(--text-tertiary);
    margin-bottom: var(--space-3);
  }

  .pv-chat-log {
    display: grid;
    gap: var(--space-2);
    max-height: 400px;
    overflow-y: auto;
    margin-bottom: var(--space-3);
  }

  .pv-chat-msg {
    padding: var(--space-3);
    border-radius: var(--radius-md);
    background: var(--bg-base);
  }

  .pv-chat-user {
    background: rgba(224, 145, 69, 0.08);
    border: 1px solid rgba(224, 145, 69, 0.12);
  }

  .pv-chat-assistant {
    background: var(--bg-elevated);
  }

  .pv-chat-error {
    background: var(--error-muted);
    border: 1px solid rgba(248, 113, 113, 0.15);
  }

  .pv-chat-role {
    font-family: var(--font-display);
    font-size: var(--text-xs);
    font-weight: 500;
    color: var(--text-tertiary);
    margin-bottom: var(--space-1);
    display: block;
  }

  .pv-chat-text {
    white-space: pre-wrap;
    font-size: var(--text-sm);
    line-height: 1.55;
  }

  .pv-chat-form {
    display: grid;
    gap: var(--space-3);
  }

  /* ── Timeline ─────────────────────────────────── */
  .pv-timeline {
    display: grid;
    gap: var(--space-2);
  }

  .pv-timeline-item {
    padding: var(--space-3) var(--space-4);
    border-radius: var(--radius-md);
    background: var(--bg-base);
  }

  .pv-timeline-top {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
    margin-bottom: var(--space-1);
  }

  .pv-timeline-top strong {
    font-family: var(--font-display);
    font-size: var(--text-sm);
    font-weight: 500;
  }

  .pv-timeline-item p {
    font-size: var(--text-sm);
    color: var(--text-secondary);
    margin-bottom: var(--space-2);
    white-space: pre-wrap;
  }

  .pv-timeline-meta {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2) var(--space-3);
    font-size: var(--text-xs);
    color: var(--text-ghost);
  }

  .pv-cron-runs {
    display: grid;
    gap: var(--space-1);
    margin-top: var(--space-2);
  }

  .pv-cron-run {
    display: grid;
    gap: 2px;
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-sm);
    background: var(--bg-surface);
    font-size: var(--text-sm);
  }

  .pv-cron-run span {
    color: var(--text-secondary);
  }

  .pv-approval-actions {
    display: flex;
    gap: var(--space-2);
    margin-top: var(--space-2);
  }

  /* ── Responsive ───────────────────────────────── */
  @media (max-width: 900px) {
    .pv-grid {
      grid-template-columns: 1fr;
    }

    .pv-phase-strip {
      flex-wrap: wrap;
    }
  }
</style>
