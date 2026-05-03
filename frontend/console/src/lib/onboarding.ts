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
  oauth_provider?: string
  // keepExistingApiKey is true on the re-entry path when the wizard
  // has prefilled a masked api_key from the server. Saving with this
  // flag set drops api_key from the patch payload so the existing
  // value on disk is preserved.
  keepExistingApiKey?: boolean
}

export type OnboardingTierBinding = {
  provider: string
  model: string
  reasoning_effort?: string
}

export type OnboardingFormState = {
  provider: OnboardingProvider
  tiers: {
    heavy: OnboardingTierBinding
    standard: OnboardingTierBinding
    light: OnboardingTierBinding
  }
}

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
export function buildConfigPayload(form: OnboardingFormState): Record<string, unknown> {
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
  // oauth_provider is intentionally omitted from the payload. The
  // backend's providerAuthConfig (internal/llm/provider.go) auto-fills
  // it from the per-kind defaults in internal/llmdefaults/. Asking the
  // user a second time made the kind selection feel redundant; if a
  // pre-existing oauth_provider value is on disk PatchYAML preserves
  // it because we never write the key.

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

  return {
    llm_providers: { [form.provider.alias.trim()]: provider },
    llm_tiers: tiers,
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
      return 'https://generativelanguage.googleapis.com'
    case 'gemini-native':
      return 'https://generativelanguage.googleapis.com'
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
    oauth_provider: entry.oauth_provider ? String(entry.oauth_provider) : undefined,
    keepExistingApiKey: apiKey.trim() !== '',
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

  return form
}
