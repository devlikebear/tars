// Follow-up messages queued while a turn runs (#971). Message text is
// content and arrives as an argument.
export const messageQueueEn = {
  title: (n: number) => (n === 1 ? '1 queued message' : `${n} queued messages`),
  hint: 'Sent one at a time as each turn ends.',
  paused: 'Paused',
  pausedHint: 'Nothing is sent until you resume.',
  resume: 'Resume',
  clear: 'Clear',
  sendNow: 'Send now',
  sendNowTitle: 'Stop the running turn and send this next',
  edit: 'Edit',
  editTitle: 'Move back into the composer',
  remove: 'Remove',
  files: (n: number) => (n === 1 ? '1 file' : `${n} files`),
  queue: 'Queue',
  queueTitle: 'Send when the running turn ends (Enter)',
  placeholderBusy: 'Queue a follow-up… it is sent when this turn ends',
  refused: (reason: string) => `Not sent: ${reason}. It is back first in the queue, paused.`,
}

export type MessageQueueTranslations = typeof messageQueueEn

export const messageQueueKo: MessageQueueTranslations = {
  title: (n: number) => `대기 중인 메시지 ${n}개`,
  hint: '턴이 끝날 때마다 하나씩 보냅니다.',
  paused: '일시정지',
  pausedHint: '다시 시작하기 전에는 보내지 않습니다.',
  resume: '다시 시작',
  clear: '모두 지우기',
  sendNow: '지금 보내기',
  sendNowTitle: '실행 중인 턴을 멈추고 이 메시지를 먼저 보냅니다',
  edit: '수정',
  editTitle: '입력창으로 되돌립니다',
  remove: '삭제',
  files: (n: number) => `파일 ${n}개`,
  queue: '대기열에 추가',
  queueTitle: '실행 중인 턴이 끝나면 보냅니다 (Enter)',
  placeholderBusy: '후속 메시지를 대기열에 넣으세요… 이 턴이 끝나면 보냅니다',
  refused: (reason: string) => `보내지 못했습니다: ${reason}. 대기열 맨 앞에 되돌리고 일시정지했습니다.`,
}
