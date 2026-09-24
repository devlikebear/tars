// Shared state for the chat workbench (#968).
//
// One instance (`chatSession`, see ./chatSession.ts) is shared by Chat,
// ChatPanel, and SessionSidebar so they stop relaying state to each other
// through callback props.
//
// The route is the source of truth for which session is active. Chat.svelte
// forwards its `sessionId` prop into `setActive`; components that switch
// sessions navigate, and the route change comes back here. The one exception
// is `adoptSession`: ChatPanel creates a session lazily on the first send and
// reports its id mid-stream, which must not reset state or remount the panel.
//
// This module imports types only. The API is injected so the store can be
// compiled and exercised under plain Node in tests.

import type {
  compactSession,
  getUsageSummary,
  listAgentRuntimeSubagents,
  getSession,
  getSessionCwd,
  getSessionEffectiveConfig,
  getSessionGoal,
  getSessionHistory,
  getSessionTasks,
  listChatTools,
  listSessions,
  renameSession,
  setSessionCwd,
} from '../api'
import type { Artifact } from '../artifacts'
import type { ChatCommandsTranslations } from '../../i18n/sections/chatCommands'
import type { SessionHealthInput, SessionHealthReport } from '../sessionHealth'
import type { TaskProgressSummary } from '../tasks'
import type { AgentRuntimeTierOption, ChatContextInfo, ChatTier, Session, SessionCwd, SessionGoal, SessionMessage, SessionTasks } from '../types'

export type ChatSessionApi = {
  listSessions: typeof listSessions
  getSession: typeof getSession
  getSessionHistory: typeof getSessionHistory
  getSessionTasks: typeof getSessionTasks
  getSessionEffectiveConfig: typeof getSessionEffectiveConfig
  listChatTools: typeof listChatTools
  getSessionCwd: typeof getSessionCwd
  setSessionCwd: typeof setSessionCwd
  getSessionGoal: typeof getSessionGoal
  renameSession: typeof renameSession
  compactSession: typeof compactSession
  getUsageSummary: typeof getUsageSummary
  listAgentRuntimeSubagents: typeof listAgentRuntimeSubagents
}

export type SessionUsage = {
  costUSD: number
  calls: number
  inputTokens: number
  outputTokens: number
}

export type GoalEventText = ChatCommandsTranslations['goalEvents']

export type ChatSessionHealth = {
  emptyReport: () => SessionHealthReport
  buildReport: (input: SessionHealthInput) => SessionHealthReport
  emptyTasks: () => TaskProgressSummary
  summarizeTasks: (tasks: SessionTasks['tasks']) => TaskProgressSummary
  // The current locale's goal event lines, read when an event arrives so a
  // locale switch applies.
  goalEventText: () => GoalEventText
}

export type ChatTasksSummary = TaskProgressSummary & { plan_goal?: string }

export type GoalEvent = {
  phase: string
  reason?: string
  goal: SessionGoal | null
}

export type CompactOutcome = Awaited<ReturnType<typeof compactSession>>

// Feedback line for a goal_event from the chat stream, or '' when the phase
// is not worth surfacing.
export function goalEventFeedback(event: GoalEvent, text: GoalEventText): string {
  switch (event.phase) {
    case 'satisfied':
      return text.satisfied(event.reason ?? '')
    case 'exhausted':
      return text.exhausted(event.reason ?? '')
    case 'auto_continue':
      return event.goal ? text.autoContinue(event.goal.auto_continue_count, event.goal.max_auto_continues) : ''
    case 'judge_error':
      return text.judgeError(event.reason ?? text.unknownReason)
    default:
      return ''
  }
}

// First user (else assistant) message, flattened and clipped to a title.
export function autoTitleFromHistory(history: Pick<SessionMessage, 'role' | 'content'>[]): string {
  const source = history.find((m) => m.role === 'user') ?? history.find((m) => m.role === 'assistant')
  if (!source) return ''
  const clean = source.content.trim().replace(/\n/g, ' ').replace(/\s+/g, ' ')
  return clean.length > 50 ? clean.slice(0, 47) + '...' : clean
}

