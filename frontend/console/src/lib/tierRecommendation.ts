import type { ChatTier, ChatTierRecommendationRequest } from './types'

export type TierRecommendation = {
  task_type: string
  recommended_tier: ChatTier
  reason: string
  confidence: number
  should_prompt: boolean
}

const heavySignals = [
  'implement',
  'refactor',
  'debug',
  'fix',
  'failing test',
  'test',
  'codebase',
  'architecture',
  'security',
  'threat model',
  'pull request',
  'pr',
  'github issue',
  'release',
  'merge',
  'git',
  'diff',
  'migration',
  'schema',
  'api',
  '구현',
  '개발',
  '리팩터',
  '리팩토',
  '버그',
  '테스트',
  '코드',
  '코드베이스',
  '아키텍처',
  '보안',
  '깃헙',
  '깃허브',
  '이슈',
  '릴리즈',
  '머지',
]

const lightSignals = [
  'summarize',
  'summary',
  'translate',
  'rewrite',
  'rephrase',
  'classify',
  'extract',
  'one sentence',
  'short answer',
  '요약',
  '번역',
  '다듬',
  '고쳐',
  '분류',
  '추출',
  '한 문장',
]

export function buildTierRecommendation(message: string): TierRecommendation {
  const text = message.trim().toLowerCase()
  const lines = text.length === 0 ? 0 : text.split(/\r?\n/).length

  if (!text) {
    return {
      task_type: 'general',
      recommended_tier: 'standard',
      reason: 'Empty prompts fall back to the standard tier.',
      confidence: 0.4,
      should_prompt: false,
    }
  }

  if (hasAny(text, heavySignals) || text.length > 900 || lines >= 8) {
    return {
      task_type: 'coding',
      recommended_tier: 'heavy',
      reason: 'Coding, repository, release, or deep reasoning work benefits from a stronger tier.',
      confidence: 0.82,
      should_prompt: true,
    }
  }

  if (hasAny(text, lightSignals) && text.length <= 320 && lines <= 3) {
    return {
      task_type: 'light_transform',
      recommended_tier: 'light',
      reason: 'Short transforms and summaries should stay cheap.',
      confidence: 0.78,
      should_prompt: true,
    }
  }

  return {
    task_type: 'general',
    recommended_tier: 'standard',
    reason: 'Open-ended chat or planning fits the standard tier.',
    confidence: 0.62,
    should_prompt: false,
  }
}

export function tierRecommendationPayload(
  recommendation: TierRecommendation,
  chosenTier: ChatTier,
): ChatTierRecommendationRequest {
  return {
    task_type: recommendation.task_type,
    recommended_tier: recommendation.recommended_tier,
    chosen_tier: chosenTier,
    reason: recommendation.reason,
    confidence: recommendation.confidence,
    accepted: chosenTier === recommendation.recommended_tier,
    source: 'console',
  }
}

function hasAny(text: string, signals: string[]): boolean {
  return signals.some((signal) => text.includes(signal))
}
