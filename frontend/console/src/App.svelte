<script lang="ts">
  import { onMount } from 'svelte'
  import {
    getEventsHistory,
    getProject,
    getProjectAutopilot,
    listApprovals,
    listCronJobs,
    listCronRuns,
    listProjectActivity,
    listProjects,
    reviewApproval,
    streamChat,
    streamEvents,
  } from './lib/api'
  import { currentProjectIdFromPath, isConsoleRoot, projectPath } from './lib/router'
  import type {
    Approval,
    ChatEvent,
    CronJob,
    CronRunRecord,
    NotificationMessage,
    Project,
    ProjectActivity,
    ProjectAutopilotRun,
  } from './lib/types'

  type ChatMessage = {
    id: string
    role: 'user' | 'assistant' | 'system' | 'error'
    text: string
  }

  let projects: Project[] = []
  let selectedProjectId: string | null = null
  let selectedProject: Project | null = null
  let selectedAutopilot: ProjectAutopilotRun | null = null
  let currentPathname = '/console'

  let projectActivity: ProjectActivity[] = []
  let projectCronJobs: CronJob[] = []
  let cronRunsByJob: Record<string, CronRunRecord[]> = {}
  let notificationItems: NotificationMessage[] = []
  let approvals: Approval[] = []

  let loadingProjects = true
  let loadingDetail = false
  let loadingPanels = false
  let listError = ''
  let detailError = ''
  let panelError = ''

  let chatInput = ''
  let chatBusy = false
  let chatError = ''
  let chatSessionId = ''
  let chatStatusLine = ''
  let eventStatusLine = 'idle'
  let approvalBusyId = ''

  let stopProjectEvents: (() => void) | null = null
  let activeEventProjectId = ''

  let chatMessages: ChatMessage[] = [
    {
      id: 'system-welcome',
      role: 'system',
      text: 'Chat is connected to /v1/chat. Messages stay local to this page for now.',
    },
  ]

  function fmt(value?: string): string {
    const text = value?.trim()
    if (!text) {
      return '—'
    }
    const date = new Date(text)
    if (Number.isNaN(date.getTime())) {
      return text
    }
    return new Intl.DateTimeFormat('en', {
      dateStyle: 'medium',
      timeStyle: 'short',
    }).format(date)
  }

  function compact(value?: string, max = 180): string {
    const text = value?.trim()
    if (!text) {
      return '—'
    }
    if (text.length <= max) {
      return text
    }
    return `${text.slice(0, max - 1)}…`
  }

  function resetPanelState() {
    projectActivity = []
    projectCronJobs = []
    cronRunsByJob = {}
    notificationItems = []
    approvals = []
    loadingPanels = false
    panelError = ''
    eventStatusLine = 'idle'
    approvalBusyId = ''
  }

  function resetChatForProject(projectId: string) {
    chatInput = ''
    chatBusy = false
    chatError = ''
    chatStatusLine = ''
    chatSessionId = ''
    chatMessages = [
      {
        id: `system-${projectId}`,
        role: 'system',
        text: `Chat is scoped to project ${projectId}. Persistent session history will be added in a later slice.`,
      },
    ]
  }

  function filterProjectNotifications(items: NotificationMessage[], projectId: string): NotificationMessage[] {
    return items
      .filter((item) => item.project_id?.trim() === projectId)
      .sort((left, right) => {
        const leftTime = new Date(left.timestamp).getTime()
        const rightTime = new Date(right.timestamp).getTime()
        return rightTime - leftTime
      })
      .slice(0, 20)
  }

  function mergeNotification(event: NotificationMessage, projectId: string) {
    notificationItems = filterProjectNotifications(
      [event, ...notificationItems.filter((item) => item.id !== event.id)],
      projectId,
    )
  }

  async function loadProjectDetail(projectId: string) {
    loadingDetail = true
    detailError = ''
    try {
      const [project, autopilot] = await Promise.all([
        getProject(projectId),
        getProjectAutopilot(projectId),
      ])
      if (selectedProjectId !== projectId) {
        return
      }
      selectedProject = project
      selectedAutopilot = autopilot
    } catch (error) {
      if (selectedProjectId !== projectId) {
        return
      }
      selectedProject = null
      selectedAutopilot = null
      detailError = error instanceof Error ? error.message : 'Failed to load project detail'
    } finally {
      if (selectedProjectId === projectId) {
        loadingDetail = false
      }
    }
  }

  async function loadProjectActivityPanel(projectId: string) {
    const items = await listProjectActivity(projectId, 20)
    if (selectedProjectId === projectId) {
      projectActivity = items
    }
  }

  async function loadProjectCronPanel(projectId: string) {
    const jobs = (await listCronJobs()).filter((job) => job.project_id?.trim() === projectId)
    const runsByJobEntries = await Promise.all(
      jobs.map(async (job) => [job.id, await listCronRuns(job.id, 5)] as const),
    )
    if (selectedProjectId === projectId) {
      projectCronJobs = jobs
      cronRunsByJob = Object.fromEntries(runsByJobEntries)
    }
  }

  async function loadProjectNotificationsPanel(projectId: string) {
    const history = await getEventsHistory(40)
    if (selectedProjectId === projectId) {
      notificationItems = filterProjectNotifications(history.items ?? [], projectId)
    }
  }

  async function loadApprovalsPanel() {
    approvals = await listApprovals()
  }

  async function loadProjectPanels(projectId: string) {
    loadingPanels = true
    panelError = ''

    const results = await Promise.allSettled([
      loadProjectActivityPanel(projectId),
      loadProjectCronPanel(projectId),
      loadProjectNotificationsPanel(projectId),
      loadApprovalsPanel(),
    ])

    if (selectedProjectId === projectId) {
      const failures = results
        .filter((result): result is PromiseRejectedResult => result.status === 'rejected')
        .map((result) => (result.reason instanceof Error ? result.reason.message : 'Panel refresh failed'))
      panelError = failures.join(' · ')
      loadingPanels = false
    }
  }

  function syncProjectEventStream(projectId: string) {
    if (activeEventProjectId === projectId) {
      return
    }
    stopProjectEvents?.()
    activeEventProjectId = projectId
    eventStatusLine = 'connecting'
    stopProjectEvents = streamEvents(
      projectId,
      (event) => {
        if (selectedProjectId !== projectId) {
          return
        }
        eventStatusLine = `${event.category} · ${event.title}`
        mergeNotification(event, projectId)
        if (event.category === 'cron' || event.category === 'watchdog') {
          void loadProjectActivityPanel(projectId)
          void loadProjectCronPanel(projectId)
          return
        }
        if (event.category === 'ops') {
          void loadApprovalsPanel()
        }
      },
      (message) => {
        if (selectedProjectId === projectId) {
          eventStatusLine = message
        }
      },
    )
  }

  function clearProjectEventStream() {
    stopProjectEvents?.()
    stopProjectEvents = null
    activeEventProjectId = ''
    eventStatusLine = 'idle'
  }

  function selectProject(projectId: string, replace = false) {
    const normalized = projectId.trim()
    if (!normalized) {
      return
    }
    const projectChanged = normalized !== selectedProjectId
    selectedProjectId = normalized
    const nextPath = projectPath(normalized)
    if (window.location.pathname !== nextPath) {
      const method = replace ? 'replaceState' : 'pushState'
      window.history[method](null, '', nextPath)
    }
    currentPathname = nextPath
    if (projectChanged) {
      resetChatForProject(normalized)
      resetPanelState()
      syncProjectEventStream(normalized)
    }
    void loadProjectDetail(normalized)
    void loadProjectPanels(normalized)
  }

  async function loadProjectsAndSelection() {
    loadingProjects = true
    listError = ''
    try {
      projects = await listProjects()
      const routeProjectId = currentProjectIdFromPath(window.location.pathname)
      if (routeProjectId && projects.some((item) => item.id === routeProjectId)) {
        selectProject(routeProjectId, true)
        return
      }
      if (projects.length > 0) {
        selectProject(projects[0].id, isConsoleRoot(window.location.pathname))
        return
      }
      clearProjectEventStream()
      selectedProjectId = null
      selectedProject = null
      selectedAutopilot = null
      resetPanelState()
    } catch (error) {
      listError = error instanceof Error ? error.message : 'Failed to load projects'
    } finally {
      loadingProjects = false
    }
  }

  function handleChatEvent(event: ChatEvent, assistantId: string) {
    switch (event.type) {
      case 'status':
        chatStatusLine = [event.phase, event.message, event.tool_name, event.skill_name].filter(Boolean).join(' · ')
        break
      case 'delta': {
        const chunk = event.text ?? ''
        if (!chunk) {
          break
        }
        const index = chatMessages.findIndex((item) => item.id === assistantId)
        if (index >= 0) {
          chatMessages[index] = {
            ...chatMessages[index],
            text: chatMessages[index].text + chunk,
          }
          chatMessages = [...chatMessages]
        }
        break
      }
      case 'done':
        chatSessionId = event.session_id?.trim() || chatSessionId
        chatStatusLine = 'response complete'
        break
      case 'error':
        chatError = event.error?.trim() || 'Chat stream failed'
        break
    }
  }

  async function submitChat() {
    const message = chatInput.trim()
    if (!message || chatBusy || !selectedProjectId) {
      return
    }

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
        {
          message,
          session_id: chatSessionId || undefined,
          project_id: selectedProjectId,
        },
        (event) => handleChatEvent(event, assistantId),
      )
    } catch (error) {
      chatError = error instanceof Error ? error.message : 'Failed to send chat request'
      chatMessages = [
        ...chatMessages,
        { id: `error-${Date.now()}`, role: 'error', text: chatError },
      ]
    } finally {
      chatBusy = false
    }
  }

  async function handleApprovalAction(approvalId: string, action: 'approve' | 'reject') {
    if (!approvalId.trim() || approvalBusyId) {
      return
    }
    approvalBusyId = approvalId
    panelError = ''
    try {
      await reviewApproval(approvalId, action)
      await loadApprovalsPanel()
    } catch (error) {
      panelError = error instanceof Error ? error.message : `Failed to ${action} approval`
    } finally {
      approvalBusyId = ''
    }
  }

  onMount(() => {
    currentPathname = window.location.pathname
    void loadProjectsAndSelection()

    const onPopState = () => {
      currentPathname = window.location.pathname
      const routeProjectId = currentProjectIdFromPath(window.location.pathname)
      if (routeProjectId) {
        selectProject(routeProjectId, true)
        return
      }
      if (projects.length > 0) {
        selectProject(projects[0].id, true)
        return
      }
      clearProjectEventStream()
      selectedProjectId = null
      selectedProject = null
      selectedAutopilot = null
      resetPanelState()
    }

    window.addEventListener('popstate', onPopState)
    return () => {
      window.removeEventListener('popstate', onPopState)
      clearProjectEventStream()
    }
  })
