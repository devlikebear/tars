# 모듈: Memory

## 역할

`internal/memory`와 chat memory hook은 장기 기억, semantic search, daily log, experiences, compaction index를 다룬다.

## 주요 파일

- `internal/memory/workspace.go`
- `internal/memory/semantic.go`
- `internal/memory/experience.go`
- `internal/tarsserver/chat_memory_hook.go`
- `internal/reflection/job_memory.go`

## 관찰

- 최신 user-facing 설명은 `MEMORY.md`, daily logs, experiences, semantic search 중심이다.
- per-turn hook은 daily log와 explicit `remember ...` hot path로 축소됐다.
- broader experience derivation은 nightly reflection으로 이동했다.
- KB Wiki/graph residue는 #410에서 제거 후보로 추적한다.
