# Go AI 에이전트 런타임 만들기 — 튜토리얼

> TARS 프로젝트를 기반으로 Go AI 채팅 런타임을 처음부터 만들어보는 실습 가이드

## 대상

- Go 기본 문법을 아는 개발자
- LLM 기반 애플리케이션 구조를 직접 만들어보고 싶은 사람
- 단일 Go 바이너리 안에 CLI, HTTP API, 웹 콘솔, 백그라운드 작업을 함께 묶는 방식을 보고 싶은 사람

## 사전지식

- Go, `net/http`, JSON, context
- 런타임: Go 1.25+
- 선택: Svelte/TypeScript 콘솔을 함께 보고 싶다면 Node.js

## 전체 커리큘럼

| Step | 주제 | 학습 목표 |
|------|------|-----------|
| 1 | 얇은 CLI | 엔트리포인트에 비즈니스 로직을 두지 않는 원칙 |
| 2 | 세션과 Transcript | JSONL 기반 대화 이력 영속화 패턴 |
| 3 | 프롬프트 빌더 + 도구 Registry | 시스템 프롬프트 조립과 tool schema 구조 |
| 4 | HTTP/SSE 채팅 엔드포인트 | 전체 파이프라인 연결 및 스트리밍 |
| 5-8 | Provider + Config + Tools | 실제 LLM, 설정, 파일/명령 도구 연결 |
| 9-11 | Compaction + Memory | 긴 대화 압축, Markdown memory, semantic search |
| 12-14 | Skill/Plugin/MCP | 외부 기능을 on-demand 확장으로 분리 |
| 15 | One-shot CLI + Console | 레거시 TUI 대신 웹 콘솔과 CLI 메시지 전송 사용 |
| 19-20 | Ops Approval + Event Stream | 위험한 운영 작업 승인과 실시간 알림 흐름 |
| 21-22 | Auth + Agent Runtime | API 인증과 비동기 run lifecycle |

## 아키텍처 한눈에 보기

```
CLI (cmd/tars/)
 ├── root tars             → /console 열기 또는 --message one-shot chat
 └── serve 명령
      └── tarsserver.Serve()
           ├── config.Load()            ← YAML/env/defaults
           ├── session.Store            ← 세션/transcript 관리
           ├── prompt.Build()           ← 시스템 프롬프트 조립
           ├── tool.Registry            ← scope별 도구 등록/실행
           ├── agent.Loop               ← LLM 호출 + tool call 반복
           ├── agentruntime.Runtime     ← 비동기 run/subagent 관리
           ├── pulse.Runtime            ← 1분 watchdog
           ├── reflection.Runtime       ← nightly memory reflection
           └── console/static + SSE     ← 웹 콘솔과 실시간 이벤트
```

## 핵심 데이터 흐름

### Chat

```
POST /v1/chat { session_id, message }
  → 세션 resolve
  → transcript에서 히스토리 읽기
  → prior context + 시스템 프롬프트 조립
  → agent loop (LLM 호출 → tool call → 반복)
  → SSE delta/status 스트리밍
  → transcript에 assistant 응답 저장
  → daily log / remember hook
```

`POST /v1/chat/prior-context/preview`는 같은 prior context builder 경로를 사용해
다음 draft message가 system prompt에 넣을 `## Prior Context` 섹션과 source/token
메타데이터를 Chat 우측 `Prior` 패널에 제공한다.

### Agent Runtime

```
POST /v1/agentruntime/runs { prompt, agent }
  → run accepted
  → executor goroutine 시작
  → status/events/reports API로 진행 상태 조회
  → completed / failed / canceled
```

## 실행 방법

```bash
# 서버 시작
go run ./cmd/tars/ serve

# one-shot 채팅 테스트
go run ./cmd/tars/ --message "안녕하세요"

# HTTP 채팅 테스트
curl -N -X POST http://127.0.0.1:43180/v1/chat \
  -H "Content-Type: application/json" \
  -d '{"message":"안녕하세요"}'

# 테스트 실행
go test ./...
```

## 현재 경계

이 튜토리얼은 clone-coding 학습용 문서입니다. 최신 TARS 본체에는 과거 `internal/project`, macOS dashboard, Autopilot, Bubble Tea TUI가 남아있지 않습니다. 현재 사용자-facing 운영 표면은 웹 콘솔(`/console`), one-shot CLI, cron, ops approval, agent runtime, pulse/reflection입니다.
