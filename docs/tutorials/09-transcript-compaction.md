# Step 9. Transcript Compaction

> 학습 목표: 긴 대화의 토큰 비용을 줄이는 압축 메커니즘과, atomic write 패턴 이해

## 원본 코드 분석 (TARS)

TARS의 `internal/session/compaction.go`는 대화가 길어지면 오래된 메시지를 요약으로 교체합니다.

### 핵심 아이디어

```
[오래된 메시지 30개] → [요약 1줄] + [최근 메시지 20개]
```

### 업계 비교: OpenClaw, Gemini CLI, TARS

| | OpenClaw | Gemini CLI | TARS |
|--|----------|------------|------|
| 트리거 | 컨텍스트 윈도우 근접 시 | 토큰 70% 도달 시 | 토큰 예산(20K) 초과 시 |
| 요약 방식 | **LLM 요약** | **LLM 요약** (XML 구조) | **규칙 기반** (기본) |
| 요약 포맷 | 자유 텍스트 | `<state_snapshot>` XML | `[COMPACTION SUMMARY]` 텍스트 |
| 보존 범위 | 최근 메시지 | 최근 30% | 최근 12K 토큰 / 최소 5개 |

OpenClaw과 Gemini CLI는 **LLM한테 요약을 시키는** 반면, TARS는 **규칙 기반 요약**을 기본으로 쓰되, 콜백으로 LLM 요약으로 교체할 수 있는 구조입니다. 최소 버전에서는 규칙 기반으로 시작합니다.

### 토큰 추정

```go
cost := len(content) / 4  // 4글자 ≈ 1토큰
```

정확하진 않지만, 비용 대비 충분히 실용적입니다. tiktoken 같은 라이브러리를 도입하면 정확도가 올라가지만, MVP에서는 과잉입니다.

### Compaction Boundary

`LoadHistory()`가 히스토리를 로드할 때, `[COMPACTION SUMMARY]`를 포함한 system 메시지를 만나면 **항상 포함하고 거기서 멈춥니다**. 이렇게 해야 요약 이후 맥락이 유지됩니다.

## 실습

### 9-1. `RewriteMessages` — Atomic Write

기존 `AppendMessage`는 한 줄 추가이지만, compaction은 전체를 교체합니다.

**`internal/session/transcript.go`에 추가:**

```go
func RewriteMessages(path string, messages []Message) error {
    tmp := path + ".tmp"
    f, err := os.Create(tmp)
    // ... 임시 파일에 쓰기 ...
    return os.Rename(tmp, path)  // atomic 교체
}
```

포인트:
- **Atomic write** — 임시 파일에 쓰고 `os.Rename`으로 교체. 중간에 실패해도 원본이 안 깨짐
- `json.NewEncoder`의 `Encode`는 자동으로 `\n`을 붙여줌 (JSONL 형식 유지)

### 9-2. `CompactTranscript` — 압축 실행

**`internal/session/compaction.go`** 핵심 흐름:

```go
const (
    DefaultKeepRecentTokens = 12000
    MinKeepRecentMessages   = 5
    CompactionThreshold     = 20000
)

func CompactTranscript(path string, keepRecentTokens int) (CompactResult, error) {
    all := ReadMessages(path)

    // 1. 전체 토큰 계산 — threshold 미만이면 중단
    // 2. 뒤에서부터 보존 범위 계산 (12K 토큰, 최소 5개)
    // 3. 오래된 메시지를 BuildCompactionSummary로 요약
    // 4. [요약 메시지] + [최근 메시지]로 RewriteMessages
}
```

### 9-3. `BuildCompactionSummary` — 규칙 기반 요약

```go
func BuildCompactionSummary(messages []Message) string {
    // "[COMPACTION SUMMARY]\nSummarized N messages.\n"
    // + 각 메시지의 role과 앞 200자 나열
}
```

나중에 LLM 요약으로 교체 가능합니다 — 함수 시그니처가 `func([]Message) string`이므로, LLM 호출 버전으로 바꿔끼우면 됩니다.

### 9-4. 채팅 핸들러에서 호출

**`internal/server/handler_chat.go`** — assistant 응답 저장 후:

```go
// 9.5 compaction 체크
session.CompactTranscript(transcriptPath, 0)
```

에러를 무시합니다 — compaction 실패가 채팅을 막으면 안 됩니다.

## 체크포인트

- [x] 토큰 예산 초과 시 자동으로 compaction이 발생한다
- [x] compaction 이후에도 `[COMPACTION SUMMARY]`를 통해 맥락이 유지된다
- [x] `RewriteMessages`가 atomic write로 파일 안전성을 보장한다

## 배운 패턴

- **Atomic write** — 임시 파일 + `os.Rename`으로 중간 실패 시 원본 보호
- **토큰 추정 `len/4`** — 정밀하지 않지만 실용적, MVP에 충분
- **Compaction boundary** — 요약 메시지를 항상 포함해서 맥락 유지
- **실패해도 계속** — compaction 실패가 핵심 기능(채팅)을 막지 않음
