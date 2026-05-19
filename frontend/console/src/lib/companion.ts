import type { NotificationMessage } from './types'

export type CompanionVisibilityInput = {
  enabled?: boolean
  needsSetup?: boolean
  loginRequired?: boolean
  zenActive?: boolean
}

export type CompanionStimulus = 'poke' | 'suggest' | 'feedback'
export type CompanionMood = 'idle' | 'spark' | 'focus' | 'warn' | 'error' | 'success'
export type CompanionLocale = 'en' | 'ko'

export type CompanionReaction = {
  mood: CompanionMood
  message: string
  detail?: string
}

type CompanionStimulusReactionKey = CompanionStimulus | 'suggest:pulse' | 'feedback:chat'
type CompanionStimulusReactionFactory = (area: string) => CompanionReaction

export type CompanionUiText = {
  actions: Record<CompanionStimulus, string>
  moods: Record<CompanionMood, string>
  buttonAria: string
  closeAria: string
  inputPlaceholder: string
  inputAria: string
  sendAria: string
  send: string
  feedbackAck: (action: string) => string
}

const companionText: Record<CompanionLocale, CompanionUiText> = {
  en: {
    actions: {
      poke: 'Poke',
      suggest: 'Suggest',
      feedback: 'Feedback',
    },
    moods: {
      idle: 'idle',
      spark: 'spark',
      focus: 'focus',
      warn: 'warn',
      error: 'error',
      success: 'success',
    },
    buttonAria: 'Talk to TARS companion',
    closeAria: 'Close companion bubble',
    inputPlaceholder: 'Ask TARS...',
    inputAria: 'Ask TARS companion',
    sendAria: 'Send companion prompt',
    send: 'Ask',
    feedbackAck: (action) => `${action} received`,
  },
  ko: {
    actions: {
      poke: '콕 찌르기',
      suggest: '제안',
      feedback: '피드백',
    },
    moods: {
      idle: '대기',
      spark: '반응',
      focus: '집중',
      warn: '주의',
      error: '오류',
      success: '좋음',
    },
    buttonAria: 'TARS 컴패니언에게 말 걸기',
    closeAria: '컴패니언 말풍선 닫기',
    inputPlaceholder: 'TARS에게 묻기...',
    inputAria: 'TARS 컴패니언에게 묻기',
    sendAria: '컴패니언 프롬프트 보내기',
    send: '묻기',
    feedbackAck: (action) => `${action} 반응 완료`,
  },
}

const companionStimulusReactions: Record<CompanionLocale, Record<CompanionStimulusReactionKey, CompanionStimulusReactionFactory>> = {
  en: {
    poke: (area) => ({
      mood: 'spark',
      message: `Awake. I am watching ${area} with you.`,
      detail: 'Tap Suggest for a next move, or ask me directly.',
    }),
    suggest: (area) => ({
      mood: 'focus',
      message: `Next move: inspect the freshest signal in ${area}, then ask one narrow follow-up.`,
      detail: 'Small prompts make the companion sharper.',
    }),
    'suggest:pulse': () => ({
      mood: 'focus',
      message: 'Pulse is already the signal board. Check the newest warn or error first.',
      detail: 'Then decide whether the next move is observe, fix, or silence noise.',
    }),
    feedback: (area) => ({
      mood: 'success',
      message: `Feedback: ${area} looks ready for a focused checkpoint.`,
      detail: 'Name the result you want and I will help compress it.',
    }),
    'feedback:chat': () => ({
      mood: 'success',
      message: 'Quick review: the chat loop is healthy when the next ask is specific and testable.',
      detail: 'If it feels vague, ask me to turn it into a tiny checklist.',
    }),
  },
  ko: {
    poke: (area) => ({
      mood: 'spark',
      message: `여기 있어요. 지금 ${area} 화면을 같이 보고 있어요.`,
      detail: '다음 수가 필요하면 제안, 상태 점검이 필요하면 피드백을 눌러요.',
    }),
    suggest: (area) => ({
      mood: 'focus',
      message: `다음 수: ${area}에서 가장 새 신호를 하나 고르고, 작은 질문 하나로 좁혀봐요.`,
      detail: '짧고 구체적인 자극일수록 제가 더 선명하게 반응해요.',
    }),
    'suggest:pulse': () => ({
      mood: 'focus',
      message: '펄스는 이미 신호판이에요. 가장 최근 경고나 오류부터 볼게요.',
      detail: '그 다음은 관찰, 수정, 소음 무시 중 하나로 좁히면 좋아요.',
    }),
    feedback: (area) => ({
      mood: 'success',
      message: `피드백: ${area}는 집중 체크포인트로 정리할 준비가 됐어요.`,
      detail: '원하는 결과를 한 문장으로 말해주면 제가 압축해볼게요.',
    }),
    'feedback:chat': () => ({
      mood: 'success',
      message: '빠른 점검: 다음 요청이 구체적이고 검증 가능하면 채팅 루프가 건강해요.',
      detail: '애매하면 작은 체크리스트로 바꿔달라고 말해줘요.',
    }),
  },
}

export function shouldShowCompanion(input: CompanionVisibilityInput): boolean {
  return !!input.enabled && !input.needsSetup && !input.loginRequired && !input.zenActive
}

export function companionEnabledFromConfigValues(values?: Record<string, unknown>): boolean {
  if (!values) return false
  return values.companion_enabled === true
}

export function companionUiText(locale?: string | null): CompanionUiText {
  return companionText[normalizeCompanionLocale(locale)]
}

export function normalizeCompanionLocale(locale?: string | null): CompanionLocale {
  return (locale || '').trim().toLowerCase().startsWith('ko') ? 'ko' : 'en'
}

