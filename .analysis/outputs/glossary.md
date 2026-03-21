# 용어집

## Workspace

TARS가 상태를 저장하는 루트 디렉터리다. `internal/memory/workspace.go`가 기본 구조를 보장한다.

## Session

대화 단위 메타데이터다. `internal/session/session.go`에서 생성하고, 최신 사용 시간과 프로젝트 연결 정보를 함께 관리한다.

## Transcript

세션별 대화 로그 JSONL 파일이다. `internal/session/transcript.go`가 append/read를 담당한다.

## Compaction Summary

길어진 transcript를 줄일 때 넣는 시스템 메시지다. `internal/session/compaction.go`가 오래된 메시지를 요약해 최근 문맥만 남긴다.

## Semantic Memory

explicit memory, compaction summary, project 문서를 embedding 기반으로 저장하고 다시 찾는 보조 인덱스다. `internal/memory/semantic.go`가 색인과 검색을 담당한다.

## Relevant Memory

프롬프트 빌드 시 현재 질문과 관련성이 높은 memory snippet만 별도 섹션으로 삽입하는 단계다. `internal/prompt/memory_retrieval.go`가 semantic hit를 우선하고, 없으면 파일 텍스트 매칭으로 fallback 한다.

## Project

프로젝트별 목적, 허용 skill/tool, Git 저장소 정보, 추가 지침을 담는 문서 단위다. `internal/project/store.go`가 저장한다.

## Project Brief

채팅에서 프로젝트를 시작할 때 먼저 모으는 요구사항 문서다. `internal/project/brief_state.go`가 session 기반 `BRIEF.md`를 관리한다.

## Workflow Policy

project brief 상태, project phase/status, autopilot state 전이 규칙을 중앙화한 정책 객체다. `internal/project/workflow_policy.go`에 있다.

## Autopilot

프로젝트 board와 activity를 읽고 `todo -> review -> done`을 반복 감독하는 PM 루프다. `internal/project/project_runner.go`가 `AUTOPILOT.json`과 `STATE.md`를 갱신한다.

## Worker Kind

프로젝트 task가 요청하는 논리적 실행자 종류다. `codex-cli`, `claude-code`, `default` 같은 값이 있으며, 실제 gateway executor 이름과 분리돼 있다.

## Dashboard

프로젝트 진행 상태를 읽기 전용 HTML로 보여주는 운영 화면이다. `internal/tarsserver/dashboard.go`가 `/dashboards`와 `/ui/projects/{id}`를 렌더링한다.

## Skill

사용자나 에이전트가 호출할 수 있는 작업 지침 문서다. `internal/skill/loader.go`가 `SKILL.md`를 읽어 메타데이터와 본문을 로드한다.

## Runtime Skill Mirror

agent가 안정된 runtime path를 보도록 loaded skill과 companion file을 workspace `_shared/skills_runtime` 아래에 복사한 결과다. `internal/skill/mirror.go`가 만든다.

## Skill Hub

원격 registry에서 skill과 plugin을 검색, 설치, 업데이트하는 배포 계층이다. `cmd/tars/skill_main.go`, `cmd/tars/plugin_main.go`, `internal/skillhub/*`가 담당한다.

## Plugin

skill 디렉터리, MCP 서버, 추가 동작을 묶는 확장 단위다. `internal/extensions/manager.go`가 skill과 함께 snapshot으로 관리한다.

## MCP Server

Model Context Protocol 서버다. `internal/mcp/client.go`가 subprocess를 띄워 툴 목록과 호출을 관리한다.

## Tool Registry

LLM에 노출할 툴과 실행 함수를 보관하는 런타임 저장소다. `internal/tool/tool.go`에 있다.

## Agent Loop

LLM 호출과 tool call을 반복 실행하는 코어 루프다. `internal/agent/loop.go`가 반복 횟수, 중복 호출 보호, 상태 훅을 관리한다.

## Gateway Runtime

비동기 agent run, 채널 메시지, 브라우저 연동 상태를 묶는 런타임이다. `internal/gateway/runtime.go`가 기본 생성기다.

## Browser Relay

브라우저 확장과 로컬 CDP 클라이언트 사이를 이어 주는 loopback WebSocket relay다. `internal/browserrelay/server.go`가 relay token, origin allowlist, ping/pong keepalive를 관리한다.

## OTP Approval

브라우저 로그인 같은 흐름에서 사람이 입력하는 일회용 코드를 chat ID 기준으로 잠시 대기하는 메모리 상태다. `internal/approval/otp.go`가 timeout 과 consume 를 관리한다.

## Provider Credential Strategy

provider별 auth mode, OAuth source, refresh 가능 여부를 strategy table로 정리한 계층이다. `internal/auth/provider_credentials.go`가 entry point다.

## Codex Credential

OpenAI Codex provider에 필요한 access token, refresh token, account ID, source path 묶음이다. `internal/auth/codex_oauth.go`가 env 또는 `~/.codex/auth.json`에서 해석하고 필요하면 refresh 후 파일 또는 macOS keychain에 다시 저장한다.

## Provider Adapter

같은 `llm.Client` 인터페이스를 만족하지만 각 provider의 HTTP 규약에 맞게 요청과 응답을 번역하는 구현체다. 이 저장소에서는 `openai_compat`, `anthropic`, `gemini_native`, `openai_codex`가 서로 다른 adapter 역할을 한다.

## Heartbeat

정해진 시간대에 실행되는 자율 점검 루틴이다. 서버 실행 모드와 automation tool에서 재사용된다.
