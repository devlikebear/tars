// Changes dock panel and the inline change cards under chat turns (#969).
// File paths, prompt previews, and diff text are content and never go
// through here.
export const changesEn = {
  heading: 'Turn changes',
  refresh: 'Refresh',
  loading: 'Loading changes…',
  loadFailed: 'Could not load the changes.',
  empty: 'No file changes recorded yet.',
  emptyHint: 'After each turn, files the agent changed in this session’s folder appear here.',
  scopeLabel: 'Diff range',
  scope: {
    turn: 'This turn',
    session: 'Session so far',
  },
  turnsLabel: 'Turns that changed files',
  untitledTurn: '(no message)',
  folder: 'Folder',
  filesLabel: 'Changed files',
  summary: (files: number, additions: number, deletions: number) =>
    `${files} ${files === 1 ? 'file' : 'files'} +${additions} −${deletions}`,
  // Keyed by internal/checkpoint's Skip* reasons; others use `skippedOther`.
  skipped: {
    too_many_files: 'Not recorded: too many files changed',
    too_large: 'Not recorded: too much data changed',
    failed: 'Not recorded: the snapshot failed',
  } as Record<string, string>,
  skippedOther: (reason: string) => `Not recorded (${reason})`,
  // Keyed by the status words the server sends; a word missing here is shown
  // as sent.
  fileStatus: {
    added: 'added',
    modified: 'modified',
    deleted: 'deleted',
    renamed: 'renamed',
  } as Record<string, string>,
  renamedFrom: (path: string) => `from ${path}`,
  binary: 'Binary file: no text diff.',
  truncated: 'This diff is too long to show in full.',
  noTextChanges: 'No text changes.',
  unknownPaths: (count: number) =>
    `${count} ${count === 1 ? 'path was' : 'paths were'} not recorded (too large or unreadable).`,
  diff: {
    loading: 'Loading diff…',
    selectFile: 'Select a file to see its diff.',
    layoutLabel: 'Diff layout',
    unified: 'Unified',
    split: 'Split',
    sideBySideLabel: 'side-by-side diff',
  },
  card: {
    label: 'Files changed in this turn',
    show: 'Show changes',
    hide: 'Hide changes',
    openPanel: 'Open in Changes',
  },
}

export type ChangesTranslations = typeof changesEn

export const changesKo: ChangesTranslations = {
  heading: '턴별 변경 사항',
  refresh: '새로 고침',
  loading: '변경 사항을 불러오는 중…',
  loadFailed: '변경 사항을 불러오지 못했습니다.',
  empty: '아직 기록된 파일 변경이 없습니다.',
  emptyHint: '턴이 끝날 때마다 에이전트가 이 세션 폴더에서 바꾼 파일이 여기에 나타납니다.',
  scopeLabel: 'Diff 범위',
  scope: {
    turn: '이번 턴',
    session: '세션 누적',
  },
  turnsLabel: '파일을 바꾼 턴',
  untitledTurn: '(메시지 없음)',
  folder: '폴더',
  filesLabel: '바뀐 파일',
  summary: (files, additions, deletions) => `파일 ${files}개 +${additions} −${deletions}`,
  skipped: {
    too_many_files: '기록 안 됨: 바뀐 파일이 너무 많음',
    too_large: '기록 안 됨: 바뀐 데이터가 너무 큼',
    failed: '기록 안 됨: 스냅샷 실패',
  },
  skippedOther: (reason) => `기록 안 됨 (${reason})`,
  fileStatus: {
    added: '추가됨',
    modified: '수정됨',
    deleted: '삭제됨',
    renamed: '이름 바뀜',
  },
  renamedFrom: (path) => `이전 이름 ${path}`,
  binary: '바이너리 파일이라 텍스트 diff가 없습니다.',
  truncated: 'diff가 너무 길어 일부만 표시합니다.',
  noTextChanges: '텍스트 변경이 없습니다.',
  unknownPaths: (count) => `기록하지 않은 경로 ${count}개(너무 크거나 읽을 수 없음)`,
  diff: {
    loading: 'diff를 불러오는 중…',
    selectFile: '파일을 고르면 diff가 보입니다.',
    layoutLabel: 'Diff 레이아웃',
    unified: '통합',
    split: '분할',
    sideBySideLabel: '나란히 보기 diff',
  },
  card: {
    label: '이번 턴에 바뀐 파일',
    show: '변경 보기',
    hide: '변경 접기',
    openPanel: '변경 사항 패널에서 열기',
  },
}
