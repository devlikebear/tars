// Onboarding wizard state + helpers.
//
// The Phase 3 SPA collects a single LLM provider + 3 tier bindings
// (heavy / standard / light) and POSTs them to the existing
// /v1/admin/config/values endpoint. Validation and payload shaping
// live here so the wizard component can stay declarative.

export type ProviderKind =
  | 'anthropic'
  | 'openai'
  | 'openai-codex'
  | 'gemini'
  | 'gemini-native'
  | 'kimi'
  | 'claude-code-cli'

export const providerKinds: ProviderKind[] = [
  'anthropic',
  'openai',
  'openai-codex',
  'gemini',
  'gemini-native',
  'kimi',
  'claude-code-cli',
]

export type AuthMode = 'api-key' | 'oauth' | 'cli'

export type OnboardingProvider = {
  alias: string
  kind: ProviderKind | ''
  auth_mode: AuthMode
  api_key: string
  base_url: string
  // keepExistingApiKey is true on the re-entry path when the wizard
  // has prefilled a masked api_key from the server. Saving with this
  // flag set drops api_key from the patch payload so the existing
  // value on disk is preserved.
  keepExistingApiKey?: boolean
  // previousAlias is the alias the wizard loaded into this provider
  // form on entry / when switching edit targets. Used by save to drop
  // the renamed entry from the on-disk provider map and to propagate
  // the rename to tier bindings that referenced the old alias.
  previousAlias?: string
}

export type OnboardingTierBinding = {
  provider: string
  model: string
  reasoning_effort?: string
}

// OnboardingTools is the wizard-side state for the Tools & Permissions
// section. Only the fields the wizard exposes — the broader ToolConfig
// surface stays out of the wizard.
//
// keepWebSearchKey mirrors keepExistingApiKey on the provider form: when
// true the wizard treats the loaded api_key as a masked placeholder and
// will NOT include it in the section's PATCH payload, preserving the
// on-disk value.
export type OnboardingTools = {
  web_search_enabled: boolean
  web_search_provider: string
  web_search_api_key: string
  keepWebSearchKey: boolean
  web_fetch_enabled: boolean
  web_fetch_allow_private_hosts: boolean
  web_fetch_private_host_allowlist: string[]
  allow_high_risk_user: boolean
}

export type OnboardingIntegrations = {
  memory_embed_provider: string
  memory_embed_api_key: string
  keepMemoryEmbedKey: boolean
  memory_embed_model: string
  memory_embed_base_url: string
  // null distinguishes "not set" from 0 so the wizard can omit the
  // dimensions key entirely from the patch (defaulting to the provider
  // baseline) rather than asserting dimensions=0.
  memory_embed_dimensions: number | null
}

export type OnboardingChannels = {
  telegram_enabled: boolean
  telegram_bot_token: string
  keepTelegramToken: boolean
  telegram_polling_enabled: boolean
  webhook_enabled: boolean
}

export type OnboardingFormState = {
  provider: OnboardingProvider
  tiers: {
    heavy: OnboardingTierBinding
    standard: OnboardingTierBinding
    light: OnboardingTierBinding
  }
  tools: OnboardingTools
  integrations: OnboardingIntegrations
  channels: OnboardingChannels
}

// optionalSections lists the wizard sections that are skippable in
// Quick mode and saved one-at-a-time via buildSectionPayload.
export const optionalSections = ['tools', 'integrations', 'channels'] as const
export type OptionalSection = (typeof optionalSections)[number]

// WizardMode controls which sections the wizard walks through. Quick
// limits the flow to provider+tiers (LLM only); Full adds the optional
// sections after tiers.
export type WizardMode = 'quick' | 'full'

export const requiredTiers = ['heavy', 'standard', 'light'] as const
export type RequiredTier = (typeof requiredTiers)[number]

export function emptyOnboardingForm(): OnboardingFormState {
  return {
    provider: {
      alias: '',
      kind: '',
      auth_mode: 'api-key',
      api_key: '',
      base_url: '',
    },
    tiers: {
      heavy: { provider: '', model: '' },
      standard: { provider: '', model: '' },
      light: { provider: '', model: '' },
    },
    tools: emptyOnboardingTools(),
    integrations: emptyOnboardingIntegrations(),
    channels: emptyOnboardingChannels(),
  }
}

