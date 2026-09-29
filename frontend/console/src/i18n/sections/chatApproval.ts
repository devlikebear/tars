// Inline tool approval cards in the chat thread (#970). Tool names, commands,
// file paths and rule text are content and arrive as arguments.
export const chatApprovalEn = {
  label: 'Tool approval',
  heading: (tool: string) => `Allow ${tool}?`,
  subagent: 'Asked by a subagent',
  reason: 'Why it asks',
  preview: {
    command: 'Command',
    file: 'File',
    url: 'URL',
    input: 'Input',
  },
  allowOnce: 'Allow once',
  allowSession: (rule: string) => `Allow ${rule} for this session`,
  deny: 'Deny',
  keysHint: 'y allow · s this session · n deny',
  keysHintNoSession: 'y allow · n deny',
  state: {
    sending: 'Sending…',
    allowed: 'Allowed once',
    allowedSession: (rule: string) => `Allowed for this session: ${rule}`,
    denied: 'Denied',
    withdrawn: 'The turn ended before an answer',
  },
  sendFailed: 'Could not send your answer. Try again.',
}

export type ChatApprovalTranslations = typeof chatApprovalEn

export const chatApprovalKo: ChatApprovalTranslations = {
  label: '도구 승인',
  heading: (tool: string) => `${tool} 실행을 허용할까요?`,
  subagent: '서브에이전트의 요청',
  reason: '확인하는 이유',
  preview: {
    command: '명령',
    file: '파일',
    url: 'URL',
    input: '입력',
  },
  allowOnce: '이번만 허용',
  allowSession: (rule: string) => `이 세션 동안 ${rule} 허용`,
  deny: '거부',
  keysHint: 'y 허용 · s 이 세션 동안 · n 거부',
  keysHintNoSession: 'y 허용 · n 거부',
  state: {
    sending: '보내는 중…',
    allowed: '이번만 허용함',
    allowedSession: (rule: string) => `이 세션 동안 허용함: ${rule}`,
    denied: '거부함',
    withdrawn: '응답하기 전에 턴이 끝났습니다',
  },
  sendFailed: '응답을 보내지 못했습니다. 다시 시도해 주세요.',
}
