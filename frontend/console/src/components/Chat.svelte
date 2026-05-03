<script lang="ts">
  import { onMount, tick } from 'svelte'
  import {
    getEventsHistory, getPulseStatus,
    getSession, createSession, renameSession, deleteSession, compactSession, getSessionHistory,
    getSessionTasks, listChatTools, getSessionConfig, updateSessionConfig,
    type SessionToolConfig,
  } from '../lib/api'
  import { emptyTaskProgressSummary, planProgressPercent, summarizeTasks, type TaskProgressSummary } from '../lib/tasks'
  import { buildSessionHealthReport, emptySessionHealthReport, type SessionHealthAction, type SessionHealthInput, type SessionHealthReport } from '../lib/sessionHealth'
  import type { ChatTierRecommendationRequest, PulseSnapshot, NotificationMessage, Session, SessionMessage, SessionTasks } from '../lib/types'
  import type { Artifact } from '../lib/artifacts'
  import SessionSidebar from './SessionSidebar.svelte'
  import ChatPanel from './ChatPanel.svelte'
  import ArtifactPanel from './ArtifactPanel.svelte'
  import SessionConfigPanel from './SessionConfigPanel.svelte'
  import ContextMonitor from './ContextMonitor.svelte'
  import PromptEditor from './PromptEditor.svelte'
  import PriorContextPanel from './PriorContextPanel.svelte'
  import ContractPanel from './ContractPanel.svelte'
  import TasksPanel from './TasksPanel.svelte'
  import GitInspector from './GitInspector.svelte'
  import SkillExtractionPanel from './SkillExtractionPanel.svelte'
  import SessionCronPanel from './SessionCronPanel.svelte'
  import SessionHealthPanel from './SessionHealthPanel.svelte'
  import DockPanelFrame from './DockPanelFrame.svelte'
  import IntegratedTerminal from './IntegratedTerminal.svelte'
  import {
    closeDockPanel,
    createDockLayout,
    moveDockPanel,
    normalizeDockLayout,
    openDockPanel,
    panelIsOpen,
    resizeDock,
    serializeDockLayout,
    type DockLayoutState,
    type DockPanelDefinition,
    type DockZone,
  } from '../lib/dock/layout'

  interface Props {
    sessionId?: string
    onNavigate: (path: string) => void
    initialPrompt?: string
  }

  let { sessionId, onNavigate, initialPrompt }: Props = $props()

  // Dashboard mini state
  // Dashboard mini state removed: projects
  let pulse: PulseSnapshot | null = $state(null)
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
      chatDraft = ''
      chatContextInfo = {}
      if (sid) loadSelectedSession(sid)
    }
  })

  // Session action state
  let renaming = $state(false)
  let renameValue = $state('')
  let actionBusy = $state(false)
  let deleteConfirm = $state(false)

  // Docked panel state
  let chatArtifacts: Artifact[] = $state([])
  let chatDraft = $state('')
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
    compaction_trigger_tokens?: number
    compaction_keep_recent_tokens?: number
    compaction_keep_recent_fraction?: number
    compaction_last_mode?: string
    used_tool_names?: string[]
    selected_skill_name?: string
    selected_skill_reason?: string
    mentioned_path_count?: number
    mentioned_paths?: string[]
    mentioned_subagent_count?: number
    mentioned_subagents?: string[]
    llm_tier?: string
    tier_recommendation?: ChatTierRecommendationRequest
  } = $state({})
  let contextRefreshVersion = $state(0)
  type ChatDockPanelID = 'sessions' | 'artifacts' | 'config' | 'context' | 'prompt' | 'prior' | 'contract' | 'tasks' | 'git' | 'skillExtraction' | 'cron' | 'health' | 'terminal'
  type ToolDockPanelID = Exclude<ChatDockPanelID, 'sessions'>

  // Below this width the chat layout collapses; dock panels render as fullscreen overlays.
  const MOBILE_LAYOUT_MAX_WIDTH = 900
  function isMobileLayout(): boolean {
    return typeof window !== 'undefined' && window.matchMedia(`(max-width: ${MOBILE_LAYOUT_MAX_WIDTH}px)`).matches
  }
  type DockSizeZone = 'left' | 'right' | 'bottom'
  const dockStorageKey = 'tars.console.chat.dockLayout.v1'
  const dockPanels: DockPanelDefinition[] = [
    { id: 'sessions', title: 'Sessions', defaultZone: 'left', closeable: false },
    { id: 'artifacts', title: 'Files', defaultZone: 'right' },
    { id: 'config', title: 'Config', defaultZone: 'right' },
    { id: 'context', title: 'Context', defaultZone: 'right' },
    { id: 'prompt', title: 'Prompt', defaultZone: 'right' },
    { id: 'prior', title: 'Prior Context', defaultZone: 'right' },
    { id: 'contract', title: 'Contract', defaultZone: 'right' },
    { id: 'tasks', title: 'Tasks', defaultZone: 'right' },
    { id: 'git', title: 'Git', defaultZone: 'right' },
    { id: 'skillExtraction', title: 'Skill Inbox', defaultZone: 'right' },
    { id: 'cron', title: 'Cron', defaultZone: 'right' },
    { id: 'health', title: 'Health', defaultZone: 'right' },
    { id: 'terminal', title: 'Terminal', defaultZone: 'bottom' },
  ]
  let dockLayout: DockLayoutState = $state(createDockLayout(dockPanels))
  let dockLayoutLoaded = $state(false)
  let terminalDockSessionId = $state('')
  let terminalDockCwd = $state('')
  let terminalDockLabel = $state('')
  let terminalDockKey = $state('')

  let activeLeftPanel = $derived(dockLayout.active.left as ChatDockPanelID | undefined)
  let activeRightPanel = $derived(dockLayout.active.right as ChatDockPanelID | undefined)
  let activeBottomPanel = $derived(dockLayout.active.bottom as ChatDockPanelID | undefined)
  let activeFullscreenPanel = $derived(dockLayout.active.fullscreen as ChatDockPanelID | undefined)
  let anyToolPanelOpen = $derived(
    panelIsOpen(dockLayout, 'artifacts') ||
    panelIsOpen(dockLayout, 'config') ||
    panelIsOpen(dockLayout, 'context') ||
    panelIsOpen(dockLayout, 'prompt') ||
    panelIsOpen(dockLayout, 'prior') ||
    panelIsOpen(dockLayout, 'contract') ||
    panelIsOpen(dockLayout, 'tasks') ||
    panelIsOpen(dockLayout, 'git') ||
    panelIsOpen(dockLayout, 'skillExtraction') ||
    panelIsOpen(dockLayout, 'cron') ||
    panelIsOpen(dockLayout, 'health'),
  )

  let sidebarRef: SessionSidebar | undefined = $state()
  let actionFeedback = $state('')
  let feedbackTimer: ReturnType<typeof setTimeout> | null = null

  function panelTitle(panelID: ChatDockPanelID): string {
    return dockPanels.find((panel) => panel.id === panelID)?.title ?? panelID
  }

  function panelCloseable(panelID: ChatDockPanelID): boolean {
    return dockPanels.find((panel) => panel.id === panelID)?.closeable !== false
  }

  function isPanelOpen(panelID: ChatDockPanelID): boolean {
    return panelIsOpen(dockLayout, panelID)
  }

  function openPanel(panelID: ChatDockPanelID) {
    dockLayout = openDockPanel(dockLayout, dockPanels, panelID)
  }

  function closePanel(panelID: ChatDockPanelID) {
    dockLayout = closeDockPanel(dockLayout, panelID)
  }

  function dockPanel(panelID: ChatDockPanelID, zone: DockZone) {
    dockLayout = moveDockPanel(dockLayout, dockPanels, panelID, zone)
  }

  function closeToolPanels() {
    for (const panelID of ['artifacts', 'config', 'context', 'prompt', 'prior', 'tasks', 'git', 'skillExtraction', 'cron', 'health', 'terminal'] as ToolDockPanelID[]) {
      dockLayout = closeDockPanel(dockLayout, panelID)
    }
  }

  function togglePanel(panelID: ChatDockPanelID) {
    if (isPanelOpen(panelID)) {
      closePanel(panelID)
    } else {
      openPanel(panelID)
    }
  }

  function dockStyle(): string {
    return [
      `--dock-left-size:${activeLeftPanel ? dockLayout.sizes.left : 0}px`,
      `--dock-right-size:${activeRightPanel ? dockLayout.sizes.right : 0}px`,
      `--dock-bottom-size:${activeBottomPanel ? dockLayout.sizes.bottom : 0}px`,
    ].join(';')
  }

  function openIntegratedTerminalDock(target: { cwd: string; label: string }) {
    if (!selectedSessionId) {
      showFeedback('Select a session first')
      return
    }
    terminalDockSessionId = selectedSessionId
    terminalDockCwd = target.cwd
    terminalDockLabel = target.label
    terminalDockKey = `${selectedSessionId}:${target.cwd}:${Date.now()}`
    openPanel('terminal')
  }

  function startDockResize(zone: DockSizeZone, event: PointerEvent) {
    event.preventDefault()
    const startX = event.clientX
    const startY = event.clientY
    const startSize = dockLayout.sizes[zone]
    const move = (next: PointerEvent) => {
      const delta = zone === 'left'
        ? next.clientX - startX
        : zone === 'right'
          ? startX - next.clientX
          : startY - next.clientY
      dockLayout = resizeDock(dockLayout, zone, startSize + delta)
    }
    const stop = () => {
      window.removeEventListener('pointermove', move)
      window.removeEventListener('pointerup', stop)
      window.removeEventListener('pointercancel', stop)
    }
    window.addEventListener('pointermove', move)
    window.addEventListener('pointerup', stop)
    window.addEventListener('pointercancel', stop)
  }

  function showFeedback(msg: string, ms = 4000) {
    if (feedbackTimer) clearTimeout(feedbackTimer)
    actionFeedback = msg
    feedbackTimer = setTimeout(() => { actionFeedback = '' }, ms)
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
    chatDraft = ''
    chatContextInfo = {}
    closeToolPanels()
    if (isMobileLayout()) {
      closePanel('sessions')
    }
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
    chatDraft = ''
    chatContextInfo = {}
    closeToolPanels()
    if (isMobileLayout()) {
      closePanel('sessions')
    }
    renaming = false
    deleteConfirm = false
    onNavigate(selectedSessionId ? `/console/chat/${encodeURIComponent(selectedSessionId)}` : '/console/chat')
  }

  function handleSessionChange() {
    sidebarRef?.load()
    // Refresh selected session title (may have been auto-titled)
    if (selectedSessionId) loadSelectedSession(selectedSessionId)
    void refreshSessionHealth()
  }

  function handleSessionForked(session: Session) {
    selectedSessionId = session.id
    selectedSession = session
    chatKey++
    chatArtifacts = []
    chatDraft = ''
    chatContextInfo = {}
    closeToolPanels()
    sidebarRef?.load()
    showFeedback(`Forked session: ${session.title || session.id.slice(0, 12)}`)
    onNavigate(`/console/chat/${encodeURIComponent(session.id)}`)
  }

  function handleArtifactsChange(arts: Artifact[]) {
    chatArtifacts = arts
    if (arts.length > 0 && !anyToolPanelOpen) {
      openPanel('artifacts')
    }
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
      const r = await compactSession(selectedSessionId)
      if (r.compacted) {
        const saved = r.tokens_before - r.tokens_after
        const pct = r.tokens_before > 0 ? Math.round((saved / r.tokens_before) * 100) : 0
        showFeedback(`Compacted ${r.compacted_count} messages (${r.original_count} → ${r.final_count}), ${pct}% tokens saved`)
      } else {
        showFeedback(r.reason || 'Nothing to compact')
      }
      sidebarRef?.load()
      chatKey++
      await refreshSessionHealth()
    } catch (e) {
      showFeedback(e instanceof Error ? e.message : 'Compact failed')
    }
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

  type TasksSummary = TaskProgressSummary & {
    plan_goal?: string
  }
  let tasksSummary: TasksSummary = $state(emptyTaskProgressSummary())
  let planStripProgress = $derived(planProgressPercent(tasksSummary))
  let hasPlanStrip = $derived(!!tasksSummary.plan_goal?.trim())
  let sessionHealth: SessionHealthReport = $state(emptySessionHealthReport())
  let sessionHealthLoading = $state(false)
  let sessionHealthRequest = 0
  let sessionHealthInputs: Omit<SessionHealthInput, 'contextInfo' | 'now'> | null = $state(null)
  let healthIssueCount = $derived(sessionHealth.recommendations.length)

  function handleToolComplete(toolName: string) {
    const taskTools = ['tasks']
    const fileTools = ['write_file', 'edit_file', 'exec', 'list_dir', 'read_file']

    if (taskTools.includes(toolName)) {
      tasksPanelRef?.load()
    }
    if (fileTools.includes(toolName) && isPanelOpen('artifacts')) {
      artifactPanelRef?.refresh()
    }
  }

  function handleTasksChanged(summary: TasksSummary) {
    tasksSummary = summary
    void refreshSessionHealth()
  }

  function rebuildSessionHealth(contextInfo = chatContextInfo) {
    if (!sessionHealthInputs) {
      sessionHealth = emptySessionHealthReport()
      return
    }
    sessionHealth = buildSessionHealthReport({
      ...sessionHealthInputs,
      contextInfo,
    })
  }

  async function refreshSessionHealth() {
    const sid = selectedSessionId
    if (!sid) {
      sessionHealthInputs = null
      sessionHealth = emptySessionHealthReport()
      return
    }
    const requestID = ++sessionHealthRequest
    sessionHealthLoading = true
    try {
      const [session, history, taskState, config, toolsResp] = await Promise.all([
        getSession(sid),
        getSessionHistory(sid),
        getSessionTasks(sid),
        getSessionConfig(sid),
        listChatTools(),
      ])
      if (requestID !== sessionHealthRequest || selectedSessionId !== sid) return
      selectedSession = session
      sessionHealthInputs = {
        session,
        messages: history as SessionMessage[],
        tasks: taskState as SessionTasks,
        config,
        tools: toolsResp.tools,
      }
      const counts = summarizeTasks(taskState.tasks)
      tasksSummary = { ...counts, plan_goal: taskState.plan?.goal }
      rebuildSessionHealth()
    } catch {
      if (requestID === sessionHealthRequest) {
        sessionHealthInputs = null
        sessionHealth = emptySessionHealthReport()
      }
    } finally {
      if (requestID === sessionHealthRequest) {
        sessionHealthLoading = false
      }
    }
  }

  async function handleHealthAction(action: SessionHealthAction) {
    switch (action) {
      case 'compact':
        await handleCompact()
        return
      case 'open_tasks':
        openPanel('tasks')
        return
      case 'open_config':
        openPanel('config')
        return
      case 'open_prior':
        openPanel('prior')
        return
      case 'open_skill_extraction':
        openPanel('skillExtraction')
        return
      case 'review_fork_points':
        closePanel('health')
        return
    }
  }

  // Fetch initial task counts when the active session changes so the
  // pulse-bar badge reflects state from prior turns, not just the current
  // chat-stream lifetime.
  $effect(() => {
    const sid = selectedSessionId
    if (!sid) {
      tasksSummary = emptyTaskProgressSummary()
      sessionHealthInputs = null
      sessionHealth = emptySessionHealthReport()
      return
    }
    void refreshSessionHealth()
  })

  async function handleArtifactOpen(path: string) {
    openPanel('artifacts')
    await tick()
    await artifactPanelRef?.openArtifactPath(path)
  }

  async function handleSlashCommand(command: string, _args: string) {
    const args = _args.trim()
    switch (command) {
      case 'clear':
        chatPanelRef?.clearThread()
        showFeedback('Chat view cleared')
        return
      case 'compact':
        if (!selectedSessionId) {
          showFeedback('Select a session first')
          return
        }
        await handleCompact()
        return
      case 'tasks':
        if (!selectedSessionId) {
          showFeedback('Select a session first')
          return
        }
        openPanel('tasks')
        return
      case 'config':
        if (!selectedSessionId) {
          showFeedback('Select a session first')
          return
        }
        openPanel('config')
        return
      case 'context':
        openPanel('context')
        return
      case 'prior':
        openPanel('prior')
        return
      case 'prompt':
        openPanel('prompt')
        return
      case 'files':
        openPanel('artifacts')
        return
      case 'cron':
        if (!selectedSessionId) {
          showFeedback('Select a session first')
          return
        }
        openPanel('cron')
        return
      case 'memory':
        {
          const query = memorySearchQueryFromSlashArgs(args)
          if (query) {
            onNavigate(`/console/memory?tab=search&q=${encodeURIComponent(query)}`)
            return
          }
          onNavigate('/console/memory')
        }
        return
      case 'skill':
        await toggleSessionSkill(args)
        return
      case 'extract-skill':
        if (!selectedSessionId) {
          showFeedback('Select a session first')
          return
        }
        openPanel('skillExtraction')
        return
    }
  }

  function memorySearchQueryFromSlashArgs(args: string): string {
    const trimmed = args.trim()
    if (!trimmed) return ''
    const searchPrefix = trimmed.match(/^search\s+([\s\S]+)$/i)
    return (searchPrefix?.[1] ?? trimmed).trim()
  }

  async function toggleSessionSkill(args: string) {
    if (!selectedSessionId) {
      showFeedback('Select a session first')
      return
    }
    const requested = args.trim()
    if (!requested) {
      openPanel('config')
      showFeedback('Usage: /skill <name>')
      return
    }
    try {
      const [toolsResp, config] = await Promise.all([
        listChatTools(),
        getSessionConfig(selectedSessionId),
      ])
      const skills = toolsResp.skills ?? []
      const match = skills.find((skill) => skill.toLowerCase() === requested.toLowerCase())
      if (!match) {
        showFeedback(`Skill not found: ${requested}`)
        return
      }
      const useCustomSkills = config.skills_custom || Array.isArray(config.skills_enabled)
      const enabledSkills = new Set(useCustomSkills ? (config.skills_enabled ?? []) : skills)
      const wasEnabled = enabledSkills.has(match)
      if (wasEnabled) {
        enabledSkills.delete(match)
      } else {
        enabledSkills.add(match)
      }
      const nextConfig: SessionToolConfig = {
        ...config,
        skills_custom: true,
        skills_enabled: [...enabledSkills],
      }
      await updateSessionConfig(selectedSessionId, nextConfig)
      showFeedback(`Skill ${match} ${wasEnabled ? 'disabled' : 'enabled'}`)
      openPanel('config')
      await refreshSessionHealth()
    } catch (err) {
      showFeedback(err instanceof Error ? err.message : 'Skill toggle failed')
    }
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
      getPulseStatus(),
      getEventsHistory(1),
    ])
    pulse = h.status === 'fulfilled' ? h.value : null
    if (e.status === 'fulfilled') {
      unreadCount = e.value.unread_count ?? 0
    }
  }

  onMount(() => {
    try {
      const stored = window.localStorage.getItem(dockStorageKey)
      if (stored) {
        dockLayout = normalizeDockLayout(JSON.parse(stored), dockPanels)
      }
    } catch {
      dockLayout = createDockLayout(dockPanels)
    } finally {
      dockLayoutLoaded = true
    }
    void loadDashboard()
  })

  $effect(() => {
    if (!dockLayoutLoaded) return
    window.localStorage.setItem(dockStorageKey, JSON.stringify(serializeDockLayout(dockLayout)))
  })
