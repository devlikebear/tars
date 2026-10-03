import { emptyOnboardingForm, defaultBaseURLForKind, type OnboardingFormState } from './onboarding.ts'

export type SetupCandidate = {
  kind: 'claude-code-cli' | 'openai-codex'
  installed: boolean
  ready: boolean
  problem?: string
  cli_path?: string
  version?: string
  source?: string
  models: string[]
  recommended: Partial<Record<'heavy' | 'standard' | 'light', string>>
}
export type SetupDiscoveryResponse = { candidates: SetupCandidate[] }

export function applyDiscoveredProvider(form: OnboardingFormState, candidate: SetupCandidate): void {
  if (!candidate.ready) return
  const alias = candidate.kind === 'claude-code-cli' ? 'claude' : 'codex'
  form.provider = {
    ...emptyOnboardingForm().provider,
    alias,
    kind: candidate.kind,
    auth_mode: candidate.kind === 'claude-code-cli' ? 'cli' : 'oauth',
    base_url: defaultBaseURLForKind(candidate.kind),
  }
  for (const tier of ['heavy', 'standard', 'light'] as const) {
    form.tiers[tier] = { provider: alias, model: candidate.recommended[tier] || '' }
  }
}
export function autoApplyDiscovery(form: OnboardingFormState, candidates: SetupCandidate[], reentry: boolean): boolean {
  if (reentry || form.provider.kind || form.provider.alias || form.provider.api_key || form.provider.base_url) return false
  if (Object.values(form.tiers).some(tier => tier.provider || tier.model)) return false
  const ready = candidates.filter(candidate => candidate.ready)
  if (ready.length !== 1) return false
  applyDiscoveredProvider(form, ready[0])
  return true
}
