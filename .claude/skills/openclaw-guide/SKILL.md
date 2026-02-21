---
name: openclaw-guide
description: "OpenClaw 소스 분석 및 TARS 관련 기능 개발 가이드. TARS(tarsncase) 프로젝트에서 OpenClaw(github.com/openclaw/openclaw)를 참고하여 기능을 개발할 때 사용. 다음 상황에서 트리거: (1) OpenClaw 아키텍처를 분석하거나 참조할 때, (2) TARS의 세션, 에이전트 루프, 도구, 스킬, 플러그인, MCP, 허트비트, 크론잡 기능을 개발할 때, (3) OpenClaw 문서에서 특정 기능의 구현 패턴을 찾을 때, (4) PLAN.md의 Phase별 개발을 진행할 때."
---

# OpenClaw Guide

OpenClaw(TypeScript)의 아키텍처와 패턴을 TARS(Go)에 맞게 변환하여 개발할 때 참조하는 스킬.

## Quick Start

### OpenClaw 소스 분석 시

Repomix MCP로 OpenClaw를 팩킹한 뒤 grep으로 필요한 부분을 검색:

```
# 1. 문서만 팩킹 (권장 — 개념/패턴 분석용)
mcp__repomix__pack_remote_repository
  remote: "https://github.com/openclaw/openclaw"
  includePatterns: "docs/**/*.md"

# 2. 소스 포함 팩킹 (구현 상세 분석 시)
mcp__repomix__pack_remote_repository
  remote: "https://github.com/openclaw/openclaw"
  compress: true

# 3. grep으로 특정 기능 검색
mcp__repomix__grep_repomix_output
  outputId: "{결과 ID}"
  pattern: "검색 패턴"
  afterLines: 50
```

### TARS 기능 개발 시

1. 개발할 기능에 해당하는 Phase 확인 → `references/phase-map.md`
2. 해당 Phase의 OpenClaw 참조 문서 확인
3. Repomix로 해당 문서/소스 검색
4. Go 관용구로 독립 구현 (TypeScript 직접 번역 금지)

## Reference Files

| File | 내용 | 참조 시점 |
|------|------|----------|
| [architecture.md](references/architecture.md) | Agent Loop, Session, System Prompt, Memory, Compaction, Workspace | Phase 1-2 핵심 구조 개발 시 |
| [tools-and-extensions.md](references/tools-and-extensions.md) | 도구 표면, 빌트인 도구, 스킬, 플러그인, MCP, 슬래시 명령 | Phase 2-5 도구/확장 개발 시 |
| [automation.md](references/automation.md) | Heartbeat, Cron Job, Hooks | Phase 3 자동화 기능 개발 시 |
| [phase-map.md](references/phase-map.md) | Phase별 OpenClaw 문서 매핑 + Repomix grep 패턴 | 개발 시작 전 참조 문서 확인 |

## Core Translation Rules

### TypeScript → Go 변환 원칙

1. **인터페이스 최소화**: OpenClaw의 TypeScript interface를 그대로 옮기지 않음. Go에서는 필요한 메서드만 가진 작은 인터페이스 사용
2. **구조체 중심**: class → struct + methods
3. **에러 처리**: Promise/try-catch → `(result, error)` 반환
4. **콜백**: callback/Promise → channel 또는 함수 파라미터
5. **스트리밍**: EventEmitter/ReadableStream → `io.Writer` 또는 `chan` 또는 콜백 함수
6. **의존성 주입**: constructor injection → 함수 파라미터 또는 struct 필드

### 개발 패턴

```
1. OpenClaw 문서에서 개념/계약 파악
2. 해당 Phase의 TARS 인터페이스(Go struct + methods) 설계
3. 실패 테스트 작성 (TDD)
4. 최소 구현
5. tars API → tars CLI 순서로 통합
6. 커밋
```

## OpenClaw Key Docs Quick Index

| 기능 | OpenClaw 문서 |
|------|--------------|
| Agent Loop | `docs/concepts/agent-loop.md` |
| Session | `docs/concepts/session.md` |
| Session Storage | `docs/reference/session-management-compaction.md` |
| System Prompt | `docs/concepts/system-prompt.md` |
| Agent Runtime | `docs/concepts/agent.md` |
| Workspace | `docs/concepts/agent-workspace.md` |
| Memory | `docs/concepts/memory.md` |
| Compaction | `docs/concepts/compaction.md` |
| Tools Index | `docs/tools/index.md` |
| Exec Tool | `docs/tools/exec.md` |
| Web Tools | `docs/tools/web.md` |
| Browser | `docs/tools/browser.md` |
| Skills | `docs/tools/skills.md` |
| Slash Commands | `docs/tools/slash-commands.md` |
| Plugins | `docs/tools/plugin.md` |
| Plugin Tools | `docs/plugins/agent-tools.md` |
| Heartbeat | `docs/gateway/heartbeat.md` |
| Cron Jobs | `docs/automation/cron-jobs.md` |
| Cron vs Heartbeat | `docs/automation/cron-vs-heartbeat.md` |
| Hooks | `docs/automation/hooks.md` |
| Templates | `docs/reference/templates/` |
| Pi Integration | `docs/concepts/pi-integration.md` |
