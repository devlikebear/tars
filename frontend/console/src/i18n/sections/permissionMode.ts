// Tool permission modes (#970): the status bar switch, ⇧Tab, and the plan
// mode bar above the composer.
export const permissionModeEn = {
  label: 'Permissions',
  modes: {
    manual: 'Ask',
    accept_edits: 'Accept edits',
    plan: 'Plan',
    auto: 'Auto',
  },
  modeHints: {
    manual: 'Ask before file edits, commands, and other risky tools',
    accept_edits: 'File edits run without asking; commands still ask',
    plan: 'Read-only: the agent plans, nothing is changed',
    auto: 'Run tools without asking (the configured tool policy still applies)',
  },
  inherit: (mode: string) => `Default (${mode})`,
  inheritPlain: 'Default',
  inheritTitle: {
    config: 'The configured default',
    override: 'Set in this folder’s .tars settings',
    session: 'Chosen for this session',
  },
  switchHint: 'Tool permission mode for this session. ⇧Tab in the composer cycles it.',
  switchFailed: 'Could not change the permission mode.',
  cliPolicy: 'CLI policy',
  cliPolicyHint: 'antigravity-cli runs its own tools under your Antigravity settings. TARS cannot ask for approval in the conversation, so modes do not apply; turn changes are still recorded.',
  planBar: {
    title: 'Plan mode: read-only',
    hint: 'When the plan looks right, approve it to start making changes.',
    approveEdits: 'Approve plan · accept edits',
    approveAuto: 'Approve plan · auto',
    proceed: 'The plan is approved. Go ahead and carry it out.',
  },
}

export type PermissionModeTranslations = typeof permissionModeEn

export const permissionModeKo: PermissionModeTranslations = {
  label: '권한',
  modes: {
    manual: '묻기',
    accept_edits: '편집 자동 허용',
    plan: '계획',
    auto: '자동',
  },
  modeHints: {
    manual: '파일 편집, 명령 등 위험한 도구 전에 묻습니다',
    accept_edits: '파일 편집은 묻지 않고, 명령은 계속 묻습니다',
    plan: '읽기 전용: 에이전트가 계획만 세우고 아무것도 바꾸지 않습니다',
    auto: '묻지 않고 도구를 실행합니다(설정된 도구 정책은 그대로 적용)',
  },
  inherit: (mode: string) => `기본값 (${mode})`,
  inheritPlain: '기본값',
  inheritTitle: {
    config: '설정의 기본값',
    override: '이 폴더의 .tars 설정에서 정한 값',
    session: '이 세션에서 고른 값',
  },
  switchHint: '이 세션의 도구 권한 모드입니다. 입력창에서 ⇧Tab으로 바꿀 수 있습니다.',
  switchFailed: '권한 모드를 바꾸지 못했습니다.',
  cliPolicy: 'CLI 정책',
  cliPolicyHint: 'antigravity-cli는 사용자의 Antigravity 설정에 따라 자기 도구를 실행합니다. TARS가 대화 안에서 승인을 물을 수 없어서 모드가 적용되지 않습니다. 턴 변경 기록은 그대로 남습니다.',
  planBar: {
    title: '계획 모드: 읽기 전용',
    hint: '계획이 괜찮으면 승인해서 변경을 시작하세요.',
    approveEdits: '계획 승인 · 편집 자동 허용',
    approveAuto: '계획 승인 · 자동',
    proceed: '계획을 승인합니다. 그대로 진행해 주세요.',
  },
}
