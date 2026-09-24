// Session health dock panel and report (SessionHealthPanel, lib/sessionHealth).
// `status`, `severity`, and `actions` are keyed by the report's own identifiers
// (SessionHealthStatus, SessionHealthSeverity, SessionHealthAction), so the
// builder and the panel index them directly.
export const sessionHealthEn = {
  panel: {
    eyebrow: 'Session Health',
    checking: 'Checking...',
    refresh: 'Refresh',
    metricsAriaLabel: 'Session health metrics',
    metrics: {
      messages: 'Messages',
      openTasks: 'Open tasks',
      riskTools: 'Risk tools',
      memory: 'Memory',
    },
    noWarnings: 'No active session warnings.',
    signalsAriaLabel: 'Session health signals',
    signals: 'Signals',
    recommendationsAriaLabel: 'Session health recommendations',
    recommendations: 'Recommendations',
    noActionNeeded: 'No action needed.',
    checkedAt: (time: string) => `Checked ${time}`,
  },
  status: {
    healthy: 'Healthy',
    watch: 'Watch',
    attention: 'Needs attention',
    critical: 'Critical',
  },
  severity: {
    info: 'Info',
    warning: 'Warning',
    error: 'Attention',
    critical: 'Critical',
  },
  actions: {
    compact: 'Compact',
    review_fork_points: 'Review Chat',
    open_tasks: 'Open Tasks',
    open_config: 'Open Config',
    open_prior: 'Open Prior',
    open_skill_extraction: 'Extract Skill',
  },
  summary: {
    critical: (count: number) => `${count} critical session issue(s) need action before continuing.`,
    attention: (count: number) => `${count} session issue(s) should be resolved soon.`,
    watch: (count: number) => `${count} session signal(s) are worth watching.`,
    healthy: 'No session warnings detected.',
  },
  // How long ago something happened, from a fractional day count.
  ago: (days: number) => `${days < 1 ? 'today' : `${Math.floor(days)}d`} ago`,
  signals: {
    contextSaturated: {
      title: 'Context is near saturation',
      detail: (messageCount: number) =>
        `${messageCount} transcript messages are loaded. Compact or split before the next major task.`,
    },
    contextLong: {
      title: 'Context is getting long',
      detail: (messageCount: number) => `${messageCount} transcript messages are loaded.`,
    },
    stalePlan: {
      title: 'Plan has gone stale',
      detail: (openTaskCount: number, ago: string) => `${openTaskCount} open task(s), last plan update ${ago}.`,
    },
    broadPermissions: {
      title: 'Broad high-risk permissions',
      detail: (toolCount: number) => `${toolCount} high-risk tool(s) are enabled for this session.`,
    },
    idlePermissions: {
      title: 'High-risk tools still enabled',
      detail: (toolCount: number) => `${toolCount} high-risk tool(s) remain enabled after the active task.`,
    },
    memoryNoise: {
      title: 'Prior context is noisy',
      detail: (memoryCount: number, memoryTokens: number) =>
        `${memoryCount} memory item(s) and ${memoryTokens} memory token(s) are attached.`,
    },
    staleSession: {
      title: 'Session has been idle',
      detail: (ago: string) => `Last updated ${ago}.`,
    },
  },
  recommendations: {
    compactLongContext: {
      title: 'Compact this session',
      detail: 'Shrink old turns into a summary so the next response has cleaner context.',
    },
    forkLongContext: {
      title: 'Split at a stable point',
      detail: 'Start the next task from a known-good message instead of carrying every turn forward.',
    },
    compactGrowingContext: {
      title: 'Compact soon',
      detail: 'The session is still workable, but context reuse is starting to cost attention.',
    },
    reviewStalePlan: {
      title: 'Review open tasks',
      detail: 'Close completed work, archive stale plan items, or rewrite the next step.',
    },
    trimPermissions: {
      title: 'Reduce session permissions',
      detail: 'Keep only the tool groups needed for the current task before enabling more automation.',
    },
    trimIdlePermissions: {
      title: 'Trim idle permissions',
      detail: 'Disable write or shell capabilities when the session is only being used for review.',
    },
    reviewPriorContext: {
      title: 'Review recalled memory',
      detail: 'Check whether the retrieved memory still matches this task before continuing.',
    },
    extractIdleSessionSkill: {
      title: 'Extract reusable work',
      detail: 'If this session produced a reusable workflow, turn it into a skill draft before archiving it.',
    },
  },
}

export type SessionHealthTranslations = typeof sessionHealthEn

