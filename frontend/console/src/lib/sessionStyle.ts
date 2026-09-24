import type { SessionStyleControl, SessionStyleResponse, SessionStyleValues } from './types'
import type { SessionConfigTranslations } from '../i18n/sections/sessionConfig'

// Fallback preview phrases come from the caller's locale ($t.sessionConfig.style.preview).
export type SessionStylePreviewLabels = SessionConfigTranslations['style']['preview']

export function sessionStylePayload(values: SessionStyleValues): SessionStyleControl {
  return {
    directness: clampStyleScore(values.directness),
    humor: clampStyleScore(values.humor),
    caution: clampStyleScore(values.caution),
    autonomy: clampStyleScore(values.autonomy),
  }
}

export function buildSessionStylePreview(response: SessionStyleResponse, labels: SessionStylePreviewLabels): string[] {
  const style = response.effective
  if (response.preview?.length) {
    return response.preview
  }
  return [
    labels.toneLine(scoreLabel(style.directness, labels.directness), scoreLabel(style.humor, labels.humor)),
    labels.cautionLine(scoreLabel(style.caution, labels.caution)),
  ]
}

export function clampStyleScore(value: number): number {
  if (!Number.isFinite(value)) return 0
  return Math.min(100, Math.max(0, Math.round(value)))
}

// Every axis shares the same bands: >= 70 high, <= 30 low, otherwise balanced.
function scoreLabel(value: number, labels: { high: string; low: string; balanced: string }): string {
  if (value >= 70) return labels.high
  if (value <= 30) return labels.low
  return labels.balanced
}
