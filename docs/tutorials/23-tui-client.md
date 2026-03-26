# Step 23. TUI 클라이언트

> 학습 목표: Bubble Tea로 터미널 기반 대화형 채팅 인터페이스 구현

## 왜 TUI인가

Step 15에서 만든 REPL 채팅 클라이언트는 `fmt` + `bufio`로 최소 동작합니다. 하지만:

- 화면 스크롤이 불가능 (터미널 히스토리에 의존)
- 스트리밍 중 입력 불가능 (동기 방식)
- 상태 표시 없음 (세션 ID, 스트리밍 상태)
- 비주얼 구분 없음 (사용자/AI/시스템 메시지)

**Bubble Tea**는 Go의 가장 인기 있는 TUI 프레임워크로, **Elm Architecture**(Model-Update-View)를 기반으로 합니다.

## Elm Architecture

```
                ┌───────────────┐
                │     Model     │ ← 상태
                └───────┬───────┘
                        │
            ┌───────────▼───────────┐
            │        View()         │ → 문자열 (화면)
            └───────────────────────┘
                        ▲
            ┌───────────┴───────────┐
            │      Update(msg)      │ ← 이벤트 처리
            └───────────────────────┘
                        ▲
                  KeyMsg, WindowSizeMsg,
                  커스텀 메시지 ...
```

1. **Model**: 앱의 전체 상태를 보유하는 struct
2. **Update**: 메시지를 받아 상태를 변경하고, 다음 명령(Cmd)을 반환
3. **View**: 현재 상태를 문자열로 렌더링

상태는 **불변적으로** 관리됩니다. Update가 새 상태를 반환하면, Bubble Tea가 View를 다시 호출합니다.

## 실습

### 23-1. 모델 구조

```go
type Model struct {
    ctx    context.Context
    cancel context.CancelFunc

    client    *chatClient
    sessionID string

    width, height int          // 터미널 크기
    input         textinput.Model  // 텍스트 입력 컴포넌트

    chatLines    []string      // 채팅 로그
    scrollOffset int           // 스크롤 위치

    history      []string      // 명령어 히스토리
    historyPos   int
    historyDraft string        // 히스토리 탐색 중 현재 입력 보존

    inflight       bool        // 스트리밍 중 여부
    inflightCancel context.CancelFunc

    asyncCh chan tea.Msg        // 비동기 메시지 채널
}
```

**핵심 설계:**
- `asyncCh`: goroutine에서 Bubble Tea로 메시지를 전달하는 채널 (버퍼 256)
- `inflight` + `inflightCancel`: 스트리밍 중 Esc로 취소 가능
- `historyDraft`: ↑키로 히스토리를 탐색할 때 현재 입력을 보존

### 23-2. 비동기 메시지 패턴

Bubble Tea는 **동기 이벤트 루프**입니다. 비동기 작업(HTTP 요청, SSE 스트리밍)은 goroutine에서 실행하고, 결과를 `tea.Msg`로 전달합니다.

```go
// 커스텀 메시지 타입
type chatDeltaMsg struct{ chunk string }
type chatDoneMsg struct { sessionID string; err error }

// asyncCh에서 메시지를 대기하는 Cmd
func waitAsync(ch <-chan tea.Msg) tea.Cmd {
    return func() tea.Msg {
        msg, ok := <-ch
        if !ok { return nil }
        return msg
    }
}
```

**흐름:**
```
goroutine (SSE 스트리밍)
    │
    │ asyncCh <- chatDeltaMsg{chunk: "Hello"}
    │
    ▼
waitAsync() ─── chatDeltaMsg ───→ Update()
    │                                │
    │                     appendToLast("Hello")
    │                                │
    ▼                                ▼
waitAsync() (다음 메시지 대기)     View() (화면 갱신)
```

`waitAsync`가 체인처럼 연결됩니다. Update에서 메시지를 처리한 후, 다시 `waitAsync`를 반환하면 Bubble Tea가 다음 메시지를 기다립니다.

### 23-3. SSE 스트리밍 통합

```go
func (m *Model) handleSubmit() tea.Cmd {
    // ...
    reqCtx, reqCancel := context.WithCancel(m.ctx)
    m.inflightCancel = reqCancel

    go func() {
        newSID, err := client.stream(reqCtx, sessionID, text,
            func(evt chatEvent) {
                switch evt.Type {
                case "delta":
                    // JSON 파싱 → asyncCh에 전송
                    select {
                    case asyncCh <- chatDeltaMsg{chunk: d.Text}:
                    case <-reqCtx.Done():  // 취소되면 무시
                    }
                }
            },
        )
        asyncCh <- chatDoneMsg{sessionID: newSID, err: err}
    }()

    return nil
}
```

**`select` + `ctx.Done()`**: 요청이 취소되면 채널 전송을 건너뜁니다. 이게 없으면 취소된 goroutine이 asyncCh에 계속 쓰려고 블록될 수 있습니다.