export class ChatSessionStore {
  // Session list (sidebar, and the P3 session board).
  sessions = $state<Session[]>([])
  sessionsLoading = $state(true)
  // null when the last load succeeded; otherwise the error message, which may
  // be empty so the caller can show its own localized text.
  sessionsError = $state<string | null>(null)

  // Active session. `activeSession` lags `activeSessionId` while it loads.
  activeSessionId = $state<string | null>(null)
  activeSession = $state<Session | null>(null)

  // Bumped whenever ChatPanel must remount with a fresh thread: session
  // switches and compaction. Never bumped by `adoptSession`.
  threadVersion = $state(0)

  // Per-session slices, reset together on every switch.
  artifacts = $state<Artifact[]>([])
  draft = $state('')
  contextInfo = $state<ChatContextInfo>({})
  // Bumped when session config changes so the context monitor refetches.
  contextVersion = $state(0)
  tasksSummary = $state<ChatTasksSummary>({ total: 0, pending: 0, in_progress: 0, completed: 0, cancelled: 0 })
  goal = $state<SessionGoal | null>(null)
  cwd = $state<SessionCwd | null>(null)
  cwdBusy = $state(false)
  health = $state<SessionHealthReport>(null as unknown as SessionHealthReport)
  healthLoading = $state(false)

  // Whether ChatPanel is streaming a response for the active session.
  streaming = $state(false)

  // Transient one-line (or multi-line) notice under the session header.
  feedback = $state('')

  // Status bar (#968). Tiers pinned per session id; '' holds the pick made
  // before a new chat has an id, and moves to the id when it is adopted.
  pinnedTiers = $state<Record<string, ChatTier>>({})
  tierOptions = $state<AgentRuntimeTierOption[]>([])
  // This month's usage for the active session, from the usage tracker.
  usage = $state<SessionUsage | null>(null)
  // The session's .tars permission-mode override; '' means the global setting.
  permissionModeOverride = $state('')
  private tierOptionsRequested = false
  private usageRequest = 0

  private healthInputs: Omit<SessionHealthInput, 'contextInfo' | 'now'> | null = null
  private healthRequest = 0
  private sessionsRequest = 0
  private feedbackTimer: ReturnType<typeof setTimeout> | null = null
  private readonly api: ChatSessionApi
  private readonly helpers: ChatSessionHealth

  constructor(api: ChatSessionApi, helpers: ChatSessionHealth) {
    this.api = api
    this.helpers = helpers
    this.health = helpers.emptyReport()
    this.tasksSummary = helpers.emptyTasks()
  }

  // Route → store. A no-op when the id is unchanged, so re-running the
  // caller's effect is harmless.
  setActive(id: string | null | undefined): void {
    const next = id?.trim() || null
    if (next === this.activeSessionId && this.threadVersion > 0) return
    this.activeSessionId = next
    this.activeSession = null
    this.resetSlices()
    this.threadVersion++
    if (next) {
      void this.refreshActive()
      void this.refreshHealth()
      void this.refreshUsage()
    }
  }

  // ChatPanel created a session on first send. Adopt it without resetting
  // state or remounting the panel that is mid-stream.
  adoptSession(id: string): void {
    const next = id.trim()
    if (!next || this.activeSessionId) return
    this.activeSessionId = next
    const draftTier = this.pinnedTiers['']
    if (draftTier) {
      const { '': _, ...rest } = this.pinnedTiers
      this.pinnedTiers = { ...rest, [next]: draftTier }
    }
    void this.refreshActive()
    void this.refreshSessions()
    void this.refreshHealth()
    void this.refreshUsage()
  }

  get pinnedTier(): ChatTier | null {
    return this.pinnedTiers[this.activeSessionId ?? ''] ?? null
  }

