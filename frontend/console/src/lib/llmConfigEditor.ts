// Settings → LLM tab: row-based editing for llm_providers / llm_tiers /
// llm_role_defaults, the three `type: "json"` config fields that otherwise
// require hand-editing JSON in a textarea.
//
// These functions are pure: they convert the raw config values (as
// returned by GET /v1/admin/config/schema) into editable row lists, and
// convert edited rows back into the map shapes PATCH /v1/admin/config/values
// expects. ConfigLLMEditor.svelte owns all DOM/state wiring; this module
// owns the data shape and validation rules, so both are testable headless.
//
// Server-side constraints these functions mirror (do not drift from them
// without updating internal/config too):
//   - internal/config/patch_raw_providers.go's rawProviderObjects only
//     accepts "kind", "auth_mode", "base_url", "api_key" per provider —
//     no passthrough fields are possible for llm_providers.
//   - internal/apihandlers/config.go's handleGetConfigSchema masks every
//     non-empty llm_providers api_key unconditionally (maskSensitiveConfigValues
//     -> maskString: first 4 chars + asterisks + last 4), the same as the
//     onboarding wizard's re-entry path — GET /schema never returns a real
//     secret back. A loaded row never shows or resubmits that masked text;
//     it tracks keepExistingApiKey (mirrors onboarding.ts's field of the
//     same name) and omits api_key from the built value while it is true,
//     which internal/config/patch_raw_providers.go's mergeRawProviderEdit
//     reads as "leave the on-disk secret alone".
//   - pkg/llm/tier.go's AllTiers() fixes tier names to exactly heavy /
//     standard / light — llm_tiers always has exactly these three rows,
//     never a variable-length list.
//   - internal/config/types.go's LLMTierBinding carries fields this editor
//     does not expose (thinking_budget, service_tier, beta_features,
//     context_window). The patch replaces each tier's object wholesale
//     (onboarding.ts's buildConfigPayload comment documents the same
//     constraint), so those fields are round-tripped through row.extra
//     rather than dropped.
//   - pkg/llm/role.go's AllRoles() is mirrored in llmKnownRoles below for
//     the role select's suggestion list; llm_role_defaults's value is a
//     free string key, so an unrecognized role here still gets a (soft)
//     "unknown role" error locally while a role this list doesn't know yet
//     is caught by the server's own validation rather than silently
//     dropped.

import { sortStrings } from './sort.js'
import { availableAuthModesForKind, suggestedAuthModeForKind, type AuthMode } from './onboarding.ts'

// LLM provider kinds accepted by internal/config/patch_validation.go's
// validateCandidate switch. Mirrors onboarding.ts's ProviderKind (kept as a
// separate type here so this module has no import-time dependency on the
// wizard beyond the AuthMode alias).
export type LLMProviderKind =
  | 'anthropic'
  | 'openai'
  | 'openai-codex'
  | 'gemini'
  | 'gemini-native'
  | 'kimi'
  | 'claude-code-cli'
  | 'antigravity-cli'

export const llmProviderKinds: LLMProviderKind[] = [
  'anthropic',
  'openai',
  'openai-codex',
  'gemini',
  'gemini-native',
  'kimi',
  'claude-code-cli',
  'antigravity-cli',
]

// Tier names are fixed by pkg/llm/tier.go's AllTiers() — llm_tiers is
// always exactly these three entries, never a user-chosen list.
export const llmTierNames = ['heavy', 'standard', 'light'] as const
export type LLMTierName = (typeof llmTierNames)[number]

// Mirrors pkg/llm/role.go's AllRoles() canonical order. Used to populate
// the role select's suggestions and to flag an unrecognized role locally;
// kept as a plain string list (not a union type) because the server is the
// source of truth and a role added there before this list catches up must
// still be enterable as free text.
export const llmKnownRoles = [
  'chat_main',
  'context_compactor',
  'memory_hook',
  'reflection_memory',
  'reflection_kb',
  'pulse_decider',
  'session_cleanup',
  'agentruntime_default',
  'agentruntime_planner',
  'goal_judge',
  'critic',
  'computer_use',
  'initiative',
  'initiative_speak',
]

export type LLMProviderRow = {
  id: string
  alias: string
  previousAlias: string
  kind: LLMProviderKind | ''
  auth_mode: AuthMode
  api_key: string
  // keepExistingApiKey mirrors onboarding.ts's field of the same name: true
  // when this row loaded with a non-empty (always masked, per the module
  // header) api_key and the user has not typed a replacement. While true,
  // buildLLMProvidersValue omits api_key from the built entry so the
  // server's own "absent key = keep the old secret" rule applies — the
  // masked text itself is never shown in the control or submitted anywhere.
  // Typing any character flips this to false (ConfigLLMEditor.svelte).
  keepExistingApiKey: boolean
  base_url: string
}

