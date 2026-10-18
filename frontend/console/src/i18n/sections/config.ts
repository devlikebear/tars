// /console/config (#1238/#1239/#1241/#1242): tab/section names, the field
// label+description overrides keyed by the server's field `key`, search and
// save-bar copy, and value-display words used by lib/configStructured.ts.
//
// Field overrides are a Partial<Record<key, {...}>> rather than a fixed set
// of named properties: the server schema (internal/config/schema.go) is the
// source of truth for *which* keys exist, and only basic/advanced-tier keys
// need an entry here. A key missing from this map (including every `en`
// entry below) falls back to the server's own English label/description —
// see lib/configI18n.ts `localizeConfigField`. `en.fields` is intentionally
// empty: the server text already is the English copy.
export interface ConfigFieldOverride {
  label: string
  description: string
}

const fieldsEn: Record<string, ConfigFieldOverride> = {}

export const configEn = {
  pageTitle: 'Settings',
  loading: 'Loading configuration...',
  noConfigFile: 'No config file configured. Start TARS with --config <path> to manage settings.',
  configPath: (path: string) => path,

  tabs: {
    general: 'General',
    llm: 'LLM',
    chatTools: 'Chat & Tools',
    automation: 'Automation',
    channelsRemote: 'Channels & Remote',
    agents: 'Agents',
    extensions: 'Extensions',
    advanced: 'Advanced',
  },

  sections: {
    Runtime: 'Runtime',
    API: 'API',
    'Remote Access': 'Remote Access',
    LLM: 'LLM',
    Memory: 'Memory',
    Usage: 'Usage',
    Automation: 'Automation',
    Compaction: 'Compaction',
    Tools: 'Tools',
    MCP: 'MCP',
    'Agent Runtime': 'Agent Runtime',
    'Work Ledger': 'Work Ledger',
    'Work Scheduler': 'Work Scheduler',
    Channels: 'Channels',
    Console: 'Console',
    Companion: 'Companion',
    Initiative: 'Initiative',
    Extensions: 'Extensions',
    Embodiment: 'Embodiment',
  } as Record<string, string>,

  basicHeading: 'Basic',
  advancedHeading: (count: number) => `Advanced (${count})`,
  emptyTab: 'Nothing to show in this tab yet.',

  search: {
    label: 'Search settings',
    placeholder: 'Search settings (label, path, env var)...',
    clear: 'Clear',
    noResults: 'No matching settings.',
    resultsCount: (count: number) => `${count} result${count === 1 ? '' : 's'}`,
    jumpTo: 'Go to field',
    filters: {
      changedFromDefault: 'Changed from default',
      envOverridden: 'Env override only',
      unsaved: 'Unsaved changes only',
    },
  },

  field: {
    changedBadge: 'Changed',
    revertToDefault: 'Reset to default',
    secretBadge: 'Secret',
    emptyPlaceholder: '(empty)',
    jsonInvalid: 'Invalid JSON',
    listHint: 'One value per line',
    envOverrideNote: (envKey: string) => `Overridden by ${envKey}`,
    envOverrideActive: (value: string) => `Active: ${value}`,
    toggleOn: 'On',
    toggleOff: 'Off',
    selectNone: '(none)',
    path: (path: string) => path,
  },

  saveBar: {
    changedCount: (count: number) => `${count} change${count === 1 ? '' : 's'}`,
    discard: 'Discard',
    save: 'Save',
    saving: 'Saving...',
    saveAndRestart: 'Save and restart',
    restartNotice: 'Saving requires a restart to apply.',
    viewChanges: 'View changes',
  },

  confirmRestart: 'Confirm restart',
  reconnect: 'Reconnect',
  savedPending: 'Saved; restart required to apply. Environment overrides take precedence.',
  applied: 'Runtime matches effective configuration.',
  saveApply: 'Save and apply',
  reconnecting: 'Restart requested; checking reconnection and runtime settings…',
  reconnectFailed: 'Could not confirm restart. Check the server, address and authentication; retry reconnecting or restart manually. Saved settings are retained.',
  invalidValue: 'Invalid value',

  // ConfigLLMEditor.svelte (#1286): row-based editing for llm_providers /
  // llm_tiers / llm_role_defaults, replacing the raw-JSON-only editor those
  // three fields used to fall back on.
  llm: {
    providersTitle: 'Providers',
    providersHint: 'Each alias is a credential and endpoint the tiers below point at.',
    addProvider: 'Add provider',
    removeProvider: 'Remove',
    aliasLabel: 'Alias',
    aliasPlaceholder: 'e.g. openai-main',
    kindLabel: 'Kind',
    kindPlaceholder: 'Pick a kind',
    authModeLabel: 'Auth mode',
    apiKeyLabel: 'API key',
    apiKeyKeepHint: '(leave blank to keep the existing key)',
    baseUrlLabel: 'Base URL',
    noProviders: 'No providers yet. Add one below.',
    tiersTitle: 'Tiers',
    tiersHint: 'heavy / standard / light are fixed; each must point at a provider above.',
    tierProviderLabel: 'Provider',
    tierProviderPlaceholder: 'Pick a provider',
    tierModelLabel: 'Model',
    tierReasoningLabel: 'Reasoning effort',
    tierReasoningDefault: '(provider default)',
    tierMaxTokensLabel: 'Max tokens',
    tierMaxTokensPlaceholder: '(provider default)',
    rolesTitle: 'Role defaults',
    rolesHint: 'Overrides the default tier for one runtime role.',
    addRole: 'Add role default',
    removeRole: 'Remove',
    roleLabel: 'Role',
    rolePlaceholder: 'e.g. pulse_decider',
    roleTierLabel: 'Tier',
    roleTierPlaceholder: 'Pick a tier',
    noRoles: 'No role overrides. Every role uses the default tier above.',
    editAsJson: 'Edit as JSON',
    editAsTable: 'Edit as table',
    jsonInvalid: 'Invalid JSON',
  },

  diff: {
    title: 'Pending Changes',
    close: 'Close',
    impact: 'Impact',
  },

  restart: {
    title: 'Server',
    label: 'Restart server',
    description: 'Apply config changes by restarting the TARS process. Service mode uses launchctl, dev mode re-execs the process.',
    confirm: 'Click again to confirm',
    restarting: 'Restarting...',
    cancel: 'Cancel',
  },

  dangerZone: {
    title: 'Danger Zone',
    resetLabel: 'Reset Workspace',
    resetDescription: 'Remove sessions, cron state, agent runtime data, logs, and memory. Config is preserved.',
    confirm: 'Click again to confirm',
    resetting: 'Resetting...',
  },

  workspaceResetSuccess: (removed: number) => `Workspace reset: ${removed} items removed. Restart TARS to reinitialize.`,
  failedResetWorkspace: 'Failed to reset workspace',

  values: {
    items: (count: number) => `${count} ${count === 1 ? 'item' : 'items'}`,
    keys: (count: number) => `${count} ${count === 1 ? 'key' : 'keys'}`,
    empty: '-',
  },

  fields: fieldsEn,
}

