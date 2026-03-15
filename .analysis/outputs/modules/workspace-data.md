# 모듈: 워크스페이스와 영속 데이터

## 핵심 파일

- `internal/memory/workspace.go`
- `internal/session/session.go`
- `internal/session/transcript.go`
- `internal/session/compaction.go`
- `internal/project/store.go`
- `internal/usage/tracker.go`
- `internal/cron/manager.go`
- `internal/schedule/store.go`
- `internal/ops/manager.go`
- `internal/research/service.go`
- `internal/gateway/runtime.go`
- `internal/browser/service.go`
- `internal/assistant/runtime.go`

## 역할

이 모듈은 로컬 파일 시스템 위에 앱 상태를 저장하고 재사용하는 계층이다. 저장소를 읽다 보면 HTTP 핸들러보다 이 계층이 더 중요한 경우가 많다. 왜냐하면 프롬프트, 세션, 사용량, 프로젝트 정책이 모두 여기에서 나온다.

## 저장 패턴

- 세션 메타데이터: 인덱스 파일
- 대화 로그: JSONL transcript
- 장기 기억과 운영 문서: Markdown
- 프로젝트 문서: `PROJECT.md`
- 리서치 보고서: Markdown + summary JSONL
- 사용량/비용: JSON
- approval 목록: JSON
- ops 이벤트: JSONL

## 중요한 관찰

- `EnsureWorkspace`는 TARS를 "문서 기반 운영 환경"으로 만든다.
- transcript compaction은 긴 세션을 버리는 대신 system summary로 압축한다.
- project store는 단순 메타데이터 저장소가 아니라 tool/skill 허용 범위를 담는 정책 저장소다.
- schedule store는 별도 DB가 아니라 cron job payload 위에 schedule 메타데이터를 얹는다.
- ops manager는 바로 삭제하지 않고 approval -> approve/reject -> apply 순서로 cleanup을 진행한다.
- gateway, browser, assistant는 모두 workspace 하위 `_shared` 경로를 공용 상태 저장소로 사용한다.

## 신규 기능을 붙일 때 체크할 점

- 이 기능이 workspace 파일을 새로 만들어야 하는가
- 세션이나 프로젝트와 연결되는가
- 비용 추적이나 background manager와 상호작용하는가
- transcript 또는 memory 정책에 영향을 주는가