export type LLMTierRow = {
  name: LLMTierName
  provider: string
  model: string
  reasoning_effort: string
  max_tokens: string
  // Fields this editor does not expose (thinking_budget, service_tier,
  // beta_features, context_window, and anything future) — carried through
  // untouched because the server replaces the tier object wholesale.
  extra: Record<string, unknown>
}

export type LLMRoleRow = {
  id: string
  role: string
  previousRole: string
  tier: string
}

let rowIdCounter = 0
function nextRowId(prefix: string): string {
  rowIdCounter += 1
  return `${prefix}-${rowIdCounter}`
}

// --- values -> rows -----------------------------------------------------

export function llmProviderRowsFromValue(value: unknown): LLMProviderRow[] {
  const providers = (value && typeof value === 'object' ? value : {}) as Record<string, unknown>
  return sortStrings(Object.keys(providers)).map((alias) => providerRowFromEntry(alias, providers[alias]))
}

function providerRowFromEntry(alias: string, entryRaw: unknown): LLMProviderRow {
  const entry = (entryRaw && typeof entryRaw === 'object' ? entryRaw : {}) as Record<string, unknown>
  const apiKey = typeof entry.api_key === 'string' ? entry.api_key : ''
  const kind = (typeof entry.kind === 'string' ? entry.kind : '') as LLMProviderKind | ''
  // A raw auth_mode is only trusted when it is actually one of the modes
  // this kind offers (availableAuthModesForKind) — a value loaded from a
  // hand-edited "Edit as JSON" paste can carry a stale or missing
  // auth_mode (e.g. omitted for a claude-code-cli/antigravity-cli entry),
  // which would otherwise default to 'api-key' even though that kind only
  // ever offers 'cli'. Falling back to suggestedAuthModeForKind keeps the
  // row's value in sync with the Auth mode <select>'s own options, so the
  // control never shows one mode while api-key validation checks another
  // (tars#1286 review).
  const rawAuthMode = typeof entry.auth_mode === 'string' ? entry.auth_mode.trim() : ''
  const validAuthModes = availableAuthModesForKind(kind)
  const authMode = (rawAuthMode && validAuthModes.includes(rawAuthMode as AuthMode) ? rawAuthMode : suggestedAuthModeForKind(kind)) as AuthMode
  return {
    id: nextRowId('provider'),
    alias,
    previousAlias: alias,
    kind,
    auth_mode: authMode,
    api_key: apiKey,
    keepExistingApiKey: apiKey.trim() !== '',
    base_url: typeof entry.base_url === 'string' ? entry.base_url : '',
  }
}

export function createLLMProviderRow(): LLMProviderRow {
  return {
    id: nextRowId('provider'),
    alias: '',
    previousAlias: '',
    kind: '',
    auth_mode: 'api-key',
    api_key: '',
    keepExistingApiKey: false,
    base_url: '',
  }
}

// Always returns exactly heavy/standard/light, in that order, regardless
// of what is present on disk — AllTiers() allows no other names.
export function llmTierRowsFromValue(value: unknown): LLMTierRow[] {
  const tiers = (value && typeof value === 'object' ? value : {}) as Record<string, unknown>
  return llmTierNames.map((name) => tierRowFromEntry(name, tiers[name]))
}

function tierRowFromEntry(name: LLMTierName, entryRaw: unknown): LLMTierRow {
  const entry = (entryRaw && typeof entryRaw === 'object' ? entryRaw : {}) as Record<string, unknown>
  const extra: Record<string, unknown> = {}
  for (const [key, val] of Object.entries(entry)) {
    if (key === 'provider' || key === 'model' || key === 'reasoning_effort' || key === 'max_tokens') continue
    extra[key] = val
  }
  const maxTokens = entry.max_tokens
  return {
    name,
    provider: typeof entry.provider === 'string' ? entry.provider : '',
    model: typeof entry.model === 'string' ? entry.model : '',
    reasoning_effort: typeof entry.reasoning_effort === 'string' ? entry.reasoning_effort : '',
    max_tokens:
      typeof maxTokens === 'number' && maxTokens > 0
        ? String(maxTokens)
        : typeof maxTokens === 'string' && maxTokens.trim() !== ''
          ? maxTokens.trim()
          : '',
    extra,
  }
}

export function llmRoleRowsFromValue(value: unknown): LLMRoleRow[] {
  const roles = (value && typeof value === 'object' ? value : {}) as Record<string, unknown>
  return sortStrings(Object.keys(roles)).map((role) => ({
    id: nextRowId('role'),
    role,
    previousRole: role,
    tier: typeof roles[role] === 'string' ? (roles[role] as string) : '',
  }))
}

