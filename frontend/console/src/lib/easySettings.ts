// Vibe mode's settings screen (/console/easy/settings, EasySettings.svelte):
// the pure decisions behind it. The screen reads the same config schema and
// remote-access status the expert Settings screen reads; nothing here calls
// the server.

import type { RemoteAccessResponse } from './types'

// --- The AI ---

// The ways the team can think, in the order the screen lists them: the
// three tiers easy setup writes (EasySetupUpdates), then any other tier the
// config names.
const knownTiers = ['heavy', 'standard', 'light']

export type ThinkingChoice = {
  // The tier's name in the config.
  tier: string
  // The model it runs, as written in the config ("opus", "claude-opus-5-5").
  model: string
  // The provider kind of its provider alias ("claude-code-cli").
  kind: string
}

type ConfigValues = Record<string, unknown>

function record(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value) ? (value as Record<string, unknown>) : {}
}

function text(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

// thinkingChoices lists the tiers a person can pick from: every tier with a
// model, known ones first.
export function thinkingChoices(values: ConfigValues): ThinkingChoice[] {
  const tiers = record(values.llm_tiers)
  const providers = record(values.llm_providers)
  const names = Object.keys(tiers).sort((a, b) => {
    const ia = knownTiers.indexOf(a)
    const ib = knownTiers.indexOf(b)
    return (ia < 0 ? 99 : ia) - (ib < 0 ? 99 : ib) || a.localeCompare(b)
  })
  const choices: ThinkingChoice[] = []
  for (const tier of names) {
    const binding = record(tiers[tier])
    const model = text(binding.model)
    if (!model) continue
    choices.push({ tier, model, kind: text(record(providers[text(binding.provider)]).kind) })
  }
  return choices
}

// chatTier is the tier the team's turns run on: the chat role's tier when
// one is set, else the default tier (standard when none is set), the way
// the server resolves it (resolveChatTier).
export function chatTier(values: ConfigValues): string {
  return text(record(values.llm_role_defaults).chat_main) || text(values.llm_default_tier) || 'standard'
}

// thinkingUpdate is the config write that moves the team to another tier.
// When the chat role names its own tier, that is the one to change, so the
// whole role map is written back with it; otherwise the default tier.
export function thinkingUpdate(values: ConfigValues, tier: string): Record<string, unknown> {
  const roles = record(values.llm_role_defaults)
  if (text(roles.chat_main)) return { llm_role_defaults: { ...roles, chat_main: tier } }
  return { llm_default_tier: tier }
}

// --- The phone ---

// phoneAccess is what the screen says about reaching this computer from a
// phone, read from GET /v1/admin/remote-access/status. Pairing needs it to
// be 'on': the pairing code carries the Tailscale address.
// 'renamed' is a machine renamed in Tailscale: the old name still holds
// TARS's sharing and the new one has none, so turning access on again
// moves it to the new name.
export type PhoneAccess = 'checking' | 'noTailscale' | 'signedOut' | 'portTaken' | 'renamed' | 'off' | 'on'

export function phoneAccess(data: RemoteAccessResponse | null): PhoneAccess {
  if (!data) return 'checking'
  const s = data.status ?? {}
  const serving = Boolean(s.serve_active ?? s.ServeActive)
  const ours = Boolean(s.owned_by_tars ?? s.OwnedByTARS)
  if (serving && ours) return 'on'
  if (!(s.installed ?? s.Installed)) return 'noTailscale'
  if (!(s.logged_in ?? s.LoggedIn)) return 'signedOut'
  if (serving) return 'portTaken'
  if (text(s.stale_host ?? s.StaleHost)) return 'renamed'
  return 'off'
}

// phoneAccessBlocker is why turning phone access on cannot work yet, from
// the server's checks (apihandlers/remote_access.go preflightChecks), in the
// order the person has to deal with them. null when nothing blocks it.
export type PhoneAccessBlocker = 'noTailscale' | 'signedOut' | 'portTaken' | 'needsPassword'

export function phoneAccessBlocker(data: RemoteAccessResponse | null): PhoneAccessBlocker | null {
  const failed = new Set((data?.checks ?? []).filter((c) => !c.ok).map((c) => c.key))
  if (failed.has('tailscale_installed')) return 'noTailscale'
  if (failed.has('tailscale_logged_in')) return 'signedOut'
  if (failed.has('serve_target_available')) return 'portTaken'
  if (failed.has('api_auth_required') || failed.has('admin_password_configured') || failed.has('user_password_configured')) return 'needsPassword'
  return null
}

// --- Restarting ---

// restarted reports whether the server answering now is a new process: the
// config schema's runtime_started_at (or process id) changed.
export function restarted(before: { runtime_started_at?: string; process_id?: number }, now: { runtime_started_at?: string; process_id?: number }): boolean {
  if (before.runtime_started_at && now.runtime_started_at) return before.runtime_started_at !== now.runtime_started_at
  return Boolean(before.process_id && now.process_id && before.process_id !== now.process_id)
}
