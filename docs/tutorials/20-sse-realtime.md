# Step 20. Runtime Event Stream

> 학습 목표: Server-Sent Events로 cron/ops/pulse/usage 알림을 콘솔에 실시간 푸시하는 구조 이해

## 왜 Event Stream인가

운영 UI는 채팅 응답만 보여주면 부족합니다. 사용자는 다음 일을 실시간으로 알아야 합니다.

- cron job 실패
- cleanup approval 생성/승인/실패
- pulse watchdog 알림
- usage limit 경고
- background watchdog 실패

이 이벤트들은 특정 채팅 세션의 SSE와 다릅니다. 그래서 TARS는 별도 runtime notification stream을 둡니다.

```
GET /v1/events/stream
GET /v1/events/history?limit=N
POST /v1/events/read
```

## 이벤트 모델

**`internal/tarsserver/notify.go`**

```go
type notificationEvent struct {
    ID        int64  `json:"id,omitempty"`
    Type      string `json:"type"`
    Category  string `json:"category"`
    Severity  string `json:"severity"`
    Title     string `json:"title"`
    Message   string `json:"message"`
    Timestamp string `json:"timestamp"`
    JobID     string `json:"job_id,omitempty"`
    SessionID string `json:"session_id,omitempty"`
    OpenPath  string `json:"open_path,omitempty"`
}
```

`Type`은 일반적으로 `notification`이며, keepalive payload는 `keepalive`입니다. `Category`는 이벤트 출처(`cron`, `ops`, `pulse`, `watchdog`, `usage`, `system`)를 나타냅니다.

## 실습

### 20-1. Event broker

```go
type eventBroker struct {
    mu     sync.RWMutex
    nextID int
    subs   map[int]eventSubscription
}

func (b *eventBroker) subscribe() (int, <-chan notificationEvent, func())
func (b *eventBroker) publish(evt notificationEvent)
```

각 구독자는 버퍼 32개짜리 채널을 받습니다. 소비자가 느려 채널이 꽉 차면 이벤트를 버립니다. 실시간 UI 신호는 best-effort여야 서버가 막히지 않습니다.

### 20-2. Dispatcher

```go
func (d *notificationDispatcher) Emit(ctx context.Context, evt notificationEvent) {
    evt.Type = "notification"
    stored, _ := d.store.append(evt)
    d.broker.publish(stored)
    d.notifier.Notify(ctx, stored)
}
```

Dispatcher는 세 가지 일을 한 곳에서 처리합니다.

- `notificationStore`에 history 저장
- `eventBroker`로 SSE subscriber에게 publish
- 설정에 따라 desktop notifier 실행

### 20-3. SSE handler

```go
func newEventStreamHandler(broker *eventBroker, logger zerolog.Logger) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "text/event-stream")
        _, ch, unsubscribe := broker.subscribe()
        defer unsubscribe()

        ping := time.NewTicker(10 * time.Second)
        for {
            select {
            case <-ping.C:
                fmt.Fprintf(w, "data: {\"type\":\"keepalive\"}\n\n")
            case evt := <-ch:
                payload, _ := json.Marshal(evt)
                fmt.Fprintf(w, "data: %s\n\n", payload)
            }
            flusher.Flush()
        }
    })
}
```

TARS의 runtime event stream은 `event:` 줄 없이 `data:` JSON payload만 보냅니다. 채팅 SSE도 같은 `data:` 중심 파서를 공유할 수 있습니다.

### 20-4. History와 read cursor

```go
GET  /v1/events/history?limit=50
POST /v1/events/read {"last_id": 123}
```

history 응답은 다음 정보를 포함합니다.

```json
{
  "items": [],
  "unread_count": 3,
  "read_cursor": 120,
  "last_id": 123
}
```

read cursor는 role별로 관리됩니다. user/admin이 보는 이벤트 범위가 달라질 수 있기 때문입니다.

### 20-5. 이벤트를 발행하는 곳

| Source | 예시 |
|--------|------|
| cron | job 실패/완료 notification |
| ops | cleanup approval required/completed/rejected |
| pulse | watchdog decision 결과 |
| usage | 사용량 제한 경고 |
| watchdog | background runner health 경고 |

발행자는 SSE 프로토콜을 알 필요가 없습니다. `newNotificationEvent(...)`를 만들고 dispatcher에 넘기면 됩니다.

## 전체 데이터 흐름

```
cron / ops / pulse / usage
  → newNotificationEvent(...)
  → notificationDispatcher.Emit(...)
      ├── notificationStore.append(...)
      ├── eventBroker.publish(...)
      └── optional desktop notifier
  → GET /v1/events/stream subscriber
  → console notification UI
```

최신 Console의 `/console/pulse`는 raw tick 목록만 보여주지 않고 incident card를 함께 렌더링합니다. 각 card는 signal kind별 likely cause, relevant details/log snippets, severity, recommended action, affected page link, safe re-check button을 보여주므로 사용자는 raw log를 먼저 뒤지지 않고 Cron, Agent Runtime, Approvals, Settings, Reflection, Chat 중 어디로 가야 하는지 바로 판단할 수 있습니다.

## 체크포인트

- [x] `/v1/events/stream`이 keepalive와 notification payload를 보낸다
- [x] 느린 subscriber가 서버 publish를 막지 않는다
- [x] `/v1/events/history`가 unread count와 last_id를 반환한다
- [x] `/v1/events/read`가 read cursor를 갱신한다
- [x] ops/cron/pulse가 같은 이벤트 파이프라인을 사용한다

## 다음 단계

Step 21에서는 외부 접근이 가능한 API를 안전하게 보호하기 위한 인증 미들웨어를 다룹니다.