### 23-4. 키 입력 처리

```go
func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
    switch msg.Type {
    case tea.KeyCtrlC:
        if m.inflight {
            m.inflightCancel()  // 스트리밍 취소
            return m, waitAsync(m.asyncCh)
        }
        return m, tea.Quit  // 종료

    case tea.KeyEscape:
        if m.inflight {
            m.inflightCancel()  // 스트리밍 취소
        } else {
            m.input.SetValue("")  // 입력 지우기
        }

    case tea.KeyEnter:
        return m, m.handleSubmit()

    case tea.KeyUp:
        m.navigateHistory(-1)  // 이전 명령어
    case tea.KeyDown:
        m.navigateHistory(1)   // 다음 명령어

    case tea.KeyPgUp:
        m.scroll(m.chatAreaHeight())   // 페이지 위로
    case tea.KeyPgDown:
        m.scroll(-m.chatAreaHeight())  // 페이지 아래로
    }
}
```

### 23-5. 히스토리 탐색

```go
func (m *Model) navigateHistory(dir int) bool {
    // 현재 위치가 끝(새 입력 중)이면 현재 입력을 저장
    if m.historyPos == len(m.history) {
        m.historyDraft = m.input.Value()
    }

    newPos := m.historyPos + dir
    if newPos < 0 || newPos > len(m.history) {
        return false
    }

    m.historyPos = newPos
    if newPos == len(m.history) {
        m.input.SetValue(m.historyDraft)  // 저장해둔 입력 복원
    } else {
        m.input.SetValue(m.history[newPos])
    }
    m.input.CursorEnd()
    return true
}
```

`historyDraft`가 핵심입니다. 사용자가 "hello"를 입력 중에 ↑키를 누르면 "hello"를 보존하고, ↓키로 돌아오면 "hello"가 복원됩니다.

### 23-6. 스크롤

```go
func (m *Model) scroll(delta int) {
    m.scrollOffset += delta
    m.scrollOffset = max(m.scrollOffset, 0)
    maxScroll := max(len(m.chatLines)-m.chatAreaHeight(), 0)
    if m.scrollOffset > maxScroll {
        m.scrollOffset = maxScroll
    }
}
```

- `scrollOffset = 0`: 화면 최하단 (최신 메시지)
- 양수: 위로 스크롤
- 새 메시지 수신 시 자동으로 `scrollOffset = 0`으로 리셋

### 23-7. 뷰 렌더링

```go
func (m *Model) View() string {
    // 1. 헤더 (1줄)
    header := headerStyle.Width(m.width).Render(" TARS")

    // 2. 채팅 영역 (height - 3줄)
    chatView := m.renderChat(m.chatAreaHeight())

    // 3. 상태바 (1줄) — 세션 ID, 스트리밍 상태, 스크롤 위치
    statusBar := m.renderStatusBar()

    // 4. 입력 (1줄)
    m.input.Width = m.width - 2
    inputView := m.input.View()

    return header + "\n" + chatView + statusBar + "\n" + inputView
}
```

**lipgloss**로 스타일을 적용합니다:
- 사용자 메시지: 하늘색
- AI 응답: 녹색
- 시스템: 회색
- 에러: 빨간색
- 도구: 주황색

### 23-8. 서버 연결

```go
// cmd/tars/chat.go
model := tui.New(serverURL, apiToken, sessionID)
p := tea.NewProgram(model, tea.WithAltScreen())
p.Run()
```

`tea.WithAltScreen()`은 alternate screen buffer를 사용합니다. TUI 종료 시 이전 터미널 내용이 복원됩니다.

## TARS 원본과의 차이

| 항목 | TARS | TARS |
|------|------|--------|
| 패널 | 3개 (채팅, 상태, 알림) | 1개 (채팅) |
| 레이아웃 | 반응형 (좁으면 세로, 넓으면 가로) | 단일 컬럼 |
| 알림 | SSE 이벤트 구독, 알림 센터 | 미구현 |
| 슬래시 명령어 | /trace, /gateway, /project 등 | /help, /session, /new, /clear |
| 세션 복구 | stale 세션 자동 복구 | 수동 /new |
| 탭 완성 | 슬래시 명령어 자동완성 | 미구현 |

## 체크포인트

- [x] `tars chat`으로 TUI가 실행된다
- [x] LLM 응답이 실시간 스트리밍된다
- [x] Page Up/Down으로 채팅 스크롤이 가능하다
- [x] ↑/↓ 키로 명령어 히스토리를 탐색할 수 있다
- [x] Esc로 스트리밍을 취소할 수 있다

## 다음 단계

Step 24에서는 브라우저 자동화를 구현합니다. Playwright 기반 브라우저 제어로 AI가 웹 페이지를 조작할 수 있게 합니다.
