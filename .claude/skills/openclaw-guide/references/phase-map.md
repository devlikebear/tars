# TARS Phase → OpenClaw Reference Map

> TARS 각 Phase 개발 시 참조해야 할 OpenClaw 문서/소스 매핑

## Phase 1: LLM 대화형 채팅

| Sub-task | OpenClaw Reference | Key Concept |
|----------|-------------------|-------------|
| 워크스페이스 부트스트랩 | `docs/concepts/agent-workspace.md`, `docs/reference/templates/` | SOUL.md, USER.md, IDENTITY.md 등 부트스트랩 파일 |
| 세션 관리 | `docs/concepts/session.md`, `docs/reference/session-management-compaction.md` | sessions.json + JSONL 2계층, sessionKey/sessionId |
| 시스템 프롬프트 | `docs/concepts/system-prompt.md` | 프롬프트 섹션 구조, 부트스트랩 파일 주입 |
| LLM Chat API | `docs/concepts/agent-loop.md` (추론 부분) | messages 배열, tool_calls, SSE 스트리밍 |
| 컨텍스트 압축 | `docs/concepts/compaction.md` | 자동/수동 압축, pre-compaction flush |
| 세션 CLI | `docs/cli/sessions.md` | 세션 목록, 전환, 검색, 내보내기 |

### Repomix grep 패턴

```
# 세션 관리 구조
"sessionKey|sessionId|sessions\.json"

# 시스템 프롬프트 조립
"buildSystemPrompt|assemblePrompt|bootstrapMax"

# JSONL transcript
"transcript|\.jsonl|appendMessage"

# 컴팩션
"compaction|compact|summarize.*messages"

# SSE 스트리밍
"text/event-stream|EventSource|onDelta|stream"
```

---

## Phase 2: 빌트인 도구 + Agent Loop

| Sub-task | OpenClaw Reference | Key Concept |
|----------|-------------------|-------------|
| 도구 인터페이스 | `docs/tools/index.md`, `docs/plugins/agent-tools.md` | Tool 스키마, allow/deny, 프로파일 |
| exec 도구 | `docs/tools/exec.md` | 셸 실행, timeout, background, 보안 |
| 웹 도구 | `docs/tools/web.md` | web_search (Brave), web_fetch |
| 브라우저 도구 | `docs/tools/browser.md` | Chrome 인스턴스, 액션, 프로파일 |
| Agent Loop | `docs/concepts/agent-loop.md` | intake → context → inference → tool exec → loop → stream → persist |
| Pi 통합 | `docs/concepts/pi-integration.md` | Tool Architecture, SessionManager |

### Repomix grep 패턴

```
# 도구 레지스트리
"registerTool|ToolRegistry|toolSchema"

# 도구 실행
"executeTool|toolResult|tool_call_id"

# Agent Loop
"agentLoop|runLoop|maxIterations|toolCalls"

# exec 도구
"ExecTool|shellCommand|spawnProcess"

# 웹 도구
"webSearch|webFetch|braveSearch"
```

---

## Phase 3: 허트비트 + 크론잡

| Sub-task | OpenClaw Reference | Key Concept |
|----------|-------------------|-------------|
| 허트비트 Agent Loop | `docs/gateway/heartbeat.md` | HEARTBEAT_OK 계약, activeHours, AI First |
| 크론잡 매니저 | `docs/automation/cron-jobs.md` | at/every/cron 스케줄, main/isolated 세션 |
| 크론 vs 허트비트 | `docs/automation/cron-vs-heartbeat.md` | 선택 기준표 |
| 훅 | `docs/automation/hooks.md` | 이벤트 기반 자동화 |

### Repomix grep 패턴

```
# 허트비트
"HEARTBEAT_OK|heartbeat|activeHours"

# 크론잡
"cronJob|cronManager|scheduleType|deleteAfterRun"

# 훅
"onToolCall|onMessage|hookEvent"
```

---

## Phase 4: 스킬 시스템

| Sub-task | OpenClaw Reference | Key Concept |
|----------|-------------------|-------------|
| 스킬 로더 | `docs/tools/skills.md` | SKILL.md YAML frontmatter, 로드 위치 |
| 스킬 작성 | `docs/tools/creating-skills.md` | 스킬 작성법 |
| 스킬 설정 | `docs/tools/skills-config.md` | 스킬 설정 |
| 슬래시 명령 | `docs/tools/slash-commands.md` | 커맨드 vs 디렉티브, 라우팅 |

### Repomix grep 패턴

```
# 스킬 로더
"loadSkill|SkillLoader|SKILL\.md|frontmatter"

# 스킬 레지스트리
"skillRegistry|skillPriority|user-invocable"

# 슬래시 명령
"slashCommand|parseCommand|commandRouter"
```

---

## Phase 5: 플러그인 + MCP

| Sub-task | OpenClaw Reference | Key Concept |
|----------|-------------------|-------------|
| MCP 클라이언트 | MCP spec + plugin architecture | stdio/SSE, JSON-RPC, tools/list, tools/call |
| 플러그인 | `docs/tools/plugin.md` | 발견, 우선순위, 런타임 |
| 플러그인 도구 | `docs/plugins/agent-tools.md` | 도구 등록 패턴, optional 도구 |
| 매니페스트 | `docs/plugins/manifest.md` | 플러그인 매니페스트 스키마 |

### Repomix grep 패턴

```
# MCP
"mcpClient|mcpServer|tools/list|tools/call|JSON-RPC"

# 플러그인
"pluginLoader|pluginManifest|tarsncase\.plugin\.json"

# 플러그인 도구
"registerPlugin|pluginTool|pluginSkill"
```

---

## Phase 6: cased 감시 데몬

| Sub-task | OpenClaw Reference | Key Concept |
|----------|-------------------|-------------|
| 프로세스 감시 | (OpenClaw 참고 없음) | PID 감시, health check |
| 자동 재시작 | (OpenClaw 참고 없음) | 재시작 정책, 안전 모드 |
| 감사 로그 | (OpenClaw 참고 없음) | LLM 통신/명령 실행 기록 |

> Phase 6은 OpenClaw에 직접 대응하는 기능이 없으므로, 일반적인 프로세스 감시 패턴 참고.

---

## Repomix 사용법 (빠른 참조)

### OpenClaw 원격 팩킹

```
mcp__repomix__pack_remote_repository
  remote: "https://github.com/openclaw/openclaw"
  includePatterns: "docs/**/*.md"  # 문서만
```

### 로컬 코드베이스 팩킹

```
mcp__repomix__pack_codebase
  directory: "{your-local-repo-path}"
  includePatterns: "**/*.go"
  compress: true
```

### grep 검색

```
mcp__repomix__grep_repomix_output
  outputId: "{팩킹 결과 ID}"
  pattern: "검색 패턴"
  afterLines: 50
```

### 전체 읽기

```
mcp__repomix__read_repomix_output
  outputId: "{팩킹 결과 ID}"
  startLine: 1
  endLine: 200
```
