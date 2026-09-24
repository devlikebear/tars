// Chat slash commands, goal and cwd feedback, and workbench jumps (Chat, lib/slash, SlashPopover, lib/workbenchActions, the session store).
export const chatCommandsEn = {
  selectSessionFirst: 'Select a session first',
  viewCleared: 'Chat view cleared',
  forkedSession: (title: string) => `Forked session: ${title}`,
  // The lazily loaded conversation column in Chat.
  chatPanel: {
    loading: 'Loading...',
    loadFailed: 'Could not load chat panel.',
  },
  cwd: {
    switched: (label: string) => `cwd → ${label}`,
    switchFailed: (message: string) => `cwd transition failed: ${message}`,
    noEligible: 'cwd: no eligible directories',
    // `/cwd list`: the active directory, then one indented line per candidate.
    activeList: (current: string, list: string) => `cwd active: ${current}\n${list}`,
    activeMarker: ' (active)',
    none: '(none)',
  },
  status: {
    failed: (message: string) => `status: ${message}`,
    loadFailed: 'failed to load codex quota',
  },
  skill: {
    usage: 'Usage: /skill <name>',
    notFound: (name: string) => `Skill not found: ${name}`,
    enabled: (name: string) => `Skill ${name} enabled`,
    disabled: (name: string) => `Skill ${name} disabled`,
    toggleFailed: 'Skill toggle failed',
  },
  // `/goal` feedback and the goal chip in the session header. `status` is a
  // label from `statuses`, or the server's word when it is not listed there.
  goal: {
    statuses: {
      active: 'active',
      satisfied: 'satisfied',
      exhausted: 'exhausted',
    },
    none: 'goal: (none) — usage: /goal <description> | /goal clear',
    show: (status: string, description: string, remaining: number, max: number) =>
      `goal [${status}]: ${description}\n  auto-continues remaining: ${remaining}/${max}`,
    cleared: 'goal cleared',
    set: (description: string) => `goal set: ${description}`,
    clearedEmpty: 'goal cleared (empty description)',
    failed: (message: string) => `goal: ${message}`,
    failedFallback: 'failed',
    chipLabel: 'goal',
    chipTitle: (status: string, description: string, used: number, max: number) =>
      `goal [${status}]: ${description}\nauto-continues used: ${used}/${max}`,
  },
  // Goal events from the chat stream (the session store's goalEventFeedback).
  goalEvents: {
    satisfied: (reason: string) => (reason ? `goal satisfied: ${reason}` : 'goal satisfied'),
    exhausted: (reason: string) =>
      reason ? `goal auto-continue budget exhausted (last: ${reason})` : 'goal auto-continue budget exhausted',
    autoContinue: (count: number, max: number) => `goal auto-continue ${count}/${max}`,
    judgeError: (reason: string) => `goal judge error: ${reason}`,
    unknownReason: 'unknown',
  },
  // Built-in slash commands (lib/slash). Command names and their arguments
  // (`clear`, `list`, `search`, `status`) are typed, so they stay English.
  slash: {
    builtins: {
      clear: { title: 'Clear', description: 'Clear the current chat view without deleting the session.' },
      status: { title: 'Status', description: 'Show the current Codex subscription quota inline.' },
      compact: { title: 'Compact', description: 'Compact the current session transcript.' },
      tasks: { title: 'Tasks', description: 'Open the session Tasks panel.' },
      config: { title: 'Config', description: 'Open session tool and skill settings.' },
      context: { title: 'Context', description: 'Open the LLM-facing context preview.' },
      prior: { title: 'Prior', description: 'Open the Prior Context preview.' },
      prompt: { title: 'Prompt', description: 'Open the session prompt editor.' },
      sysprompt: { title: 'System Prompt', description: 'Open the session prompt editor.' },
      files: { title: 'Files', description: 'Open the session Files panel.' },
      cron: { title: 'Cron', description: 'Open session cron jobs.' },
      memory: { title: 'Memory Search', description: 'Open Memory search; pass "search <query>" to prefill the query.' },
      skill: { title: 'Skill', description: 'Toggle a skill for the current session: /skill <name>.' },
      extractSkill: { title: 'Extract Skill', description: 'Open reusable skill candidates for the current session.' },
      cwd: { title: 'Active CWD', description: 'Show or switch the active working directory: /cwd | /cwd list | /cwd <path>.' },
      goal: {
        title: 'Session Goal',
        description: 'Set/clear an autonomous session goal: /goal <description> | /goal clear | /goal status.',
      },
    },
    // Skills and commands that ship without a description.
    noDescription: 'No description provided.',
  },
  slashPopover: {
    ariaLabel: 'Slash command suggestions',
    sections: {
      builtin: 'Built-in',
      command: 'Commands',
      skill: 'Skills',
    },
    kinds: {
      command: 'CMD',
      skill: 'SKILL',
    },
    // Shown when the server sends no source for the candidate.
    sources: {
      builtin: 'built-in',
      command: 'command',
      skill: 'skill',
    },
  },
  // Plan jumps in the session header's work strip (lib/workbenchActions).
  // `task` is the active task title, or '' when no task is in progress.
  // `/status` report lines (lib/codexStatus). The two window labels share a
  // width so their bars line up.
  codexStatus: {
    title: 'Codex status:',
    noTiers: 'Codex status: no openai-codex tiers configured.',
    awaiting: 'Awaiting first request…',
    noWindowData: '(no window data)',
    primary: 'primary',
    weekly: 'weekly ',
    resetsWithin: (reset: string, total: string) => `(resets ${reset} / ${total})`,
    resets: (reset: string) => `(resets ${reset})`,
    window: (total: string) => `(${total} window)`,
  },
  workbench: {
    ariaLabel: 'Workbench actions',
    tasks: 'Tasks',
    tasksTitle: (task: string) => `Open active plan tasks${task ? ` for ${task}` : ''}`,
    evidence: 'Evidence',
    evidenceTitle: (task: string) => `Open plan evidence${task ? ` for ${task}` : ''}`,
    agentRuntime: 'Agent Runtime',
    agentRuntimeTitle: 'Open Agent Runtime runs',
    git: 'Git',
    gitTitle: 'Open Git Inspector',
  },
}

