// Static well-known model catalog used by the onboarding wizard's
// Step 2 datalist. Seeded from OpenRouter's public catalog
// (https://openrouter.ai/api/v1/models) snapshotted on 2026-05-03.
//
// Two adjustments vs. the raw OpenRouter response:
//   1. The OpenRouter id format is `<provider-prefix>/<model-id>`
//      (e.g. `openai/gpt-5.4`). The wizard sends just the trailing
//      part to the per-kind native API, so the prefix is dropped
//      before the entry lands here.
//   2. Anthropic models on Anthropic's native API use the dash form
//      (`claude-opus-4-7-20XXXXXX`) while OpenRouter exposes a
//      friendlier `claude-opus-4.7`. We list the dash form here
//      because the wizard targets the native API; the dot form
//      still works for users on OpenRouter routing.
//
// Refreshing the catalog: re-fetch
//   curl https://openrouter.ai/api/v1/models | jq '.data[] | .id'
// then prune to the entries that map to a TARS provider kind, drop
// preview/non-chat variants you do not want surfaced as defaults,
// and bump the SNAPSHOT_DATE constant below.

import type { ProviderKind } from './onboarding'

export const SNAPSHOT_DATE = '2026-05-03'
export const SNAPSHOT_SOURCE = 'OpenRouter /api/v1/models'

// Per-kind well-known model ids, ordered by typical user preference
// (newest / heaviest first). The wizard surfaces these as datalist
// suggestions; users may type any other id by hand.
export const wellKnownModelsByKind: Record<ProviderKind, string[]> = {
  openai: [
    'gpt-5.5-pro',
    'gpt-5.5',
    'gpt-5.4-pro',
    'gpt-5.4',
    'gpt-5.4-mini',
    'gpt-5.4-nano',
    'gpt-5.3-chat',
    'gpt-4o',
    'gpt-4o-mini',
  ],
  'openai-codex': [
    'gpt-5.4',
    'gpt-5.3-codex',
  ],
  anthropic: [
    'claude-opus-4-7',
    'claude-opus-4-6',
    'claude-sonnet-4-6',
    'claude-haiku-4-5',
  ],
  gemini: [
    'gemini-3.1-pro-preview',
    'gemini-3.1-flash-lite-preview',
    'gemini-2.5-pro',
    'gemini-2.5-flash',
  ],
  'gemini-native': [
    'gemini-3.1-pro-preview',
    'gemini-3.1-flash-lite-preview',
    'gemini-2.5-pro',
    'gemini-2.5-flash',
  ],
  kimi: [
    'kimi-k2.6',
    'moonshot-v1-128k',
  ],
  // claude-code-cli accepts short aliases that the local CLI maps to
  // its current pinned defaults — the wizard never sends a pinned
  // dated id here.
  'claude-code-cli': [
    'sonnet',
    'opus',
    'haiku',
  ],
}

// popularModelsForKind returns the curated catalog entries for a
// kind, or an empty list when the kind is unknown / unset. The
// wizard datalist degrades gracefully (no autocomplete) when this
// is empty.
export function popularModelsForKind(kind: ProviderKind | ''): string[] {
  if (kind === '') return []
  return wellKnownModelsByKind[kind] ?? []
}
