import type { ConfigFieldMeta } from './types'

export const quickStartFieldKeys = [
  'api_auth_mode',
  'llm_providers',
  'llm_tiers',
  'llm_default_tier',
  'workspace_dir',
  'telegram_bot_token',
  'embodiment_enabled',
  'embodiment_providers_json',
  'pulse_enabled',
  'reflection_enabled',
  'log_level',
  'session_telegram_scope',
] as const

export type QuickStartFieldKey = typeof quickStartFieldKeys[number]

export type QuickStartStatusKind = 'ready' | 'attention' | 'optional'

export type QuickStartStatus = {
  kind: QuickStartStatusKind
  label: string
  message: string
}

export type QuickStartDefinition = {
  key: QuickStartFieldKey
  title: string
  description: string
  required: boolean
}

export type QuickStartItem = QuickStartDefinition & {
  field: ConfigFieldMeta
  value: unknown
  dirty: boolean
  status: QuickStartStatus
}

export const quickStartDefinitions: QuickStartDefinition[] = [
  {
    key: 'api_auth_mode',
    title: 'API auth mode',
    description: 'Local-only setups can stay off; exposed servers should require bearer auth.',
    required: true,
  },
  {
    key: 'llm_providers',
    title: 'LLM provider',
    description: 'Configure at least one provider credential or OAuth-backed provider.',
    required: true,
  },
  {
    key: 'llm_tiers',
    title: 'LLM tiers',
    description: 'Bind heavy, standard, and light tiers to provider/model pairs.',
    required: true,
  },
  {
    key: 'llm_default_tier',
    title: 'Default tier',
    description: 'Fallback tier used when a role has no explicit override.',
    required: true,
  },
  {
    key: 'workspace_dir',
    title: 'Workspace directory',
    description: 'Location for sessions, logs, memory, and runtime state.',
    required: true,
  },
  {
    key: 'telegram_bot_token',
    title: 'Telegram bot token',
    description: 'Optional bot token for Telegram channel access and notifications.',
    required: false,
  },
  {
    key: 'embodiment_enabled',
    title: 'Embodied Bot',
    description: 'Enable perception and body-provider routing for Stack-chan or host devices.',
    required: true,
  },
  {
    key: 'embodiment_providers_json',
    title: 'Embodied Bot providers',
    description: 'Declare Stack-chan or host body providers and their capabilities.',
    required: false,
  },
  {
    key: 'pulse_enabled',
    title: 'Pulse watchdog',
    description: 'Enable periodic workspace health checks.',
    required: true,
  },
  {
    key: 'reflection_enabled',
    title: 'Nightly reflection',
    description: 'Enable the nightly maintenance and memory extraction batch.',
    required: true,
  },
  {
    key: 'log_level',
    title: 'Log level',
    description: 'Choose normal or debug logging verbosity.',
    required: true,
  },
  {
    key: 'session_telegram_scope',
    title: 'Telegram session scope',
    description: 'Choose how Telegram messages map onto chat sessions.',
    required: true,
  },
]

export function buildQuickStartItems(
  schema: ConfigFieldMeta[],
  values: Record<string, unknown>,
  dirtyFields: Record<string, unknown>,
): QuickStartItem[] {
  const fieldsByKey = new Map(schema.map((field) => [field.key, field]))
  return quickStartDefinitions.flatMap((definition) => {
    const field = fieldsByKey.get(definition.key)
    if (!field) return []
    const value = Object.prototype.hasOwnProperty.call(dirtyFields, field.key) ? dirtyFields[field.key] : values[field.key]
    return [{
      ...definition,
      field,
      value,
      dirty: Object.prototype.hasOwnProperty.call(dirtyFields, field.key),
      status: validateQuickStartValue(definition, value),
    }]
  })
}

export function quickStartProgress(items: QuickStartItem[]): { ready: number; total: number } {
  const required = items.filter((item) => item.required)
  return {
    ready: required.filter((item) => item.status.kind === 'ready').length,
    total: required.length,
  }
}

function validateQuickStartValue(definition: QuickStartDefinition, value: unknown): QuickStartStatus {
  if (!definition.required && !hasValue(value)) {
    return { kind: 'optional', label: 'optional', message: 'Optional; leave empty unless this channel is enabled.' }
  }

  switch (definition.key) {
    case 'llm_providers':
      return validateLLMProviders(value)
    case 'llm_tiers':
      return validateLLMTiers(value)
    case 'embodiment_providers_json':
      return validateEmbodimentProviders(value)
    case 'workspace_dir':
      return stringReady(value, 'workspace path configured', 'Set a workspace directory before saving runtime data.')
    case 'telegram_bot_token':
      return hasValue(value)
        ? { kind: 'ready', label: 'set', message: 'Telegram token is configured.' }
        : { kind: 'optional', label: 'optional', message: 'Optional unless Telegram channels or notifications are used.' }
    default:
      return hasValue(value)
        ? { kind: 'ready', label: 'ready', message: 'Configured.' }
        : { kind: 'attention', label: 'needs value', message: 'Set a value before first run.' }
  }
}

function validateEmbodimentProviders(value: unknown): QuickStartStatus {
  if (!Array.isArray(value)) {
    return { kind: 'attention', label: 'invalid', message: 'Provider descriptors must be a list.' }
  }
  const missingName = value.some((entry) => {
    const provider = asRecord(entry)
    return !hasValue(provider?.name)
  })
  if (missingName) {
    return { kind: 'attention', label: 'incomplete', message: 'Each body provider needs a name.' }
  }
  return { kind: 'ready', label: 'ready', message: `${value.length} body provider${value.length === 1 ? '' : 's'} configured.` }
}

function validateLLMProviders(value: unknown): QuickStartStatus {
  const providers = asRecord(value)
  if (!providers || Object.keys(providers).length === 0) {
    return { kind: 'attention', label: 'needs provider', message: 'Add at least one LLM provider.' }
  }
  const hasCredential = Object.values(providers).some((entry) => {
    const provider = asRecord(entry)
    if (!provider) return false
    return hasValue(provider.api_key) || String(provider.auth_mode || '').toLowerCase() === 'oauth'
  })
  if (!hasCredential) {
    return { kind: 'attention', label: 'needs key', message: 'Add an API key or OAuth auth mode for one provider.' }
  }
  return { kind: 'ready', label: 'ready', message: 'Provider credentials are configured.' }
}

function validateLLMTiers(value: unknown): QuickStartStatus {
  const tiers = asRecord(value)
  const missing = ['heavy', 'standard', 'light'].filter((tier) => !tiers?.[tier])
  if (missing.length > 0) {
    return { kind: 'attention', label: 'incomplete', message: `Missing ${missing.join(', ')} tier bindings.` }
  }
  return { kind: 'ready', label: 'ready', message: 'Heavy, standard, and light tiers are present.' }
}

function stringReady(value: unknown, ready: string, attention: string): QuickStartStatus {
  return hasValue(value)
    ? { kind: 'ready', label: 'ready', message: ready }
    : { kind: 'attention', label: 'needs value', message: attention }
}

function hasValue(value: unknown): boolean {
  if (typeof value === 'string') return value.trim().length > 0
  if (typeof value === 'boolean') return true
  if (Array.isArray(value)) return value.length > 0
  if (value && typeof value === 'object') return Object.keys(value as Record<string, unknown>).length > 0
  return value !== null && value !== undefined
}

function asRecord(value: unknown): Record<string, unknown> | null {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return null
  return value as Record<string, unknown>
}
