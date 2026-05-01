import type { ChatToolInfo, SessionToolConfig } from './api'
import type { Session, SessionMessage, SessionTasks } from './types'

export type SessionHealthStatus = 'healthy' | 'watch' | 'attention' | 'critical'
export type SessionHealthSeverity = 'info' | 'warning' | 'error' | 'critical'
export type SessionHealthAction =
  | 'compact'
  | 'review_fork_points'
  | 'open_tasks'
  | 'open_config'
  | 'open_prior'
  | 'open_skill_extraction'

export type SessionHealthSignal = {
  kind: 'long_context' | 'stale_plan' | 'broad_permissions' | 'memory_noise' | 'stale_session'
  severity: SessionHealthSeverity
  title: string
  detail: string
}

export type SessionHealthRecommendation = {
  id: string
  severity: SessionHealthSeverity
  title: string
  detail: string
  action: SessionHealthAction
  actionLabel: string
}

export type SessionHealthReport = {
  status: SessionHealthStatus
  badgeLabel: string
  summary: string
  signals: SessionHealthSignal[]
  recommendations: SessionHealthRecommendation[]
  metrics: {
    messageCount: number
    openTaskCount: number
    highRiskToolCount: number
    memoryCount: number
    memoryTokens: number
    contextTokenPercent?: number
  }
  checkedAt: string
}

export type SessionHealthContextInfo = {
  system_prompt_tokens?: number
  history_tokens?: number
  memory_count?: number
  memory_tokens?: number
  compaction_trigger_tokens?: number
}

export type SessionHealthInput = {
  session?: Session | null
  messages?: SessionMessage[] | null
  tasks?: SessionTasks | null
  config?: SessionToolConfig | null
  tools?: ChatToolInfo[] | null
  contextInfo?: SessionHealthContextInfo | null
  now?: Date
}

const severityRank: Record<SessionHealthSeverity, number> = {
  info: 0,
  warning: 1,
  error: 2,
  critical: 3,
}

export function emptySessionHealthReport(now = new Date()): SessionHealthReport {
  return {
    status: 'healthy',
    badgeLabel: 'Healthy',
    summary: 'No session warnings detected.',
    signals: [],
    recommendations: [],
    metrics: {
      messageCount: 0,
      openTaskCount: 0,
      highRiskToolCount: 0,
      memoryCount: 0,
      memoryTokens: 0,
    },
    checkedAt: now.toISOString(),
  }
}

