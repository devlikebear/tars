import { sortStrings } from './sort.js'

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

export type LLMProviderDraft = {
  id: string
  originalAlias: string
  alias: string
  kind: string
  auth_mode: string
  base_url: string
  api_key: string
}

export type LLMProviderDraftField =
  | 'alias'
  | 'kind'
  | 'auth_mode'
  | 'base_url'
  | 'api_key'

export type LLMProviderDraftErrors = Record<string, Partial<Record<LLMProviderDraftField, string>>>

export type LLMProviderSettingsValue = {
  kind: string
  auth_mode: string
  base_url: string
  api_key: string
}

export type LLMProvidersBuildResult =
  | { ok: true; value: Record<string, LLMProviderSettingsValue> }
  | { ok: false; errors: LLMProviderDraftErrors }

export type EmbodimentProviderDraft = {
  id: string
  originalName: string
  name: string
  enabled: boolean
  transport: string
  endpoint: string
  capabilities: string[]
  session_id: string
  agent: string
  owner_only_directive: boolean
  salience_min_sound_level: string
  min_trigger_interval: string
  max_triggers_per_hour: string
  trigger_observations: boolean
}

export type EmbodimentProviderDraftField =
  | 'name'
  | 'enabled'
  | 'transport'
  | 'endpoint'
  | 'capabilities'
  | 'session_id'
  | 'agent'
  | 'owner_only_directive'
  | 'salience_min_sound_level'
  | 'min_trigger_interval'
  | 'max_triggers_per_hour'
  | 'trigger_observations'

export type EmbodimentProviderDraftErrors = Record<string, Partial<Record<EmbodimentProviderDraftField, string>>>

export type EmbodimentProviderSettingsValue = {
  name: string
  enabled: boolean
  transport: string
  endpoint: string
  capabilities: string[]
  session_id: string
  agent: string
  owner_only_directive: boolean
  salience_min_sound_level: number
  min_trigger_interval: string
  max_triggers_per_hour: number
  trigger_observations: boolean
}

export type EmbodimentProvidersBuildResult =
  | { ok: true; value: EmbodimentProviderSettingsValue[] }
  | { ok: false; errors: EmbodimentProviderDraftErrors }

export const LLM_PROVIDER_KINDS = [
  'anthropic',
  'openai',
  'openai-codex',
  'kimi',
  'gemini',
  'gemini-native',
  'claude-code-cli',
] as const

export const LLM_PROVIDER_AUTH_MODES = ['api-key', 'oauth', 'cli'] as const

export const EMBODIMENT_PROVIDER_TRANSPORTS = ['mcp', 'webhook'] as const
export const EMBODIMENT_PROVIDER_CAPABILITIES = ['vision', 'hearing', 'speech', 'expression', 'motion', 'led'] as const

export type EmbodimentCapabilityGroup = 'perception' | 'actuation'

export type EmbodimentCapabilityDetail = {
  id: string
  label: string
  group: EmbodimentCapabilityGroup
  description: string
}

export const EMBODIMENT_PROVIDER_CAPABILITY_DETAILS: EmbodimentCapabilityDetail[] = [
  {
    id: 'vision',
    label: 'Vision',
    group: 'perception',
    description: 'Provider can send camera snapshots or visual observations to TARS.',
  },
  {
    id: 'hearing',
    label: 'Hearing',
    group: 'perception',
    description: 'Provider can send microphone/audio Percepts to TARS.',
  },
  {
    id: 'speech',
    label: 'Speech',
    group: 'actuation',
    description: 'TARS may route speak actions back to this provider.',
  },
  {
    id: 'expression',
    label: 'Expression',
    group: 'actuation',
    description: 'TARS may route facial/emotion expression actions to this provider.',
  },
  {
    id: 'motion',
    label: 'Motion',
    group: 'actuation',
    description: 'TARS may route head or motion actions to this provider.',
  },
  {
    id: 'led',
    label: 'LED',
    group: 'actuation',
    description: 'TARS may route LED color or status-light actions to this provider.',
  },
]

export type EmbodimentProviderPresetID = 'host' | 'stackchan' | 'custom'