  // Pin a tier for every turn of the active session, or null to let the
  // server choose again.
  setPinnedTier(tier: ChatTier | null): void {
    const key = this.activeSessionId ?? ''
    const { [key]: _, ...rest } = this.pinnedTiers
    this.pinnedTiers = tier ? { ...rest, [key]: tier } : rest
  }

  // The configured tiers, fetched once for the status bar picker.
  async loadTierOptions(): Promise<void> {
    if (this.tierOptionsRequested) return
    this.tierOptionsRequested = true
    try {
      const resp = await this.api.listAgentRuntimeSubagents()
      this.tierOptions = resp.tiers ?? []
    } catch {
      this.tierOptionsRequested = false
    }
  }

  async refreshUsage(): Promise<void> {
    const id = this.activeSessionId
    const request = ++this.usageRequest
    if (!id) {
      this.usage = null
      return
    }
    try {
      const summary = await this.api.getUsageSummary({ period: 'month', sessionId: id })
      if (request !== this.usageRequest || this.activeSessionId !== id) return
      this.usage = {
        costUSD: summary.total_cost_usd,
        calls: summary.total_calls,
        inputTokens: summary.total_input_tokens,
        outputTokens: summary.total_output_tokens,
      }
    } catch {
      if (request === this.usageRequest) this.usage = null
    }
  }

  // Force ChatPanel to reload the thread for the same session.
  remountThread(): void {
    this.threadVersion++
  }

  // Overlapping refreshes (several callers fire one after a new session)
  // can resolve out of order; only the latest request may update the list.
  async refreshSessions(): Promise<void> {
    const request = ++this.sessionsRequest
    this.sessionsLoading = true
    this.sessionsError = null
    try {
      const sessions = await this.api.listSessions(true, 'include')
      if (request === this.sessionsRequest) this.sessions = sessions
    } catch (err) {
      // Keep the previous list.
      if (request === this.sessionsRequest) this.sessionsError = err instanceof Error ? err.message : ''
    } finally {
      if (request === this.sessionsRequest) this.sessionsLoading = false
    }
  }

  // Reload the active session record plus its cwd and goal.
  async refreshActive(): Promise<void> {
    const id = this.activeSessionId
    if (!id) return
    try {
      const session = await this.api.getSession(id)
      if (this.activeSessionId === id) this.activeSession = session
    } catch { /* keep the previous record */ }
    await Promise.all([this.refreshCwd(), this.refreshGoal()])
  }

  async refreshCwd(): Promise<void> {
    const id = this.activeSessionId
    if (!id) {
      this.cwd = null
      return
    }
    try {
      const cwd = await this.api.getSessionCwd(id)
      if (this.activeSessionId === id) this.cwd = cwd
    } catch {
      if (this.activeSessionId === id) this.cwd = null
    }
  }

  async setCwd(target: string): Promise<void> {
    const id = this.activeSessionId
    if (!id) throw new Error('no active session')
    if (this.cwdBusy) return
    this.cwdBusy = true
    try {
      await this.api.setSessionCwd(id, target)
      await this.refreshCwd()
    } finally {
      this.cwdBusy = false
    }
  }

  async refreshGoal(): Promise<void> {
    const id = this.activeSessionId
    if (!id) {
      this.goal = null
      return
    }
    try {
      const resp = await this.api.getSessionGoal(id)
      if (this.activeSessionId === id) this.goal = resp.goal
    } catch {
      if (this.activeSessionId === id) this.goal = null
    }
  }

  setGoal(goal: SessionGoal | null): void {
    this.goal = goal
  }

  applyGoalEvent(event: GoalEvent): void {
    this.goal = event.goal
    const message = goalEventFeedback(event, this.helpers.goalEventText())
    if (message) this.notify(message)
  }

  setArtifacts(artifacts: Artifact[]): void {
    this.artifacts = artifacts
  }

