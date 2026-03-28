<script lang="ts">
  import { onMount } from 'svelte'
  import { getProject, getProjectAutopilot, listProjects, streamChat } from './lib/api'
  import { currentProjectIdFromPath, isConsoleRoot, projectPath } from './lib/router'
  import type { ChatEvent, Project, ProjectAutopilotRun } from './lib/types'

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

  let loadingProjects = true
  let loadingDetail = false
  let listError = ''
  let detailError = ''

  let chatInput = ''
  let chatBusy = false
  let chatError = ''
  let chatSessionId = ''
  let chatStatusLine = ''
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
    }
    void loadProjectDetail(normalized)
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
      selectedProjectId = null
      selectedProject = null
      selectedAutopilot = null
    } catch (error) {
      listError = error instanceof Error ? error.message : 'Failed to load projects'
    } finally {
      loadingProjects = false
    }
  }

  async function loadProjectDetail(projectId: string) {
    loadingDetail = true
    detailError = ''
    try {
      const [project, autopilot] = await Promise.all([
        getProject(projectId),
        getProjectAutopilot(projectId),
      ])
      selectedProject = project
      selectedAutopilot = autopilot
    } catch (error) {
      selectedProject = null
      selectedAutopilot = null
      detailError = error instanceof Error ? error.message : 'Failed to load project detail'
    } finally {
      loadingDetail = false
    }
  }

  function handleChatEvent(event: ChatEvent, assistantId: string) {
    switch (event.type) {
      case 'status':
        chatStatusLine = [event.phase, event.message, event.tool_name].filter(Boolean).join(' · ')
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

  onMount(() => {
    currentPathname = window.location.pathname
    void loadProjectsAndSelection()

    const onPopState = () => {
      currentPathname = window.location.pathname
      const routeProjectId = currentProjectIdFromPath(window.location.pathname)
      if (routeProjectId) {
        const projectChanged = routeProjectId !== selectedProjectId
        selectedProjectId = routeProjectId
        if (projectChanged) {
          resetChatForProject(routeProjectId)
        }
        void loadProjectDetail(routeProjectId)
        return
      }
      if (projects.length > 0) {
        selectedProjectId = projects[0].id
        resetChatForProject(projects[0].id)
        void loadProjectDetail(projects[0].id)
        return
      }
      selectedProjectId = null
      selectedProject = null
      selectedAutopilot = null
    }

    window.addEventListener('popstate', onPopState)
    return () => window.removeEventListener('popstate', onPopState)
  })
</script>

<section class="shell">
  <header class="hero">
    <div>
      <div class="eyebrow">TARS Console</div>
      <h1>Project operator console</h1>
      <p class="lead">
        The console now supports project navigation, live project-scoped chat, and phase/run inspection from
        <code>/console</code>.
      </p>
    </div>
    <div class="hero-card">
      <div class="meta-label">Current path</div>
      <div class="mono">{currentPathname}</div>
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
                <p>{project.objective}</p>
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
            {#if loadingDetail}
              <span class="panel-pill">refreshing</span>
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
