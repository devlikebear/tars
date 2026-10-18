// CASE's Bridge panel (CaseBridge.svelte, case-companion phase 2): ask CASE
// about TARS from any screen.
export const caseBridgeEn = {
  title: 'Bridge',
  subtitle: 'Ask CASE what is going on in TARS',
  closeAria: 'Close the Bridge',
  looking: (where: string) => `Looking at: ${where}`,
  routes: {
    board: 'the board',
    home: 'the system overview',
    chat: 'a chat',
    focus: 'a focus pipeline',
    ops: 'Ops',
    cron: 'Cron',
    pulse: 'Pulse',
    other: 'this screen',
  } as Record<string, string>,
  scope: {
    label: 'Answer about',
    all: 'All of TARS',
    screen: 'This screen',
  },
  inputPlaceholder: 'Ask CASE about TARS...',
  inputAria: 'Ask CASE about TARS',
  send: 'Ask',
  empty: 'CASE reads the live state of TARS: pipelines, running turns, approvals, cron, cost and the watchdog. It cannot change anything.',
  thinking: 'Reading the status...',
  cardsAria: 'Where to look',
  severity: {
    block: 'Needs you',
    info: 'Info',
  },
  errors: {
    busy: 'CASE is still answering the last question.',
    unanswered: "CASE couldn't answer. Try again.",
    network: "Couldn't reach TARS. Try again.",
  },
  followUps: {
    stuck: 'Is anything stuck?',
    missed: 'What changed in the last hour?',
    focusWaiting: 'What is the pipeline waiting for?',
    cronWhy: 'Why did the cron job fail?',
    approvalsOldest: 'Which approval has waited longest?',
    usageTop: 'Where did today’s cost go?',
    pulseWhat: 'What is the watchdog warning about?',
  },
  openFromBubble: 'Ask about TARS',
}

export type CaseBridgeTranslations = typeof caseBridgeEn

export const caseBridgeKo: CaseBridgeTranslations = {
  title: 'Bridge',
  subtitle: 'CASE에게 TARS 상황 묻기',
  closeAria: 'Bridge 닫기',
  looking: (where: string) => `보는 곳: ${where}`,
  routes: {
    board: '보드',
    home: '시스템 개요',
    chat: '채팅',
    focus: '포커스 파이프라인',
    ops: 'Ops',
    cron: 'Cron',
    pulse: 'Pulse',
    other: '이 화면',
  },
  scope: {
    label: '답할 범위',
    all: 'TARS 전체',
    screen: '이 화면',
  },
  inputPlaceholder: 'CASE에게 TARS 상황 묻기...',
  inputAria: 'CASE에게 TARS 상황 묻기',
  send: '묻기',
  empty: 'CASE는 TARS의 지금 상태(파이프라인, 실행 중인 턴, 승인 대기, cron, 비용, 감시)를 읽고 답해요. 무언가를 바꾸지는 않아요.',
  thinking: '상태를 읽는 중...',
  cardsAria: '볼 곳',
  severity: {
    block: '확인 필요',
    info: '정보',
  },
  errors: {
    busy: 'CASE가 아직 앞 질문에 답하고 있어요.',
    unanswered: 'CASE가 답하지 못했어요. 다시 물어봐 줘.',
    network: 'TARS에 연결할 수 없어요. 다시 시도해 줘.',
  },
  followUps: {
    stuck: '막힌 거 있어?',
    missed: '지난 한 시간 동안 뭐가 바뀌었어?',
    focusWaiting: '파이프라인이 뭘 기다리고 있어?',
    cronWhy: 'cron 작업은 왜 실패했어?',
    approvalsOldest: '가장 오래 기다린 승인은 뭐야?',
    usageTop: '오늘 비용은 어디에 쓰였어?',
    pulseWhat: '감시가 경고하는 건 뭐야?',
  },
  openFromBubble: 'TARS 상황 묻기',
}