export function emptyOnboardingTools(): OnboardingTools {
  return {
    web_search_enabled: false,
    web_search_provider: '',
    web_search_api_key: '',
    keepWebSearchKey: false,
    web_fetch_enabled: false,
    web_fetch_allow_private_hosts: false,
    web_fetch_private_host_allowlist: [],
    allow_high_risk_user: false,
  }
}

export function emptyOnboardingIntegrations(): OnboardingIntegrations {
  return {
    memory_embed_provider: '',
    memory_embed_api_key: '',
    keepMemoryEmbedKey: false,
    memory_embed_model: '',
    memory_embed_base_url: '',
    memory_embed_dimensions: null,
  }
}

export function emptyOnboardingChannels(): OnboardingChannels {
  return {
    telegram_enabled: false,
    telegram_bot_token: '',
    keepTelegramToken: false,
    telegram_polling_enabled: false,
    webhook_enabled: false,
  }
}

// validateProviderStep returns user-facing error messages for the
// provider step. Empty array means the step is ready to advance.
//
// api_key is required only when auth_mode is "api-key" — oauth flows
// inherit credentials from the launcher (e.g. claude-code-cli), so an
// empty key is legal.
export function validateProviderStep(form: OnboardingFormState): string[] {
  const errs: string[] = []
  const alias = form.provider.alias.trim()
  if (alias === '') {
    errs.push('Provider alias is required.')
  } else if (!/^[a-z0-9][a-z0-9_-]*$/i.test(alias)) {
    errs.push('Provider alias must be alphanumeric (with optional - or _).')
  }
  if (form.provider.kind === '') {
    errs.push('Pick a provider kind.')
  }
  if (form.provider.auth_mode === 'api-key' && !form.provider.keepExistingApiKey && form.provider.api_key.trim() === '') {
    errs.push('API key is required for api-key auth mode.')
  }
  return errs
}

// validateTiersStep returns user-facing error messages for the tier
// bindings step. All three tiers must point at a non-empty provider
// alias and a non-empty model. The provider alias must match one of
// the aliases known to the wizard (step-1 alias + any existing providers).
export function validateTiersStep(form: OnboardingFormState, allKnownAliases?: string[]): string[] {
  const errs: string[] = []
  const knownAliases = new Set<string>()
  if (form.provider.alias.trim() !== '') {
    knownAliases.add(form.provider.alias.trim())
  }
  if (allKnownAliases) {
    for (const a of allKnownAliases) {
      const trimmed = a.trim()
      if (trimmed !== '') knownAliases.add(trimmed)
    }
  }
  for (const tier of requiredTiers) {
    const binding = form.tiers[tier]
    const provider = binding.provider.trim()
    const model = binding.model.trim()
    if (provider === '') {
      errs.push(`${tier}: provider is required.`)
    } else if (knownAliases.size > 0 && !knownAliases.has(provider)) {
      errs.push(`${tier}: provider "${provider}" is not configured in this wizard.`)
    }
    if (model === '') {
      errs.push(`${tier}: model is required.`)
    }
  }
  return errs
}

// validateForm returns the union of provider + tier errors. The
// review step uses this to gate the Save button.
export function validateForm(form: OnboardingFormState, allKnownAliases?: string[]): string[] {
  return [...validateProviderStep(form), ...validateTiersStep(form, allKnownAliases)]
}