  setContextInfo(info: ChatContextInfo): void {
    this.contextInfo = info
    this.rebuildHealth()
  }

  bumpContextVersion(): void {
    this.contextVersion++
  }

  setTasksSummary(summary: ChatTasksSummary): void {
    this.tasksSummary = summary
    void this.refreshHealth()
  }

  setStreaming(streaming: boolean): void {
    this.streaming = streaming
  }

  // A turn finished or was cancelled: titles, message counts, and health
  // may all have moved.
  async turnSettled(): Promise<void> {
    await Promise.all([this.refreshSessions(), this.refreshActive(), this.refreshHealth(), this.refreshUsage()])
  }

  async refreshHealth(): Promise<void> {
    const id = this.activeSessionId
    if (!id) {
      this.healthInputs = null
      this.health = this.helpers.emptyReport()
      this.tasksSummary = this.helpers.emptyTasks()
      return
    }
    const request = ++this.healthRequest
    this.healthLoading = true
    try {
      const [session, history, taskState, config, toolsResp] = await Promise.all([
        this.api.getSession(id),
        this.api.getSessionHistory(id),
        this.api.getSessionTasks(id),
        this.api.getSessionEffectiveConfig(id),
        this.api.listChatTools(id),
      ])
      if (request !== this.healthRequest || this.activeSessionId !== id) return
      this.activeSession = session
      this.healthInputs = {
        session,
        messages: history,
        tasks: taskState,
        config: config.effective.tool_config,
        tools: toolsResp.tools,
      }
      this.permissionModeOverride = config.effective.claude_code_cli_permission_mode?.trim() ?? ''
      this.tasksSummary = { ...this.helpers.summarizeTasks(taskState.tasks), plan_goal: taskState.plan?.goal }
      this.rebuildHealth()
    } catch {
      if (request === this.healthRequest) {
        this.healthInputs = null
        this.health = this.helpers.emptyReport()
      }
    } finally {
      if (request === this.healthRequest) this.healthLoading = false
    }
  }

  async rename(title: string): Promise<void> {
    const id = this.activeSessionId
    const trimmed = title.trim()
    if (!id || !trimmed) return
    await this.api.renameSession(id, trimmed)
    await Promise.all([this.refreshActive(), this.refreshSessions()])
  }

  // Title the session from its first message. Resolves to the new title,
  // or '' when the transcript is empty.
  async autoTitle(): Promise<string> {
    const id = this.activeSessionId
    if (!id) return ''
    const title = autoTitleFromHistory(await this.api.getSessionHistory(id))
    if (title) await this.rename(title)
    return title
  }

  // Compact the active session and reload its thread.
  async compact(): Promise<CompactOutcome | null> {
    const id = this.activeSessionId
    if (!id) return null
    const result = await this.api.compactSession(id)
    this.remountThread()
    await Promise.all([this.refreshSessions(), this.refreshHealth()])
    return result
  }

  notify(message: string, ms = 4000): void {
    if (this.feedbackTimer) clearTimeout(this.feedbackTimer)
    this.feedback = message
    this.feedbackTimer = setTimeout(() => {
      this.feedback = ''
      this.feedbackTimer = null
    }, ms)
  }

  // Also called when the console language changes: the report's text is
  // built in the active language, so it has to be built again.
  rebuildHealth(): void {
    if (!this.healthInputs) {
      this.health = this.helpers.emptyReport()
      return
    }
    this.health = this.helpers.buildReport({ ...this.healthInputs, contextInfo: this.contextInfo })
  }

  private resetSlices(): void {
    this.artifacts = []
    this.draft = ''
    this.contextInfo = {}
    this.goal = null
    this.cwd = null
    this.healthInputs = null
    this.health = this.helpers.emptyReport()
    this.tasksSummary = this.helpers.emptyTasks()
    this.streaming = false
    this.usage = null
    this.permissionModeOverride = ''
  }
}