export function createLLMRoleRow(): LLMRoleRow {
  return { id: nextRowId('role'), role: '', previousRole: '', tier: '' }
}

// --- rows -> values + validation ----------------------------------------

export type LLMBuildResult = { value: Record<string, unknown>; errors: string[] }

const ALIAS_PATTERN = /^[a-z0-9][a-z0-9_-]*$/i

export function buildLLMProvidersValue(rows: LLMProviderRow[]): LLMBuildResult {
  const errors: string[] = []
  const seen = new Set<string>()
  const value: Record<string, unknown> = {}
  rows.forEach((row, index) => {
    const alias = row.alias.trim()
    const label = alias || `#${index + 1}`
    if (alias === '') {
      errors.push(`Provider ${label}: alias is required.`)
      return
    }
    if (!ALIAS_PATTERN.test(alias)) {
      errors.push(`Provider "${alias}": alias must be alphanumeric (with optional - or _).`)
      return
    }
    if (seen.has(alias)) {
      errors.push(`Provider "${alias}": alias is used more than once.`)
      return
    }
    seen.add(alias)
    if (row.kind === '') {
      errors.push(`Provider "${alias}": pick a provider kind.`)
      return
    }
    if (!llmProviderKinds.includes(row.kind as LLMProviderKind)) {
      errors.push(`Provider "${alias}": unknown provider kind "${row.kind}".`)
      return
    }
    if (row.auth_mode === 'api-key' && !row.keepExistingApiKey && row.api_key.trim() === '') {
      errors.push(`Provider "${alias}": API key is required for api-key auth mode.`)
      return
    }
    const entry: Record<string, unknown> = {
      kind: row.kind,
      auth_mode: row.auth_mode || 'api-key',
    }
    // keepExistingApiKey true means the row still shows the masked value's
    // blank placeholder, untouched — omit api_key so
    // internal/config/patch_raw_providers.go's mergeRawProviderEdit reads
    // the missing field as "leave the on-disk secret alone" (see module
    // header). A never-set key (a fresh row left on oauth/cli) is also
    // omitted rather than sent as an explicit empty string — only a typed
    // replacement is ever submitted, mirroring onboarding.ts's
    // buildConfigPayload exactly. There is no UI path to explicitly clear
    // an existing key to empty, the same limitation the onboarding wizard
    // already has.
    if (!row.keepExistingApiKey && row.api_key.trim() !== '') {
      entry.api_key = row.api_key.trim()
    }
    if (row.base_url.trim() !== '') entry.base_url = row.base_url.trim()
    value[alias] = entry
  })
  return { value, errors }
}

export function buildLLMTiersValue(rows: LLMTierRow[], knownProviderAliases: ReadonlySet<string>): LLMBuildResult {
  const errors: string[] = []
  const value: Record<string, unknown> = {}
  for (const row of rows) {
    const provider = row.provider.trim()
    const model = row.model.trim()
    if (provider === '') {
      errors.push(`${row.name}: provider is required.`)
    } else if (!knownProviderAliases.has(provider)) {
      errors.push(`${row.name}: provider "${provider}" is not one of the providers above.`)
    }
    if (model === '') {
      errors.push(`${row.name}: model is required.`)
    }
    // reasoning_effort/max_tokens are always present (not deleted when
    // blank/zero): internal/config's LLMTierBinding has no `omitempty` json
    // tags, so GET /schema always returns every tier field, zero values
    // included. Deleting these keys here instead of zeroing them would
    // make an *untouched* row's rebuilt value structurally different from
    // the one it loaded from (extra keys gained/lost), which would read as
    // "changed" even though nothing was edited.
    const entry: Record<string, unknown> = {
      ...row.extra,
      provider,
      model,
      reasoning_effort: row.reasoning_effort.trim(),
    }
    const maxTokensText = row.max_tokens.trim()
    let maxTokens = 0
    if (maxTokensText !== '') {
      const n = Number(maxTokensText)
      if (!Number.isFinite(n) || !Number.isInteger(n) || n <= 0) {
        errors.push(`${row.name}: max tokens must be a positive integer.`)
      } else {
        maxTokens = n
      }
    }
    entry.max_tokens = maxTokens
    value[row.name] = entry
  }
  return { value, errors }
}

