import type { PulseSignal, PulseTickOutcome } from './types'

export type PulseIncidentAction = {
  label: string
  path: string
}

export type PulseIncidentCard = {
  id: string
  kind: string
  severity: PulseSignal['severity']
  title: string
  cause: string
  evidence: string[]
  recommendedAction: string
  primaryAction: PulseIncidentAction
  checkedAt: string
}

const maxEvidenceItems = 5

export function buildPulseIncidentCards(ticks: PulseTickOutcome[] = [], limit = 8): PulseIncidentCard[] {
  const cards: PulseIncidentCard[] = []
  for (const tick of ticks) {
    for (const signal of tick.signals ?? []) {
      cards.push(buildCard(tick, signal, cards.length))
      if (cards.length >= limit) return cards
    }
    if (!tick.signals?.length && tick.err) {
      cards.push(buildRuntimeErrorCard(tick, cards.length))
      if (cards.length >= limit) return cards
    }
  }
  return cards
}

function buildCard(tick: PulseTickOutcome, signal: PulseSignal, index: number): PulseIncidentCard {
  const title = nonEmpty(tick.decision?.title) || titleForKind(signal.kind)
  const evidence = compactEvidence([
    signal.summary,
    ...kindEvidence(signal),
    tick.decision ? `Decision: ${tick.decision.action} (${tick.decision.severity})` : '',
    tick.autofix_attempt ? `Autofix attempted: ${tick.autofix_attempt}` : '',
    tick.autofix_ok ? 'Autofix completed successfully' : '',
    tick.autofix_err ? `Autofix failed: ${tick.autofix_err}` : '',
    tick.notify_delivered ? 'Notification delivered' : '',
  ])

  return {
    id: `${tick.at || signal.at || 'tick'}-${signal.kind}-${index}`,
    kind: signal.kind,
    severity: signal.severity,
    title,
    cause: causeForSignal(signal),
    evidence,
    recommendedAction: recommendedActionForSignal(signal),
    primaryAction: primaryActionForSignal(signal),
    checkedAt: signal.at || tick.at,
  }
}

function buildRuntimeErrorCard(tick: PulseTickOutcome, index: number): PulseIncidentCard {
  return {
    id: `${tick.at || 'tick'}-pulse_error-${index}`,
    kind: 'pulse_error',
    severity: 'error',
    title: 'Pulse tick failed',
    cause: tick.err || 'Pulse returned an error while checking system health.',
    evidence: compactEvidence([tick.err, tick.skip_reason]),
    recommendedAction: 'Open Pulse and re-check after reviewing the server logs.',
    primaryAction: { label: 'Open Pulse', path: '/console/pulse' },
    checkedAt: tick.at,
  }
}

function titleForKind(kind: string): string {
  switch (kind) {
    case 'cron_failures':
      return 'Cron jobs failing'
    case 'stuck_agentruntime_run':
      return 'Agent runtime run stuck'
    case 'disk_usage':
      return 'Disk pressure'
    case 'delivery_failures':
      return 'Telegram delivery failing'
    case 'reflection_failure':
      return 'Reflection failures'
    case 'stalled_chat':
      return 'Chat session stalled'
    default:
      return humanizeKind(kind)
  }
}

function causeForSignal(signal: PulseSignal): string {
  const details = signal.details ?? {}
  switch (signal.kind) {
    case 'cron_failures': {
      const job = detailString(details, 'worst_job_name') || detailString(details, 'worst_job_id') || 'a cron job'
      const failures = detailString(details, 'worst_failures')
      return failures
        ? `Cron job "${job}" has failed ${failures} consecutive time(s).`
        : signal.summary
    }
    case 'stuck_agentruntime_run': {
      const count = detailString(details, 'stuck_count') || 'A'
      const minutes = detailString(details, 'stuck_minutes_minimum') || 'configured'
      return `${count} agent runtime run(s) exceeded the ${minutes} minute running window.`
    }
    case 'disk_usage': {
      const pct = detailNumber(details, 'disk_used_percent')
      const warn = detailNumber(details, 'warn_threshold')
      const critical = detailNumber(details, 'critical_threshold')
      if (pct !== '') {
        return `Disk usage is ${pct}% (warn ${warn || 'configured'}%, critical ${critical || 'configured'}%).`
      }
      return signal.summary
    }
    case 'delivery_failures': {
      const failures = detailString(details, 'failures') || 'Multiple'
      const window = detailString(details, 'window') || 'the configured window'
      return `${failures} Telegram delivery failure(s) occurred in ${window}.`
    }
    case 'reflection_failure': {
      const failures = detailString(details, 'consecutive_failures') || 'Multiple'
      return `Reflection has failed ${failures} consecutive nightly run(s).`
    }
    case 'stalled_chat': {
      const title = detailString(details, 'session_title') || detailString(details, 'session_id') || 'A chat session'
      const age = detailString(details, 'age_minutes')
      return age
        ? `Session "${title}" has waited ${age} minute(s) for user input.`
        : `Session "${title}" appears to be waiting for user input.`
    }
    default:
      return signal.summary
  }
}

