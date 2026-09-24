import type { SessionHealthTranslations } from '../i18n/sections/sessionHealth'
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

// Report text comes from `strings` (the sessionHealth i18n section), so callers
// pick the locale; the builders stay pure.
export function emptySessionHealthReport(strings: SessionHealthTranslations, now = new Date()): SessionHealthReport {
  return {
    status: 'healthy',
    badgeLabel: strings.status.healthy,
    summary: strings.summary.healthy,
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

export function buildSessionHealthReport(strings: SessionHealthTranslations, input: SessionHealthInput): SessionHealthReport {
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

  const signalText = strings.signals
  const recommendationText = strings.recommendations
  const actionLabel = strings.actions

  if (messageCount >= 160 || (contextTokenPercent ?? 0) >= 95) {
    addSignal(signals, 'long_context', 'critical', signalText.contextSaturated.title, signalText.contextSaturated.detail(messageCount))
    addRecommendation(recommendations, {
      id: 'compact-long-context',
      severity: 'critical',
      title: recommendationText.compactLongContext.title,
      detail: recommendationText.compactLongContext.detail,
      action: 'compact',
      actionLabel: actionLabel.compact,
    })
    addRecommendation(recommendations, {
      id: 'fork-long-context',
      severity: 'warning',
      title: recommendationText.forkLongContext.title,
      detail: recommendationText.forkLongContext.detail,
      action: 'review_fork_points',
      actionLabel: actionLabel.review_fork_points,
    })
  } else if (messageCount >= 80 || (contextTokenPercent ?? 0) >= 75) {
    addSignal(signals, 'long_context', 'warning', signalText.contextLong.title, signalText.contextLong.detail(messageCount))
    addRecommendation(recommendations, {
      id: 'compact-growing-context',
      severity: 'warning',
      title: recommendationText.compactGrowingContext.title,
      detail: recommendationText.compactGrowingContext.detail,
      action: 'compact',
      actionLabel: actionLabel.compact,
    })
  }

  const stalePlan = stalePlanAgeDays(tasks, now)
  if (stalePlan !== null && openTaskCount > 0) {
    const severity: SessionHealthSeverity = stalePlan >= 7 ? 'error' : 'warning'
    addSignal(signals, 'stale_plan', severity, signalText.stalePlan.title, signalText.stalePlan.detail(openTaskCount, strings.ago(stalePlan)))
    addRecommendation(recommendations, {
      id: 'review-stale-plan',
      severity,
      title: recommendationText.reviewStalePlan.title,
      detail: recommendationText.reviewStalePlan.detail,
      action: 'open_tasks',
      actionLabel: actionLabel.open_tasks,
    })
  }

  if (hasSessionWork && highRiskToolCount >= 3) {
    addSignal(signals, 'broad_permissions', 'error', signalText.broadPermissions.title, signalText.broadPermissions.detail(highRiskToolCount))
    addRecommendation(recommendations, {
      id: 'trim-permissions',
      severity: 'error',
      title: recommendationText.trimPermissions.title,
      detail: recommendationText.trimPermissions.detail,
      action: 'open_config',
      actionLabel: actionLabel.open_config,
    })
  } else if (hasSessionWork && highRiskToolCount > 0 && openTaskCount === 0) {
    addSignal(signals, 'broad_permissions', 'warning', signalText.idlePermissions.title, signalText.idlePermissions.detail(highRiskToolCount))
    addRecommendation(recommendations, {
      id: 'trim-idle-permissions',
      severity: 'warning',
      title: recommendationText.trimIdlePermissions.title,
      detail: recommendationText.trimIdlePermissions.detail,
      action: 'open_config',
      actionLabel: actionLabel.open_config,
    })
  }

  if (memoryCount >= 10 || memoryTokens >= 3000) {
    addSignal(signals, 'memory_noise', 'warning', signalText.memoryNoise.title, signalText.memoryNoise.detail(memoryCount, memoryTokens))
    addRecommendation(recommendations, {
      id: 'review-prior-context',
      severity: 'warning',
      title: recommendationText.reviewPriorContext.title,
      detail: recommendationText.reviewPriorContext.detail,
      action: 'open_prior',
      actionLabel: actionLabel.open_prior,
    })
  }

  const idleDays = daysSince(input.session?.updated_at, now)
  if (idleDays !== null && idleDays >= 7 && openTaskCount === 0 && messageCount > 0) {
    addSignal(signals, 'stale_session', 'info', signalText.staleSession.title, signalText.staleSession.detail(strings.ago(idleDays)))
    addRecommendation(recommendations, {
      id: 'extract-idle-session-skill',
      severity: 'info',
      title: recommendationText.extractIdleSessionSkill.title,
      detail: recommendationText.extractIdleSessionSkill.detail,
      action: 'open_skill_extraction',
      actionLabel: actionLabel.open_skill_extraction,
    })
  }

  const maxSeverity = signals.reduce<SessionHealthSeverity | null>((max, signal) => {
    if (!max || severityRank[signal.severity] > severityRank[max]) return signal.severity
    return max
  }, null)

  const status = statusFromSeverity(maxSeverity)
  return {
    status,
    badgeLabel: strings.status[status],
    summary: summaryForStatus(strings, status, signals.length),
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

function summaryForStatus(strings: SessionHealthTranslations, status: SessionHealthStatus, signalCount: number): string {
  switch (status) {
    case 'critical':
      return strings.summary.critical(signalCount)
    case 'attention':
      return strings.summary.attention(signalCount)
    case 'watch':
      return strings.summary.watch(signalCount)
    default:
      return strings.summary.healthy
  }
}
