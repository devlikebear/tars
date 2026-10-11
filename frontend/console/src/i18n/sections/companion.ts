// Floating companion bot "CASE" (CompanionPet.svelte, lib/companion.ts). The
// bubble is a list of what is waiting on the user right now; it carries no
// fixed copy keyed by stimulus or screen. The code identifier and the
// config key (`companion`, `companion.enabled`) keep their name; only the
// user-visible strings say CASE.
export const companionEn = {
  header: 'CASE',
  buttonAria: 'Talk to CASE',
  closeAria: "Close CASE's bubble",
  inputPlaceholder: 'Ask CASE...',
  inputAria: 'Ask CASE',
  sendAria: 'Send to CASE',
  send: 'Ask',
  emptyLine: 'Nothing is waiting on you right now.',
  badgeAria: (count: number) => `${count} waiting on you`,
  dismissFailureAria: 'Dismiss this failure',
  lines: {
    pending: (count: number, title: string) =>
      count === 1
        ? `1 approval waiting in ${title || 'a session'}`
        : `${count} approvals waiting in ${title || 'a session'}`,
    queued: (count: number, title: string) =>
      count === 1
        ? `1 unattended approval waiting in ${title || 'a session'}`
        : `${count} unattended approvals waiting in ${title || 'a session'}`,
    running: (title: string) => `Running: ${title || 'a session'}`,
    failure: (label: string) => `Failed: ${label}`,
  },
  // CASE's bubble question-and-answer (tars#1236): a normal chat turn on
  // the main session, answered inline instead of in a new session.
  ask: {
    inThreadNote: 'Answering in this chat.',
    continueInChat: 'Continue in chat',
    openMainSession: 'Open the main session',
    dismissAria: 'Dismiss this answer',
    errors: {
      busy: 'The main session is busy right now.',
      network: "Couldn't reach TARS. Try again.",
      limit: 'Usage limit reached. Try again later.',
      error: "CASE couldn't answer. Try again.",
    },
  },
}

export type CompanionTranslations = typeof companionEn

export const companionKo: CompanionTranslations = {
  header: 'CASE',
  buttonAria: 'CASE에게 말 걸기',
  closeAria: 'CASE 말풍선 닫기',
  inputPlaceholder: 'CASE에게 묻기...',
  inputAria: 'CASE에게 묻기',
  sendAria: 'CASE에게 보내기',
  send: '묻기',
  emptyLine: '지금 기다리는 건 없어요.',
  badgeAria: (count: number) => `${count}건 기다리는 중`,
  dismissFailureAria: '이 실패 지우기',
  lines: {
    pending: (count: number, title: string) => `승인 대기 ${count}건 · ${title || '세션'}`,
    queued: (count: number, title: string) => `무인 실행 승인 대기 ${count}건 · ${title || '세션'}`,
    running: (title: string) => `실행 중: ${title || '세션'}`,
    failure: (label: string) => `실패: ${label}`,
  },
  ask: {
    inThreadNote: '이 채팅에서 답하고 있어요.',
    continueInChat: '채팅에서 이어가기',
    openMainSession: '메인 세션 열기',
    dismissAria: '이 답 지우기',
    errors: {
      busy: '메인 세션이 지금 작업 중이에요.',
      network: 'TARS에 연결할 수 없어요. 다시 시도해줘.',
      limit: '사용량 한도에 도달했어요. 잠시 후 다시 시도해줘.',
      error: 'CASE가 답하지 못했어요. 다시 시도해줘.',
    },
  },
}