export function buildLLMRoleDefaultsValue(rows: LLMRoleRow[]): LLMBuildResult {
  const errors: string[] = []
  const seen = new Set<string>()
  const value: Record<string, unknown> = {}
  for (const row of rows) {
    const role = row.role.trim().toLowerCase()
    const tier = row.tier.trim().toLowerCase()
    if (role === '') {
      errors.push('Role default: role is required.')
      continue
    }
    if (seen.has(role)) {
      errors.push(`Role default "${role}": role is set more than once.`)
      continue
    }
    seen.add(role)
    if (!llmKnownRoles.includes(role)) {
      errors.push(`Role default "${role}": unrecognized role.`)
    }
    if (tier === '') {
      errors.push(`Role default "${role}": pick a tier.`)
      continue
    }
    if (!llmTierNames.includes(tier as LLMTierName)) {
      errors.push(`Role default "${role}": unknown tier "${tier}".`)
      continue
    }
    value[role] = tier
  }
  return { value, errors }
}

// --- cross-field helpers -------------------------------------------------

export type LLMEditorState = {
  providers: LLMProviderRow[]
  tiers: LLMTierRow[]
  roles: LLMRoleRow[]
}

export function llmEditorStateFromValues(
  providersValue: unknown,
  tiersValue: unknown,
  roleDefaultsValue: unknown,
): LLMEditorState {
  return {
    providers: llmProviderRowsFromValue(providersValue),
    tiers: llmTierRowsFromValue(tiersValue),
    roles: llmRoleRowsFromValue(roleDefaultsValue),
  }
}

export type LLMEditorBuildResult = {
  providers: LLMBuildResult
  tiers: LLMBuildResult
  roles: LLMBuildResult
}

// buildLLMEditorValues runs all three builders together so the tier
// builder can validate its provider references against the *edited*
// provider alias set, not the one the page loaded with — but only while
// that edited set is itself free of errors. When providers currently has
// an error (anywhere — not necessarily on the alias a tier points at),
// llm_providers will not be part of this save (ConfigLLMEditor.svelte only
// emits a section once it is error-free), so the providers map that
// actually ends up on disk is whatever was last known good, not the
// in-progress rows. trustedProviderAliases is that fallback set; a tier
// referencing an alias that only exists in the broken, unsaved edit (e.g.
// a rename typed moments before fixing an unrelated row) must still read
// as an error, or the tier could emit on its own and reach the server
// pointing at an alias that was never actually written (tars#1286 review).
// Callers that have no such baseline (every existing test, and a
// first-mount build) omit the argument, which reproduces the previous
// always-use-the-fresh-build behavior exactly.
export function buildLLMEditorValues(
  state: LLMEditorState,
  trustedProviderAliases?: ReadonlySet<string>,
): LLMEditorBuildResult {
  const providers = buildLLMProvidersValue(state.providers)
  const knownAliases = providers.errors.length === 0
    ? new Set(Object.keys(providers.value))
    : (trustedProviderAliases ?? new Set(Object.keys(providers.value)))
  const tiers = buildLLMTiersValue(state.tiers, knownAliases)
  const roles = buildLLMRoleDefaultsValue(state.roles)
  return { providers, tiers, roles }
}

// renameLLMProviderAlias keeps tier rows referencing the renamed alias in
// sync, mirroring onboarding.ts's propagateAliasToTiers. Returns a new
// array; the caller is responsible for reassigning state (Svelte reactivity
// requires a new reference).
export function renameLLMProviderAlias(tierRows: LLMTierRow[], previousAlias: string, nextAlias: string): LLMTierRow[] {
  const prev = previousAlias.trim()
  const next = nextAlias.trim()
  if (prev === '' || prev === next) return tierRows
  return tierRows.map((row) => (row.provider.trim() === prev ? { ...row, provider: next } : row))
}

// providerRowsToDraftValue seeds the "Edit as JSON" textarea. Unlike
// buildLLMProvidersValue it is lossless and never validates: it must not
// omit api_key for a keepExistingApiKey row (that field still holds the
// masked text the row loaded with — see the LLMProviderRow doc comment —
// so including it verbatim round-trips back through
// llmProviderRowsFromValue to the same keepExistingApiKey: true state;
// omitting it, as buildLLMProvidersValue correctly does for the real
// patch, would instead make every untouched row re-parse with an empty
// api_key and keepExistingApiKey: false, turning every existing api-key
// provider into one validation reports as missing its key, with no way
// back to "keep existing" short of reloading the page). It also must not
// drop a row with a validation error (an in-progress edit should still be
// visible as JSON, not silently vanish when switching views) — tars#1286
// review.
export function providerRowsToDraftValue(rows: LLMProviderRow[]): Record<string, unknown> {
  const value: Record<string, unknown> = {}
  for (const row of rows) {
    value[row.alias] = {
      kind: row.kind,
      auth_mode: row.auth_mode,
      api_key: row.api_key,
      base_url: row.base_url,
    }
  }
  return value
}
