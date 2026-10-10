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
}