export type EmbodimentProviderPreset = {
  id: EmbodimentProviderPresetID
  title: string
  buttonLabel: string
  description: string
  name: string
  transport: string
  endpoint: string
  capabilities: string[]
  session_id: string
  agent: string
  owner_only_directive: boolean
  salience_min_sound_level: string
  min_trigger_interval: string
  max_triggers_per_hour: string
  trigger_observations: boolean
  tip: string
}

export const EMBODIMENT_PROVIDER_PRESETS: EmbodimentProviderPreset[] = [
  {
    id: 'host',
    title: 'Mac Host',
    buttonLabel: 'Add Mac Host',
    description: 'Mac mic/camera companion with local speech output.',
    name: 'host',
    transport: 'mcp',
    endpoint: 'tars-stackchan-host',
    capabilities: ['hearing', 'speech'],
    session_id: 'sess_main',
    agent: '',
    owner_only_directive: false,
    salience_min_sound_level: '0.6',
    min_trigger_interval: '30s',
    max_triggers_per_hour: '60',
    trigger_observations: true,
    tip: 'Use with the tars-stackchan-host MCP server. Add Vision only when imagesnap or ffmpeg camera capture is available.',
  },
  {
    id: 'stackchan',
    title: 'StackChan',
    buttonLabel: 'Add StackChan',
    description: 'Physical Stack-chan body through the tars-stackchan MCP server.',
    name: 'stackchan',
    transport: 'mcp',
    endpoint: 'tars-stackchan',
    capabilities: ['vision', 'hearing', 'speech', 'expression', 'motion', 'led'],
    session_id: 'sess_main',
    agent: '',
    owner_only_directive: true,
    salience_min_sound_level: '0.6',
    min_trigger_interval: '30s',
    max_triggers_per_hour: '60',
    trigger_observations: false,
    tip: 'Use with the tars-stackchan MCP server. Uncheck Vision for audio-only firmware or when camera capture is disabled.',
  },
  {
    id: 'custom',
    title: 'Custom',
    buttonLabel: 'Add Custom',
    description: 'Start from a generic MCP provider and adjust every field.',
    name: 'provider',
    transport: 'mcp',
    endpoint: 'provider',
    capabilities: ['hearing', 'speech'],
    session_id: 'sess_main',
    agent: '',
    owner_only_directive: true,
    salience_min_sound_level: '0.6',
    min_trigger_interval: '30s',
    max_triggers_per_hour: '60',
    trigger_observations: false,
    tip: 'For MCP transport, Endpoint must match an mcp.servers key. For webhook transport, use the full inbound URL.',
  },
]

// Tier-binding service_tier choices. Provider-level service_tier is no
// longer exposed — the per-tier value flows through to the provider API.
export const LLM_TIER_SERVICE_TIERS = ['', 'auto', 'default', 'flex', 'priority'] as const

const API_KEY_MASK_PATTERNS = [/^\*+$/, /^[*•]+$/, /\*{3,}/]

export function isMaskedAPIKey(value: string): boolean {
  if (!value) return false
  return API_KEY_MASK_PATTERNS.some((pattern) => pattern.test(value))
}

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
    const keys = sortStrings(Object.keys(value as Record<string, unknown>))
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
  return sortStrings(Object.keys(record))
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
  return sortStrings(Object.keys(record).map((alias) => alias.trim()).filter(Boolean))
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

export function makeLLMProviderDrafts(value: unknown): LLMProviderDraft[] {
  const record = asRecord(value)
  if (!record) return []
  return sortStrings(Object.keys(record))
    .map((alias, index) => {
      const provider = asRecord(record[alias]) || {}
      return {
        id: `provider-${index}-${alias}`,
        originalAlias: alias,
        alias,
        kind: readString(provider, 'kind'),
        auth_mode: readString(provider, 'auth_mode', 'authMode'),
        base_url: readString(provider, 'base_url', 'baseURL'),
        api_key: readString(provider, 'api_key', 'apiKey'),
      }
    })
}

