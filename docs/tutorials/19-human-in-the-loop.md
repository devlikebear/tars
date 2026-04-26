# Step 19. Ops Approval

> 학습 목표: 위험한 운영 작업을 계획과 승인 단계로 분리하는 Human-in-the-loop 패턴 이해

## 왜 Approval인가

TARS는 로컬 파일, 로그, 임시 디렉터리, cron 실행 기록처럼 실제 사용자 환경에 영향을 줄 수 있는 데이터를 다룹니다. 자동화가 이런 작업을 바로 실행하면:

- 의도하지 않은 파일 삭제가 발생할 수 있음
- cleanup 후보를 사람이 확인할 시간이 없음
- 운영 작업의 책임 경계가 흐려짐

그래서 최신 TARS의 Human-in-the-loop는 project/autopilot phase가 아니라 **ops approval** 흐름으로 모읍니다.

```
계획 생성 → approval 저장 → 사용자 승인/거절 → 적용 → 이벤트 기록
```

## 핵심 API

| API | 역할 |
|-----|------|
| `GET /v1/ops/status` | 디스크/프로세스 상태 조회 |
| `POST /v1/ops/cleanup/plan` | 삭제 후보와 approval ID 생성 |
| `GET /v1/ops/approvals` | approval 목록 조회 |
| `POST /v1/ops/approvals/{id}/approve` | 승인 후 cleanup 적용 |
| `POST /v1/ops/approvals/{id}/reject` | cleanup 거절 |
| `POST /v1/ops/cleanup/apply` | 승인된 approval ID로 cleanup 적용 |

CLI도 같은 API를 사용합니다.

```bash
tars approve list
tars approve run <approval_id>
tars approve reject <approval_id>
```

## 실습

### 19-1. Cleanup plan 타입

**`internal/ops`**

```go
type CleanupCandidate struct {
    Path      string `json:"path"`
    SizeBytes int64  `json:"size_bytes"`
    Reason    string `json:"reason,omitempty"`
}

type CleanupPlan struct {
    ApprovalID string             `json:"approval_id"`
    CreatedAt  time.Time          `json:"created_at"`
    TotalBytes int64              `json:"total_bytes"`
    Candidates []CleanupCandidate `json:"candidates"`
}
```

`CreateCleanupPlan`은 후보만 찾고 즉시 삭제하지 않습니다. 이 경계가 approval 패턴의 핵심입니다.

### 19-2. Approval 상태

```go
type Approval struct {
    ID          string      `json:"id"`
    Type        string      `json:"type"`
    Status      string      `json:"status"`
    RequestedAt time.Time   `json:"requested_at"`
    UpdatedAt   time.Time   `json:"updated_at"`
    ReviewedAt  *time.Time  `json:"reviewed_at,omitempty"`
    Plan        CleanupPlan `json:"plan"`
    Note        string      `json:"note,omitempty"`
}
```

approval은 `workspace/ops/approvals.json`에 저장됩니다. 서버 재시작 후에도 pending approval을 다시 볼 수 있어야 하기 때문입니다.

### 19-3. HTTP handler

**`internal/tarsserver/handler_ops.go`**

```go
mux.HandleFunc("/v1/ops/cleanup/plan", func(w http.ResponseWriter, r *http.Request) {
    plan, err := manager.CreateCleanupPlan(r.Context())
    if err != nil {
        writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "ops cleanup plan failed"})
        return
    }
    emit(r.Context(), newNotificationEvent("ops", "warn", "Cleanup approval required", "approval_id="+plan.ApprovalID))
    writeJSON(w, http.StatusOK, plan)
})
```

계획 생성 시 notification event를 함께 발행합니다. 콘솔은 이벤트 스트림/히스토리를 통해 “승인이 필요하다”는 상태를 보여줄 수 있습니다.

### 19-4. 승인과 적용

```go
case "approve":
    err = manager.Approve(approvalID)
    if err == nil {
        result, applyErr := manager.ApplyCleanup(r.Context(), approvalID)
        // 성공/실패를 notification event로 남김
    }
case "reject":
    err = manager.Reject(approvalID)
```

승인 API는 현재 cleanup을 즉시 적용합니다. 실패하면 approval note에 에러를 남기고, event severity를 `error`로 발행합니다.

### 19-5. CLI wrapper

**`cmd/tars/approval_main.go`**

```go
switch action {
case "list":
    items, err := client.ListApprovals(ctx)
case "run":
    err := client.ApproveCleanup(ctx, approvalID)
    result, err := client.ApplyCleanup(ctx, approvalID)
case "reject":
    err := client.RejectCleanup(ctx, approvalID)
}
```

CLI는 별도 로직을 갖지 않고 protocol client를 통해 서버 API를 호출합니다. cleanup 정책은 서버의 `ops.Manager`에만 둡니다.

## 전체 흐름

```
사용자/콘솔/펄스
  → POST /v1/ops/cleanup/plan
  → approval_id 생성, candidates 저장
  → event: Cleanup approval required
  → 사용자 확인
  → approve 또는 reject
  → 적용 결과 event + approval note
```

## 체크포인트

- [x] cleanup plan 생성만으로 파일이 삭제되지 않는다
- [x] approval ID가 저장되고 목록에서 조회된다
- [x] approve 후 삭제 결과가 반환된다
- [x] reject 후 cleanup이 실행되지 않는다
- [x] 결과가 `/v1/events/*` notification pipeline에 남는다

## 다음 단계

Step 20에서는 이 approval/cron/pulse 결과를 콘솔에 실시간으로 전달하는 runtime event stream을 다룹니다.