</script>

<section class="shell">
  <header class="hero">
    <div>
      <div class="eyebrow">TARS Console</div>
      <h1>Project operator console</h1>
      <p class="lead">
        The console now supports project navigation, phase/run inspection, project-scoped chat, and live project
        notifications from <code>/console</code>.
      </p>
    </div>
    <div class="hero-card">
      <div class="meta-label">Current path</div>
      <div class="mono">{currentPathname}</div>
      <div class="meta-label">Event stream</div>
      <div class="meta-value">{eventStatusLine}</div>
      <div class="meta-label">Chat session</div>
      <div class="meta-value mono">{chatSessionId || 'new'}</div>
    </div>
  </header>

  <section class="workspace">
    <aside class="sidebar">
      <div class="panel-title-row">
        <h2>Projects</h2>
        {#if loadingProjects}
          <span class="panel-pill">loading</span>
        {/if}
      </div>

      {#if listError}
        <div class="error-box">{listError}</div>
      {:else if !loadingProjects && projects.length === 0}
        <div class="empty-box">
          <h3>No projects yet</h3>
          <p>Create a project through the API or existing CLI, then return to this console.</p>
        </div>
      {:else}
        <div class="project-list">
          {#each projects as project}
            <button
              type="button"
              class:selected={project.id === selectedProjectId}
              class="project-item"
              onclick={() => selectProject(project.id)}
            >
              <div class="project-item-top">
                <strong>{project.name}</strong>
                <span class="status-chip">{project.status || 'unknown'}</span>
              </div>
              <div class="project-item-meta">{project.type || 'project'}</div>
              {#if project.objective}
                <p>{compact(project.objective, 120)}</p>
              {/if}
              <div class="project-item-time">updated {fmt(project.updated_at)}</div>
            </button>
          {/each}
        </div>
      {/if}
    </aside>

    <main class="detail">
      <div class="detail-header">
        <div>
          <div class="panel-title-row">
            <h2>Project detail</h2>
            {#if loadingDetail || loadingPanels}
              <span class="panel-pill">{loadingDetail ? 'refreshing detail' : 'refreshing panels'}</span>
            {/if}
          </div>
          {#if selectedProject}
            <h3>{selectedProject.name}</h3>
            <p class="detail-subtitle">{selectedProject.objective || 'No objective recorded yet.'}</p>
          {:else if !loadingProjects}
            <h3>Select a project</h3>
            <p class="detail-subtitle">Choose a project from the left sidebar to inspect its current phase.</p>
          {/if}
        </div>
      </div>

      {#if detailError}
        <div class="error-box">{detailError}</div>
      {:else if selectedProject}
        {#if panelError}
          <div class="error-box compact">{panelError}</div>
        {/if}

        <div class="detail-grid">
          <article class="card">
            <div class="card-label">Identity</div>
            <dl class="facts">
              <div><dt>ID</dt><dd class="mono">{selectedProject.id}</dd></div>
              <div><dt>Type</dt><dd>{selectedProject.type || '—'}</dd></div>
              <div><dt>Status</dt><dd>{selectedProject.status || '—'}</dd></div>
              <div><dt>Updated</dt><dd>{fmt(selectedProject.updated_at)}</dd></div>
            </dl>
          </article>

          <article class="card">
            <div class="card-label">Repository</div>
            <dl class="facts">
              <div><dt>Workspace path</dt><dd class="mono">{selectedProject.path || '—'}</dd></div>
              <div><dt>Git repo</dt><dd class="mono">{selectedProject.git_repo || '—'}</dd></div>
              <div><dt>Created</dt><dd>{fmt(selectedProject.created_at)}</dd></div>
            </dl>
          </article>

          <article class="card card-wide">
            <div class="card-label">Phase / run</div>
            {#if selectedAutopilot}
              <div class="phase-grid">
                <div class="phase-item">
                  <span>Phase</span>
                  <strong>{selectedAutopilot.phase || '—'}</strong>
                </div>
                <div class="phase-item">
                  <span>Phase status</span>
                  <strong>{selectedAutopilot.phase_status || '—'}</strong>
                </div>
                <div class="phase-item">
                  <span>Run status</span>
                  <strong>{selectedAutopilot.status || '—'}</strong>
                </div>
                <div class="phase-item">
                  <span>Iterations</span>
                  <strong>{selectedAutopilot.iterations}</strong>
                </div>
              </div>

              <dl class="facts stacked">
                <div><dt>Next action</dt><dd>{selectedAutopilot.next_action || '—'}</dd></div>
                <div><dt>Summary</dt><dd>{selectedAutopilot.summary || '—'}</dd></div>
                <div><dt>Message</dt><dd>{selectedAutopilot.message || '—'}</dd></div>
                <div><dt>Run ID</dt><dd class="mono">{selectedAutopilot.run_id || '—'}</dd></div>
                <div><dt>Updated</dt><dd>{fmt(selectedAutopilot.updated_at)}</dd></div>
              </dl>
            {:else}
              <div class="empty-box">
                <h3>Autopilot not started</h3>
                <p>This project does not have an active autopilot run yet, or no persisted run was found.</p>
              </div>
            {/if}
          </article>

          <article class="card card-wide">
            <div class="card-label">Chat</div>
            <div class="chat-meta">
              <span>Project</span>
              <strong>{selectedProject.id}</strong>
              <span>Status</span>
              <strong>{chatBusy ? 'streaming' : chatStatusLine || 'idle'}</strong>
            </div>
            <div class="chat-log">
              {#each chatMessages as item}
                <div class={`chat-row ${item.role}`}>
                  <div class="chat-role">{item.role}</div>
                  <div class="chat-text">{item.text || '…'}</div>
                </div>
              {/each}
            </div>
            {#if chatError}
              <div class="error-box compact">{chatError}</div>
            {/if}
            <form
              class="chat-form"
              onsubmit={(event) => {
                event.preventDefault()
                void submitChat()
              }}
            >
              <textarea
                bind:value={chatInput}
                rows="4"
                placeholder="Ask TARS about this project..."
              ></textarea>
              <button type="submit" disabled={chatBusy || !chatInput.trim()}>
                {chatBusy ? 'Streaming...' : 'Send'}
              </button>
            </form>
          </article>

          <article class="card card-wide">
            <div class="panel-title-row">
              <div class="card-label">Recent activity</div>
              <span class="panel-pill">{projectActivity.length} items</span>
            </div>
            {#if projectActivity.length === 0}
              <div class="empty-box compact-box">
                <p>No project activity has been recorded yet.</p>
              </div>
            {:else}
              <div class="timeline-list">
                {#each projectActivity as item}
                  <div class="timeline-item">
                    <div class="timeline-title">
                      <strong>{item.kind}</strong>
                      <span>{item.status || item.source}</span>
                    </div>
                    <p>{item.message || 'No activity message'}</p>
                    <div class="timeline-meta">
                      <span>{fmt(item.timestamp)}</span>
                      {#if item.task_id}
                        <span class="mono">{item.task_id}</span>
                      {/if}
                      {#if item.agent}
                        <span>{item.agent}</span>
                      {/if}
                    </div>
                  </div>
                {/each}
              </div>
            {/if}
          </article>

          <article class="card card-wide">
            <div class="panel-title-row">
              <div class="card-label">Cron</div>
              <span class="panel-pill">{projectCronJobs.length} jobs</span>
            </div>
            {#if projectCronJobs.length === 0}
              <div class="empty-box compact-box">
                <p>No cron jobs are attached to this project yet.</p>
              </div>
            {:else}
              <div class="cron-job-list">
                {#each projectCronJobs as job}
                  <div class="cron-job-card">
                    <div class="timeline-title">
                      <strong>{job.name}</strong>
                      <span>{job.enabled ? job.schedule : 'disabled'}</span>
                    </div>
                    <p>{compact(job.prompt, 180)}</p>
                    <div class="timeline-meta">
                      <span>last run {fmt(job.last_run_at)}</span>
                      {#if job.last_run_error}
                        <span class="severity-error">{compact(job.last_run_error, 90)}</span>
                      {/if}
                    </div>
                    {#if (cronRunsByJob[job.id] ?? []).length > 0}
                      <div class="cron-run-list">
                        {#each cronRunsByJob[job.id] ?? [] as run}
                          <div class="cron-run-item">
                            <strong>{fmt(run.ran_at)}</strong>
                            <span>{compact(run.error || run.response || 'No output', 180)}</span>
                          </div>
                        {/each}
                      </div>
                    {/if}
                  </div>
                {/each}
              </div>
            {/if}
          </article>

          <article class="card card-wide">
            <div class="panel-title-row">
              <div class="card-label">Notifications</div>
              <span class="panel-pill">{notificationItems.length} project events</span>
            </div>
            {#if notificationItems.length === 0}
              <div class="empty-box compact-box">
                <p>No project-scoped notifications are available yet.</p>
              </div>
            {:else}
              <div class="timeline-list">
                {#each notificationItems as item}
                  <div class="timeline-item">
                    <div class="timeline-title">
                      <strong>{item.title}</strong>
                      <span>{item.severity}</span>
                    </div>
                    <p>{item.message}</p>
                    <div class="timeline-meta">
                      <span>{fmt(item.timestamp)}</span>
                      <span>{item.category}</span>
                      {#if item.job_id}
                        <span class="mono">{item.job_id}</span>
                      {/if}
                    </div>
                  </div>
                {/each}
              </div>
            {/if}
          </article>

          <article class="card card-wide">
            <div class="panel-title-row">
              <div class="card-label">Approvals</div>
              <span class="panel-pill">{approvals.length} total</span>
            </div>
            {#if approvals.length === 0}
              <div class="empty-box compact-box">
                <p>No ops approvals are waiting right now.</p>
              </div>
            {:else}
              <div class="approval-list">
                {#each approvals as approval}
                  <div class="approval-item">
                    <div class="timeline-title">
                      <strong>{approval.type}</strong>
                      <span>{approval.status}</span>
                    </div>
                    <p>
                      requested {fmt(approval.requested_at)} · {approval.plan.candidates.length} candidates ·
                      {approval.plan.total_bytes} bytes
                    </p>
                    {#if approval.note}
                      <p>{approval.note}</p>
                    {/if}
                    {#if approval.status === 'pending'}
                      <div class="approval-actions">
                        <button
                          type="button"
                          class="ghost-button"
                          disabled={approvalBusyId === approval.id}
                          onclick={() => {
                            void handleApprovalAction(approval.id, 'approve')
                          }}
                        >
                          {approvalBusyId === approval.id ? 'Working...' : 'Approve'}
                        </button>
                        <button
                          type="button"
                          class="ghost-button danger"
                          disabled={approvalBusyId === approval.id}
                          onclick={() => {
                            void handleApprovalAction(approval.id, 'reject')
                          }}
                        >
                          Reject
                        </button>
                      </div>
                    {/if}
                  </div>
                {/each}
              </div>
            {/if}
          </article>

          <article class="card card-wide">
            <div class="card-label">Instructions</div>
            <div class="body-copy">
              {selectedProject.body || 'No project instructions body recorded yet.'}
            </div>
          </article>
        </div>
      {/if}
    </main>
  </section>
</section>
