export type ConfigDisplaySummary = {
  kind: 'empty' | 'scalar' | 'array' | 'object'
  text: string
  preview: string[]
  raw: string
}

export type StructuredJSONParseResult =
  | { ok: true; value: unknown }
  | { ok: false; error: string }

export type LLMTierDraft = {
  id: string
  originalName: string
  name: string
  provider: string
  model: string
  reasoning_effort: string
  thinking_budget: string
  service_tier: string
}

export type LLMTierDraftField =
  | 'name'
  | 'provider'
  | 'model'
  | 'reasoning_effort'
  | 'thinking_budget'
  | 'service_tier'

export type LLMTierDraftErrors = Record<string, Partial<Record<LLMTierDraftField, string>>>

export type LLMTierBindingValue = {
  provider: string
  model: string
  reasoning_effort: string
  thinking_budget: number
  service_tier: string
}

export type LLMTiersBuildResult =
  | { ok: true; value: Record<string, LLMTierBindingValue> }
  | { ok: false; errors: LLMTierDraftErrors }

export function formatConfigDisplayValue(value: unknown): ConfigDisplaySummary {
  if (value === undefined || value === null || value === '') {
    return { kind: 'empty', text: '-', preview: [], raw: '-' }
  }
  if (Array.isArray(value)) {
    return {
      kind: 'array',
      text: `${value.length} ${value.length === 1 ? 'item' : 'items'}`,
      preview: value.slice(0, 3).map((item) => previewValue(item)),
      raw: prettyConfigJSON(value),
    }
  }
  if (typeof value === 'object') {
    const keys = Object.keys(value as Record<string, unknown>).sort()
    return {
      kind: 'object',
      text: `${keys.length} ${keys.length === 1 ? 'key' : 'keys'}`,
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

export function parseStructuredJSONEdit(raw: string): StructuredJSONParseResult {
  const trimmed = raw.trim()
  if (!trimmed) {
    return { ok: false, error: 'JSON is required.' }
  }
  try {
    return { ok: true, value: JSON.parse(trimmed) }
  } catch (error) {
    const detail = error instanceof Error ? error.message : String(error)
    return { ok: false, error: `Invalid JSON: ${detail}` }
  }
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

export function makeLLMTierDrafts(value: unknown): LLMTierDraft[] {
  const record = asRecord(value)
  if (!record) return []
  return Object.keys(record)
    .sort()
    .map((name, index) => {
      const binding = asRecord(record[name]) || {}
      return {
        id: `tier-${index}-${name}`,
        originalName: name,
        name,
        provider: readString(binding, 'provider'),
        model: readString(binding, 'model'),
        reasoning_effort: readString(binding, 'reasoning_effort', 'reasoningEffort'),
        thinking_budget: readBudgetString(binding),
        service_tier: readString(binding, 'service_tier', 'serviceTier'),
      }
    })
}

export function extractLLMProviderAliases(value: unknown): string[] {
  const record = asRecord(value)
  if (!record) return []
  return Object.keys(record)
    .map((alias) => alias.trim())
    .filter(Boolean)
    .sort()
}

export function buildLLMTiersFromDrafts(drafts: LLMTierDraft[], providerAliases: string[]): LLMTiersBuildResult {
  const providerSet = new Set(providerAliases.map((provider) => provider.trim()).filter(Boolean))
  const trimmedNames = drafts.map((draft) => draft.name.trim())
  const nameCounts = new Map<string, number>()
  for (const name of trimmedNames) {
    if (!name) continue
    nameCounts.set(name, (nameCounts.get(name) || 0) + 1)
  }

  const errors: LLMTierDraftErrors = {}
  const value: Record<string, LLMTierBindingValue> = {}

  drafts.forEach((draft, index) => {
    const rowErrors: Partial<Record<LLMTierDraftField, string>> = {}
    const id = draft.id || `tier-${index}`
    const name = draft.name.trim()
    const provider = draft.provider.trim()
    const model = draft.model.trim()
    const reasoningEffort = draft.reasoning_effort.trim()
    const budgetText = draft.thinking_budget.trim()
    const serviceTier = draft.service_tier.trim()

    if (!name) {
      rowErrors.name = 'Tier name is required.'
    } else if ((nameCounts.get(name) || 0) > 1) {
      rowErrors.name = 'Tier name must be unique.'
    }
    if (!provider) {
      rowErrors.provider = 'Provider is required.'
    } else if (providerSet.size > 0 && !providerSet.has(provider)) {
      rowErrors.provider = 'Choose a configured provider.'
    }
    if (!model) {
      rowErrors.model = 'Model is required.'
    }

    let thinkingBudget = 0
    if (budgetText) {
      if (!/^\d+$/.test(budgetText)) {
        rowErrors.thinking_budget = 'Thinking budget must be 0 or greater.'
      } else {
        thinkingBudget = Number(budgetText)
        if (!Number.isSafeInteger(thinkingBudget)) {
          rowErrors.thinking_budget = 'Thinking budget must be a safe integer.'
        }
      }
    }

    if (Object.keys(rowErrors).length > 0) {
      errors[id] = rowErrors
      return
    }

    value[name] = {
      provider,
      model,
      reasoning_effort: reasoningEffort,
      thinking_budget: thinkingBudget,
      service_tier: serviceTier,
    }
  })

  if (Object.keys(errors).length > 0) {
    return { ok: false, errors }
  }
  return { ok: true, value }
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
  for (const key of Object.keys(value as Record<string, unknown>).sort()) {
    out[key] = sortStable((value as Record<string, unknown>)[key])
  }
  return out
}

function asRecord(value: unknown): Record<string, unknown> | null {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return null
  return value as Record<string, unknown>
}

function readString(record: Record<string, unknown>, key: string, fallbackKey?: string): string {
  const value = record[key] ?? (fallbackKey ? record[fallbackKey] : undefined)
  if (value === undefined || value === null) return ''
  return String(value)
}

function readBudgetString(record: Record<string, unknown>): string {
  const value = record.thinking_budget ?? record.thinkingBudget
  if (value === undefined || value === null || value === '') return ''
  return String(value)
}