// buildConfigPayload converts the form into the `updates` map shape
// that PATCH /v1/admin/config/values expects. Empty optional fields
// are omitted so the resulting YAML stays clean.
//
// existingProviders is the full provider map already on disk (e.g.
// values.llm_providers from the schema endpoint). The wizard edits a
// single alias at a time, but the backend's alias-keyed PATCH replaces
// the on-disk alias set with what the patch sends — so we must include
// every existing alias here, then overlay the wizard's currently
// edited alias on top. previousAlias (when the user renamed) is
// removed so the rename actually takes effect on disk.
export function buildConfigPayload(
  form: OnboardingFormState,
  existingProviders: Record<string, unknown> = {},
): Record<string, unknown> {
  const provider: Record<string, unknown> = {
    kind: form.provider.kind,
    auth_mode: form.provider.auth_mode,
  }
  // Skip api_key when the user opted to keep the existing value on
  // disk (re-entry path with masked prefill). PatchYAML preserves the
  // original key when the field is absent from the updates map.
  if (!form.provider.keepExistingApiKey && form.provider.api_key.trim() !== '') {
    provider.api_key = form.provider.api_key.trim()
  }
  if (form.provider.base_url.trim() !== '') {
    provider.base_url = form.provider.base_url.trim()
  }

  const tiers: Record<string, Record<string, unknown>> = {}
  for (const tier of requiredTiers) {
    const binding = form.tiers[tier]
    const entry: Record<string, unknown> = {
      provider: binding.provider.trim(),
      model: binding.model.trim(),
    }
    if (binding.reasoning_effort && binding.reasoning_effort.trim() !== '') {
      entry.reasoning_effort = binding.reasoning_effort.trim()
    }
    tiers[tier] = entry
  }

  const alias = form.provider.alias.trim()
  const previousAlias = (form.provider.previousAlias || '').trim()
  const providers: Record<string, unknown> = {}
  for (const [key, value] of Object.entries(existingProviders)) {
    if (key === previousAlias && previousAlias !== alias) continue
    providers[key] = value
  }
  if (alias) providers[alias] = provider

  return {
    llm_providers: providers,
    llm_tiers: tiers,
  }
}

// propagateAliasToTiers rewrites tier bindings so the wizard's tier
// step stays in sync with step 1's alias. Tiers that referenced the
// previous alias are rebound to the new one; tiers with an empty
// provider are filled with the new alias. Tiers already pointing at a
// different existing alias are left alone (the user may be
// intentionally mixing providers across tiers).
export function propagateAliasToTiers(
  form: OnboardingFormState,
  previousAlias: string,
  nextAlias: string,
): void {
  const prev = previousAlias.trim()
  const next = nextAlias.trim()
  if (next === '') return
  for (const tier of requiredTiers) {
    const current = form.tiers[tier].provider.trim()
    if (current === '' || (prev !== '' && current === prev)) {
      form.tiers[tier].provider = next
    }
  }
}

// resetTierModelsForKindChange clears the model field of every tier
// binding when the wizard's provider kind changes. Models are
// kind-specific (e.g. gpt-5.4 makes no sense once kind flips to
// anthropic) so any previous value is almost certainly invalid; the
// user must re-pick from the per-kind suggestion list.
export function resetTierModelsForKindChange(form: OnboardingFormState): void {
  for (const tier of requiredTiers) {
    form.tiers[tier].model = ''
  }
}

// defaultBaseURLForKind returns the canonical base URL for a provider
// kind so the wizard can prefill base_url when the user picks a kind.
// Returning empty string means "no canonical default" — the user must
// fill it in manually (claude-code-cli is local, openai-codex is
// resolved by oauth handshake, etc.).
export function defaultBaseURLForKind(kind: ProviderKind | ''): string {
  switch (kind) {
    case 'anthropic':
      return 'https://api.anthropic.com'
    case 'openai':
      return 'https://api.openai.com/v1'
    case 'gemini':
      // Must match backend llmdefaults.GeminiBaseURL — the OpenAI-compat
      // surface lives at /v1beta/openai. Without the path, every chat
      // turn 404s with an empty body.
      return 'https://generativelanguage.googleapis.com/v1beta/openai'
    case 'gemini-native':
      // Must match backend llmdefaults.GeminiNativeBaseURL.
      return 'https://generativelanguage.googleapis.com/v1beta'
    case 'kimi':
      return 'https://api.moonshot.ai/v1'
    case 'openai-codex':
      return 'https://chatgpt.com/backend-api'
    case 'claude-code-cli':
      return ''
    default:
      return ''
  }
}

// availableAuthModesForKind returns the closed set of auth_mode
// values that are valid for a given provider kind. Mirrors the
// per-kind defaults in internal/llmdefaults/defaults.go: most kinds
// authenticate with an API key, openai-codex uses an OAuth handshake,
// and claude-code-cli delegates to the local `claude-code` CLI.
//
// The wizard uses this to populate the auth_mode select with only
// the modes that make sense for the picked kind, instead of letting
// the user pick a structurally-invalid combination like
// claude-code-cli + api-key.
export function availableAuthModesForKind(kind: ProviderKind | ''): AuthMode[] {
  switch (kind) {
    case 'openai-codex':
      return ['oauth']
    case 'claude-code-cli':
      return ['cli']
    case 'anthropic':
      return ['api-key', 'oauth']
    case '':
      return ['api-key', 'oauth', 'cli']
    default:
      return ['api-key']
  }
}

