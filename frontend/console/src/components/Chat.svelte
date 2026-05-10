<script lang="ts">
  import { onMount, tick } from 'svelte'
  import {
    getCodexUsage,
    getEventsHistory, getPulseStatus,
    getSession, createSession, renameSession, deleteSession, compactSession, getSessionHistory,
    getSessionTasks, listChatTools, getSessionEffectiveConfig, updateSessionLocalConfig,
    getSessionCwd, setSessionCwd,
    type SessionToolConfig,
  } from '../lib/api'
  import { formatCodexStatusLines } from '../lib/codexStatus'
  import { t } from '../i18n'
  import { emptyTaskProgressSummary, planProgressPercent, summarizeTasks, type TaskProgressSummary } from '../lib/tasks'
  import { buildSessionHealthReport, emptySessionHealthReport, type SessionHealthAction, type SessionHealthInput, type SessionHealthReport } from '../lib/sessionHealth'
  import { buildWorkbenchActions, type WorkbenchAction } from '../lib/workbenchActions'
  import type { ChatTierRecommendationRequest, PulseSnapshot, NotificationMessage, Session, SessionCwd, SessionMessage, SessionTasks } from '../lib/types'
  import type { Artifact } from '../lib/artifacts'
  import { loadChatComponent } from '../lib/chatComponents'
  import SessionSidebar from './SessionSidebar.svelte'
  import SessionConfigPanel from './SessionConfigPanel.svelte'
  import ContextMonitor from './ContextMonitor.svelte'
  import PromptEditor from './PromptEditor.svelte'
  import PriorContextPanel from './PriorContextPanel.svelte'
  import TasksPanel from './TasksPanel.svelte'
  import GitInspector from './GitInspector.svelte'
  import SkillExtractionPanel from './SkillExtractionPanel.svelte'
  import SessionCronPanel from './SessionCronPanel.svelte'
  import SessionHealthPanel from './SessionHealthPanel.svelte'
  import DockPanelFrame from './DockPanelFrame.svelte'
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
  import { zenMode } from '../lib/zenMode.svelte'

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

  // Session active-cwd HUD: cwdState mirrors GET /cwd; cwdDropdownOpen
  // controls a tiny popover anchored to the chip; cwdBusy guards
  // concurrent transitions while a PUT is in flight.
  let cwdState: SessionCwd | null = $state(null)
  let cwdDropdownOpen = $state(false)
  let cwdBusy = $state(false)

  // Session action state
  let renaming = $state(false)
  let renameValue = $state('')
  let actionBusy = $state(false)
  let deleteConfirm = $state(false)
  let sessionMenuOpen = $state(false)

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
    command_count?: number
    command_names?: string[]
    memory_count?: number
    memory_tokens?: number
    compaction_trigger_tokens?: number
    compaction_keep_recent_tokens?: number
    compaction_keep_recent_fraction?: number
    compaction_last_mode?: string
    used_tool_names?: string[]
    selected_skill_name?: string
    selected_skill_reason?: string
    selected_command_name?: string
    selected_command_reason?: string
    mentioned_path_count?: number
    mentioned_paths?: string[]
    mentioned_subagent_count?: number
    mentioned_subagents?: string[]
    llm_tier?: string
    tier_recommendation?: ChatTierRecommendationRequest
  } = $state({})
  let contextRefreshVersion = $state(0)
  type ChatDockPanelID = 'sessions' | 'artifacts' | 'config' | 'context' | 'prompt' | 'prior' | 'tasks' | 'git' | 'skillExtraction' | 'cron' | 'health' | 'terminal'
  type ToolDockPanelID = Exclude<ChatDockPanelID, 'sessions'>

  // Below this width the chat layout collapses; dock panels render as fullscreen overlays.
  const MOBILE_LAYOUT_MAX_WIDTH = 900
  function isMobileLayout(): boolean {
    return typeof window !== 'undefined' && window.matchMedia(`(max-width: ${MOBILE_LAYOUT_MAX_WIDTH}px)`).matches
  }
  type DockSizeZone = 'left' | 'right' | 'bottom'
  const dockStorageKey = 'tars.console.chat.dockLayout.v1'
  function buildDockPanels(translations: typeof $t): DockPanelDefinition[] {
    return [
      { id: 'sessions', title: translations.chat.panels.sessions, defaultZone: 'left', closeable: false },
      { id: 'artifacts', title: translations.chat.panels.files, defaultZone: 'right' },
      { id: 'config', title: translations.chat.panels.config, defaultZone: 'right' },
      { id: 'context', title: translations.chat.panels.context, defaultZone: 'right' },
      { id: 'prompt', title: translations.chat.panels.prompt, defaultZone: 'right' },
      { id: 'prior', title: translations.chat.panels.priorFull, defaultZone: 'right' },
      { id: 'tasks', title: translations.chat.panels.tasks, defaultZone: 'right' },
      { id: 'git', title: translations.chat.panels.git, defaultZone: 'right' },
      { id: 'skillExtraction', title: translations.chat.panels.skillsInbox, defaultZone: 'right' },
      { id: 'cron', title: translations.chat.panels.cron, defaultZone: 'right' },
      { id: 'health', title: translations.chat.panels.health, defaultZone: 'right' },
      { id: 'terminal', title: translations.chat.panels.terminal, defaultZone: 'bottom' },
    ]
  }
  const dockPanels: DockPanelDefinition[] = $derived(buildDockPanels($t))
  let dockLayout: DockLayoutState = $state(createDockLayout(buildDockPanels($t)))
  let dockLayoutLoaded = $state(false)
  interface TerminalDockTab {
    id: string
    cwd: string
    label: string
  }
  let terminalDockSessionId = $state('')
  let terminalDockTabs = $state<TerminalDockTab[]>([])
  let terminalDockActiveId = $state<string | null>(null)

  let activeLeftPanel = $derived(dockLayout.active.left as ChatDockPanelID | undefined)
  let activeRightPanel = $derived(dockLayout.active.right as ChatDockPanelID | undefined)
  let activeBottomPanel = $derived(dockLayout.active.bottom as ChatDockPanelID | undefined)
  let activeFullscreenPanel = $derived(dockLayout.active.fullscreen as ChatDockPanelID | undefined)

  // Which zone the terminal panel currently lives in. Tracked separately
  // so the terminal can render from a single, stable parent — moving it
  // between zones via CSS instead of unmounting/remounting the dock-pane
  // wrapper, which would tear down xterm + the WebSocket every time the
  // user dragged the panel (#667).
  type TerminalZone = 'left' | 'right' | 'bottom' | 'fullscreen' | null
  let terminalActiveZone: TerminalZone = $derived(
    activeFullscreenPanel === 'terminal'
      ? 'fullscreen'
      : activeBottomPanel === 'terminal'
        ? 'bottom'
        : activeLeftPanel === 'terminal'
          ? 'left'
          : activeRightPanel === 'terminal'
            ? 'right'
            : null,
  )
  let anyToolPanelOpen = $derived(
    panelIsOpen(dockLayout, 'artifacts') ||
    panelIsOpen(dockLayout, 'config') ||
    panelIsOpen(dockLayout, 'context') ||
    panelIsOpen(dockLayout, 'prompt') ||
    panelIsOpen(dockLayout, 'prior') ||
    panelIsOpen(dockLayout, 'tasks') ||
    panelIsOpen(dockLayout, 'git') ||
    panelIsOpen(dockLayout, 'skillExtraction') ||
    panelIsOpen(dockLayout, 'cron') ||
    panelIsOpen(dockLayout, 'health'),
  )

  let sidebarRef: SessionSidebar | undefined = $state()
  type ChatPanelHandle = {
    sendMessageText: (text: string) => Promise<void>
    clearThread: () => void
    exportAsMarkdown: () => string
  }
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
      showFeedback($t.chat.panels.dockEmpty)
      return
    }
    if (terminalDockSessionId !== selectedSessionId) {
      terminalDockSessionId = selectedSessionId
      terminalDockTabs = []
      terminalDockActiveId = null
    }
    const existing = terminalDockTabs.find((t) => t.cwd === target.cwd && t.label === target.label)
    if (existing) {
      terminalDockActiveId = existing.id
    } else {
      const id = `${target.cwd}:${target.label}:${Date.now()}:${Math.random().toString(36).slice(2, 6)}`
      terminalDockTabs = [...terminalDockTabs, { id, cwd: target.cwd, label: target.label }]
      terminalDockActiveId = id
    }
    openPanel('terminal')
  }

  function closeTerminalTab(id: string) {
    const idx = terminalDockTabs.findIndex((t) => t.id === id)
    if (idx === -1) return
    const next = terminalDockTabs.filter((t) => t.id !== id)
    terminalDockTabs = next
    if (terminalDockActiveId === id) {
      terminalDockActiveId = next.length === 0
        ? null
        : (next[Math.min(idx, next.length - 1)]?.id ?? null)
    }
    if (next.length === 0) {
      closePanel('terminal')
    }
  }

  function addTerminalTab(cwd: string, label: string) {
    if (!selectedSessionId) {
      showFeedback($t.chat.panels.dockEmpty)
      return
    }
    if (terminalDockSessionId !== selectedSessionId) {
      terminalDockSessionId = selectedSessionId
      terminalDockTabs = []
    }
    const id = `${cwd}:${label}:${Date.now()}:${Math.random().toString(36).slice(2, 6)}`
    terminalDockTabs = [...terminalDockTabs, { id, cwd, label }]
    terminalDockActiveId = id
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
    if (!value?.trim()) return $t.chat.statusStrip.neverTick
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return value
    if (date.getFullYear() <= 1) return $t.chat.statusStrip.neverTick
    const seconds = Math.floor((Date.now() - date.getTime()) / 1000)
    const labels = $t.sessions.relativeTime
    if (seconds < 60) return labels.secondsAgo(seconds)
    if (seconds < 3600) return labels.minutesAgo(Math.floor(seconds / 60))
    if (seconds < 86400) return labels.hoursAgo(Math.floor(seconds / 3600))
    return labels.daysAgo(Math.floor(seconds / 86400))
  }

  async function loadSelectedSession(id: string) {
    try {
      selectedSession = await getSession(id)
    } catch { /* ignore */ }
    void refreshCwdState(id)
  }

  async function refreshCwdState(id: string | null) {
    if (!id) {
      cwdState = null
      return
    }
    try {
      cwdState = await getSessionCwd(id)
    } catch {
      cwdState = null
    }
  }

  function shortCwdLabel(path: string): string {
    if (!path) return ''
    const home = '/Users/'
    if (path.startsWith(home)) {
      const trimmed = path.slice(home.length)
      const slash = trimmed.indexOf('/')
      const tail = slash >= 0 ? trimmed.slice(slash) : ''
      return `~${tail}`
    }
    return path
  }

  async function transitionCwd(target: string) {
    if (!selectedSessionId) {
      showFeedback('Select a session first')
      return
    }
    if (cwdBusy) return
    cwdBusy = true
    try {
      await setSessionCwd(selectedSessionId, target)
      cwdDropdownOpen = false
      await refreshCwdState(selectedSessionId)
      showFeedback(`cwd → ${shortCwdLabel(target)}`)
    } catch (err) {
      showFeedback(`cwd transition failed: ${err instanceof Error ? err.message : String(err)}`)
    } finally {
      cwdBusy = false
    }
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

  function closeSessionMenu() {
    sessionMenuOpen = false
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
        showFeedback($t.chat.feedback.compacted(r.compacted_count, r.original_count, r.final_count, pct))
      } else {
        showFeedback(r.reason || $t.chat.feedback.nothingToCompact)
      }
      sidebarRef?.load()
      chatKey++
      await refreshSessionHealth()
    } catch (e) {
      showFeedback(e instanceof Error ? e.message : $t.chat.feedback.compactFailed)
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

  let chatPanelRef: ChatPanelHandle | undefined = $state()
  let tasksPanelRef: { load: () => void; openEvidence: () => Promise<void> } | undefined = $state()
  let artifactPanelRef: { refresh: () => void; openArtifactPath: (path: string) => Promise<void> } | undefined = $state()

  type TasksSummary = TaskProgressSummary & {
    plan_goal?: string
  }
  let tasksSummary: TasksSummary = $state(emptyTaskProgressSummary())
  let planStripProgress = $derived(planProgressPercent(tasksSummary))
  let hasPlanStrip = $derived(!!tasksSummary.plan_goal?.trim())
  let workbenchActions = $derived(buildWorkbenchActions({
    sessionId: selectedSessionId,
    hasPlan: hasPlanStrip,
    activeTaskTitle: tasksSummary.active_task_title,
  }))
  let sessionHealth: SessionHealthReport = $state(emptySessionHealthReport())
  let sessionHealthLoading = $state(false)
  let sessionHealthRequest = 0
  let sessionHealthInputs: Omit<SessionHealthInput, 'contextInfo' | 'now'> | null = $state(null)
  let healthIssueCount = $derived(sessionHealth.recommendations.length)

  function handleToolComplete(toolName: string) {
    const taskTools = ['tasks']
    const fileTools = ['write_file', 'edit_file', 'exec', 'list_dir', 'read_file', 'apply_patch']

    if (taskTools.includes(toolName)) {
      tasksPanelRef?.load()
    }
    if (fileTools.includes(toolName)) {
      artifactPanelRef?.refresh()
    }
  }

  function handleTasksChanged(summary: TasksSummary) {
    tasksSummary = summary
    void refreshSessionHealth()
  }

  async function handleWorkbenchAction(action: WorkbenchAction) {
    if (action.id === 'agentruntime') {
      onNavigate('/console/agentruntime')
      return
    }
    if (action.id === 'git') {
      openPanel('git')
      return
    }
    if (action.id === 'evidence') {
      openPanel('tasks')
      await tick()
      await tasksPanelRef?.openEvidence()
      return
    }
    openPanel('tasks')
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
        getSessionEffectiveConfig(sid),
        listChatTools(sid),
      ])
      if (requestID !== sessionHealthRequest || selectedSessionId !== sid) return
      selectedSession = session
      sessionHealthInputs = {
        session,
        messages: history as SessionMessage[],
        tasks: taskState as SessionTasks,
        config: config.effective.tool_config,
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
      case 'status': {
        try {
          const res = await getCodexUsage()
          const lines = formatCodexStatusLines(res.tiers ?? [])
          showFeedback(lines.join('\n'), 10_000)
        } catch (err) {
          showFeedback(`status: ${err instanceof Error ? err.message : 'failed to load codex quota'}`)
        }
        return
      }
      case 'cwd':
        if (!selectedSessionId) {
          showFeedback('Select a session first')
          return
        }
        if (args === '' || args.toLowerCase() === 'list') {
          await refreshCwdState(selectedSessionId)
          if (!cwdState) {
            showFeedback('cwd: no eligible directories')
            return
          }
          const eligible = cwdState.eligible.length
            ? cwdState.eligible.map((p, i) => `  ${i + 1}. ${shortCwdLabel(p)}${p === cwdState!.current ? ' (active)' : ''}`).join('\n')
            : '  (none)'
          showFeedback(`cwd active: ${shortCwdLabel(cwdState.current)}\n${eligible}`)
          return
        }
        await transitionCwd(args)
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
        listChatTools(selectedSessionId),
        getSessionEffectiveConfig(selectedSessionId),
      ])
      const skills = toolsResp.skills ?? []
      const match = skills.find((skill) => skill.toLowerCase() === requested.toLowerCase())
      if (!match) {
        showFeedback(`Skill not found: ${requested}`)
        return
      }
      const effectiveToolConfig = config.effective.tool_config
      const useCustomSkills = effectiveToolConfig.skills_custom || Array.isArray(effectiveToolConfig.skills_enabled)
      const enabledSkills = new Set(useCustomSkills ? (effectiveToolConfig.skills_enabled ?? []) : skills)
      const wasEnabled = enabledSkills.has(match)
      if (wasEnabled) {
        enabledSkills.delete(match)
      } else {
        enabledSkills.add(match)
      }
      const nextConfig: SessionToolConfig = {
        ...effectiveToolConfig,
        skills_custom: true,
        skills_enabled: [...enabledSkills],
      }
      await updateSessionLocalConfig(selectedSessionId, nextConfig)
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
      <span class="pulse-lbl">{$t.chat.statusStrip.pulseTicks}</span>
    </div>
    <div class="pulse-sep"></div>
    <div class="pulse-item">
      <span class="pulse-val">{pulse?.last_tick_at ? relativeTime(pulse.last_tick_at) : $t.chat.statusStrip.neverTick}</span>
      <span class="pulse-lbl">{$t.chat.statusStrip.lastTick}</span>
    </div>
    <div class="pulse-sep"></div>
    <div class="pulse-item">
      <span class="pulse-val">{unreadCount}</span>
      <span class="pulse-lbl">{$t.chat.statusStrip.unread}</span>
    </div>
    <div class="pulse-sep"></div>
    </div>
    <div class="pulse-panel-toggles">
      <button type="button" class="pulse-toggle-btn" class:active={isPanelOpen('sessions')} onclick={() => togglePanel('sessions')} title={$t.chat.panels.sessionsTooltip}>{$t.chat.panels.sessions}</button>
      <button type="button" class="pulse-toggle-btn" class:active={isPanelOpen('artifacts')} onclick={() => togglePanel('artifacts')} title={$t.chat.panels.filesTooltip}>{$t.chat.panels.files}{#if chatArtifacts.length > 0}{$t.chat.panels.filesCount(chatArtifacts.length)}{/if}</button>
      <button type="button" class="pulse-toggle-btn" class:active={isPanelOpen('config')} onclick={() => togglePanel('config')} title={$t.chat.panels.configTooltip}>{$t.chat.panels.config}</button>
      <button type="button" class="pulse-toggle-btn" class:active={isPanelOpen('context')} onclick={() => togglePanel('context')} title={$t.chat.panels.contextTooltip}>{$t.chat.panels.context}</button>
      <button type="button" class="pulse-toggle-btn" class:active={isPanelOpen('prompt')} onclick={() => togglePanel('prompt')} title={$t.chat.panels.promptTooltip}>{$t.chat.panels.prompt}</button>
      <button type="button" class="pulse-toggle-btn" class:active={isPanelOpen('prior')} onclick={() => togglePanel('prior')} title={$t.chat.panels.priorTooltip}>{$t.chat.panels.prior}</button>
      <button type="button" class="pulse-toggle-btn" class:active={isPanelOpen('tasks')} onclick={() => togglePanel('tasks')} title={tasksSummary.total > 0 ? $t.chat.panels.tasksProgressTooltip(tasksSummary.completed, tasksSummary.in_progress, tasksSummary.pending) : $t.chat.panels.tasksTooltip}>{$t.chat.panels.tasks}{#if tasksSummary.total > 0}{$t.chat.panels.tasksCount(tasksSummary.completed, tasksSummary.total)}{/if}</button>
      <button type="button" class="pulse-toggle-btn" class:active={isPanelOpen('git')} onclick={() => togglePanel('git')} title={$t.chat.panels.gitTooltip}>{$t.chat.panels.git}</button>
      <button type="button" class="pulse-toggle-btn" class:active={isPanelOpen('skillExtraction')} onclick={() => togglePanel('skillExtraction')} title={$t.chat.panels.skillsTooltip}>{$t.chat.panels.skills}</button>
      <button type="button" class="pulse-toggle-btn" class:active={isPanelOpen('cron')} onclick={() => togglePanel('cron')} title={$t.chat.panels.cronTooltip}>{$t.chat.panels.cron}</button>
      <button type="button" class="pulse-toggle-btn" class:active={isPanelOpen('health')} onclick={() => togglePanel('health')} title={sessionHealth.summary}>{$t.chat.panels.health}{#if healthIssueCount > 0}{$t.chat.panels.healthCount(healthIssueCount)}{/if}</button>
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
        {#await loadChatComponent('artifact-panel')}
          <div class="dock-empty">Loading...</div>
        {:then module}
          {@const ArtifactPanelRoute = module.default}
          <ArtifactPanelRoute
            bind:this={artifactPanelRef}
            artifacts={chatArtifacts}
            sessionId={selectedSessionId || ''}
            onClose={() => closePanel(panelID)}
            onOpenIntegratedTerminal={openIntegratedTerminalDock}
          />
        {:catch}
          <div class="dock-empty">{$t.chat.panels.dockEmpty}</div>
        {/await}
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
            showFeedback(path ? $t.chat.feedback.savedSkillDraft(path) : $t.chat.feedback.savedSkillDraftPlain)
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
      {:else}
        <div class="dock-empty">{$t.chat.panels.dockEmpty}</div>
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
      {#if activeLeftPanel !== 'terminal'}
        <aside class="dock-pane dock-left">
          {@render renderDockPanel(activeLeftPanel, 'left')}
        </aside>
      {/if}
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
              <span>{$t.chat.session.healthBadge}</span>
              <strong>{sessionHealth.badgeLabel}</strong>
            </button>
            {#if cwdState}
              <div class="cwd-hud">
                <button
                  type="button"
                  class="cwd-chip"
                  title={cwdState.current}
                  disabled={cwdBusy}
                  onclick={() => { cwdDropdownOpen = !cwdDropdownOpen }}
                >
                  <span class="cwd-chip-label">cwd</span>
                  <strong>{shortCwdLabel(cwdState.current)}</strong>
                </button>
                {#if cwdDropdownOpen}
                  <div class="cwd-dropdown" role="menu">
                    {#each cwdState.eligible as path (path)}
                      <button
                        type="button"
                        class="cwd-dropdown-item"
                        class:active={path === cwdState.current}
                        disabled={cwdBusy}
                        title={path}
                        onclick={() => transitionCwd(path)}
                      >
                        {shortCwdLabel(path)}
                        {#if path === cwdState.current}<span class="cwd-active-marker">●</span>{/if}
                      </button>
                    {/each}
                  </div>
                {/if}
              </div>
            {/if}
          </div>
          <div class="session-actions">
            <button
              type="button"
              class="btn btn-ghost btn-sm zen-toggle"
              class:active={zenMode.active}
              aria-pressed={zenMode.active}
              onclick={() => zenMode.toggle()}
              title={zenMode.active ? $t.chat.session.actions.zenExitTooltip : $t.chat.session.actions.zenEnterTooltip}
            >
              {zenMode.active ? $t.chat.session.actions.zenExit : $t.chat.session.actions.zenEnter}
            </button>
            {#if !zenMode.active}
              <div class="session-menu">
                <button
                  type="button"
                  class="btn btn-ghost btn-sm session-menu-trigger"
                  aria-haspopup="menu"
                  aria-expanded={sessionMenuOpen}
                  onclick={() => { sessionMenuOpen = !sessionMenuOpen }}
                >
                  {$t.sessions.actions.more}
                </button>
                {#if sessionMenuOpen}
                  <div class="session-menu-popover" role="menu">
                    {#if !isMainSession()}
                      <button type="button" role="menuitem" disabled={actionBusy} onclick={() => { closeSessionMenu(); startRename() }}>{$t.chat.session.actions.rename}</button>
                      <button type="button" role="menuitem" disabled={actionBusy} onclick={() => { closeSessionMenu(); void handleAutoTitle() }} title={$t.chat.session.actions.aiTitleTooltip}>{$t.chat.session.actions.aiTitle}</button>
                    {/if}
                    <button type="button" role="menuitem" disabled={actionBusy} onclick={() => { closeSessionMenu(); void handleCompact() }} title={$t.chat.session.actions.compactTooltip}>{$t.chat.session.actions.compact}</button>
                    <button type="button" role="menuitem" onclick={() => { closeSessionMenu(); void handleCopyChat() }} title={$t.chat.session.actions.copyAllTooltip}>{$t.chat.session.actions.copyAll}</button>
                    <button type="button" role="menuitem" onclick={() => { closeSessionMenu(); handleDownloadChat() }} title={$t.chat.session.actions.downloadTooltip}>{$t.chat.session.actions.download}</button>
                    <button type="button" role="menuitem" disabled={actionBusy} onclick={() => { closeSessionMenu(); openPanel('skillExtraction') }} title={$t.chat.session.actions.extractSkillTooltip}>{$t.chat.session.actions.extractSkill}</button>
                    {#if !isMainSession()}
                      <span class="session-menu-divider"></span>
                      <button type="button" role="menuitem" class="danger" disabled={actionBusy} onclick={() => { closeSessionMenu(); void handleDelete() }}>
                        {deleteConfirm ? $t.chat.session.actions.confirmDelete : $t.chat.session.actions.delete}
                      </button>
                    {/if}
                  </div>
                {/if}
              </div>
            {/if}
          </div>
        </div>
      {:else}
        <div class="session-header">
          <h3 class="session-title new-chat-title">{$t.chat.session.newChat}</h3>
          <div class="session-actions">
            <button
              type="button"
              class="btn btn-ghost btn-sm zen-toggle"
              class:active={zenMode.active}
              aria-pressed={zenMode.active}
              onclick={() => zenMode.toggle()}
              title={zenMode.active ? $t.chat.session.actions.zenExitTooltip : $t.chat.session.actions.zenEnterTooltip}
            >
              {zenMode.active ? $t.chat.session.actions.zenExit : $t.chat.session.actions.zenEnter}
            </button>
          </div>
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
          title={$t.chat.planStrip.openTitle}
        >
          <span class="plan-strip-goal">
            <span class="plan-strip-label">{$t.chat.planStrip.label}</span>
            <strong>{tasksSummary.plan_goal}</strong>
            {#if tasksSummary.active_task_title}
              <span class="plan-strip-active" title={$t.chat.planStrip.activeTaskTooltip(tasksSummary.active_task_title)}>
                <span class="plan-strip-active-dot" aria-hidden="true"></span>
                {tasksSummary.active_task_title}
              </span>
            {/if}
          </span>
          <span class="plan-strip-progress">
            <span class="plan-strip-bar" aria-label={$t.chat.planStrip.progressAria(planStripProgress)}>
              <span class="plan-strip-fill" style={`width: ${planStripProgress}%`}></span>
            </span>
            <span class="plan-strip-count">{$t.chat.planStrip.tasksSuffix(tasksSummary.completed, tasksSummary.total)}</span>
          </span>
        </button>
      {/if}

      {#if workbenchActions.length > 0}
        <div class="workbench-action-strip" aria-label="Workbench actions">
          {#each workbenchActions as action}
            <button
              type="button"
              class="workbench-action"
              onclick={() => { void handleWorkbenchAction(action) }}
              title={action.title}
            >
              {action.label}
            </button>
          {/each}
        </div>
      {/if}

      {#key chatKey}
        {#await loadChatComponent('chat-panel')}
          <div class="chat-panel-loading">Loading...</div>
        {:then module}
          {@const ChatPanelRoute = module.default}
          <ChatPanelRoute
            bind:this={chatPanelRef}
            sessionId={selectedSessionId || undefined}
            {initialPrompt}
            onSessionChange={handleSessionChange}
            onArtifactsChange={handleArtifactsChange}
            onContextInfo={(info: typeof chatContextInfo) => {
              chatContextInfo = info
              rebuildSessionHealth(info)
            }}
            onToolComplete={handleToolComplete}
            onTasksChanged={handleTasksChanged}
            onSlashCommand={handleSlashCommand}
            onDraftChange={(draft: string) => { chatDraft = draft }}
            onSessionForked={handleSessionForked}
            onSessionReady={(id: string) => {
              if (!selectedSessionId) {
                selectedSessionId = id
                void loadSelectedSession(id)
                sidebarRef?.load()
              }
            }}
            onArtifactOpen={handleArtifactOpen}
          />
        {:catch}
          <div class="chat-panel-loading">Could not load chat panel.</div>
        {/await}
      {/key}
    </main>

    {#if activeRightPanel}
      {#if activeRightPanel !== 'terminal'}
        <aside class="dock-pane dock-right">
          {@render renderDockPanel(activeRightPanel, 'right')}
        </aside>
      {/if}
      <button type="button" class="dock-resizer dock-resizer-right" aria-label="Resize right dock" onpointerdown={(event) => startDockResize('right', event)}></button>
    {/if}

    {#if activeBottomPanel}
      {#if activeBottomPanel !== 'terminal'}
        <section class="dock-pane dock-bottom">
          {@render renderDockPanel(activeBottomPanel, 'bottom')}
        </section>
      {/if}
      <button type="button" class="dock-resizer dock-resizer-bottom" aria-label="Resize bottom dock" onpointerdown={(event) => startDockResize('bottom', event)}></button>
    {/if}

    {#if activeFullscreenPanel && activeFullscreenPanel !== 'terminal'}
      <section class="dock-pane dock-fullscreen">
        {@render renderDockPanel(activeFullscreenPanel, 'fullscreen')}
      </section>
    {/if}

    <!-- Terminal panel rendered from a single, stable parent so its xterm
         instance + WebSocket survive zone changes. data-zone selects the
         grid area / fullscreen positioning via CSS (#667). -->
    {#if terminalActiveZone && terminalDockSessionId && terminalDockTabs.length > 0}
      <section class="dock-pane dock-terminal" data-zone={terminalActiveZone}>
        <DockPanelFrame
          title={panelTitle('terminal')}
          zone={terminalActiveZone}
          closeable={panelCloseable('terminal')}
          onDock={(nextZone) => dockPanel('terminal', nextZone)}
          onClose={() => closePanel('terminal')}
        >
          {#await loadChatComponent('terminal-tabs')}
            <div class="dock-empty">Loading...</div>
          {:then module}
            {@const TerminalTabsRoute = module.default}
            <TerminalTabsRoute
              sessionId={terminalDockSessionId}
              tabs={terminalDockTabs}
              activeId={terminalDockActiveId}
              onActivate={(id: string) => { terminalDockActiveId = id }}
              onCloseTab={closeTerminalTab}
              onAddTab={addTerminalTab}
            />
          {:catch}
            <div class="dock-empty">{$t.chat.panels.dockEmpty}</div>
          {/await}
        </DockPanelFrame>
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
  .chat-panel-loading {
    flex: 1;
    min-height: 260px;
    display: grid;
    place-items: center;
    color: var(--text-secondary);
    font-family: var(--font-display);
  }
  .chat-page {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-height: 0;
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

  /* Terminal pane is rendered once and re-positioned via data-zone so the
     xterm instance + PTY survive zone moves. Mirrors the styling of the
     regular dock-left/right/bottom/fullscreen panes per zone. */
  .dock-terminal[data-zone='left'] {
    grid-area: left;
    border-right: 1px solid var(--border-subtle);
  }

  .dock-terminal[data-zone='right'] {
    grid-area: right;
    border-left: 1px solid var(--border-subtle);
  }

  .dock-terminal[data-zone='bottom'] {
    grid-area: bottom;
    border-top: 1px solid var(--border-subtle);
  }

  .dock-terminal[data-zone='fullscreen'] {
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
    padding: var(--space-3) var(--space-4) var(--space-2);
    flex-shrink: 0;
    min-height: 44px;
    border-bottom: 1px solid color-mix(in srgb, var(--border-subtle) 72%, transparent);
  }

  .session-title-row {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex: 1;
    min-width: 0;
  }

  .session-title {
    flex: 0 1 auto;
    min-width: 80px;
    max-width: min(36vw, 420px);
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

  /* Active-cwd HUD: chip mirrors the health badge dimensions so the
     header row stays balanced; dropdown is absolutely positioned so it
     doesn't shift the rest of the row when opened. */
  .cwd-hud {
    position: relative;
    display: inline-flex;
    flex-shrink: 0;
  }

  .cwd-chip {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    max-width: 240px;
    padding: 3px var(--space-2);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    background: var(--surface-base);
    color: var(--text-secondary);
    cursor: pointer;
    font-size: var(--text-xs);
    font-family: var(--font-mono);
    transition:
      background var(--duration-fast) var(--ease-out),
      border-color var(--duration-fast) var(--ease-out);
  }

  .cwd-chip:hover:not(:disabled) {
    border-color: var(--primary);
    background: var(--surface-elevated);
  }

  .cwd-chip:disabled {
    cursor: progress;
    opacity: 0.7;
  }

  .cwd-chip-label {
    color: var(--text-tertiary);
    font-family: var(--font-display);
  }

  .cwd-chip strong {
    min-width: 0;
    overflow: hidden;
    color: var(--primary);
    font-weight: 600;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .cwd-dropdown {
    position: absolute;
    top: calc(100% + 4px);
    right: 0;
    z-index: 50;
    display: flex;
    flex-direction: column;
    min-width: 220px;
    max-width: 360px;
    padding: var(--space-1);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    background: var(--surface-elevated);
    box-shadow: var(--shadow-md, 0 4px 12px rgba(0, 0, 0, 0.25));
  }

  .cwd-dropdown-item {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-1) var(--space-2);
    border: none;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-primary);
    cursor: pointer;
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    text-align: left;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .cwd-dropdown-item:hover:not(:disabled) {
    background: var(--surface-base);
  }

  .cwd-dropdown-item.active {
    color: var(--primary);
  }

  .cwd-active-marker {
    margin-left: auto;
    color: var(--primary);
    font-size: var(--text-xs);
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

  .session-menu {
    position: relative;
    display: inline-flex;
  }

  .session-menu-trigger {
    min-width: 64px;
  }

  .session-menu-popover {
    position: absolute;
    top: calc(100% + 6px);
    right: 0;
    z-index: 55;
    display: flex;
    flex-direction: column;
    min-width: 190px;
    padding: var(--space-1);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md);
    background: var(--surface-elevated);
    box-shadow: var(--shadow-md, 0 12px 28px rgba(0, 0, 0, 0.28));
  }

  .session-menu-popover button {
    display: flex;
    align-items: center;
    width: 100%;
    min-height: 30px;
    padding: var(--space-1) var(--space-2);
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-secondary);
    cursor: pointer;
    font-family: var(--font-display);
    font-size: var(--text-xs);
    text-align: left;
  }

  .session-menu-popover button:hover:not(:disabled) {
    background: var(--surface-base);
    color: var(--text-primary);
  }

  .session-menu-popover button:disabled {
    cursor: not-allowed;
    opacity: 0.55;
  }

  .session-menu-popover button.danger {
    color: var(--error);
  }

  .session-menu-divider {
    height: 1px;
    margin: var(--space-1) 0;
    background: var(--border-subtle);
  }

  .zen-toggle.active {
    color: var(--primary);
    border-color: var(--primary);
  }

  .plan-progress-strip {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    width: calc(100% - var(--space-8));
    min-height: 36px;
    margin: var(--space-2) var(--space-4) var(--space-2);
    padding: var(--space-1) var(--space-3);
    border: 1px solid color-mix(in srgb, var(--border-subtle) 78%, transparent);
    border-radius: var(--radius-md);
    background: color-mix(in srgb, var(--surface) 70%, transparent);
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

  .plan-strip-active {
    display: inline-flex;
    flex-shrink: 1;
    align-items: center;
    gap: var(--space-1);
    min-width: 0;
    overflow: hidden;
    color: var(--text-secondary);
    font-size: var(--text-xs);
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .plan-strip-active-dot {
    display: inline-block;
    flex-shrink: 0;
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--accent);
    animation: plan-strip-active-pulse 1.4s ease-in-out infinite;
  }

  @keyframes plan-strip-active-pulse {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.4; }
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

  .workbench-action-strip {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-1);
    width: calc(100% - var(--space-8));
    margin: 0 var(--space-4) var(--space-2);
  }

  .workbench-action {
    min-height: 26px;
    padding: 0 var(--space-2);
    border: 1px solid color-mix(in srgb, var(--border-subtle) 76%, transparent);
    border-radius: 999px;
    background: transparent;
    color: var(--text-tertiary);
    cursor: pointer;
    font-family: var(--font-display);
    font-size: var(--text-xs);
    font-weight: 600;
    transition:
      background var(--duration-fast) var(--ease-out),
      border-color var(--duration-fast) var(--ease-out),
      color var(--duration-fast) var(--ease-out);
  }

  .workbench-action:hover {
    border-color: var(--primary);
    background: color-mix(in srgb, var(--primary) 7%, var(--surface-raised));
    color: var(--text-primary);
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
    .dock-fullscreen,
    .dock-terminal {
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
      flex-wrap: wrap;
      gap: 4px;
      padding: var(--space-1) var(--space-3) var(--space-2);
      border-top: 1px solid var(--border-subtle);
      flex-shrink: 0;
    }
    .session-header {
      flex-wrap: wrap;
      align-items: flex-start;
    }
    .session-title-row {
      flex-basis: 1px;
      min-width: 0;
    }
    .session-title {
      max-width: none;
    }
    .session-actions {
      flex-wrap: nowrap;
      justify-content: flex-end;
    }
    .session-menu-popover {
      right: 0;
      max-width: calc(100vw - var(--space-6));
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
