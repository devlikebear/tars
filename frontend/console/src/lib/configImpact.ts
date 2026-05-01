import type { ConfigFieldMeta } from './types'

export type ConfigImpactPreview = {
  items: string[]
}

export function buildConfigImpactPreview(
  field: ConfigFieldMeta | undefined,
  oldValue: unknown,
  newValue: unknown,
): ConfigImpactPreview {
  if (!field) return { items: [] }
  const items = [
    ...subsystemImpactHints(field),
    ...(field.impact ?? []),
  ]

  if (field.key === 'pulse_interval') {
    items.push(...pulseIntervalImpact(oldValue, newValue))
  }
  if (field.key === 'log_level' && String(newValue).toLowerCase() === 'debug' && String(oldValue).toLowerCase() !== 'debug') {
    items.push('Debug mode can increase log volume by 5-10x on busy workspaces.')
  }
  if (field.key === 'usage_limit_mode' && String(newValue).toLowerCase() === 'hard') {
    items.push('Hard limit mode can block LLM calls once the configured budget is reached.')
  }
  if (field.key === 'pulse_min_severity') {
    items.push('Changing severity changes which pulse decisions reach notifications.')
  }

  return { items: uniqueItems(items) }
}

function subsystemImpactHints(field: ConfigFieldMeta): string[] {
  const key = field.key.toLowerCase()
  if (key.startsWith('llm_')) {
    return ['Affected subsystem: LLM routing. Changes can alter model/provider selection, cost, latency, and quality.']
  }
  if (key.startsWith('api_') || key.startsWith('dashboard_')) {
    return ['Affected subsystem: Auth/API. Changes can affect console access, API clients, and admin operations.']
  }
  if (key.startsWith('pulse_')) {
    return ['Affected subsystem: Pulse watchdog. Changes can alter incident detection, notifications, and autofix behavior.']
  }
  if (key.startsWith('reflection_')) {
    return ['Affected subsystem: Reflection. Changes can alter nightly memory extraction and cleanup timing.']
  }
  if (key.startsWith('cron_') || key.startsWith('schedule_')) {
    return ['Affected subsystem: Cron scheduling. Changes can alter job timing, history retention, or timezone interpretation.']
  }
  if (key.startsWith('memory_')) {
    return ['Affected subsystem: Memory. Changes can alter recall, embedding compatibility, and indexing behavior.']
  }
  if (key.startsWith('agentruntime_') || key.startsWith('agent_')) {
    return ['Affected subsystem: Agent Runtime. Changes can alter subagent execution, persistence, routing, and cost.']
  }
  if (key.startsWith('skills_') || key.startsWith('plugins_') || key.startsWith('mcp_')) {
    return ['Affected subsystem: Extensions. Changes can alter skill, plugin, or MCP discovery and runtime tool inventory.']
  }
  if (key.startsWith('tools_')) {
    return ['Affected subsystem: Tools. Changes can alter chat tool availability, network reach, and high-risk capabilities.']
  }
  if (key.startsWith('channels_') || key.startsWith('telegram_')) {
    return ['Affected subsystem: Channels. Changes can alter Telegram, webhook, or local dispatch behavior.']
  }
  if (key.startsWith('usage_')) {
    return ['Affected subsystem: Usage. Changes can alter budget enforcement and cost reporting.']
  }
  if (key.startsWith('compaction_')) {
    return ['Affected subsystem: Compaction. Changes can alter when and how long transcripts are compressed.']
  }
  if (key.startsWith('assistant_')) {
    return ['Affected subsystem: Assistant. Changes can alter voice activation and audio processing.']
  }
  if (key.startsWith('log_')) {
    return ['Affected subsystem: Logging. Changes can alter observability, retention, and troubleshooting detail.']
  }
  if (key.startsWith('session_') || key.startsWith('workspace_') || key.startsWith('plan_')) {
    return ['Affected subsystem: Runtime. Changes can alter session defaults, workspace state, or planning behavior.']
  }
  return []
}

export function parseDurationSeconds(value: unknown): number | null {
  const text = String(value ?? '').trim().toLowerCase()
  if (!text) return null
  const match = text.match(/^(\d+(?:\.\d+)?)(ms|s|m|h)?$/)
  if (!match) return null
  const amount = Number.parseFloat(match[1])
  if (!Number.isFinite(amount) || amount <= 0) return null
  const unit = match[2] ?? 's'
  switch (unit) {
    case 'ms':
      return amount / 1000
    case 's':
      return amount
    case 'm':
      return amount * 60
    case 'h':
      return amount * 3600
    default:
      return null
  }
}

function pulseIntervalImpact(oldValue: unknown, newValue: unknown): string[] {
  const oldSeconds = parseDurationSeconds(oldValue)
  const newSeconds = parseDurationSeconds(newValue)
  if (!oldSeconds || !newSeconds) return []

  const items: string[] = []
  if (newSeconds < oldSeconds) {
    items.push(`Signal detection latency can improve by about ${formatDuration(oldSeconds - newSeconds)}.`)
  } else if (newSeconds > oldSeconds) {
    items.push(`Signal detection latency can slow by about ${formatDuration(newSeconds - oldSeconds)}.`)
  }

  const volumeRatio = oldSeconds / newSeconds
  if (Math.abs(volumeRatio - 1) >= 0.05) {
    items.push(`Pulse tick volume changes by about ${formatRatio(volumeRatio)}x.`)
  }
  return items
}

function formatDuration(seconds: number): string {
  if (seconds >= 3600 && seconds % 3600 === 0) return `${seconds / 3600}h`
  if (seconds >= 60 && seconds % 60 === 0) return `${seconds / 60}m`
  return `${Math.round(seconds)}s`
}

function formatRatio(value: number): string {
  if (value >= 10) return String(Math.round(value))
  if (value >= 1) return value.toFixed(1).replace(/\.0$/, '')
  return value.toFixed(2).replace(/0$/, '').replace(/\.0$/, '')
}

function uniqueItems(items: string[]): string[] {
  const seen = new Set<string>()
  const result: string[] = []
  for (const item of items.map((raw) => raw.trim()).filter(Boolean)) {
    const key = item.toLowerCase()
    if (seen.has(key)) continue
    seen.add(key)
    result.push(item)
  }
  return result
}