export function buildSessionHealthReport(input: SessionHealthInput): SessionHealthReport {
  const now = input.now ?? new Date()
  const messages = input.messages ?? []
  const tasks = input.tasks ?? { tasks: [] }
  const contextInfo = input.contextInfo ?? {}
  const signals: SessionHealthSignal[] = []
  const recommendations: SessionHealthRecommendation[] = []

  const messageCount = messages.length
  const openTaskCount = (tasks.tasks ?? []).filter((task) => task.status === 'pending' || task.status === 'in_progress').length
  const highRiskToolCount = countEnabledHighRiskTools(input.config ?? {}, input.tools ?? [])
  const memoryCount = contextInfo.memory_count ?? 0
  const memoryTokens = contextInfo.memory_tokens ?? 0
  const contextTokenPercent = contextPercent(contextInfo)
  const hasSessionWork = messageCount > 0 || openTaskCount > 0

  if (messageCount >= 160 || (contextTokenPercent ?? 0) >= 95) {
    addSignal(signals, 'long_context', 'critical', 'Context is near saturation', `${messageCount} transcript messages are loaded. Compact or split before the next major task.`)
    addRecommendation(recommendations, {
      id: 'compact-long-context',
      severity: 'critical',
      title: 'Compact this session',
      detail: 'Shrink old turns into a summary so the next response has cleaner context.',
      action: 'compact',
      actionLabel: 'Compact',
    })
    addRecommendation(recommendations, {
      id: 'fork-long-context',
      severity: 'warning',
      title: 'Split at a stable point',
      detail: 'Start the next task from a known-good message instead of carrying every turn forward.',
      action: 'review_fork_points',
      actionLabel: 'Review Chat',
    })
  } else if (messageCount >= 80 || (contextTokenPercent ?? 0) >= 75) {
    addSignal(signals, 'long_context', 'warning', 'Context is getting long', `${messageCount} transcript messages are loaded.`)
    addRecommendation(recommendations, {
      id: 'compact-growing-context',
      severity: 'warning',
      title: 'Compact soon',
      detail: 'The session is still workable, but context reuse is starting to cost attention.',
      action: 'compact',
      actionLabel: 'Compact',
    })
  }

  const stalePlan = stalePlanAgeDays(tasks, now)
  if (stalePlan !== null && openTaskCount > 0) {
    const severity: SessionHealthSeverity = stalePlan >= 7 ? 'error' : 'warning'
    addSignal(signals, 'stale_plan', severity, 'Plan has gone stale', `${openTaskCount} open task(s), last plan update ${formatDays(stalePlan)} ago.`)
    addRecommendation(recommendations, {
      id: 'review-stale-plan',
      severity,
      title: 'Review open tasks',
      detail: 'Close completed work, archive stale plan items, or rewrite the next step.',
      action: 'open_tasks',
      actionLabel: 'Open Tasks',
    })
  }

  if (hasSessionWork && highRiskToolCount >= 3) {
    addSignal(signals, 'broad_permissions', 'error', 'Broad high-risk permissions', `${highRiskToolCount} high-risk tool(s) are enabled for this session.`)
    addRecommendation(recommendations, {
      id: 'trim-permissions',
      severity: 'error',
      title: 'Reduce session permissions',
      detail: 'Keep only the tool groups needed for the current task before enabling more automation.',
      action: 'open_config',
      actionLabel: 'Open Config',
    })
  } else if (hasSessionWork && highRiskToolCount > 0 && openTaskCount === 0) {
    addSignal(signals, 'broad_permissions', 'warning', 'High-risk tools still enabled', `${highRiskToolCount} high-risk tool(s) remain enabled after the active task.`)
    addRecommendation(recommendations, {
      id: 'trim-idle-permissions',
      severity: 'warning',
      title: 'Trim idle permissions',
      detail: 'Disable write or shell capabilities when the session is only being used for review.',
      action: 'open_config',
      actionLabel: 'Open Config',
    })
  }

  if (memoryCount >= 10 || memoryTokens >= 3000) {
    addSignal(signals, 'memory_noise', 'warning', 'Prior context is noisy', `${memoryCount} memory item(s) and ${memoryTokens} memory token(s) are attached.`)
    addRecommendation(recommendations, {
      id: 'review-prior-context',
      severity: 'warning',
      title: 'Review recalled memory',
      detail: 'Check whether the retrieved memory still matches this task before continuing.',
      action: 'open_prior',
      actionLabel: 'Open Prior',
    })
  }

  const idleDays = daysSince(input.session?.updated_at, now)
  if (idleDays !== null && idleDays >= 7 && openTaskCount === 0 && messageCount > 0) {
    addSignal(signals, 'stale_session', 'info', 'Session has been idle', `Last updated ${formatDays(idleDays)} ago.`)
    addRecommendation(recommendations, {
      id: 'extract-idle-session-skill',
      severity: 'info',
      title: 'Extract reusable work',
      detail: 'If this session produced a reusable workflow, turn it into a skill draft before archiving it.',
      action: 'open_skill_extraction',
      actionLabel: 'Extract Skill',
    })
  }

  const maxSeverity = signals.reduce<SessionHealthSeverity | null>((max, signal) => {
    if (!max || severityRank[signal.severity] > severityRank[max]) return signal.severity
    return max
  }, null)

  const status = statusFromSeverity(maxSeverity)
  return {
    status,
    badgeLabel: badgeLabel(status),
    summary: summaryForStatus(status, signals.length),
    signals,
    recommendations,
    metrics: {
      messageCount,
      openTaskCount,
      highRiskToolCount,
      memoryCount,
      memoryTokens,
      ...(contextTokenPercent !== undefined ? { contextTokenPercent } : {}),
    },
    checkedAt: now.toISOString(),
  }
}

