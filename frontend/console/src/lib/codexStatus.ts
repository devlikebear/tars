import type { CodexUsageTier } from './types'

// formatCodexStatusLines turns the per-tier snapshot list into one line per
// tier suitable for inline display in the chat feedback bar (called by the
// `/status` slash command). Tiers whose provider isn't `openai-codex` are
// dropped so the message stays focused on the subscription.
export function formatCodexStatusLines(tiers: CodexUsageTier[]): string[] {
  const codexTiers = tiers.filter((tier) => (tier.provider ?? '').toLowerCase() === 'openai-codex')
  if (codexTiers.length === 0) {
    return ['Codex status: no openai-codex tiers configured.']
  }

  const lines: string[] = ['Codex status:']
  for (const tier of codexTiers) {
    lines.push(formatTierLine(tier))
  }
  return lines
}

function formatTierLine(tier: CodexUsageTier): string {
  const head = `  [${tier.tier}]${tier.model ? ` ${tier.model}` : ''}`
  if (!tier.snapshot) {
    return `${head} · Awaiting first request…`
  }
  const parts: string[] = []
  if (tier.snapshot.primary) {
    parts.push(formatWindow('primary', tier.snapshot.primary.used_percent, tier.snapshot.primary.reset_after_seconds))
  }
  if (tier.snapshot.secondary) {
    parts.push(formatWindow('weekly', tier.snapshot.secondary.used_percent, tier.snapshot.secondary.reset_after_seconds))
  }
  if (parts.length === 0) {
    return `${head} · (no window data)`
  }
  return `${head} · ${parts.join(' · ')}`
}

function formatWindow(label: string, usedPercent: number, resetAfterSeconds?: number): string {
  const pct = `${usedPercent.toFixed(1)}%`
  const reset = formatReset(resetAfterSeconds)
  if (reset) {
    return `${label} ${pct} (resets ${reset})`
  }
  return `${label} ${pct}`
}

// Local copy of formatResetCountdown — kept self-contained so this module
// has no relative imports beyond the type-only ones, which makes it cheap
// to unit test under `node --experimental-strip-types --test` (which can't
// resolve extensionless ESM imports between source files).
function formatReset(seconds: number | undefined | null): string {
  if (seconds === undefined || seconds === null || !Number.isFinite(seconds) || seconds < 0) {
    return ''
  }
  const total = Math.floor(seconds)
  if (total < 60) return '<1m'
  const hours = Math.floor(total / 3600)
  const minutes = Math.floor((total % 3600) / 60)
  if (hours === 0) return `${minutes}m`
  if (minutes === 0) return `${hours}h`
  return `${hours}h ${minutes}m`
}
