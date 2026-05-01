import type { SessionStyleControl, SessionStyleResponse, SessionStyleValues } from './types'

export function sessionStylePayload(values: SessionStyleValues): SessionStyleControl {
  return {
    directness: clampStyleScore(values.directness),
    humor: clampStyleScore(values.humor),
    caution: clampStyleScore(values.caution),
    autonomy: clampStyleScore(values.autonomy),
  }
}

export function buildSessionStylePreview(response: SessionStyleResponse): string[] {
  const style = response.effective
  if (response.preview?.length) {
    return response.preview
  }
  return [
    `${directnessPreview(style.directness)}; ${humorPreview(style.humor)}.`,
    `${cautionPreview(style.caution)}; autonomy stays bounded by explicit consent.`,
  ]
}

export function clampStyleScore(value: number): number {
  if (!Number.isFinite(value)) return 0
  return Math.min(100, Math.max(0, Math.round(value)))
}

function directnessPreview(value: number): string {
  if (value >= 70) return 'direct answers first'
  if (value <= 30) return 'softer exploratory answers'
  return 'balanced directness'
}

function humorPreview(value: number): string {
  if (value >= 70) return 'warmer humor'
  if (value <= 30) return 'rare humor'
  return 'occasional warmth'
}

function cautionPreview(value: number): string {
  if (value >= 70) return 'more verify-before-act behavior'
  if (value <= 30) return 'fewer caveats on reversible work'
  return 'moderate risk checks'
}