export const sessionHealthKo: SessionHealthTranslations = {
  panel: {
    eyebrow: '세션 상태',
    checking: '확인 중...',
    refresh: '새로고침',
    metricsAriaLabel: '세션 상태 지표',
    metrics: {
      messages: '메시지',
      openTasks: '열린 작업',
      riskTools: '위험 도구',
      memory: '메모리',
    },
    noWarnings: '활성 세션 경고가 없습니다.',
    signalsAriaLabel: '세션 상태 신호',
    signals: '신호',
    recommendationsAriaLabel: '세션 상태 권장 조치',
    recommendations: '권장 조치',
    noActionNeeded: '필요한 조치가 없습니다.',
    checkedAt: (time) => `${time} 확인`,
  },
  status: {
    healthy: '양호',
    watch: '관찰 필요',
    attention: '조치 필요',
    critical: '심각',
  },
  severity: {
    info: '정보',
    warning: '경고',
    error: '조치 필요',
    critical: '심각',
  },
  actions: {
    compact: '압축',
    review_fork_points: '대화 검토',
    open_tasks: '작업 열기',
    open_config: '설정 열기',
    open_prior: '이전 컨텍스트 열기',
    open_skill_extraction: '스킬 추출',
  },
  summary: {
    critical: (count) => `계속하기 전에 조치가 필요한 심각한 세션 문제가 ${count}건 있습니다.`,
    attention: (count) => `곧 해결해야 할 세션 문제가 ${count}건 있습니다.`,
    watch: (count) => `지켜볼 만한 세션 신호가 ${count}건 있습니다.`,
    healthy: '감지된 세션 경고가 없습니다.',
  },
  ago: (days) => (days < 1 ? '오늘' : `${Math.floor(days)}일 전`),
  signals: {
    contextSaturated: {
      title: '컨텍스트 포화 임박',
      detail: (messageCount) =>
        `대화 메시지 ${messageCount}건이 로드되어 있습니다. 다음 주요 작업 전에 압축하거나 분할하세요.`,
    },
    contextLong: {
      title: '컨텍스트가 길어지는 중',
      detail: (messageCount) => `대화 메시지 ${messageCount}건이 로드되어 있습니다.`,
    },
    stalePlan: {
      title: '오래된 계획',
      detail: (openTaskCount, ago) => `열린 작업 ${openTaskCount}건, 마지막 계획 업데이트 ${ago}.`,
    },
    broadPermissions: {
      title: '광범위한 고위험 권한',
      detail: (toolCount) => `이 세션에 고위험 도구 ${toolCount}개가 활성화되어 있습니다.`,
    },
    idlePermissions: {
      title: '고위험 도구가 아직 활성화됨',
      detail: (toolCount) => `진행 중인 작업이 끝난 뒤에도 고위험 도구 ${toolCount}개가 활성화되어 있습니다.`,
    },
    memoryNoise: {
      title: '이전 컨텍스트에 잡음이 많음',
      detail: (memoryCount, memoryTokens) =>
        `메모리 항목 ${memoryCount}개, 메모리 토큰 ${memoryTokens}개가 첨부되어 있습니다.`,
    },
    staleSession: {
      title: '유휴 세션',
      detail: (ago) => `마지막 업데이트: ${ago}.`,
    },
  },
  recommendations: {
    compactLongContext: {
      title: '이 세션 압축',
      detail: '오래된 턴을 요약으로 줄여 다음 응답이 더 깔끔한 컨텍스트를 받도록 하세요.',
    },
    forkLongContext: {
      title: '안정적인 지점에서 분할',
      detail: '모든 턴을 이어 가지 말고, 정상으로 확인된 메시지에서 다음 작업을 시작하세요.',
    },
    compactGrowingContext: {
      title: '곧 압축 필요',
      detail: '세션은 아직 쓸 만하지만, 컨텍스트 재사용 비용이 커지기 시작했습니다.',
    },
    reviewStalePlan: {
      title: '열린 작업 검토',
      detail: '완료된 작업은 닫고, 오래된 계획 항목은 보관하거나 다음 단계를 다시 작성하세요.',
    },
    trimPermissions: {
      title: '세션 권한 줄이기',
      detail: '자동화를 더 켜기 전에 현재 작업에 필요한 도구 그룹만 남기세요.',
    },
    trimIdlePermissions: {
      title: '유휴 권한 정리',
      detail: '세션을 검토 용도로만 쓴다면 쓰기나 셸 기능을 끄세요.',
    },
    reviewPriorContext: {
      title: '회상된 메모리 검토',
      detail: '계속하기 전에 불러온 메모리가 여전히 이 작업에 맞는지 확인하세요.',
    },
    extractIdleSessionSkill: {
      title: '재사용할 작업 추출',
      detail: '이 세션에서 재사용할 만한 워크플로가 나왔다면, 보관하기 전에 스킬 초안으로 만드세요.',
    },
  },
}
