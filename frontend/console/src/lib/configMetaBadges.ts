import type { ConfigFieldMeta } from './types'
import { sortStrings } from './sort.js'

export type ConfigMetaBadgeTone = 'default' | 'modified' | 'restart' | 'live' | 'secret'

export type ConfigMetaBadge = {
  label: string
  tone: ConfigMetaBadgeTone
  title: string
}

export function buildConfigMetaBadges(
  field: ConfigFieldMeta,
  value: unknown,
  isDirty: boolean,
  updatedAt?: string,
  now = new Date()
): ConfigMetaBadge[] {
  const badges: ConfigMetaBadge[] = []
  const hasDefault = Object.prototype.hasOwnProperty.call(field, 'default_value')
  const matchesDefault = hasDefault && configValuesEqual(value, field.default_value)

  if (matchesDefault) {
    badges.push({ label: 'default', tone: 'default', title: 'Matches the default value.' })
  } else if (isDirty) {
    badges.push({ label: 'modified just now', tone: 'modified', title: 'Pending change differs from the default value.' })
  } else if (hasDefault) {
    const relative = formatRelativeConfigTime(updatedAt, now)
    badges.push({
      label: relative ? `modified ${relative}` : 'modified',
      tone: 'modified',
      title: 'Current value differs from the default value.',
    })
  }

  if (field.requires_restart === true) {
    badges.push({ label: 'requires restart', tone: 'restart', title: 'Saving this setting updates YAML; restart TARS to apply it.' })
  } else if (field.requires_restart === false) {
    badges.push({ label: 'live', tone: 'live', title: 'This setting can apply without restarting TARS.' })
  }

  if (field.sensitive) {
    badges.push({ label: 'secret', tone: 'secret', title: 'Value is masked in the form view.' })
  }

  return badges
}

export function formatRelativeConfigTime(updatedAt?: string, now = new Date()): string {
  if (!updatedAt) return ''
  const updated = new Date(updatedAt)
  if (Number.isNaN(updated.getTime())) return ''
  const elapsedMs = Math.max(0, now.getTime() - updated.getTime())
  const elapsedSeconds = Math.floor(elapsedMs / 1000)
  if (elapsedSeconds < 60) return 'just now'
  const elapsedMinutes = Math.floor(elapsedSeconds / 60)
  if (elapsedMinutes < 60) return `${elapsedMinutes}m ago`
  const elapsedHours = Math.floor(elapsedMinutes / 60)
  if (elapsedHours < 48) return `${elapsedHours}h ago`
  const elapsedDays = Math.floor(elapsedHours / 24)
  return `${elapsedDays}d ago`
}

function configValuesEqual(a: unknown, b: unknown): boolean {
  if (a === b) return true
  const objectLikeA = typeof a === 'object' && a !== null
  const objectLikeB = typeof b === 'object' && b !== null
  if (objectLikeA || objectLikeB) {
    return stableStringify(a) === stableStringify(b)
  }
  return String(a ?? '') === String(b ?? '')
}

function stableStringify(value: unknown): string {
  if (value === null || typeof value !== 'object') return JSON.stringify(value)
  if (Array.isArray(value)) return `[${value.map((item) => stableStringify(item)).join(',')}]`
  const record = value as Record<string, unknown>
  return `{${sortStrings(Object.keys(record)).map((key) => `${JSON.stringify(key)}:${stableStringify(record[key])}`).join(',')}}`
}
