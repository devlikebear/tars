import type { ChatCommandsTranslations } from '../i18n/sections/chatCommands'
import type { CodexUsageTier } from './types'

export type CodexStatusText = ChatCommandsTranslations['codexStatus']

const BAR_WIDTH = 10

// formatCodexStatusLines turns the per-tier snapshot list into one line per
// tier suitable for inline display in the chat feedback bar (called by the
// `/status` slash command). Tiers whose provider isn't `openai-codex` are
// dropped so the message stays focused on the subscription.
//
// Each tier line includes a 10-cell ASCII progress bar (█ filled, ░ empty)
// per window so the user can eyeball usage without parsing percentages.
// `text` is the console locale's wording for the report.
export function formatCodexStatusLines(tiers: CodexUsageTier[], text: CodexStatusText): string[] {
  const codexTiers = tiers.filter((tier) => (tier.provider ?? '').toLowerCase() === 'openai-codex')
  if (codexTiers.length === 0) {
    return [text.noTiers]
  }

  const lines: string[] = [text.title]
  for (const tier of codexTiers) {
    appendTierLines(lines, tier, text)
  }
  return lines
}

function appendTierLines(lines: string[], tier: CodexUsageTier, text: CodexStatusText): void {
  const head = `[${tier.tier}]${tier.model ? ` ${tier.model}` : ''}`
  if (!tier.snapshot) {
    lines.push(`  ${head}  ${text.awaiting}`)
    return
  }
  lines.push(`  ${head}`)
  if (tier.snapshot.primary) {
    lines.push(formatWindowLine(text.primary, text, tier.snapshot.primary.used_percent, tier.snapshot.primary.reset_after_seconds, tier.snapshot.primary.window_minutes))
  }
  if (tier.snapshot.secondary) {
    lines.push(formatWindowLine(text.weekly, text, tier.snapshot.secondary.used_percent, tier.snapshot.secondary.reset_after_seconds, tier.snapshot.secondary.window_minutes))
  }
  if (!tier.snapshot.primary && !tier.snapshot.secondary) {
    lines.push(`    ${text.noWindowData}`)
  }
}

function formatWindowLine(
  label: string,
  text: CodexStatusText,
  usedPercent: number,
  resetAfterSeconds?: number,
  windowMinutes?: number,
): string {
  const bar = formatBar(usedPercent)
  const pct = `${usedPercent.toFixed(1).padStart(5)}%`
  const detail = formatDetail(text, resetAfterSeconds, windowMinutes)
  return `    ${label}  ${bar}  ${pct}${detail}`
}

function formatBar(usedPercent: number, width = BAR_WIDTH): string {
  const clamped = Math.max(0, Math.min(100, Number.isFinite(usedPercent) ? usedPercent : 0))
  const filled = Math.round((clamped / 100) * width)
  const safeFilled = Math.max(0, Math.min(width, filled))
  return '█'.repeat(safeFilled) + '░'.repeat(width - safeFilled)
}

function formatDetail(text: CodexStatusText, resetAfterSeconds?: number, windowMinutes?: number): string {
  const reset = formatReset(resetAfterSeconds)
  const total = formatWindowTotal(windowMinutes)
  if (reset && total) return `  ${text.resetsWithin(reset, total)}`
  if (reset) return `  ${text.resets(reset)}`
  if (total) return `  ${text.window(total)}`
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