function addSignal(
  signals: SessionHealthSignal[],
  kind: SessionHealthSignal['kind'],
  severity: SessionHealthSeverity,
  title: string,
  detail: string,
) {
  signals.push({ kind, severity, title, detail })
}

function addRecommendation(recommendations: SessionHealthRecommendation[], recommendation: SessionHealthRecommendation) {
  if (recommendations.some((item) => item.id === recommendation.id || item.action === recommendation.action)) return
  recommendations.push(recommendation)
}

function countEnabledHighRiskTools(config: SessionToolConfig, tools: ChatToolInfo[]): number {
  if (tools.length === 0) return 0
  const enabledNames = enabledToolNames(config, tools)
  return tools.filter((tool) => tool.high_risk && enabledNames.has(tool.name)).length
}

function enabledToolNames(config: SessionToolConfig, tools: ChatToolInfo[]): Set<string> {
  const usesCustomList = config.tools_custom || Array.isArray(config.tools_enabled)
  const names = new Set(usesCustomList ? (config.tools_enabled ?? []) : tools.map((tool) => tool.name))

  for (const disabled of config.tools_disabled ?? []) {
    names.delete(disabled)
  }

  const allowGroups = new Set(config.tools_allow_groups ?? [])
  const denyGroups = new Set(config.tools_deny_groups ?? [])
  for (const tool of tools) {
    if (!names.has(tool.name)) continue
    if (allowGroups.size > 0 && (!tool.group || !allowGroups.has(tool.group))) {
      names.delete(tool.name)
      continue
    }
    if (tool.group && denyGroups.has(tool.group)) {
      names.delete(tool.name)
    }
  }

  return names
}

function stalePlanAgeDays(tasks: SessionTasks, now: Date): number | null {
  const plan = tasks.plan
  if (!plan) return null
  const status = plan.status || 'executing'
  if (status === 'completed' || status === 'aborted') return null
  const age = daysSince(plan.updated_at || plan.created_at, now)
  if (age === null || age < 3) return null
  return age
}

function contextPercent(contextInfo: SessionHealthContextInfo): number | undefined {
  const trigger = contextInfo.compaction_trigger_tokens ?? 0
  if (trigger <= 0) return undefined
  const used = (contextInfo.system_prompt_tokens ?? 0) + (contextInfo.history_tokens ?? 0) + (contextInfo.memory_tokens ?? 0)
  if (used <= 0) return undefined
  return Math.max(0, Math.round((used / trigger) * 100))
}

function daysSince(value: string | undefined, now: Date): number | null {
  if (!value?.trim()) return null
  const date = new Date(value)
  if (Number.isNaN(date.getTime()) || date.getFullYear() <= 1) return null
  return Math.max(0, (now.getTime() - date.getTime()) / 86_400_000)
}

function formatDays(days: number): string {
  if (days < 1) return 'today'
  return `${Math.floor(days)}d`
}

function statusFromSeverity(severity: SessionHealthSeverity | null): SessionHealthStatus {
  switch (severity) {
    case 'critical':
      return 'critical'
    case 'error':
      return 'attention'
    case 'warning':
    case 'info':
      return 'watch'
    default:
      return 'healthy'
  }
}

function badgeLabel(status: SessionHealthStatus): string {
  switch (status) {
    case 'critical':
      return 'Critical'
    case 'attention':
      return 'Needs attention'
    case 'watch':
      return 'Watch'
    default:
      return 'Healthy'
  }
}

function summaryForStatus(status: SessionHealthStatus, signalCount: number): string {
  switch (status) {
    case 'critical':
      return `${signalCount} critical session issue(s) need action before continuing.`
    case 'attention':
      return `${signalCount} session issue(s) should be resolved soon.`
    case 'watch':
      return `${signalCount} session signal(s) are worth watching.`
    default:
      return 'No session warnings detected.'
  }
}
