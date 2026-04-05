<script lang="ts">
  import { onMount, onDestroy, tick } from 'svelte'
  import {
    getEventsHistory, getHeartbeatStatus, streamEvents,
    getSession, createSession, renameSession, deleteSession, compactSession, getSessionHistory,
  } from '../lib/api'
  import type { HeartbeatStatus, NotificationMessage, Session } from '../lib/types'
  import type { Artifact } from '../lib/artifacts'
  import SessionSidebar from './SessionSidebar.svelte'
  import ChatPanel from './ChatPanel.svelte'
  import ArtifactPanel from './ArtifactPanel.svelte'
  import SessionConfigPanel from './SessionConfigPanel.svelte'
  import ContextMonitor from './ContextMonitor.svelte'
  import PromptEditor from './PromptEditor.svelte'
  import TasksPanel from './TasksPanel.svelte'
  import SessionCronPanel from './SessionCronPanel.svelte'

  interface Props {
    sessionId?: string
    onNavigate: (path: string) => void
    initialPrompt?: string
  }

  let { sessionId, onNavigate, initialPrompt }: Props = $props()

  // Dashboard mini state
  // Dashboard mini state removed: projects
  let heartbeat: HeartbeatStatus | null = $state(null)
  let unreadCount = $state(0)

  // Session selection — synced from sessionId prop
  let selectedSessionId: string | null = $state(null)
  let selectedSession: Session | null = $state(null)
  let chatKey = $state(0)
  let lastPropSessionId: string | undefined = undefined

  $effect(() => {
    const sid = sessionId
    if (sid !== lastPropSessionId) {
      lastPropSessionId = sid
      selectedSessionId = sid || null
      selectedSession = null
      chatKey++
      chatContextInfo = {}
      if (sid) loadSelectedSession(sid)
    }
  })

  // Session action state
  let renaming = $state(false)
  let renameValue = $state('')
  let actionBusy = $state(false)
  let deleteConfirm = $state(false)

  // Right panel state
  let chatArtifacts: Artifact[] = $state([])
  let chatContextInfo: {
    system_prompt_tokens?: number
    history_tokens?: number
    history_messages?: number
    tool_count?: number
    tool_names?: string[]
    skill_count?: number
    skill_names?: string[]
    memory_count?: number
    memory_tokens?: number
    used_tool_names?: string[]
    selected_skill_name?: string
    selected_skill_reason?: string
  } = $state({})
  let contextRefreshVersion = $state(0)
  type RightPanel = 'none' | 'artifacts' | 'config' | 'context' | 'prompt' | 'tasks' | 'cron'
  let rightPanel = $state<RightPanel>('none')

  let sidebarRef: SessionSidebar | undefined = $state()
  let stopStream: (() => void) | null = null

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

  async function loadSelectedSession(id: string) {
    try {
      selectedSession = await getSession(id)
    } catch { /* ignore */ }
  }

  function handleSelectSession(session: Session) {
    selectedSessionId = session.id
    selectedSession = session
    chatKey++
    chatArtifacts = []
    chatContextInfo = {}
    rightPanel = 'none'
    renaming = false
    deleteConfirm = false
    onNavigate(`/console/chat/${encodeURIComponent(session.id)}`)
  }

  async function handleNewSession() {
    try {
      const sess = await createSession()
      selectedSessionId = sess.id
      selectedSession = sess
    } catch {
      selectedSessionId = null
      selectedSession = null
    }
    chatKey++
    chatArtifacts = []
    chatContextInfo = {}
    rightPanel = 'none'
    renaming = false
    deleteConfirm = false
    onNavigate(selectedSessionId ? `/console/chat/${encodeURIComponent(selectedSessionId)}` : '/console/chat')
  }

  function handleSessionChange() {
    sidebarRef?.load()
    // Refresh selected session title (may have been auto-titled)
    if (selectedSessionId) loadSelectedSession(selectedSessionId)
  }

  function handleArtifactsChange(arts: Artifact[]) {
    chatArtifacts = arts
    if (arts.length > 0 && rightPanel === 'none') {
      rightPanel = 'artifacts'
    }
  }

  function togglePanel(panel: RightPanel) {
    rightPanel = rightPanel === panel ? 'none' : panel
  }

  // Session actions
  function startRename() {
    if (!selectedSession || selectedSession.kind === 'main') return
    renaming = true
    renameValue = selectedSession.title || selectedSession.id.slice(0, 12)
  }

  async function commitRename() {
    if (!selectedSessionId || !renameValue.trim()) { renaming = false; return }
    actionBusy = true
    try {
      await renameSession(selectedSessionId, renameValue.trim())
      await loadSelectedSession(selectedSessionId)
      sidebarRef?.load()
    } catch { /* ignore */ }
    renaming = false
    actionBusy = false
  }

  async function handleAutoTitle() {
    if (!selectedSessionId || !selectedSession) return
    actionBusy = true
    try {
      const history = await getSessionHistory(selectedSessionId)
      const userMsgs = history.filter((m) => m.role === 'user')
      const assistantMsgs = history.filter((m) => m.role === 'assistant')
      let title = ''
      if (userMsgs.length > 0) {
        const raw = userMsgs[0].content.trim()
        const clean = raw.replace(/\n/g, ' ').replace(/\s+/g, ' ')
        title = clean.length > 50 ? clean.slice(0, 47) + '...' : clean
      } else if (assistantMsgs.length > 0) {
        const raw = assistantMsgs[0].content.trim()
        const clean = raw.replace(/\n/g, ' ').replace(/\s+/g, ' ')
        title = clean.length > 50 ? clean.slice(0, 47) + '...' : clean
      }
      if (title) {
        await renameSession(selectedSessionId, title)
        await loadSelectedSession(selectedSessionId)
        sidebarRef?.load()
      }
    } catch { /* ignore */ }
    actionBusy = false
  }

  async function handleCompact() {
    if (!selectedSessionId) return
    actionBusy = true
    try {
      await compactSession(selectedSessionId)
      sidebarRef?.load()
    } catch { /* ignore */ }
    actionBusy = false
  }

  async function handleDelete() {
    if (!selectedSessionId) return
    if (!deleteConfirm) { deleteConfirm = true; return }
    actionBusy = true
    try {
      await deleteSession(selectedSessionId)
      sidebarRef?.load()
      handleNewSession()
    } catch { /* ignore */ }
    actionBusy = false
    deleteConfirm = false
  }

  function isMainSession(): boolean {
    return selectedSession?.kind === 'main'
  }

  let chatPanelRef: ChatPanel | undefined = $state()
  let tasksPanelRef: { load: () => void } | undefined = $state()
  let artifactPanelRef: { refresh: () => void; openArtifactPath: (path: string) => Promise<void> } | undefined = $state()

  function handleToolComplete(toolName: string) {
    const taskTools = ['tasks']
    const fileTools = ['write_file', 'edit_file', 'exec', 'list_dir', 'read_file']

    if (taskTools.includes(toolName)) {
      tasksPanelRef?.load()
    }
    if (fileTools.includes(toolName) && rightPanel === 'artifacts') {
      artifactPanelRef?.refresh()
    }
  }

  async function handleArtifactOpen(path: string) {
    rightPanel = 'artifacts'
    await tick()
    await artifactPanelRef?.openArtifactPath(path)
  }

  function handleCopyChat() {
    const md = chatPanelRef?.exportAsMarkdown()
    if (md) navigator.clipboard.writeText(md).catch(() => {})
  }

  function handleDownloadChat() {
    const md = chatPanelRef?.exportAsMarkdown()
    if (!md) return
    const title = selectedSession?.title || 'chat'
    const blob = new Blob([md], { type: 'text/markdown' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `${title.replace(/[^a-zA-Z0-9가-힣-_ ]/g, '').slice(0, 50)}.md`
    a.click()
    URL.revokeObjectURL(url)
  }

  async function loadDashboard() {
    const [h, e] = await Promise.allSettled([
      getHeartbeatStatus(),
      getEventsHistory(1),
    ])
    heartbeat = h.status === 'fulfilled' ? h.value : null
    if (e.status === 'fulfilled') {
      unreadCount = e.value.unread_count ?? 0
    }
  }

  onMount(() => {
    void loadDashboard()
    stopStream = streamEvents(
      () => { unreadCount++ },
    )
  })

  onDestroy(() => {
    stopStream?.()
  })
</script>

<div class="chat-page">
  <!-- Mini dashboard pulse -->
  <div class="chat-pulse">
    <div class="pulse-item">
      <span class="pulse-val" class:warn={!!heartbeat?.last_error}>
        {heartbeat?.interval || 'off'}
      </span>
      <span class="pulse-lbl">Heartbeat</span>
    </div>
    <div class="pulse-sep"></div>
    <div class="pulse-item">
      <span class="pulse-val">{heartbeat?.last_run_at ? relativeTime(heartbeat.last_run_at) : 'never'}</span>
      <span class="pulse-lbl">Last run</span>
    </div>
    <div class="pulse-sep"></div>
    <div class="pulse-item">
      <span class="pulse-val">{unreadCount}</span>
      <span class="pulse-lbl">Unread</span>
    </div>
    <div class="pulse-sep"></div>
    <div class="pulse-panel-toggles">
      <button type="button" class="pulse-toggle-btn" class:active={rightPanel === 'artifacts'} onclick={() => togglePanel('artifacts')} title="Files browser">Files{#if chatArtifacts.length > 0} ({chatArtifacts.length}){/if}</button>
      <button type="button" class="pulse-toggle-btn" class:active={rightPanel === 'config'} onclick={() => togglePanel('config')} title="Session tool config">Config</button>
      <button type="button" class="pulse-toggle-btn" class:active={rightPanel === 'context'} onclick={() => togglePanel('context')} title="Context monitor">Context</button>
      <button type="button" class="pulse-toggle-btn" class:active={rightPanel === 'prompt'} onclick={() => togglePanel('prompt')} title="Prompt editor">Prompt</button>
      <button type="button" class="pulse-toggle-btn" class:active={rightPanel === 'tasks'} onclick={() => togglePanel('tasks')} title="Session tasks">Tasks</button>
      <button type="button" class="pulse-toggle-btn" class:active={rightPanel === 'cron'} onclick={() => togglePanel('cron')} title="Session cron jobs">Cron</button>
    </div>
  </div>

  <div class="chat-layout" class:has-right-panel={rightPanel !== 'none'}>
    <!-- Session sidebar -->
    <aside class="chat-sidebar">
      <SessionSidebar
        bind:this={sidebarRef}
        selectedSessionId={selectedSessionId}
        onSelect={handleSelectSession}
        onNewSession={handleNewSession}
      />
    </aside>

    <!-- Chat area -->
    <main class="chat-main">
      <!-- Session header with actions -->
      {#if selectedSession}
        <div class="session-header">
          <div class="session-title-row">
            {#if renaming}
              <!-- svelte-ignore a11y_autofocus -->
              <input
                class="session-rename-input"
                bind:value={renameValue}
                autofocus
                onkeydown={(e) => { if (e.key === 'Enter') commitRename(); if (e.key === 'Escape') { renaming = false } }}
                onblur={() => commitRename()}
              />
            {:else}
              <h3 class="session-title">{selectedSession.title || selectedSession.id.slice(0, 12)}</h3>
            {/if}
          </div>
          <div class="session-actions">
            {#if !isMainSession()}
              <button class="btn btn-ghost btn-sm" disabled={actionBusy} onclick={startRename}>Rename</button>
              <button class="btn btn-ghost btn-sm" disabled={actionBusy} onclick={handleAutoTitle} title="Generate title from first message">AI Title</button>
            {/if}
            <button class="btn btn-ghost btn-sm" disabled={actionBusy} onclick={handleCompact} title="Compress transcript">Compact</button>
            <span class="session-actions-sep"></span>
            <button class="btn btn-ghost btn-sm" onclick={handleCopyChat} title="Copy conversation to clipboard">Copy All</button>
            <button class="btn btn-ghost btn-sm" onclick={handleDownloadChat} title="Download as markdown file">Download</button>
            {#if !isMainSession()}
              <span class="session-actions-sep"></span>
              <button class="btn btn-danger btn-sm" disabled={actionBusy} onclick={handleDelete}>
                {deleteConfirm ? 'Confirm?' : 'Delete'}
              </button>
            {/if}
          </div>
        </div>
      {:else}
        <div class="session-header">
          <h3 class="session-title new-chat-title">New Chat</h3>
        </div>
      {/if}

      {#key chatKey}
        <ChatPanel
          bind:this={chatPanelRef}
          sessionId={selectedSessionId || undefined}
          {initialPrompt}
          onSessionChange={handleSessionChange}
          onArtifactsChange={handleArtifactsChange}
          onContextInfo={(info) => { chatContextInfo = info }}
          onToolComplete={handleToolComplete}
          onSessionReady={(id) => {
            if (!selectedSessionId) {
              selectedSessionId = id
              void loadSelectedSession(id)
              sidebarRef?.load()
            }
          }}
          onArtifactOpen={handleArtifactOpen}
        />
      {/key}
    </main>

    <!-- Right panel -->
    {#if rightPanel !== 'none'}
      <aside class="chat-right-panel">
        {#if rightPanel === 'artifacts'}
          <ArtifactPanel bind:this={artifactPanelRef} artifacts={chatArtifacts} sessionId={selectedSessionId || ''} onClose={() => { rightPanel = 'none' }} />
        {:else if rightPanel === 'config' && (selectedSessionId || true)}
          <SessionConfigPanel
            sessionId={selectedSessionId ?? ''}
            onClose={() => { rightPanel = 'none' }}
            onChange={() => { contextRefreshVersion += 1 }}
          />
        {:else if rightPanel === 'context'}
          <ContextMonitor
            sessionId={selectedSessionId ?? ''}
            contextInfo={chatContextInfo}
            refreshVersion={contextRefreshVersion}
            onClose={() => { rightPanel = 'none' }}
          />
        {:else if rightPanel === 'prompt'}
          <PromptEditor sessionId={selectedSessionId ?? ''} onClose={() => { rightPanel = 'none' }} />
        {:else if rightPanel === 'tasks' && selectedSessionId}
          <TasksPanel bind:this={tasksPanelRef} sessionId={selectedSessionId} onClose={() => rightPanel = 'none'} />
        {:else if rightPanel === 'cron' && selectedSessionId}
          <SessionCronPanel sessionId={selectedSessionId} sessionKind={selectedSession?.kind ?? ''} onClose={() => rightPanel = 'none'} />
        {/if}
      </aside>
    {/if}
  </div>
</div>

<style>
  .chat-page {
    display: flex;
    flex-direction: column;
    height: calc(100vh - var(--header-height));
    animation: fadeIn var(--duration-normal) var(--ease-out);
  }

  @keyframes fadeIn {
    from { opacity: 0; transform: translateY(8px); }
    to { opacity: 1; transform: translateY(0); }
  }

  /* Mini dashboard */
  .chat-pulse {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    padding: var(--space-2) var(--space-4);
    background: var(--bg-surface);
    border-bottom: 1px solid var(--border-subtle);
    flex-shrink: 0;
    position: sticky;
    top: 0;
    z-index: 10;
  }

  .pulse-item {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }
  .pulse-val {
    font-family: var(--font-display);
    font-size: var(--text-sm);
    font-weight: 600;
    color: var(--text-primary);
  }
  .pulse-val.warn { color: var(--error); }
  .pulse-lbl {
    font-size: var(--text-xs);
    color: var(--text-ghost);
  }
  .pulse-sep {
    width: 1px;
    height: 16px;
    background: var(--border-subtle);
    flex-shrink: 0;
  }

  .pulse-panel-toggles {
    display: flex;
    gap: 2px;
  }

  .pulse-toggle-btn {
    background: none;
    border: 1px solid var(--border-subtle);
    color: var(--text-ghost);
    font-family: var(--font-mono);
    font-size: 10px;
    cursor: pointer;
    padding: 2px 8px;
    border-radius: var(--radius-sm);
    transition: all var(--duration-fast);
  }
  .pulse-toggle-btn:hover {
    color: var(--text-primary);
    border-color: var(--border-default);
  }
  .pulse-toggle-btn.active {
    color: var(--accent);
    border-color: var(--accent);
    background: rgba(224, 145, 69, 0.08);
  }

  /* Layout */
  .chat-layout {
    flex: 1;
    display: grid;
    grid-template-columns: 280px 1fr;
    min-height: 0;
  }
  .chat-layout.has-right-panel {
    grid-template-columns: 280px 1fr 300px;
  }

  .chat-sidebar {
    border-right: 1px solid var(--border-subtle);
    background: var(--bg-surface);
    padding: var(--space-3);
    overflow: hidden;
  }

  .chat-main {
    display: flex;
    flex-direction: column;
    min-height: 0;
    padding: var(--space-4);
    padding-top: 0;
    overflow: hidden;
  }

  .chat-right-panel {
    overflow: hidden;
    min-height: 0;
  }

  /* Session header */
  .session-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    padding: var(--space-3) var(--space-4);
    flex-shrink: 0;
    min-height: 44px;
  }

  .session-title-row {
    flex: 1;
    min-width: 0;
  }

  .session-title {
    font-family: var(--font-display);
    font-size: var(--text-base);
    font-weight: 500;
    color: var(--text-primary);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    margin: 0;
  }

  .new-chat-title {
    color: var(--text-tertiary);
  }

  .session-rename-input {
    width: 100%;
    padding: var(--space-1) var(--space-2);
    font-size: var(--text-base);
    font-family: var(--font-display);
    background: var(--bg-base);
    border: 1px solid var(--accent);
    border-radius: var(--radius-sm);
    color: var(--text-primary);
    outline: none;
  }

  .session-actions {
    display: flex;
    align-items: center;
    gap: var(--space-1);
    flex-shrink: 0;
  }

  .session-actions-sep {
    width: 1px;
    height: 16px;
    background: var(--border-subtle);
    margin: 0 var(--space-1);
  }

  @media (max-width: 768px) {
    .chat-layout, .chat-layout.has-right-panel {
      grid-template-columns: 1fr;
    }
    .chat-sidebar, .chat-right-panel {
      display: none;
    }
    .chat-pulse {
      flex-wrap: wrap;
      gap: var(--space-2);
    }
    .pulse-sep { display: none; }
    .session-actions {
      flex-wrap: wrap;
    }
  }
</style>
