// Integrated terminal (TerminalTabs, IntegratedTerminal).
export const terminalEn = {
  tabs: {
    closeTab: 'Close tab',
    newTab: 'New tab',
    newTabTitle: 'New shell in same directory',
  },
  // Connection states shown in the status button.
  status: {
    connecting: 'Connecting',
    connected: 'Connected',
    exited: 'Exited',
    disconnected: 'Disconnected',
  },
  // Shown when the server sends no message of its own.
  errors: {
    terminalError: 'Terminal error',
    invalidMessage: 'Invalid terminal message',
    connectionFailed: 'Terminal connection failed',
  },
  header: {
    reconnect: 'Reconnect',
    clickToReconnect: 'Click to reconnect',
    fontSettings: 'Terminal font settings',
    find: 'Find',
    findTitle: (shortcut: string) => `Find (${shortcut})`,
    close: 'Close',
  },
  search: {
    placeholder: 'Find in terminal…',
    caseSensitive: 'Case sensitive',
    regex: 'Regex',
    previous: 'Previous (Shift+Enter)',
    next: 'Next (Enter)',
    close: 'Close (Esc)',
  },
  settings: {
    font: 'Font',
    size: 'Size',
    systemFont: 'System default',
    reset: 'Reset',
    resetTitle: 'Reset to defaults',
    close: 'Close',
  },
  menu: {
    copy: 'Copy',
    paste: 'Paste',
    clear: 'Clear',
    saveBuffer: 'Save buffer…',
  },
}

export type TerminalTranslations = typeof terminalEn

export const terminalKo: TerminalTranslations = {
  tabs: {
    closeTab: '탭 닫기',
    newTab: '새 탭',
    newTabTitle: '같은 디렉터리에서 새 셸 열기',
  },
  status: {
    connecting: '연결 중',
    connected: '연결됨',
    exited: '종료됨',
    disconnected: '연결 끊김',
  },
  errors: {
    terminalError: '터미널 오류',
    invalidMessage: '잘못된 터미널 메시지',
    connectionFailed: '터미널 연결 실패',
  },
  header: {
    reconnect: '다시 연결',
    clickToReconnect: '클릭하여 다시 연결',
    fontSettings: '터미널 글꼴 설정',
    find: '찾기',
    findTitle: (shortcut) => `찾기 (${shortcut})`,
    close: '닫기',
  },
  search: {
    placeholder: '터미널에서 찾기…',
    caseSensitive: '대/소문자 구분',
    regex: '정규식',
    previous: '이전 (Shift+Enter)',
    next: '다음 (Enter)',
    close: '닫기 (Esc)',
  },
  settings: {
    font: '글꼴',
    size: '크기',
    systemFont: '시스템 기본값',
    reset: '초기화',
    resetTitle: '기본값으로 초기화',
    close: '닫기',
  },
  menu: {
    copy: '복사',
    paste: '붙여넣기',
    clear: '지우기',
    saveBuffer: '버퍼 저장…',
  },
}
