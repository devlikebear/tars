import { sortStrings } from './sort.js'

export type ConfigDisplaySummary = {
  kind: 'empty' | 'scalar' | 'array' | 'object'
  text: string
  preview: string[]
  raw: string
}

// `text` carries the current-language words for array/object summaries
// (i18n/sections/config.ts `values`); defaults keep English for callers
// (and existing tests) that pass none.
export interface ConfigValueText {
  items: (count: number) => string
  keys: (count: number) => string
  empty: string
}

const defaultValueText: ConfigValueText = {
  items: (count) => `${count} ${count === 1 ? 'item' : 'items'}`,
  keys: (count) => `${count} ${count === 1 ? 'key' : 'keys'}`,
  empty: '-',
}

export function formatConfigDisplayValue(value: unknown, text: ConfigValueText = defaultValueText): ConfigDisplaySummary {
  if (value === undefined || value === null || value === '') {
    return { kind: 'empty', text: text.empty, preview: [], raw: text.empty }
  }
  if (Array.isArray(value)) {
    return {
      kind: 'array',
      text: text.items(value.length),
      preview: value.slice(0, 3).map((item) => previewValue(item)),
      raw: prettyConfigJSON(value),
    }
  }
  if (typeof value === 'object') {
    const keys = sortStrings(Object.keys(value as Record<string, unknown>))
    return {
      kind: 'object',
      text: text.keys(keys.length),
      preview: keys.slice(0, 4),
      raw: prettyConfigJSON(value),
    }
  }
  return { kind: 'scalar', text: String(value), preview: [], raw: String(value) }
}

export function prettyConfigJSON(value: unknown): string {
  if (value === undefined || value === null || value === '') return '{}'
  if (typeof value === 'string') {
    const trimmed = value.trim()
    if (!trimmed) return '{}'
    try {
      return JSON.stringify(JSON.parse(trimmed), null, 2)
    } catch {
      return trimmed
    }
  }
  return JSON.stringify(value, null, 2)
}

export function stringifyConfigValue(value: unknown): string {
  return formatConfigDisplayValue(value).raw
}

export function configValuesEqual(a: unknown, b: unknown): boolean {
  if (a === b) return true
  const objectLikeA = typeof a === 'object' && a !== null
  const objectLikeB = typeof b === 'object' && b !== null
  if (objectLikeA || objectLikeB) {
    return stableStringify(a) === stableStringify(b)
  }
  return String(a ?? '') === String(b ?? '')
}

function previewValue(value: unknown): string {
  if (value === undefined || value === null) return 'null'
  if (typeof value === 'string') return value
  if (typeof value === 'number' || typeof value === 'boolean') return String(value)
  if (Array.isArray(value)) return `${value.length} items`
  if (typeof value === 'object') return `${Object.keys(value as Record<string, unknown>).length} keys`
  return String(value)
}

function stableStringify(value: unknown): string {
  return JSON.stringify(sortStable(value))
}

function sortStable(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(sortStable)
  if (!value || typeof value !== 'object') return value
  const out: Record<string, unknown> = {}
  for (const key of sortStrings(Object.keys(value as Record<string, unknown>))) {
    out[key] = sortStable((value as Record<string, unknown>)[key])
  }
  return out
}