export type ConfigTranslations = typeof configEn

// -- Korean field overrides (#1241). Avoid writing two bare English words
// back to back (e.g. "Tailscale Serve"): the i18nSectionsKorean test flags
// any untranslated two-word English run, so a kept-in-English proper noun
// (Tailscale, Perplexity, jev...) stays a single token with Korean on
// at least one side. --
const fieldsKo: Record<string, ConfigFieldOverride> = {
  // Runtime
  workspace_dir: { label: '워크스페이스 디렉터리', description: '세션과 워크스페이스 데이터를 저장할 디렉터리' },
  plan_clarify_mode: {
    label: '계획 명확화 모드',
    description: '다단계 요청이 모호할 때 계획을 세우기 전 처리 방식: smart = LLM이 모호함을 직접 판단, auto = 항상 바로 계획, ask = 항상 먼저 1~3개의 확인 질문을 합니다.',
  },
  session_auto_archive_days: { label: '세션 자동 보관(일)', description: '이 일수 동안 갱신되지 않은 세션을 보관합니다. 0이면 끕니다' },
  session_auto_delete_days: { label: '보관된 세션 자동 삭제(일)', description: '보관된 뒤 이 일수 동안 손대지 않은 세션을 함께 저장된 것과 모두 삭제합니다. 0이면 끕니다' },
  session_telegram_scope: { label: 'Telegram 세션 범위', description: 'Telegram 메시지의 세션 범위 지정 방식' },
  style_directness_default: { label: '말투 직접성 기본값', description: '세션 말투 설정의 직접성 슬라이더 전역 기본값' },
  style_humor_default: { label: '말투 유머 기본값', description: '세션 말투 설정의 유머 슬라이더 전역 기본값' },
  style_caution_default: { label: '말투 신중함 기본값', description: '세션 말투 설정의 신중함 슬라이더 전역 기본값' },
  style_autonomy_default: { label: '말투 자율성 기본값', description: '세션 말투 설정의 자율성 슬라이더 전역 기본값' },
  log_level: { label: '로그 레벨', description: '로깅 상세도' },
  log_file: { label: '로그 파일', description: '로그 파일 경로(비우면 표준 에러로 출력)' },

  // API
  api_auth_mode: { label: '인증 모드', description: 'API 인증 모드' },
  dashboard_auth_mode: { label: '대시보드 인증 모드', description: "대시보드 인증 모드. 'off'는 /v1/*는 보호한 채 대시보드 인증만 끕니다" },
  api_user_token: { label: '사용자 토큰', description: '사용자 등급 베어러 토큰(읽기·채팅 등 일반 작업)' },
  api_admin_token: { label: '관리자 토큰', description: '관리자 등급 베어러 토큰(제어 작업, 에이전트 런타임, 설정)' },
  api_allow_insecure_local_auth: { label: '루프백 인증 생략 허용', description: '127.0.0.1 루프백 요청에 인증 토큰 없이 접근을 허용합니다' },

  // Remote Access
  remote_access_tailscale_serve_enabled: { label: 'Tailscale 원격 접속', description: 'Tailscale로 로컬 콘솔을 외부에 노출합니다' },
  remote_access_tailscale_serve_https_port: { label: 'Tailscale 접속용 HTTPS 포트', description: 'Tailscale 안에 공개할 HTTPS 포트' },

  // LLM
  llm_providers: { label: '프로바이더', description: '별칭으로 키를 둔 프로바이더 풀입니다. 각 항목은 프로바이더 단위로 kind·auth·base_url·api_key를 정의합니다' },
  llm_tiers: { label: '티어', description: 'heavy/standard/light(또는 사용자 정의 티어)로 키를 둔 티어 바인딩입니다. 각 항목이 프로바이더·모델·추론 설정을 묶습니다' },
  llm_default_tier: { label: '기본 티어', description: '명시적 override가 없는 역할이 쓰는 티어' },
  llm_role_defaults: { label: '역할별 기본 티어', description: '런타임 역할 이름 → 티어 별칭 매핑(예: chat_main -> standard)' },

  // Memory
  memory_semantic_enabled: { label: '시맨틱 메모리', description: '벡터 임베딩 기반 시맨틱 메모리를 사용합니다' },
  memory_embed_provider: { label: '임베딩 프로바이더', description: '임베딩 프로바이더' },
  memory_embed_base_url: { label: '임베딩 기본 URL', description: '임베딩 API의 기본 주소' },
  memory_embed_api_key: { label: '임베딩 API 키', description: '임베딩 프로바이더의 API 키' },
  memory_embed_model: { label: '임베딩 모델', description: '임베딩 모델 식별자' },
  memory_embed_dimensions: { label: '임베딩 차원', description: '임베딩 벡터 차원 수' },

  // Usage
  usage_limit_daily_usd: { label: '일일 한도(USD)', description: '하루 LLM 지출 최대 금액(USD)' },
  usage_limit_weekly_usd: { label: '주간 한도(USD)', description: '일주일 LLM 지출 최대 금액(USD)' },
  usage_limit_monthly_usd: { label: '월간 한도(USD)', description: '한 달 LLM 지출 최대 금액(USD)' },
  usage_daily_token_budget: { label: '일일 토큰 예산', description: '콘솔 사용량 표시에 쓰는 하루 입력+출력 토큰 예산. 0이면 칩을 끕니다' },
  usage_limit_mode: { label: '한도 적용 방식', description: '한도 적용 방식' },

  // Automation
  agent_max_iterations: {
    label: '최대 반복 횟수',
    description: '폭주 방지 상한: 한 턴이 돌 수 있는 최대 도구 라운드 수. 반복 호출이나 연속된 도구 오류가 있으면 더 일찍 멈춥니다',
  },
  pulse_enabled: { label: 'Pulse 사용', description: 'Pulse 시스템 감시를 사용합니다' },
  pulse_active_hours: { label: 'Pulse 동작 시간', description: 'Pulse가 동작하는 시간대(HH:MM-HH:MM)' },
  pulse_timezone: { label: 'Pulse 시간대', description: 'Pulse 동작 시간에 쓰는 시간대' },
  pulse_min_severity: { label: 'Pulse 최소 심각도', description: '알림을 보낼 최소 심각도' },
  pulse_decider: { label: 'Pulse 판단 방식', description: '신호가 있는 틱을 분류하는 방식: rules(모델 없음) 또는 llm' },
  pulse_allowed_autofixes_json: { label: 'Pulse 자동수정 허용 목록', description: '판단 로직이 실행할 수 있는 자동수정 목록' },
  pulse_notify_telegram: { label: 'Pulse 알림을 Telegram으로', description: 'Pulse 알림을 Telegram으로 전달합니다' },
  pulse_notify_session_events: { label: 'Pulse 알림을 세션 이벤트로', description: 'Pulse 알림을 세션 이벤트 스트림으로 전달합니다' },
  reflection_enabled: { label: '야간 회고 사용', description: '야간 회고 일괄 처리를 사용합니다' },
  reflection_sleep_window: { label: '회고 수면 시간대', description: '회고를 실행할 수 있는 HH:MM-HH:MM 시간대' },
  reflection_timezone: { label: '회고 시간대', description: '회고 수면 시간대 평가에 쓰는 시간대' },
  notify_command: { label: '알림 명령', description: '알림이 발생하면 실행할 셸 명령' },
  notify_when_no_clients: { label: '구독자 없어도 알림', description: 'SSE로 연결된 클라이언트가 없어도 알림을 보냅니다' },

  // Compaction
  compaction_trigger_tokens: { label: '압축 트리거 토큰', description: '자동 압축을 유발하는 예상 transcript 토큰 수' },
  compaction_llm_mode: { label: '압축 LLM 모드', description: '압축이 LLM을 호출할 수 있는지, 결정적 방식만 쓸지' },

  // Tools
  tools_web_search_enabled: { label: '웹 검색', description: '웹 검색 도구를 사용합니다' },
  tools_web_fetch_enabled: { label: '웹 가져오기', description: '웹 가져오기 도구를 사용합니다' },
  tools_default_set: { label: '기본 도구 세트', description: '채팅 세션의 기본 도구 정책/프로필' },
  tools_allow_high_risk_user: { label: '고위험 도구 허용', description: '사용자 등급에 고위험 도구 접근을 허용합니다' },
  tools_web_search_api_key: { label: '검색 API 키', description: '웹 검색 프로바이더의 API 키' },
  tools_web_search_provider: { label: '검색 프로바이더', description: '웹 검색 백엔드' },
  tools_web_search_perplexity_api_key: { label: 'Perplexity의 API 키', description: '검색 프로바이더가 Perplexity일 때 쓰는 API 키' },
  tools_web_search_perplexity_model: { label: 'Perplexity 모델', description: 'Perplexity 기반 검색에 쓰는 모델 이름' },
  tools_web_search_perplexity_base_url: { label: 'Perplexity 기본 주소', description: 'Perplexity 호출에 사용할 주소를 재정의합니다' },
  tools_exec_max_timeout_ms: {
    label: 'exec 최대 타임아웃(ms)',
    description: 'exec 도구 호출 1회의 타임아웃 상한(ms). 빌드·CI 대기 같은 장시간 명령이 이 한도를 씁니다',
  },
  tools_process_max_timeout_ms: {
    label: '백그라운드 프로세스 최대 타임아웃(ms)',
    description: '백그라운드 명령(exec의 background:true 옵션)과 process 도구 wait 동작 1회의 타임아웃 상한',
  },
  tools_web_fetch_allow_private_hosts: { label: '내부 호스트 접근 허용', description: '사설/내부 호스트에서 가져오기를 허용합니다' },
  tools_web_fetch_private_host_allowlist_json: { label: '내부 호스트 허용 목록', description: '웹 가져오기 요청에 명시적으로 허용할 사설 호스트 목록' },
  tools_apply_patch_enabled: { label: '패치 적용 도구', description: 'apply-patch 도구를 사용합니다' },
  tools_message_enabled: { label: '메시지 도구', description: '메시지/알림 도구를 사용합니다' },
  tools_agentruntime_enabled: { label: '에이전트 런타임 도구', description: '에이전트 런타임 디스패치 도구를 사용합니다' },
  tools_computer_use_backend: { label: 'computer_use 백엔드', description: '판단 백엔드: llm(기본으로는 설정된 light 티어) 또는 jev(jev.base_url)' },
  tools_computer_use_enabled: {
    label: 'computer_use 사용',
    description: '데스크톱 앱을 접근성 트리로 조작하는 computer_use 도구를 사용합니다. 화면 텍스트는 기본적으로 설정된 LLM 프로바이더로 보내지며, backend: jev이면 jev.base_url로 보냅니다',
  },
  tools_computer_use_cua_driver_path: { label: 'cua-driver 경로', description: '비우면 CUA_DRIVER_PATH, 그다음 PATH를 씁니다' },
  tools_computer_use_max_steps: { label: 'computer_use 최대 단계', description: 'computer_use 호출 1회의 기본 단계 예산(상한 50)' },
  tools_computer_use_total_timeout_seconds: { label: 'computer_use 전체 타임아웃(초)', description: 'computer_use 호출 1회의 총 시간 예산' },
  tools_computer_use_expose_values: { label: 'computer_use 값 전송', description: '화면 텍스트와 함께 요소 값(입력 내용)을 보냅니다. 보안 필드는 절대 보내지 않습니다' },

  // MCP
  mcp_command_allowlist_json: { label: 'MCP 명령 허용 목록', description: '내장되거나 설치된 MCP 서버가 실행할 수 있는 명령' },
  mcp_servers_json: { label: 'MCP 서버', description: 'YAML에 직접 설정한 MCP 서버 카탈로그 항목(선택)' },

  // Agent Runtime
  agentruntime_enabled: { label: '사용', description: '다중 에이전트 오케스트레이션을 위한 에이전트 런타임을 사용합니다' },
  agentruntime_default_agent: { label: '기본 에이전트', description: '디스패치된 작업의 기본 에이전트 이름' },
  agentruntime_subagents_max_threads: { label: '최대 서브에이전트 스레드', description: '동시에 실행할 수 있는 서브에이전트 스레드 최대 수' },
  agentruntime_subagents_max_depth: { label: '최대 서브에이전트 깊이', description: '서브에이전트 중첩 최대 깊이' },
  agentruntime_consensus_enabled: { label: '합의 모드 사용', description: '고급 옵트인: subagents_run에 mode=consensus를 노출하고 합의 실행을 허용합니다' },
  agentruntime_consensus_max_fanout: { label: '합의 최대 분기', description: '고급 옵트인: 합의 작업 1건에 띄울 수 있는 최대 변형 수' },
  agentruntime_consensus_budget_usd: { label: '합의 예산(USD)', description: '고급 옵트인: 합의 실행 1회의 비용 상한' },

  // Work Ledger
  work_ledger_enabled: { label: '사용', description: '추가 전용으로 쌓이는 작업 원장과 그 읽기 전용 투영을 사용합니다' },

  // Work Scheduler
  work_scheduler_enabled: { label: '사용', description: '작업 원장 단계를 채팅 요청과 독립적으로 실행합니다' },
  work_scheduler_max_workers: { label: '최대 워커 수', description: '동시에 실행할 수 있는 지속 단계 실행 최대 수' },

  // Channels
  channels_webhook_enabled: { label: '웹훅 채널', description: '인바운드 웹훅 채널을 사용합니다' },
  channels_telegram_enabled: { label: 'Telegram 채널', description: 'Telegram 봇 채널을 사용합니다' },
  channels_telegram_dm_policy: { label: 'Telegram의 DM 정책', description: 'DM 접근 정책' },
  channels_telegram_polling_enabled: { label: 'Telegram 폴링', description: '업데이트 수신을 위한 Telegram 롱폴링을 사용합니다' },
  telegram_bot_token: { label: 'Telegram 봇 토큰', description: '@BotFather에서 받은 봇 토큰' },

  // Console
  console_default_mode: { label: '콘솔 기본 모드', description: '/console이 열리는 모드: focus(개발 파이프라인) 또는 advanced(워크벤치). 비우면 advanced.' },

  // Companion
  companion_enabled: { label: '사용', description: '콘솔에 떠 있는 TARS 컴패니언을 표시합니다' },

  // Initiative
  initiative_enabled: { label: '사용', description: 'TARS가 먼저 말할지 스스로 판단하게 합니다(shadow 모드는 판단만 기록합니다)' },
  initiative_mode: { label: '모드', description: 'shadow는 말하지 않고 판단만 기록합니다. live는 콘솔로 greet/check_in을 작성해 전달합니다' },
  initiative_backend: { label: '텍스트 신호 백엔드', description: 'llm은 설정된 initiative 역할(기본 light 티어)에 묻고, jev는 jev.base_url에 묻습니다' },
  initiative_quiet_hours: { label: '조용한 시간', description: '이 시간대(HH:MM-HH:MM)에는 먼저 말하지 않습니다' },
  initiative_timezone: { label: '시간대', description: '조용한 시간 판단에 쓰는 IANA 시간대(비우면 서버 로컬 시간)' },
  initiative_daily_cap: { label: '하루 최대 횟수', description: '하루에 말할 수 있는 최대 횟수' },
  initiative_cooldown: { label: '쿨다운', description: '먼저 말한 뒤 다음까지의 최소 간격' },
  initiative_daily_text_calls: { label: '하루 텍스트 신호 호출 수', description: '하루에 텍스트 신호 백엔드(llm 또는 jev)를 호출할 수 있는 최대 횟수' },
  initiative_daily_speak_calls: { label: '하루 작성 호출 수', description: 'live 모드 작성 LLM 호출을 하루에 할 수 있는 최대 횟수' },
  initiative_telegram: {
    label: 'Telegram으로 전달',
    description: '콘솔이 연결되어 있지 않을 때 live 모드로 말한 내용을 Telegram 기본 페어링 채팅으로 대신 전달합니다. 실제 메시지가 사용자 휴대폰으로 갑니다',
  },
  jev_base_url: { label: 'jev 서버 주소', description: '사용자 원문은 루프백 호스트일 때만 보내는 /v1/systemone 서버의 기본 주소입니다' },
  jev_api_key: { label: 'jev용 API 키', description: '호스팅된 jev 서버에 쓰는 베어러 토큰' },
  jev_model: { label: 'jev 모델', description: '요청마다 함께 보낼 모델 이름' },

  // Extensions
  skills_enabled: { label: '스킬 사용', description: '스킬 정의를 불러오고 제공합니다' },
  skills_extra_dirs_json: { label: '스킬 추가 디렉터리', description: '사용자 스킬 정의를 찾을 추가 디렉터리' },
  plugins_enabled: { label: '플러그인 사용', description: '플러그인 정의를 불러오고 제공합니다' },
  plugins_extra_dirs_json: { label: '플러그인 추가 디렉터리', description: '사용자 플러그인 정의를 찾을 추가 디렉터리' },
  plugins_allow_mcp_servers: { label: '플러그인의 MCP 등록 허용', description: '플러그인이 MCP 서버를 등록하도록 허용합니다' },
}