function recommendedActionForSignal(signal: PulseSignal): string {
  switch (signal.kind) {
    case 'cron_failures':
      return 'Open Cron and inspect the failing job history before the next scheduled run.'
    case 'stuck_agentruntime_run':
      return 'Open Agent Runtime and decide whether to inspect, cancel, or restart the affected run.'
    case 'disk_usage':
      return 'Open Approvals and review a cleanup plan before deleting local workspace files.'
    case 'delivery_failures':
      return 'Open Settings and verify Telegram delivery configuration or credentials.'
    case 'reflection_failure':
      return 'Open Reflection, inspect the latest nightly result, and run maintenance manually if needed.'
    case 'stalled_chat':
      return 'Open Chat to answer the blocked session or confirm the configured auto-resume path.'
    default:
      return 'Open the affected page and inspect the related system details.'
  }
}

function primaryActionForSignal(signal: PulseSignal): PulseIncidentAction {
  switch (signal.kind) {
    case 'cron_failures':
      return { label: 'Open Cron', path: '/console/cron' }
    case 'stuck_agentruntime_run':
      return { label: 'Open Runtime', path: '/console/agentruntime' }
    case 'disk_usage':
      return { label: 'Open Approvals', path: '/console/approvals' }
    case 'delivery_failures':
      return { label: 'Open Settings', path: '/console/config' }
    case 'reflection_failure':
      return { label: 'Open Reflection', path: '/console/reflection' }
    case 'stalled_chat': {
      const sessionID = detailString(signal.details ?? {}, 'session_id')
      return sessionID
        ? { label: 'Open Chat', path: `/console/chat/${encodeURIComponent(sessionID)}` }
        : { label: 'Open Chat', path: '/console/chat' }
    }
    default:
      return { label: 'Open Pulse', path: '/console/pulse' }
  }
}

function kindEvidence(signal: PulseSignal): string[] {
  const details = signal.details ?? {}
  const rows: string[] = []
  const preferredKeys = [
    'worst_job_error',
    'worst_failures',
    'stuck_count',
    'oldest_started_at',
    'disk_used_percent',
    'disk_free_bytes',
    'failures',
    'consecutive_failures',
    'last_run_at',
    'age_minutes',
    'resume_mode',
    'block_reason',
    'can_auto_resume',
  ]
  for (const key of preferredKeys) {
    if (details[key] !== undefined && details[key] !== null && details[key] !== '') {
      rows.push(`${humanizeKind(key)}: ${formatDetailValue(details[key])}`)
    }
  }
  return rows
}

function compactEvidence(values: Array<string | undefined>): string[] {
  const seen = new Set<string>()
  const out: string[] = []
  for (const value of values) {
    const trimmed = nonEmpty(value)
    if (!trimmed || seen.has(trimmed)) continue
    seen.add(trimmed)
    out.push(trimmed)
    if (out.length >= maxEvidenceItems) break
  }
  return out
}

function detailString(details: Record<string, unknown>, key: string): string {
  const value = details[key]
  if (value === undefined || value === null) return ''
  return String(value).trim()
}

function detailNumber(details: Record<string, unknown>, key: string): string {
  const value = Number(details[key])
  if (!Number.isFinite(value)) return ''
  return value.toFixed(value % 1 === 0 ? 0 : 1)
}

function formatDetailValue(value: unknown): string {
  if (Array.isArray(value)) return `${value.length} item${value.length === 1 ? '' : 's'}`
  if (typeof value === 'object' && value !== null) return 'structured details'
  return String(value)
}

function humanizeKind(value: string): string {
  return value
    .replace(/[_-]+/g, ' ')
    .replace(/\b\w/g, (char) => char.toUpperCase())
}

function nonEmpty(value?: string): string {
  return value?.trim() ?? ''
}
