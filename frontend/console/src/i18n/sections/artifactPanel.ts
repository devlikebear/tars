// Files dock panel (ArtifactPanel, ArtifactPanelHeader).
export const artifactPanelEn = {
  header: {
    title: 'Files',
    sessionTab: 'Session',
    workspaceTab: 'Workspace',
  },
  loading: 'Loading...',
  actions: {
    cancel: 'Cancel',
    create: 'Create',
    save: 'Save',
    rename: 'Rename',
  },
  time: {
    secondsAgo: (n: number) => `${n}s ago`,
    minutesAgo: (n: number) => `${n}m ago`,
    hoursAgo: (n: number) => `${n}h ago`,
    daysAgo: (n: number) => `${n}d ago`,
  },
  session: {
    empty: 'No files created in this session yet.',
    // Badge for Artifact.action ('created' | 'modified' from lib/artifacts).
    action: {
      created: 'created',
      modified: 'modified',
    },
  },
  workDir: {
    requiredTitle: 'Session artifact directory is required',
    removeTitle: 'Remove current directory',
    addTitle: 'Add directory',
    addWorkingTitle: 'Add working directory',
  },
  picker: {
    title: 'Select Directory',
    selectHere: 'Select Here',
    noSubdirectories: 'No subdirectories',
  },
  folder: {
    newFolder: 'New Folder',
    newFolderPlaceholder: 'New folder name',
    namePlaceholder: 'Folder name',
  },
  workspace: {
    emptyDirectory: 'Empty directory',
    shell: 'Shell',
    // e2e/chat-workbench.spec.ts clicks button[title^="Open integrated terminal"].
    shellTitle: (target: string) => `Open integrated terminal at ${target}`,
    openApp: 'Open App',
    openAppTitle: (target: string) => `Open macOS Terminal at ${target}`,
    opening: 'Opening...',
    terminalOpened: (app: string, target: string) => `${app} opened at ${target}`,
  },
  preview: {
    loadingFile: 'Loading file...',
    modes: {
      preview: 'Preview',
      code: 'Code',
      text: 'Text',
      raw: 'Raw',
    },
    copy: 'Copy',
    copied: 'Copied!',
    download: 'Download',
    binaryFile: 'Binary file',
    imageUnavailable: 'Image preview unavailable.',
  },
  errors: {
    createFolder: 'Failed to create folder',
    listFiles: 'Failed to list files',
    renameFolder: 'Failed to rename folder',
    openTerminal: 'Failed to open terminal',
    readFile: 'Failed to read file',
  },
}

export type ArtifactPanelTranslations = typeof artifactPanelEn

export const artifactPanelKo: ArtifactPanelTranslations = {
  header: {
    title: '파일',
    sessionTab: '세션',
    workspaceTab: '워크스페이스',
  },
  loading: '불러오는 중...',
  actions: {
    cancel: '취소',
    create: '생성',
    save: '저장',
    rename: '이름 변경',
  },
  time: {
    secondsAgo: (n) => `${n}초 전`,
    minutesAgo: (n) => `${n}분 전`,
    hoursAgo: (n) => `${n}시간 전`,
    daysAgo: (n) => `${n}일 전`,
  },
  session: {
    empty: '이 세션에서 생성된 파일이 아직 없습니다.',
    action: {
      created: '생성됨',
      modified: '수정됨',
    },
  },
  workDir: {
    requiredTitle: '세션 아티팩트 디렉터리는 필수입니다',
    removeTitle: '현재 디렉터리 제거',
    addTitle: '디렉터리 추가',
    addWorkingTitle: '작업 디렉터리 추가',
  },
  picker: {
    title: '디렉터리 선택',
    selectHere: '여기 선택',
    noSubdirectories: '하위 디렉터리 없음',
  },
  folder: {
    newFolder: '새 폴더',
    newFolderPlaceholder: '새 폴더 이름',
    namePlaceholder: '폴더 이름',
  },
  workspace: {
    emptyDirectory: '빈 디렉터리',
    shell: '셸',
    shellTitle: (target) => `${target}에서 통합 터미널 열기`,
    openApp: '앱 열기',
    openAppTitle: (target) => `${target}에서 macOS 터미널 열기`,
    opening: '여는 중...',
    terminalOpened: (app, target) => `${target}에서 ${app} 열림`,
  },
  preview: {
    loadingFile: '파일 불러오는 중...',
    modes: {
      preview: '미리보기',
      code: '코드',
      text: '텍스트',
      raw: '원본',
    },
    copy: '복사',
    copied: '복사됨',
    download: '다운로드',
    binaryFile: '바이너리 파일',
    imageUnavailable: '이미지 미리보기를 사용할 수 없습니다.',
  },
  errors: {
    createFolder: '폴더를 생성하지 못했습니다',
    listFiles: '파일 목록을 불러오지 못했습니다',
    renameFolder: '폴더 이름을 변경하지 못했습니다',
    openTerminal: '터미널을 열지 못했습니다',
    readFile: '파일을 읽지 못했습니다',
  },
}
