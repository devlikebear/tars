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
// This module imports types and pure helpers only. The API is injected so
// the store can be compiled and exercised under plain Node in tests.

import { nextPermissionMode, permissionModeCycle } from '../chatApproval.ts'
import { isPinnableTier } from '../tierRecommendation.ts'
import { userVisibleText } from '../consoleContext.ts'
import type {
  PermissionMode,
  PermissionModeView,
  getPermissionMode,
  setPermissionMode,
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
  setSessionTierPin,
} from '../api'
import type { Artifact } from '../artifacts'
import type { ChatCommandsTranslations } from '../../i18n/sections/chatCommands'
import type { SessionHealthInput, SessionHealthProvider, SessionHealthReport } from '../sessionHealth'
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
  getPermissionMode: typeof getPermissionMode
  setPermissionMode: typeof setPermissionMode
  renameSession: typeof renameSession
  setSessionTierPin: typeof setSessionTierPin
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
  const content = source.role === 'user' ? userVisibleText(source.content) : source.content
  const clean = content.trim().replace(/\n/g, ' ').replace(/\s+/g, ' ')
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
  // The configured default tier: what an unpinned turn runs on when no turn
  // has told us otherwise yet.
  defaultTier = $state('')
  // This month's usage for the active session, from the usage tracker.
  usage = $state<SessionUsage | null>(null)
  // The session's .tars permission-mode override; '' means the global setting.
  permissionModeOverride = $state('')
  // The session's tool permission mode (#970); null until loaded.
  permission = $state<PermissionModeView | null>(null)
  permissionError = $state('')
  private permissionRequest = 0
  // Why the last pin could not be saved; '' when it was.
  tierPinError = $state('')
  private tierOptionsRequested = false
  // Bumped by every local pin change, so a session record fetched before
  // it cannot put the old pin back.
  private tierPinEdits = 0
  private usageRequest = 0

  private healthInputs: Omit<SessionHealthInput, 'contextInfo' | 'provider' | 'now'> | null = null
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
      void this.refreshPermission()
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
      // The turn carried the pin, but save it here too so the session keeps
      // it whatever order the server's writes and our reads land in.
      this.writePin('', null)
      void this.setPinnedTier(draftTier)
    }
    void this.refreshActive()
    void this.refreshSessions()
    void this.refreshHealth()
    void this.refreshUsage()
    void this.refreshPermission()
  }

  async refreshPermission(): Promise<void> {
    const id = this.activeSessionId
    if (!id) {
      this.setPermissionView(null)
      return
    }
    const request = ++this.permissionRequest
    try {
      const view = await this.api.getPermissionMode(id)
      if (request === this.permissionRequest && this.activeSessionId === id) this.setPermissionView(view)
    } catch {
      if (request === this.permissionRequest) this.setPermissionView(null)
    }
  }

  // setPermissionMode switches the active session's mode; '' returns to the
  // default. The bar shows the new mode at once and falls back on failure.
  async setPermissionMode(mode: PermissionMode | ''): Promise<boolean> {
    const id = this.activeSessionId
    if (!id) return false
    const before = this.permission
    if (before && mode) this.setPermissionView({ ...before, mode, effective: mode, claude_code_effective: mode, source: 'session' })
    this.permissionError = ''
    const request = ++this.permissionRequest
    try {
      const view = await this.api.setPermissionMode(id, mode)
      if (request === this.permissionRequest && this.activeSessionId === id) this.setPermissionView(view)
      return true
    } catch (err) {
      if (request === this.permissionRequest && this.activeSessionId === id) {
        this.setPermissionView(before)
        this.permissionError = err instanceof Error ? err.message : String(err)
      }
      return false
    }
  }

  // cyclePermissionMode steps to the next mode (⇧Tab in the composer).
  // current is the mode the bar shows, which for an inheriting session
  // depends on the provider.
  cyclePermissionMode(current?: string): Promise<boolean> {
    return this.setPermissionMode(nextPermissionMode(current ?? this.permission?.mode ?? this.permission?.effective ?? 'manual'))
  }

  // applyPermissionMode follows a mode change the server made during a turn
  // (an approved plan).
  applyPermissionMode(mode: string): void {
    if (!this.permission || !permissionModeCycle.includes(mode as PermissionMode)) return
    const next = mode as PermissionMode
    this.setPermissionView({ ...this.permission, mode: next, effective: next, claude_code_effective: next, source: 'session' })
  }

  // The permission view also feeds the health report (the Claude Code flag).
  private setPermissionView(view: PermissionModeView | null): void {
    this.permission = view
    this.rebuildHealth()
  }

  get pinnedTier(): ChatTier | null {
    return this.pinnedTiers[this.activeSessionId ?? ''] ?? null
  }

  // Pin a tier for every turn of the active session, or null to let the
  // server choose again. The pin is saved on the session (tier_pin), so it
  // outlives a reload; before a new chat has an id it waits under '' and is
  // saved when the session is adopted. When the save fails the previous
  // pick comes back and tierPinError says why.
  async setPinnedTier(tier: ChatTier | null): Promise<boolean> {
    const id = this.activeSessionId
    const key = id ?? ''
    const before = this.pinnedTiers[key] ?? null
    this.writePin(key, tier)
    const edit = ++this.tierPinEdits
    this.tierPinError = ''
    if (!id) return true
    try {
      await this.api.setSessionTierPin(id, tier)
      return true
    } catch (err) {
      if (edit === this.tierPinEdits) {
        this.writePin(key, before)
        this.tierPinError = err instanceof Error ? err.message : String(err)
      }
      return false
    }
  }

  private writePin(key: string, tier: ChatTier | null): void {
    const { [key]: _, ...rest } = this.pinnedTiers
    this.pinnedTiers = tier ? { ...rest, [key]: tier } : rest
    this.rebuildHealth()
  }

  // Take the pin from a session record the server sent.
  private syncPinFromSession(session: Session): void {
    this.writePin(session.id, isPinnableTier(session.tier_pin ?? '') ? (session.tier_pin as ChatTier) : null)
  }

  // The configured tiers, fetched once for the status bar picker.
  async loadTierOptions(): Promise<void> {
    if (this.tierOptionsRequested) return
    this.tierOptionsRequested = true
    try {
      const resp = await this.api.listAgentRuntimeSubagents()
      this.tierOptions = resp.tiers ?? []
      this.defaultTier = resp.default_tier?.trim() ?? ''
      this.rebuildHealth()
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
    const pinEdits = this.tierPinEdits
    try {
      const session = await this.api.getSession(id)
      if (this.activeSessionId === id) this.activeSession = session
      if (pinEdits === this.tierPinEdits) this.syncPinFromSession(session)
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
    this.health = this.helpers.buildReport({ ...this.healthInputs, contextInfo: this.contextInfo, provider: this.healthProvider() })
  }

  // The provider the next turn runs on, as far as the console knows: the
  // pinned tier's kind, else the last turn's provider, else the default
  // tier's kind.
  private healthProvider(): SessionHealthProvider {
    const kindOf = (tier: string) => this.tierOptions.find((option) => option.name === tier)?.kind?.trim() ?? ''
    const pinned = this.pinnedTier
    const kind = pinned ? kindOf(pinned) : this.contextInfo.llm_provider?.trim() || kindOf(this.defaultTier)
    const claudeCodeFlag = this.permission?.claude_code_flag?.trim()
    return { kind, ...(claudeCodeFlag ? { claudeCodeFlag } : {}) }
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
    this.permission = null
    this.permissionError = ''
    this.tierPinError = ''
  }
}