export function buildLLMProvidersFromDrafts(drafts: LLMProviderDraft[]): LLMProvidersBuildResult {
  const trimmedAliases = drafts.map((draft) => draft.alias.trim())
  const aliasCounts = new Map<string, number>()
  for (const alias of trimmedAliases) {
    if (!alias) continue
    aliasCounts.set(alias, (aliasCounts.get(alias) || 0) + 1)
  }

  const errors: LLMProviderDraftErrors = {}
  const value: Record<string, LLMProviderSettingsValue> = {}

  drafts.forEach((draft, index) => {
    const rowErrors: Partial<Record<LLMProviderDraftField, string>> = {}
    const id = draft.id || `provider-${index}`
    const alias = draft.alias.trim()
    const kind = draft.kind.trim()
    const authMode = draft.auth_mode.trim()
    const baseURL = draft.base_url.trim()
    const apiKey = draft.api_key

    if (!alias) {
      rowErrors.alias = 'Provider alias is required.'
    } else if ((aliasCounts.get(alias) || 0) > 1) {
      rowErrors.alias = 'Provider alias must be unique.'
    }
    if (!kind) {
      rowErrors.kind = 'Kind is required.'
    }

    if (Object.keys(rowErrors).length > 0) {
      errors[id] = rowErrors
      return
    }

    value[alias] = {
      kind,
      auth_mode: authMode,
      base_url: baseURL,
      api_key: apiKey,
    }
  })

  if (Object.keys(errors).length > 0) {
    return { ok: false, errors }
  }
  return { ok: true, value }
}

export function makeEmbodimentProviderDrafts(value: unknown): EmbodimentProviderDraft[] {
  const providers = Array.isArray(value)
    ? value
    : Object.entries(asRecord(value) || {}).map(([name, provider]) => ({
        ...(asRecord(provider) || {}),
        name: readString(asRecord(provider) || {}, 'name') || name,
      }))
  return providers
    .map((entry, index) => {
      const provider = asRecord(entry) || {}
      return {
        id: `embodiment-provider-${index}-${readString(provider, 'name') || index}`,
        originalName: readString(provider, 'name'),
        name: readString(provider, 'name'),
        enabled: readBool(provider, 'enabled'),
        transport: readString(provider, 'transport'),
        endpoint: readString(provider, 'endpoint'),
        capabilities: readStringList(provider, 'capabilities'),
        session_id: readString(provider, 'session_id', 'sessionID'),
        agent: readString(provider, 'agent'),
        owner_only_directive: readBool(provider, 'owner_only_directive', 'ownerOnlyDirective'),
        salience_min_sound_level: readNumberString(provider, 'salience_min_sound_level', 'salienceMinSoundLevel'),
        min_trigger_interval: readString(provider, 'min_trigger_interval', 'minTriggerInterval'),
        max_triggers_per_hour: readNumberString(provider, 'max_triggers_per_hour', 'maxTriggersPerHour'),
        trigger_observations: readBool(provider, 'trigger_observations', 'triggerObservations'),
      }
    })
}

export function makeEmbodimentProviderPresetDraft(
  presetID: EmbodimentProviderPresetID | string,
  id: string,
  existingNames: string[] = [],
): EmbodimentProviderDraft {
  const preset = EMBODIMENT_PROVIDER_PRESETS.find((item) => item.id === presetID) || EMBODIMENT_PROVIDER_PRESETS[2]
  const name = uniqueEmbodimentProviderName(preset.name, existingNames)
  return {
    id,
    originalName: '',
    name,
    enabled: true,
    transport: preset.transport,
    endpoint: preset.endpoint,
    capabilities: [...preset.capabilities],
    session_id: preset.session_id,
    agent: preset.agent,
    owner_only_directive: preset.owner_only_directive,
    salience_min_sound_level: preset.salience_min_sound_level,
    min_trigger_interval: preset.min_trigger_interval,
    max_triggers_per_hour: preset.max_triggers_per_hour,
    trigger_observations: preset.trigger_observations,
  }
}