</script>

<div class="chat-page">
  <!-- Mini dashboard pulse -->
  <div class="chat-pulse">
    <div class="chat-pulse-stats">
    <div class="pulse-item">
      <span class="pulse-val" class:warn={!!pulse?.last_err}>
        {pulse?.total_ticks ?? 0}
      </span>
      <span class="pulse-lbl">Pulse ticks</span>
    </div>
    <div class="pulse-sep"></div>
    <div class="pulse-item">
      <span class="pulse-val">{pulse?.last_tick_at ? relativeTime(pulse.last_tick_at) : 'never'}</span>
      <span class="pulse-lbl">Last tick</span>
    </div>
    <div class="pulse-sep"></div>
    <div class="pulse-item">
      <span class="pulse-val">{unreadCount}</span>
      <span class="pulse-lbl">Unread</span>
    </div>
    <div class="pulse-sep"></div>
    </div>
    <div class="pulse-panel-toggles">
      <button type="button" class="pulse-toggle-btn" class:active={isPanelOpen('sessions')} onclick={() => togglePanel('sessions')} title="Session list">Sessions</button>
      <button type="button" class="pulse-toggle-btn" class:active={isPanelOpen('artifacts')} onclick={() => togglePanel('artifacts')} title="Files browser">Files{#if chatArtifacts.length > 0} ({chatArtifacts.length}){/if}</button>
      <button type="button" class="pulse-toggle-btn" class:active={isPanelOpen('context')} onclick={() => togglePanel('context')} title="Context monitor">Context</button>
      <button type="button" class="pulse-toggle-btn" class:active={isPanelOpen('prompt')} onclick={() => togglePanel('prompt')} title="Prompt editor">Prompt</button>
      <button type="button" class="pulse-toggle-btn" class:active={isPanelOpen('prior')} onclick={() => togglePanel('prior')} title="Prior Context preview">Prior</button>
      <button type="button" class="pulse-toggle-btn" class:active={isPanelOpen('contract')} onclick={() => togglePanel('contract')} title="Task contract">Contract</button>
      <button type="button" class="pulse-toggle-btn" class:active={isPanelOpen('tasks')} onclick={() => togglePanel('tasks')} title={tasksSummary.total > 0 ? `${tasksSummary.completed} done · ${tasksSummary.in_progress} in progress · ${tasksSummary.pending} pending` : 'Session tasks'}>Tasks{#if tasksSummary.total > 0} ({tasksSummary.completed}/{tasksSummary.total}){/if}</button>
      <button type="button" class="pulse-toggle-btn" class:active={isPanelOpen('git')} onclick={() => togglePanel('git')} title="Git Inspector">Git</button>
      <button type="button" class="pulse-toggle-btn" class:active={isPanelOpen('skillExtraction')} onclick={() => togglePanel('skillExtraction')} title="Skill Extraction Inbox">Skills</button>
      <button type="button" class="pulse-toggle-btn" class:active={isPanelOpen('cron')} onclick={() => togglePanel('cron')} title="Session cron jobs">Cron</button>
      <button type="button" class="pulse-toggle-btn" class:active={isPanelOpen('health')} onclick={() => togglePanel('health')} title={sessionHealth.summary}>Health{#if healthIssueCount > 0} ({healthIssueCount}){/if}</button>
    </div>
  </div>

  {#snippet renderDockPanel(panelID: ChatDockPanelID, zone: DockZone)}
    <DockPanelFrame
      title={panelTitle(panelID)}
      {zone}
      closeable={panelCloseable(panelID)}
      onDock={(nextZone) => dockPanel(panelID, nextZone)}
      onClose={() => closePanel(panelID)}
    >
      {#if panelID === 'sessions'}
        <SessionSidebar
          bind:this={sidebarRef}
          selectedSessionId={selectedSessionId}
          onSelect={handleSelectSession}
          onNewSession={handleNewSession}
        />
      {:else if panelID === 'artifacts'}
        <ArtifactPanel
          bind:this={artifactPanelRef}
          artifacts={chatArtifacts}
          sessionId={selectedSessionId || ''}
          onClose={() => closePanel(panelID)}
          onOpenIntegratedTerminal={openIntegratedTerminalDock}
        />
      {:else if panelID === 'config' && selectedSessionId}
        <SessionConfigPanel
          sessionId={selectedSessionId ?? ''}
          onClose={() => closePanel(panelID)}
          onChange={() => {
            contextRefreshVersion += 1
            void refreshSessionHealth()
          }}
        />
      {:else if panelID === 'context'}
        <ContextMonitor
          sessionId={selectedSessionId ?? ''}
          contextInfo={chatContextInfo}
          refreshVersion={contextRefreshVersion}
          onClose={() => closePanel(panelID)}
        />
      {:else if panelID === 'prompt'}
        <PromptEditor sessionId={selectedSessionId ?? ''} onClose={() => closePanel(panelID)} />
      {:else if panelID === 'prior'}
        <PriorContextPanel sessionId={selectedSessionId ?? ''} draftQuery={chatDraft} onClose={() => closePanel(panelID)} />
      {:else if panelID === 'contract' && selectedSessionId}
        <ContractPanel sessionId={selectedSessionId} onClose={() => closePanel(panelID)} />
      {:else if panelID === 'tasks' && selectedSessionId}
        <TasksPanel
          bind:this={tasksPanelRef}
          sessionId={selectedSessionId}
          onClose={() => closePanel(panelID)}
          onSendMessage={async (text) => { await chatPanelRef?.sendMessageText(text) }}
        />
      {:else if panelID === 'git' && selectedSessionId}
        <GitInspector sessionId={selectedSessionId} onClose={() => closePanel(panelID)} />
      {:else if panelID === 'skillExtraction' && selectedSessionId}
        <SkillExtractionPanel
          sessionId={selectedSessionId}
          onClose={() => closePanel(panelID)}
          onApproved={(path) => {
            showFeedback(path ? `Saved skill draft: ${path}` : 'Saved skill draft')
            contextRefreshVersion += 1
          }}
        />
      {:else if panelID === 'cron' && selectedSessionId}
        <SessionCronPanel sessionId={selectedSessionId} sessionKind={selectedSession?.kind ?? ''} onClose={() => closePanel(panelID)} />
      {:else if panelID === 'health' && selectedSessionId}
        <SessionHealthPanel
          report={sessionHealth}
          loading={sessionHealthLoading}
          onRefresh={() => { void refreshSessionHealth() }}
          onAction={(action) => { void handleHealthAction(action) }}
        />
      {:else if panelID === 'terminal' && terminalDockSessionId}
        {#key terminalDockKey}
          <IntegratedTerminal
            sessionId={terminalDockSessionId}
            cwd={terminalDockCwd}
            label={terminalDockLabel}
            onClose={() => closePanel(panelID)}
          />
        {/key}
      {:else}
        <div class="dock-empty">Select a session to use this panel.</div>
      {/if}
    </DockPanelFrame>
  {/snippet}

  <div
    class="chat-layout dock-layout"
    style={dockStyle()}
    class:has-left-panel={!!activeLeftPanel}
    class:has-right-panel={!!activeRightPanel}
    class:has-bottom-panel={!!activeBottomPanel}
  >
    {#if activeLeftPanel}
      <aside class="dock-pane dock-left">
        {@render renderDockPanel(activeLeftPanel, 'left')}
      </aside>
      <button type="button" class="dock-resizer dock-resizer-left" aria-label="Resize left dock" onpointerdown={(event) => startDockResize('left', event)}></button>
    {/if}

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
            <button
              type="button"
              class={`session-health-badge health-${sessionHealth.status}`}
              onclick={() => openPanel('health')}
              title={sessionHealth.summary}
            >
              <span>Health</span>
              <strong>{sessionHealth.badgeLabel}</strong>
            </button>
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
            <button class="btn btn-ghost btn-sm" disabled={actionBusy} onclick={() => openPanel('skillExtraction')} title="Extract reusable skills from this session">Extract Skill</button>
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

      {#if actionFeedback}
        <div class="action-feedback">{actionFeedback}</div>
      {/if}

      {#if hasPlanStrip}
        <button
          type="button"
          class="plan-progress-strip"
          class:active={isPanelOpen('tasks')}
          onclick={() => togglePanel('tasks')}
          title="Open session tasks"
        >
          <span class="plan-strip-goal">
            <span class="plan-strip-label">Plan</span>
            <strong>{tasksSummary.plan_goal}</strong>
          </span>
          <span class="plan-strip-progress">
            <span class="plan-strip-bar" aria-label={`${planStripProgress}% complete`}>
              <span class="plan-strip-fill" style={`width: ${planStripProgress}%`}></span>
            </span>
            <span class="plan-strip-count">{tasksSummary.completed}/{tasksSummary.total} tasks</span>
          </span>
        </button>
      {/if}

      {#key chatKey}
        <ChatPanel
          bind:this={chatPanelRef}
          sessionId={selectedSessionId || undefined}
          {initialPrompt}
          onSessionChange={handleSessionChange}
          onArtifactsChange={handleArtifactsChange}
          onContextInfo={(info) => {
            chatContextInfo = info
            rebuildSessionHealth(info)
          }}
          onToolComplete={handleToolComplete}
          onTasksChanged={handleTasksChanged}
          onSlashCommand={handleSlashCommand}
          onDraftChange={(draft) => { chatDraft = draft }}
          onSessionForked={handleSessionForked}
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

    {#if activeRightPanel}
      <aside class="dock-pane dock-right">
        {@render renderDockPanel(activeRightPanel, 'right')}
      </aside>
      <button type="button" class="dock-resizer dock-resizer-right" aria-label="Resize right dock" onpointerdown={(event) => startDockResize('right', event)}></button>
    {/if}

    {#if activeBottomPanel}
      <section class="dock-pane dock-bottom">
        {@render renderDockPanel(activeBottomPanel, 'bottom')}
      </section>
      <button type="button" class="dock-resizer dock-resizer-bottom" aria-label="Resize bottom dock" onpointerdown={(event) => startDockResize('bottom', event)}></button>
    {/if}

    {#if activeFullscreenPanel}
      <section class="dock-pane dock-fullscreen">
        {@render renderDockPanel(activeFullscreenPanel, 'fullscreen')}
      </section>
    {/if}
  </div>
</div>

<style>
  .action-feedback {
    padding: 6px 14px;
    font-size: 0.82rem;
    color: var(--primary);
    background: color-mix(in srgb, var(--primary) 10%, transparent);
    border-bottom: 1px solid color-mix(in srgb, var(--primary) 25%, transparent);
    text-align: center;
  }
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
    background: var(--surface);
    border-bottom: 1px solid var(--border-subtle);
    flex-shrink: 0;
    position: sticky;
    top: 0;
    z-index: 10;
    min-width: 0;
    overflow: hidden;
  }

  .chat-pulse-stats {
    display: contents;
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
    flex-shrink: 0;
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
    color: var(--primary);
    border-color: var(--primary);
    background: rgba(224, 145, 69, 0.08);
  }

  /* Layout */
  .chat-layout {
    flex: 1;
    display: grid;
    grid-template-columns: var(--dock-left-size, 0px) minmax(0, 1fr) var(--dock-right-size, 0px);
    grid-template-rows: minmax(0, 1fr) var(--dock-bottom-size, 0px);
    grid-template-areas:
      "left main right"
      "bottom bottom bottom";
    min-height: 0;
    position: relative;
  }

  .dock-pane {
    background: var(--surface);
    overflow: hidden;
    min-width: 0;
    min-height: 0;
  }

  .dock-left {
    grid-area: left;
    border-right: 1px solid var(--border-subtle);
  }

  .dock-right {
    grid-area: right;
    border-left: 1px solid var(--border-subtle);
  }

  .dock-bottom {
    grid-area: bottom;
    border-top: 1px solid var(--border-subtle);
  }

  .dock-fullscreen {
    position: absolute;
    inset: var(--space-3);
    z-index: 30;
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-lg);
    box-shadow: 0 24px 80px rgba(0, 0, 0, 0.45);
  }

  .dock-resizer {
    position: absolute;
    z-index: 20;
    padding: 0;
    border: 0;
    background: transparent;
  }

  .dock-resizer:hover,
  .dock-resizer:focus-visible {
    background: color-mix(in srgb, var(--primary) 35%, transparent);
    outline: none;
  }

  .dock-resizer-left {
    top: 0;
    bottom: var(--dock-bottom-size, 0px);
    left: calc(var(--dock-left-size, 0px) - 3px);
    width: 6px;
    cursor: col-resize;
  }

  .dock-resizer-right {
    top: 0;
    right: calc(var(--dock-right-size, 0px) - 3px);
    bottom: var(--dock-bottom-size, 0px);
    width: 6px;
    cursor: col-resize;
  }

  .dock-resizer-bottom {
    right: 0;
    bottom: calc(var(--dock-bottom-size, 0px) - 3px);
    left: 0;
    height: 6px;
    cursor: row-resize;
  }

  .dock-empty {
    padding: var(--space-4);
    color: var(--text-tertiary);
    font-size: var(--text-sm);
  }

  .chat-main {
    grid-area: main;
    display: flex;
    flex-direction: column;
    min-height: 0;
    min-width: 0;
    padding: var(--space-4);
    padding-top: 0;
    overflow: hidden;
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
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex: 1;
    min-width: 0;
  }

  .session-title {
    flex: 1;
    min-width: 0;
    font-family: var(--font-display);
    font-size: var(--text-base);
    font-weight: 500;
    color: var(--text-primary);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    margin: 0;
  }

  .session-health-badge {
    display: inline-flex;
    flex-shrink: 0;
    align-items: center;
    gap: var(--space-1);
    max-width: 190px;
    padding: 3px var(--space-2);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface-base);
    color: var(--text-secondary);
    cursor: pointer;
    font-size: var(--text-xs);
    transition:
      background var(--duration-fast) var(--ease-out),
      border-color var(--duration-fast) var(--ease-out);
  }

  .session-health-badge:hover {
    border-color: var(--border-default);
    background: var(--surface-elevated);
  }

  .session-health-badge span {
    color: var(--text-tertiary);
  }

  .session-health-badge strong {
    min-width: 0;
    overflow: hidden;
    color: var(--text-primary);
    font-family: var(--font-display);
    font-weight: 600;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .session-health-badge.health-watch {
    border-color: color-mix(in srgb, var(--warning) 45%, var(--border-subtle));
    color: var(--warning);
  }

  .session-health-badge.health-attention,
  .session-health-badge.health-critical {
    border-color: color-mix(in srgb, var(--error) 50%, var(--border-subtle));
    color: var(--error);
  }

  .new-chat-title {
    color: var(--text-tertiary);
  }

  .session-rename-input {
    width: 100%;
    padding: var(--space-1) var(--space-2);
    font-size: var(--text-base);
    font-family: var(--font-display);
    background: var(--surface-base);
    border: 1px solid var(--primary);
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

  .plan-progress-strip {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    width: calc(100% - var(--space-8));
    min-height: 42px;
    margin: 0 var(--space-4) var(--space-3);
    padding: var(--space-2) var(--space-3);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md);
    background: var(--surface);
    color: var(--text-primary);
    cursor: pointer;
    text-align: left;
    transition:
      background var(--duration-fast) var(--ease-out),
      border-color var(--duration-fast) var(--ease-out);
  }

  .plan-progress-strip:hover,
  .plan-progress-strip.active {
    border-color: var(--primary);
    background: color-mix(in srgb, var(--primary) 8%, var(--surface));
  }

  .plan-strip-goal {
    display: flex;
    min-width: 0;
    align-items: center;
    gap: var(--space-2);
  }

  .plan-strip-label {
    flex-shrink: 0;
    padding: 2px var(--space-2);
    border-radius: var(--radius-sm);
    background: var(--primary-muted);
    color: var(--primary-text);
    font-family: var(--font-display);
    font-size: var(--text-xs);
    font-weight: 600;
  }

  .plan-strip-goal strong {
    min-width: 0;
    overflow: hidden;
    color: var(--text-primary);
    font-family: var(--font-display);
    font-size: var(--text-sm);
    font-weight: 600;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .plan-strip-progress {
    display: flex;
    flex-shrink: 0;
    align-items: center;
    gap: var(--space-2);
    color: var(--text-tertiary);
    font-size: var(--text-xs);
  }

  .plan-strip-bar {
    display: block;
    width: 86px;
    height: 6px;
    overflow: hidden;
    border-radius: 999px;
    background: var(--surface-inset);
  }

  .plan-strip-fill {
    display: block;
    height: 100%;
    border-radius: inherit;
    background: var(--primary);
    transition: width 0.3s var(--ease-out);
  }

  .plan-strip-count {
    min-width: 56px;
    font-family: var(--font-mono);
    text-align: right;
  }

  @media (max-width: 900px) {
    .chat-layout {
      grid-template-columns: 1fr;
      grid-template-rows: minmax(0, 1fr);
      grid-template-areas: "main";
    }
    .dock-resizer {
      display: none;
    }
    /* On mobile, the grid collapses to a single column; render any active
       dock panel as a fullscreen overlay over the chat area so the user can
       still see/use it (otherwise the panel toggles would do nothing
       visible). */
    .dock-left,
    .dock-right,
    .dock-bottom,
    .dock-fullscreen {
      position: absolute;
      inset: var(--space-2);
      z-index: 30;
      border: 1px solid var(--border-strong);
      border-radius: var(--radius-lg);
      box-shadow: 0 24px 80px rgba(0, 0, 0, 0.45);
    }
    .chat-pulse {
      flex-wrap: nowrap;
      gap: 0;
      padding: 0;
      flex-direction: column;
      align-items: stretch;
      overflow: visible;
    }
    .chat-pulse-stats {
      display: flex;
      align-items: center;
      gap: var(--space-3);
      padding: var(--space-1) var(--space-3);
      overflow-x: auto;
    }
    .pulse-sep { display: none; }
    .pulse-panel-toggles {
      overflow-x: auto;
      -webkit-overflow-scrolling: touch;
      padding: var(--space-1) var(--space-3);
      border-top: 1px solid var(--border-subtle);
      flex-shrink: 0;
      scrollbar-width: none;
    }
    .pulse-panel-toggles::-webkit-scrollbar { display: none; }
    .session-actions {
      flex-wrap: wrap;
    }
    .plan-progress-strip {
      align-items: flex-start;
      flex-direction: column;
    }
    .plan-strip-progress {
      width: 100%;
    }
    .plan-strip-bar {
      flex: 1;
      width: auto;
    }
  }
</style>
