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
  const items = [...(field.impact ?? [])]

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