// suggestedAuthModeForKind picks the natural default auth mode per
// provider kind — the first entry of availableAuthModesForKind.
export function suggestedAuthModeForKind(kind: ProviderKind | ''): AuthMode {
  return availableAuthModesForKind(kind)[0]
}

// allAliasesFromConfigValues returns all provider aliases from the config
// values map, sorted alphabetically. Used to populate the provider selector
// in re-entry mode and the tier provider dropdowns.
export function allAliasesFromConfigValues(values: Record<string, unknown>): string[] {
  const providers = (values?.llm_providers ?? {}) as Record<string, unknown>
  return Object.keys(providers).sort()
}

// providerFromConfigValues extracts the OnboardingProvider fields for a
// specific alias from the config values map.
export function providerFromConfigValues(
  values: Record<string, unknown>,
  alias: string,
): OnboardingProvider {
  const providers = (values?.llm_providers ?? {}) as Record<string, Record<string, unknown>>
  const entry = providers[alias] || {}
  const kind = String(entry.kind ?? '') as ProviderKind | ''
  const apiKey = String(entry.api_key ?? '')
  return {
    alias,
    kind,
    auth_mode: (String(entry.auth_mode ?? 'api-key') as AuthMode) || 'api-key',
    api_key: apiKey,
    base_url: String(entry.base_url ?? ''),
    keepExistingApiKey: apiKey.trim() !== '',
    previousAlias: alias,
  }
}

// formFromConfigValues maps a config schema's values map into the
// wizard's form shape. Used by the re-entry path so the wizard opens
// with the existing provider + tier bindings prefilled.
//
// Picks the FIRST provider alias as the active edit target. Call
// providerFromConfigValues to switch to a different alias.
//
// Non-empty api_key values in the returned form have keepExistingApiKey=true
// so saving without typing a new value leaves the on-disk credential untouched.
//
// Tools / Integrations / Channels are populated from flat-keyed values
// (tools_*, memory_embed_*, channels_*) and follow the same masked-secret
// pattern via keep*Key flags.
export function formFromConfigValues(values: Record<string, unknown>): OnboardingFormState {
  const form = emptyOnboardingForm()
  const aliases = allAliasesFromConfigValues(values)
  if (aliases.length > 0) {
    form.provider = providerFromConfigValues(values, aliases[0])
  }

  const tiers = (values?.llm_tiers ?? {}) as Record<string, Record<string, unknown>>
  for (const tier of requiredTiers) {
    const binding = tiers[tier] || {}
    if (binding.provider) form.tiers[tier].provider = String(binding.provider)
    if (binding.model) form.tiers[tier].model = String(binding.model)
    if (binding.reasoning_effort) form.tiers[tier].reasoning_effort = String(binding.reasoning_effort)
  }

  form.tools = toolsFromConfigValues(values)
  form.integrations = integrationsFromConfigValues(values)
  form.channels = channelsFromConfigValues(values)

  return form
}

// readBool / readString / readStringList parse loosely-typed values from
// the schema map. The schema endpoint may emit booleans as JSON booleans
// or as strings (depending on YAML round-tripping); these helpers normalize
// to the wizard's typed shape.
function readBool(values: Record<string, unknown>, key: string): boolean {
  const v = values?.[key]
  if (typeof v === 'boolean') return v
  if (typeof v === 'string') return v.toLowerCase() === 'true'
  return false
}

function readString(values: Record<string, unknown>, key: string): string {
  const v = values?.[key]
  return v == null ? '' : String(v)
}

function readStringList(values: Record<string, unknown>, key: string): string[] {
  const v = values?.[key]
  if (Array.isArray(v)) return v.map((item) => String(item))
  if (typeof v === 'string' && v.trim() !== '') {
    try {
      const parsed = JSON.parse(v)
      if (Array.isArray(parsed)) return parsed.map((item) => String(item))
    } catch {
      // not JSON — treat as a single entry
      return [v]
    }
  }
  return []
}