export function companionReactionForStimulus(stimulus: CompanionStimulus, routeView?: string, locale?: string | null): CompanionReaction {
  const lang = normalizeCompanionLocale(locale)
  const area = companionAreaLabel(routeView, lang)
  return companionStimulusReactions[lang][companionStimulusReactionKey(stimulus, routeView)](area)
}

export function companionReactionFromEvent(event: NotificationMessage, locale?: string | null): CompanionReaction | null {
  if (!event || event.type === 'keepalive') return null
  const lang = normalizeCompanionLocale(locale)
  const category = (event.category || '').trim().toLowerCase()
  const severity = (event.severity || '').trim().toLowerCase()
  const title = clipText(event.title || event.category || 'Runtime signal', 70)
  const message = clipText(event.message || '', 120)

  if (category === 'embodiment') {
    return companionReactionFromEmbodimentMessage(title, message, lang)
  }
  if (severity === 'critical' || severity === 'error') {
    return {
      mood: 'error',
      message: lang === 'ko' ? `확인 필요: ${title}.` : `Needs attention: ${title}.`,
      detail: message,
    }
  }
  if (severity === 'warn' || category === 'pulse' || category === 'watchdog') {
    return {
      mood: 'warn',
      message: lang === 'ko' ? `신호 감지: ${title}.` : `Signal noticed: ${title}.`,
      detail: message,
    }
  }
  if (category === 'cron' || category === 'ops' || category === 'usage') {
    return {
      mood: 'focus',
      message: lang === 'ko' ? `콘솔 업데이트: ${title}.` : `Console update: ${title}.`,
      detail: message,
    }
  }
  return null
}

export function companionPromptForAsk(raw: string, routeView?: string, locale?: string | null): string {
  const lang = normalizeCompanionLocale(locale)
  const text = clipText(raw.trim(), 600)
  const route = (routeView || 'unknown').trim()
  if (lang === 'ko') {
    return [
      'TARS 콘솔 안의 컴패니언처럼 답해줘.',
      `현재 콘솔 영역: ${companionAreaLabel(routeView, lang)} (${route}).`,
      '짧게 답하고, 실용적인 다음 행동 하나만 제안해. 내가 명시적으로 요청하지 않으면 도구를 실행하지 마.',
      `사용자 자극: ${text}`,
    ].join('\n')
  }
  return [
    'Act as the TARS companion inside the Console.',
    `Current console area: ${companionAreaLabel(routeView, lang)} (${route}).`,
    'Answer briefly, give one practical next action, and do not run tools unless I explicitly ask.',
    `User stimulus: ${text}`,
  ].join('\n')
}

export function companionAskHandoffReaction(locale?: string | null): CompanionReaction {
  return normalizeCompanionLocale(locale) === 'ko'
    ? {
        mood: 'focus',
        message: '전체 TARS 채팅으로 넘길게요.',
        detail: '컴패니언이 현재 콘솔 맥락을 붙여서 이어갑니다.',
      }
    : {
        mood: 'focus',
        message: 'Opening the full TARS chat.',
        detail: 'The companion will hand this off with console context attached.',
      }
}

function companionReactionFromEmbodimentMessage(title: string, message: string, locale: CompanionLocale): CompanionReaction {
  const lower = message.toLowerCase()
  const summary = stripEmbodimentPrefix(message)
  if (lower.includes('vision') || lower.includes('camera') || lower.includes('image')) {
    return {
      mood: 'focus',
      message: locale === 'ko' ? `몸 신호를 봤어요: ${summary}` : `I saw a body signal: ${summary}`,
      detail: title,
    }
  }
  if (lower.includes('audio') || lower.includes('voice') || lower.includes('sound') || lower.includes('owner')) {
    return {
      mood: 'focus',
      message: locale === 'ko' ? `소리를 들었어요: ${summary}` : `I heard a body signal: ${summary}`,
      detail: title,
    }
  }
  return {
    mood: 'focus',
    message: locale === 'ko' ? `몸 신호 수신: ${summary}` : `Body signal received: ${summary}`,
    detail: title,
  }
}

function stripEmbodimentPrefix(message: string): string {
  const trimmed = message.trim()
  const [, rest] = /^[a-z_ -]+:\s*(.+)$/i.exec(trimmed) || []
  return clipText(rest || trimmed || 'percept received', 90)
}

function companionStimulusReactionKey(stimulus: CompanionStimulus, routeView?: string): CompanionStimulusReactionKey {
  if (stimulus === 'suggest' && routeView === 'pulse') return 'suggest:pulse'
  if (stimulus === 'feedback' && routeView === 'chat') return 'feedback:chat'
  return stimulus
}

function companionAreaLabel(routeView?: string, locale: CompanionLocale = 'en'): string {
  const labels: Record<CompanionLocale, Record<string, string>> = {
    en: {
      chat: 'this chat',
      pulse: 'Pulse',
      reflection: 'Reflection',
      agentruntime: 'Agent Runtime',
      tasks: 'Plans',
      config: 'Settings',
      logs: 'Logs',
      analytics: 'Analytics',
      memory: 'Memory',
      home: 'Mission Control',
      default: 'the Console',
    },
    ko: {
      chat: '이 채팅',
      pulse: '펄스',
      reflection: '리플렉션',
      agentruntime: '에이전트 런타임',
      tasks: '계획',
      config: '설정',
      logs: '로그',
      analytics: '분석',
      memory: '메모리',
      home: '미션 컨트롤',
      default: '콘솔',
    },
  }
  return labels[locale][routeView || 'default'] || labels[locale].default
}

function clipText(value: string, max: number): string {
  const text = value.replace(/\s+/g, ' ').trim()
  if (text.length <= max) return text
  return `${text.slice(0, Math.max(0, max - 1)).trim()}...`
}
