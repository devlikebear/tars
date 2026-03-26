# Go AI 에이전트 런타임 만들기 — 튜토리얼

> TARS 프로젝트를 기반으로 Go AI 채팅 런타임을 처음부터 만들어보는 실습 가이드

## 대상

- Go 기본 문법을 아는 개발자
- LLM 기반 애플리케이션 구조를 직접 만들어보고 싶은 사람

## 사전지식

- Go, `net/http`, JSON, context
- 런타임: Go 1.22+

## 전체 커리큘럼

| Step | 주제 | 학습 목표 |
|------|------|-----------|
| 1 | 얇은 CLI | 엔트리포인트에 비즈니스 로직을 두지 않는 원칙 |
| 2 | 세션과 Transcript | JSONL 기반 대화 이력 영속화 패턴 |
| 3 | 프롬프트 빌더 + 도구 Registry | 시스템 프롬프트 조립과 tool schema 구조 |
| 4 | HTTP/SSE 채팅 엔드포인트 | 전체 파이프라인 연결 및 스트리밍 |

## 아키텍처 한눈에 보기

```
CLI (cmd/tars/)
 └── serve 명령
      └── server.Serve()
           ├── session.Store     ← 세션/transcript 관리
           ├── prompt.Build()    ← 시스템 프롬프트 조립
           ├── tool.Registry     ← 도구 등록/실행
           ├── agent.Loop        ← LLM 호출 + tool call 반복
           └── SSEWriter         ← 실시간 스트리밍 응답
```

## 핵심 데이터 흐름

```
POST /v1/chat { session_id, message }
  → 세션 resolve
  → transcript에서 히스토리 읽기
  → 프롬프트 조립 (시스템 + 히스토리 + 유저)
  → agent loop (LLM 호출 → tool call → 반복)
  → SSE delta 스트리밍
  → transcript에 assistant 응답 저장
  → SSE done
```

## 실행 방법

```bash
# 서버 시작
go run ./cmd/tars/ serve

# 채팅 테스트
curl -N -X POST http://localhost:8080/v1/chat \
  -H "Content-Type: application/json" \
  -d '{"message":"안녕하세요"}'

# 테스트 실행
go test ./...
```