export const configKo: ConfigTranslations = {
  pageTitle: '설정',
  loading: '설정 불러오는 중...',
  noConfigFile: '구성된 설정 파일이 없습니다. TARS를 --config <경로>로 실행해 설정을 관리하세요.',
  configPath: (path) => path,

  tabs: {
    general: '일반',
    llm: 'LLM',
    chatTools: '채팅·도구',
    automation: '자동화',
    channelsRemote: '채널·원격',
    agents: '에이전트',
    extensions: '확장',
    advanced: '고급',
  },

  sections: {
    Runtime: '런타임',
    API: 'API',
    'Remote Access': '원격 접속',
    LLM: 'LLM',
    Memory: '메모리',
    Usage: '사용량',
    Automation: '자동화',
    Compaction: '컨텍스트 압축',
    Tools: '도구',
    MCP: 'MCP',
    'Agent Runtime': '에이전트 런타임',
    'Work Ledger': '작업 원장',
    'Work Scheduler': '작업 스케줄러',
    Channels: '채널',
    Console: '콘솔',
    Companion: '컴패니언',
    Initiative: 'Initiative',
    Extensions: '확장',
    Embodiment: 'Embodiment',
  },

  basicHeading: '기본',
  advancedHeading: (count) => `고급 (${count}개)`,
  emptyTab: '이 탭에는 아직 보여줄 설정이 없습니다.',

  search: {
    label: '설정 검색',
    placeholder: '설정 검색 (라벨 · 경로 · 환경변수)...',
    clear: '지우기',
    noResults: '일치하는 설정이 없습니다.',
    resultsCount: (count) => `${count}개 결과`,
    jumpTo: '필드로 이동',
    filters: {
      changedFromDefault: '기본값에서 바뀐 것만',
      envOverridden: '환경변수가 덮어쓴 것만',
      unsaved: '저장 안 된 변경만',
    },
  },

  field: {
    changedBadge: '변경됨',
    revertToDefault: '기본값으로 되돌리기',
    secretBadge: '비밀',
    emptyPlaceholder: '(없음)',
    jsonInvalid: '올바르지 않은 JSON입니다',
    listHint: '줄마다 값 하나씩',
    envOverrideNote: (envKey) => `${envKey} 환경변수가 덮어씀`,
    envOverrideActive: (value) => `적용 중: ${value}`,
    toggleOn: '켜짐',
    toggleOff: '꺼짐',
    selectNone: '(없음)',
    path: (path) => path,
  },

  saveBar: {
    changedCount: (count) => `${count}개 변경`,
    discard: '되돌리기',
    save: '저장',
    saving: '저장 중...',
    saveAndRestart: '저장 후 재시작',
    restartNotice: '저장한 내용을 적용하려면 재시작이 필요합니다.',
    viewChanges: '변경 내역 보기',
  },

  confirmRestart: '재시작 확인',
  reconnect: '다시 연결',
  savedPending: '저장됨 · 적용하려면 재시작이 필요합니다. 환경변수가 우선 적용됩니다.',
  applied: '실행 중 서버와 적용 대상 설정이 일치합니다.',
  saveApply: '저장 후 적용',
  reconnecting: '재시작 요청됨 · 연결 복구와 실행 설정을 확인 중입니다…',
  reconnectFailed: '재시작을 확인할 수 없습니다. 서버 상태·주소·인증 정보를 확인하고 다시 연결하거나 수동 재시작하세요. 저장한 설정은 유지됩니다.',
  invalidValue: '올바르지 않은 값입니다',

  llm: {
    providersTitle: '프로바이더',
    providersHint: '각 별칭은 아래 티어가 가리키는 자격증명·엔드포인트 하나입니다.',
    addProvider: '프로바이더 추가',
    removeProvider: '삭제',
    aliasLabel: '별칭',
    aliasPlaceholder: '예: openai-main',
    kindLabel: '종류',
    kindPlaceholder: '종류 선택',
    authModeLabel: '인증 방식',
    apiKeyLabel: 'API 키',
    apiKeyKeepHint: '(비워 두면 기존 키를 유지합니다)',
    baseUrlLabel: '기본 URL',
    noProviders: '아직 프로바이더가 없습니다. 아래에서 추가하세요.',
    tiersTitle: '티어',
    tiersHint: 'heavy / standard / light는 고정이며, 각각 위의 프로바이더 하나를 가리켜야 합니다.',
    tierProviderLabel: '프로바이더',
    tierProviderPlaceholder: '프로바이더 선택',
    tierModelLabel: '모델',
    tierReasoningLabel: '추론 강도',
    tierReasoningDefault: '(프로바이더 기본값)',
    tierMaxTokensLabel: '최대 토큰',
    tierMaxTokensPlaceholder: '(프로바이더 기본값)',
    rolesTitle: '역할별 기본 티어',
    rolesHint: '런타임 역할 하나의 기본 티어를 덮어씁니다.',
    addRole: '역할 기본값 추가',
    removeRole: '삭제',
    roleLabel: '역할',
    rolePlaceholder: '예: pulse_decider',
    roleTierLabel: '티어',
    roleTierPlaceholder: '티어 선택',
    noRoles: '역할별 override가 없습니다. 모든 역할이 위의 기본 티어를 씁니다.',
    editAsJson: 'JSON으로 편집',
    editAsTable: '표로 편집',
    jsonInvalid: '올바르지 않은 JSON입니다',
  },

  diff: {
    title: '저장 전 변경 내역',
    close: '닫기',
    impact: '영향',
  },

  restart: {
    title: '서버',
    label: '서버 재시작',
    description: 'TARS 프로세스를 재시작해 설정 변경을 적용합니다. 서비스 모드는 launchctl을, 개발 모드는 프로세스 재실행을 씁니다.',
    confirm: '다시 클릭해 확인',
    restarting: '재시작 중...',
    cancel: '취소',
  },

  dangerZone: {
    title: '위험 구역',
    resetLabel: '워크스페이스 초기화',
    resetDescription: '세션, cron 상태, 에이전트 런타임 데이터, 로그, 메모리를 제거합니다. 설정은 유지됩니다.',
    confirm: '다시 클릭해 확인',
    resetting: '초기화 중...',
  },

  workspaceResetSuccess: (removed) => `워크스페이스 초기화: ${removed}개 항목 제거됨. TARS를 재시작해 다시 초기화하세요.`,
  failedResetWorkspace: '워크스페이스 초기화 실패',

  values: {
    items: (count) => `${count}개 항목`,
    keys: (count) => `${count}개 키`,
    empty: '-',
  },

  fields: fieldsKo,
}