function readNumberOrNull(values: Record<string, unknown>, key: string): number | null {
  const v = values?.[key]
  if (typeof v === 'number' && Number.isFinite(v) && v > 0) return v
  if (typeof v === 'string') {
    const n = Number(v)
    if (Number.isFinite(n) && n > 0) return n
  }
  return null
}

export function toolsFromConfigValues(values: Record<string, unknown>): OnboardingTools {
  const apiKey = readString(values, 'tools_web_search_api_key')
  return {
    web_search_enabled: readBool(values, 'tools_web_search_enabled'),
    web_search_provider: readString(values, 'tools_web_search_provider'),
    web_search_api_key: apiKey,
    keepWebSearchKey: apiKey.trim() !== '',
    web_fetch_enabled: readBool(values, 'tools_web_fetch_enabled'),
    web_fetch_allow_private_hosts: readBool(values, 'tools_web_fetch_allow_private_hosts'),
    web_fetch_private_host_allowlist: readStringList(values, 'tools_web_fetch_private_host_allowlist_json'),
    allow_high_risk_user: readBool(values, 'tools_allow_high_risk_user'),
  }
}

export function integrationsFromConfigValues(values: Record<string, unknown>): OnboardingIntegrations {
  const apiKey = readString(values, 'memory_embed_api_key')
  return {
    memory_embed_provider: readString(values, 'memory_embed_provider'),
    memory_embed_api_key: apiKey,
    keepMemoryEmbedKey: apiKey.trim() !== '',
    memory_embed_model: readString(values, 'memory_embed_model'),
    memory_embed_base_url: readString(values, 'memory_embed_base_url'),
    memory_embed_dimensions: readNumberOrNull(values, 'memory_embed_dimensions'),
  }
}

export function channelsFromConfigValues(values: Record<string, unknown>): OnboardingChannels {
  const token = readString(values, 'telegram_bot_token')
  const telegramEnabled = readBool(values, 'channels_telegram_enabled')
  // Polling on a disabled channel is a runtime no-op; if the on-disk
  // config landed in that inconsistent state (e.g. the user disabled
  // the channel via direct YAML edit but left polling=true), clear
  // polling on load so the wizard's polling-requires-channel validator
  // doesn't block save when the user has no UI affordance to flip it
  // back (the polling checkbox only renders when the channel is on).
  const pollingEnabled =
    telegramEnabled && readBool(values, 'channels_telegram_polling_enabled')
  return {
    telegram_enabled: telegramEnabled,
    telegram_bot_token: token,
    keepTelegramToken: token.trim() !== '',
    telegram_polling_enabled: pollingEnabled,
    webhook_enabled: readBool(values, 'channels_webhook_enabled'),
  }
}

// validateToolsStep flags configurations the user almost certainly
// doesn't want persisted. Enabling web_search without an api_key (and
// without the keep-existing flag) would write a useless config — the
// next chat turn would call the search tool and fail. Block save here
// rather than waiting for runtime errors.
export function validateToolsStep(form: OnboardingFormState): string[] {
  const errs: string[] = []
  const t = form.tools
  if (t.web_search_enabled && !t.keepWebSearchKey && t.web_search_api_key.trim() === '') {
    errs.push('Web search is enabled but no API key was provided.')
  }
  return errs
}

// validateIntegrationsStep treats memory_embed as opt-in — only flag a
// missing api_key when the user explicitly picked a provider, since an
// empty provider means "use defaults / disable".
export function validateIntegrationsStep(form: OnboardingFormState): string[] {
  const errs: string[] = []
  const i = form.integrations
  if (
    i.memory_embed_provider.trim() !== '' &&
    !i.keepMemoryEmbedKey &&
    i.memory_embed_api_key.trim() === ''
  ) {
    errs.push('Memory embeddings provider is set but no API key was provided.')
  }
  return errs
}

