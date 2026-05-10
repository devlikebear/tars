import type { CodexUsageTier } from './types'

const BAR_WIDTH = 10

// formatCodexStatusLines turns the per-tier snapshot list into one line per
// tier suitable for inline display in the chat feedback bar (called by the
// `/status` slash command). Tiers whose provider isn't `openai-codex` are
// dropped so the message stays focused on the subscription.
//
// Each tier line includes a 10-cell ASCII progress bar (█ filled, ░ empty)
// per window so the user can eyeball usage without parsing percentages.
export function formatCodexStatusLines(tiers: CodexUsageTier[]): string[] {
  const codexTiers = tiers.filter((tier) => (tier.provider ?? '').toLowerCase() === 'openai-codex')
  if (codexTiers.length === 0) {
    return ['Codex status: no openai-codex tiers configured.']
  }

  const lines: string[] = ['Codex status:']
  for (const tier of codexTiers) {
    appendTierLines(lines, tier)
  }
  return lines
}

function appendTierLines(lines: string[], tier: CodexUsageTier): void {
  const head = `[${tier.tier}]${tier.model ? ` ${tier.model}` : ''}`
  if (!tier.snapshot) {
    lines.push(`  ${head}  Awaiting first request…`)
    return
  }
  lines.push(`  ${head}`)
  if (tier.snapshot.primary) {
    lines.push(formatWindowLine('primary', tier.snapshot.primary.used_percent, tier.snapshot.primary.reset_after_seconds, tier.snapshot.primary.window_minutes))
  }
  if (tier.snapshot.secondary) {
    lines.push(formatWindowLine('weekly ', tier.snapshot.secondary.used_percent, tier.snapshot.secondary.reset_after_seconds, tier.snapshot.secondary.window_minutes))
  }
  if (!tier.snapshot.primary && !tier.snapshot.secondary) {
    lines.push('    (no window data)')
  }
}

function formatWindowLine(
  label: string,
  usedPercent: number,
  resetAfterSeconds?: number,
  windowMinutes?: number,
): string {
  const bar = formatBar(usedPercent)
  const pct = `${usedPercent.toFixed(1).padStart(5)}%`
  const detail = formatDetail(resetAfterSeconds, windowMinutes)
  return `    ${label}  ${bar}  ${pct}${detail}`
}

function formatBar(usedPercent: number, width = BAR_WIDTH): string {
  const clamped = Math.max(0, Math.min(100, Number.isFinite(usedPercent) ? usedPercent : 0))
  const filled = Math.round((clamped / 100) * width)
  const safeFilled = Math.max(0, Math.min(width, filled))
  return '█'.repeat(safeFilled) + '░'.repeat(width - safeFilled)
}

function formatDetail(resetAfterSeconds?: number, windowMinutes?: number): string {
  const reset = formatReset(resetAfterSeconds)
  const total = formatWindowTotal(windowMinutes)
  if (reset && total) return `  (resets ${reset} / ${total})`
  if (reset) return `  (resets ${reset})`
  if (total) return `  (${total} window)`
  return ''
}

function formatWindowTotal(minutes: number | undefined | null): string {
  if (minutes === undefined || minutes === null || !Number.isFinite(minutes) || minutes <= 0) {
    return ''
  }
  if (minutes % 1440 === 0) return `${minutes / 1440}d`
  if (minutes % 60 === 0) return `${minutes / 60}h`
  return `${minutes}m`
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
