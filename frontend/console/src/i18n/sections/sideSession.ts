// Side session dock panel (#971). Session titles and messages are content
// and never go through here.
export const sideSessionEn = {
  pick: 'Session to show beside this one',
  none: 'Choose a session…',
  hint: 'Pick a session to watch and answer it next to the one you are in.',
  open: 'Open',
  openTitle: 'Make this the main session',
  close: 'Close side session',
  running: 'Working…',
  placeholder: 'Message this session…',
  send: 'Send',
}

export type SideSessionTranslations = typeof sideSessionEn

export const sideSessionKo: SideSessionTranslations = {
  pick: '옆에 띄울 세션',
  none: '세션 선택…',
  hint: '세션을 골라 지금 세션 옆에서 지켜보고 답하세요.',
  open: '열기',
  openTitle: '이 세션을 메인으로 엽니다',
  close: '옆 세션 닫기',
  running: '작업 중…',
  placeholder: '이 세션에 메시지…',
  send: '보내기',
}