// validateChannelsStep enforces the telegram preconditions: enabling
// the channel requires a bot token, and polling requires the channel
// itself to be enabled (the runtime ignores polling on disabled channels).
export function validateChannelsStep(form: OnboardingFormState): string[] {
  const errs: string[] = []
  const c = form.channels
  if (c.telegram_enabled && !c.keepTelegramToken && c.telegram_bot_token.trim() === '') {
    errs.push('Telegram is enabled but no bot token was provided.')
  }
  if (c.telegram_polling_enabled && !c.telegram_enabled) {
    errs.push('Telegram polling requires the Telegram channel to be enabled.')
  }
  if (
    c.telegram_polling_enabled &&
    c.telegram_enabled &&
    !c.keepTelegramToken &&
    c.telegram_bot_token.trim() === ''
  ) {
    errs.push('Telegram polling requires a bot token.')
  }
  return errs
}

// buildToolsPayload returns ONLY the tools_* keys this section owns, so
// a per-section save does not touch unrelated config. JSON-encoded list
// fields use the *_json key shape that the backend's stringListField
// handler expects.
export function buildToolsPayload(form: OnboardingFormState): Record<string, unknown> {
  const t = form.tools
  const out: Record<string, unknown> = {
    tools_web_search_enabled: t.web_search_enabled,
    tools_web_fetch_enabled: t.web_fetch_enabled,
    tools_web_fetch_allow_private_hosts: t.web_fetch_allow_private_hosts,
    tools_allow_high_risk_user: t.allow_high_risk_user,
    tools_web_fetch_private_host_allowlist_json: JSON.stringify(t.web_fetch_private_host_allowlist),
  }
  const provider = t.web_search_provider.trim()
  if (provider !== '') out.tools_web_search_provider = provider
  if (!t.keepWebSearchKey && t.web_search_api_key.trim() !== '') {
    out.tools_web_search_api_key = t.web_search_api_key.trim()
  }
  return out
}

export function buildIntegrationsPayload(form: OnboardingFormState): Record<string, unknown> {
  const i = form.integrations
  const out: Record<string, unknown> = {}
  const provider = i.memory_embed_provider.trim()
  if (provider !== '') out.memory_embed_provider = provider
  if (i.memory_embed_model.trim() !== '') out.memory_embed_model = i.memory_embed_model.trim()
  if (i.memory_embed_base_url.trim() !== '') out.memory_embed_base_url = i.memory_embed_base_url.trim()
  if (i.memory_embed_dimensions !== null && i.memory_embed_dimensions > 0) {
    out.memory_embed_dimensions = i.memory_embed_dimensions
  }
  if (!i.keepMemoryEmbedKey && i.memory_embed_api_key.trim() !== '') {
    out.memory_embed_api_key = i.memory_embed_api_key.trim()
  }
  return out
}

export function buildChannelsPayload(form: OnboardingFormState): Record<string, unknown> {
  const c = form.channels
  const out: Record<string, unknown> = {
    channels_telegram_enabled: c.telegram_enabled,
    channels_telegram_polling_enabled: c.telegram_polling_enabled,
    channels_webhook_enabled: c.webhook_enabled,
  }
  if (!c.keepTelegramToken && c.telegram_bot_token.trim() !== '') {
    out.telegram_bot_token = c.telegram_bot_token.trim()
  }
  return out
}

// buildSectionPayload returns the partial PATCH payload for a single
// optional section. The wizard saves sections one at a time so users
// can edit Channels without touching Tools, etc.
export function buildSectionPayload(
  section: OptionalSection,
  form: OnboardingFormState,
): Record<string, unknown> {
  switch (section) {
    case 'tools':
      return buildToolsPayload(form)
    case 'integrations':
      return buildIntegrationsPayload(form)
    case 'channels':
      return buildChannelsPayload(form)
  }
}

// validateSectionStep dispatches to the right validator for a given
// optional section.
export function validateSectionStep(section: OptionalSection, form: OnboardingFormState): string[] {
  switch (section) {
    case 'tools':
      return validateToolsStep(form)
    case 'integrations':
      return validateIntegrationsStep(form)
    case 'channels':
      return validateChannelsStep(form)
  }
}

// parsePrivateHostAllowlistInput maps a free-form textarea value (one
// host per line, with optional comments / blank lines) into the typed
// list the wizard stores. Mirrors the YAML allowlist semantics — order
// preserved, duplicates allowed (caller can dedupe if needed).
export function parsePrivateHostAllowlistInput(input: string): string[] {
  return input
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter((line) => line !== '' && !line.startsWith('#'))
}

export function formatPrivateHostAllowlistInput(hosts: string[]): string {
  return hosts.join('\n')
}