export function buildEmbodimentProvidersFromDrafts(drafts: EmbodimentProviderDraft[]): EmbodimentProvidersBuildResult {
  const trimmedNames = drafts.map((draft) => draft.name.trim())
  const nameCounts = new Map<string, number>()
  for (const name of trimmedNames) {
    if (!name) continue
    nameCounts.set(name, (nameCounts.get(name) || 0) + 1)
  }

  const errors: EmbodimentProviderDraftErrors = {}
  const value: EmbodimentProviderSettingsValue[] = []

  drafts.forEach((draft, index) => {
    const rowErrors: Partial<Record<EmbodimentProviderDraftField, string>> = {}
    const id = draft.id || `embodiment-provider-${index}`
    const name = draft.name.trim()
    const transport = draft.transport.trim().toLowerCase()
    const endpoint = draft.endpoint.trim()
    const capabilities = normalizeStringList(draft.capabilities)
    const salienceText = draft.salience_min_sound_level.trim()
    const maxTriggersText = draft.max_triggers_per_hour.trim()

    if (!name) {
      rowErrors.name = 'Provider name is required.'
    } else if ((nameCounts.get(name) || 0) > 1) {
      rowErrors.name = 'Provider name must be unique.'
    }
    if (!transport) {
      rowErrors.transport = 'Transport is required.'
    }
    if (!endpoint) {
      rowErrors.endpoint = 'Endpoint is required.'
    }
    if (capabilities.length === 0) {
      rowErrors.capabilities = 'Choose at least one capability.'
    }

    let salience = 0
    if (salienceText) {
      salience = Number(salienceText)
      if (!Number.isFinite(salience)) {
        rowErrors.salience_min_sound_level = 'Salience threshold must be a number.'
      } else if (salience < 0 || salience > 1) {
        rowErrors.salience_min_sound_level = 'Salience threshold must be between 0 and 1.'
      }
    }

    let maxTriggers = 0
    if (maxTriggersText) {
      if (!/^\d+$/.test(maxTriggersText)) {
        rowErrors.max_triggers_per_hour = 'Max triggers must be 0 or greater.'
      } else {
        maxTriggers = Number(maxTriggersText)
        if (!Number.isSafeInteger(maxTriggers)) {
          rowErrors.max_triggers_per_hour = 'Max triggers must be a safe integer.'
        }
      }
    }

    if (Object.keys(rowErrors).length > 0) {
      errors[id] = rowErrors
      return
    }

    value.push({
      name,
      enabled: !!draft.enabled,
      transport,
      endpoint,
      capabilities,
      session_id: draft.session_id.trim(),
      agent: draft.agent.trim(),
      owner_only_directive: !!draft.owner_only_directive,
      salience_min_sound_level: salience,
      min_trigger_interval: draft.min_trigger_interval.trim(),
      max_triggers_per_hour: maxTriggers,
      trigger_observations: !!draft.trigger_observations,
    })
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
  for (const key of sortStrings(Object.keys(value as Record<string, unknown>))) {
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

function readBool(record: Record<string, unknown>, key: string, fallbackKey?: string): boolean {
  const value = record[key] ?? (fallbackKey ? record[fallbackKey] : undefined)
  return value === true
}

function readStringList(record: Record<string, unknown>, key: string): string[] {
  const value = record[key]
  if (!Array.isArray(value)) return []
  return normalizeStringList(value)
}

function normalizeStringList(values: unknown[]): string[] {
  const out: string[] = []
  const seen = new Set<string>()
  for (const value of values) {
    const normalized = String(value).trim().toLowerCase()
    if (!normalized || seen.has(normalized)) continue
    seen.add(normalized)
    out.push(normalized)
  }
  return out
}

function uniqueEmbodimentProviderName(base: string, existingNames: string[]): string {
  const normalized = new Set(existingNames.map((name) => name.trim()).filter(Boolean))
  const fallback = base.trim() || 'provider'
  if (!normalized.has(fallback)) return fallback
  let index = 2
  let candidate = `${fallback}${index}`
  while (normalized.has(candidate)) {
    index += 1
    candidate = `${fallback}${index}`
  }
  return candidate
}

function readNumberString(record: Record<string, unknown>, key: string, fallbackKey?: string): string {
  const value = record[key] ?? (fallbackKey ? record[fallbackKey] : undefined)
  if (value === undefined || value === null || value === '') return ''
  return String(value)
}

function readBudgetString(record: Record<string, unknown>): string {
  const value = record.thinking_budget ?? record.thinkingBudget
  if (value === undefined || value === null || value === '') return ''
  return String(value)
}