export type ChatCommandsTranslations = typeof chatCommandsEn

export const chatCommandsKo: ChatCommandsTranslations = {
  selectSessionFirst: '먼저 세션을 선택하세요',
  viewCleared: '채팅 화면 지워짐',
  forkedSession: (title) => `분기된 세션: ${title}`,
  chatPanel: {
    loading: '불러오는 중...',
    loadFailed: '채팅 패널을 불러올 수 없습니다.',
  },
  cwd: {
    switched: (label) => `cwd → ${label}`,
    switchFailed: (message) => `cwd 전환 실패: ${message}`,
    noEligible: 'cwd: 사용 가능한 디렉터리 없음',
    activeList: (current, list) => `활성 cwd: ${current}\n${list}`,
    activeMarker: ' (활성)',
    none: '(없음)',
  },
  status: {
    failed: (message) => `상태: ${message}`,
    loadFailed: 'Codex 할당량을 불러오지 못했습니다',
  },
  skill: {
    usage: '사용법: /skill <이름>',
    notFound: (name) => `스킬을 찾을 수 없음: ${name}`,
    enabled: (name) => `스킬 ${name} 활성화됨`,
    disabled: (name) => `스킬 ${name} 비활성화됨`,
    toggleFailed: '스킬 전환 실패',
  },
  goal: {
    statuses: {
      active: '진행 중',
      satisfied: '달성',
      exhausted: '소진',
    },
    none: '목표: (없음) — 사용법: /goal <설명> | /goal clear',
    show: (status, description, remaining, max) =>
      `목표 [${status}]: ${description}\n  남은 자동 계속: ${remaining}/${max}`,
    cleared: '목표 해제됨',
    set: (description) => `목표 설정됨: ${description}`,
    clearedEmpty: '목표 해제됨 (빈 설명)',
    failed: (message) => `목표: ${message}`,
    failedFallback: '실패',
    chipLabel: '목표',
    chipTitle: (status, description, used, max) =>
      `목표 [${status}]: ${description}\n자동 계속 사용: ${used}/${max}`,
  },
  goalEvents: {
    satisfied: (reason) => (reason ? `목표 달성: ${reason}` : '목표 달성'),
    exhausted: (reason) => (reason ? `목표 자동 계속 한도 소진 (마지막: ${reason})` : '목표 자동 계속 한도 소진'),
    autoContinue: (count, max) => `목표 자동 계속 ${count}/${max}`,
    judgeError: (reason) => `목표 판정 오류: ${reason}`,
    unknownReason: '알 수 없음',
  },
  slash: {
    builtins: {
      clear: { title: '지우기', description: '세션을 삭제하지 않고 현재 채팅 화면을 지웁니다.' },
      status: { title: '상태', description: '현재 Codex 구독 할당량을 바로 표시합니다.' },
      compact: { title: '압축', description: '현재 세션의 대화 기록을 압축합니다.' },
      tasks: { title: '작업', description: '세션 작업 패널을 엽니다.' },
      config: { title: '설정', description: '세션 도구 및 스킬 설정을 엽니다.' },
      context: { title: '컨텍스트', description: 'LLM에 전달되는 컨텍스트 미리보기를 엽니다.' },
      prior: { title: '이전', description: '이전 컨텍스트 미리보기를 엽니다.' },
      prompt: { title: '프롬프트', description: '세션 프롬프트 편집기를 엽니다.' },
      sysprompt: { title: '시스템 프롬프트', description: '세션 프롬프트 편집기를 엽니다.' },
      files: { title: '파일', description: '세션 파일 패널을 엽니다.' },
      cron: { title: '크론', description: '세션 크론 작업을 엽니다.' },
      memory: { title: '메모리 검색', description: '메모리 검색을 엽니다. "search <검색어>"를 붙이면 검색어가 미리 입력됩니다.' },
      skill: { title: '스킬', description: '현재 세션에서 스킬을 켜거나 끕니다: /skill <이름>.' },
      extractSkill: { title: '스킬 추출', description: '현재 세션의 재사용 가능한 스킬 후보를 엽니다.' },
      cwd: { title: '활성 CWD', description: '활성 작업 디렉터리를 표시하거나 전환합니다: /cwd | /cwd list | /cwd <경로>.' },
      goal: {
        title: '세션 목표',
        description: '자율 세션 목표를 설정하거나 해제합니다: /goal <설명> | /goal clear | /goal status.',
      },
    },
    noDescription: '설명이 없습니다.',
  },
  slashPopover: {
    ariaLabel: '슬래시 명령 추천',
    sections: {
      builtin: '기본 제공',
      command: '명령',
      skill: '스킬',
    },
    kinds: {
      command: '명령',
      skill: '스킬',
    },
    sources: {
      builtin: '기본 제공',
      command: '명령',
      skill: '스킬',
    },
  },
  codexStatus: {
    title: 'Codex 상태:',
    noTiers: 'Codex 상태: openai-codex 티어가 설정되어 있지 않습니다.',
    awaiting: '첫 요청 대기 중…',
    noWindowData: '(기간 데이터 없음)',
    primary: '기본',
    weekly: '주간',
    resetsWithin: (reset, total) => `(${reset} 후 초기화 / ${total})`,
    resets: (reset) => `(${reset} 후 초기화)`,
    window: (total) => `(${total} 기간)`,
  },
  workbench: {
    ariaLabel: '워크벤치 동작',
    tasks: '작업',
    tasksTitle: (task) => (task ? `활성 계획 작업 열기: ${task}` : '활성 계획 작업 열기'),
    evidence: '증빙',
    evidenceTitle: (task) => (task ? `계획 증빙 열기: ${task}` : '계획 증빙 열기'),
    agentRuntime: '에이전트 런타임',
    agentRuntimeTitle: '에이전트 런타임 실행 열기',
    git: 'Git',
    gitTitle: 'Git 인스펙터 열기',
  },
}
