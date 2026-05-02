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

export type AuthMode = 'api-key' | 'oauth'

export type OnboardingProvider = {
  alias: string
  kind: ProviderKind | ''
  auth_mode: AuthMode
  api_key: string
  base_url: string
  oauth_provider?: string
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
  if (form.provider.auth_mode === 'api-key' && form.provider.api_key.trim() === '') {
    errs.push('API key is required for api-key auth mode.')
  }
  return errs
}

// validateTiersStep returns user-facing error messages for the tier
// bindings step. All three tiers must point at a non-empty provider
// alias and a non-empty model. The provider alias should match the
// one configured on the previous step (or another tier's provider).
export function validateTiersStep(form: OnboardingFormState): string[] {
  const errs: string[] = []
  const knownAliases = new Set<string>()
  if (form.provider.alias.trim() !== '') {
    knownAliases.add(form.provider.alias.trim())
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
export function validateForm(form: OnboardingFormState): string[] {
  return [...validateProviderStep(form), ...validateTiersStep(form)]
}

// buildConfigPayload converts the form into the `updates` map shape
// that PATCH /v1/admin/config/values expects. Empty optional fields
// are omitted so the resulting YAML stays clean.
export function buildConfigPayload(form: OnboardingFormState): Record<string, unknown> {
  const provider: Record<string, unknown> = {
    kind: form.provider.kind,
    auth_mode: form.provider.auth_mode,
  }
  if (form.provider.api_key.trim() !== '') {
    provider.api_key = form.provider.api_key.trim()
  }
  if (form.provider.base_url.trim() !== '') {
    provider.base_url = form.provider.base_url.trim()
  }
  if (form.provider.auth_mode === 'oauth' && form.provider.oauth_provider) {
    const op = form.provider.oauth_provider.trim()
    if (op !== '') provider.oauth_provider = op
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

// suggestedAuthModeForKind picks the natural default auth mode per
// provider kind. The wizard calls this when the user changes kind so
// the form switches between api-key and oauth flows automatically.
export function suggestedAuthModeForKind(kind: ProviderKind | ''): AuthMode {
  switch (kind) {
    case 'openai-codex':
    case 'claude-code-cli':
      return 'oauth'
    default:
      return 'api-key'
  }
}
