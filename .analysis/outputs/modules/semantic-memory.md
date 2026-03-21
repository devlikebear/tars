# 모듈: Semantic Memory

## 핵심 파일

- `internal/memory/semantic.go`
- `internal/memory/gemini_embed.go`
- `internal/prompt/memory_retrieval.go`
- `internal/tool/memory_search.go`
- `internal/tool/memory_save.go`
- `internal/tarsserver/helpers_chat.go`
- `internal/tarsserver/helpers_memory.go`

## 역할

이 모듈은 explicit experience, transcript compaction summary, project 문서를 embedding 기반으로 다시 찾게 만드는 보조 기억 계층이다. 기존 파일 기반 memory를 대체하지 않고, prompt/tool 쪽 retrieval 품질을 높이는 추가 인덱스 역할을 한다.

## 색인 대상

- `memory_save`가 남기는 explicit experience
- transcript compaction 후 추출된 summary 와 durable memory 후보
- 프로젝트 문서(`PROJECT.md`, `STATE.md`, 서사형 문서 등)

즉, 이 계층은 "대화를 그대로 저장"하지 않고 재사용 가치가 있는 문맥만 별도 구조로 뽑아낸다.

## 검색 흐름

1. `prompt/memory_retrieval.go`가 relevant memory를 찾을 때 semantic search를 먼저 시도한다.
2. hit가 없으면 기존 파일 line search로 fallback 한다.
3. `tool/memory_search.go`도 semantic index를 우선하고, 없으면 `MEMORY.md`와 daily log를 읽는다.
4. project/session 일치도, lexical boost, recency, importance가 점수에 같이 반영된다.

## 중요한 관찰

- `SemanticConfig`는 generic provider surface처럼 보이지만 현재 실제 embedder는 Gemini 하나만 구현돼 있다.
- semantic index는 workspace 파일을 truth source로 유지하고, source hash/state만 별도 저장한다.
- compaction 훅이 semantic memory까지 갱신하므로 transcript 정리와 기억 축적이 서로 연결돼 있다.
