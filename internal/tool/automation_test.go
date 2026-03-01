package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tarsncase/internal/cron"
)

func TestCronListTool_ReturnsJobs(t *testing.T) {
	root := t.TempDir()
	store := cron.NewStore(root)
	if _, err := store.CreateWithOptions(cron.CreateInput{
		Name:      "morning",
		Prompt:    "check inbox",
		Schedule:  "every:1h",
		Enabled:   true,
		HasEnable: true,
	}); err != nil {
		t.Fatalf("create cron job: %v", err)
	}

	tl := NewCronListTool(store)
	result, err := tl.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("execute cron_list: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success result, got %s", result.Text())
	}

	var body struct {
		Count int        `json:"count"`
		Jobs  []cron.Job `json:"jobs"`
	}
	if err := json.Unmarshal([]byte(result.Text()), &body); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if body.Count != 1 || len(body.Jobs) != 1 {
		t.Fatalf("expected one cron job, got %+v", body)
	}
}

func TestCronCreateUpdateDeleteTools_Workflow(t *testing.T) {
	root := t.TempDir()
	store := cron.NewStore(root)

	create := NewCronCreateTool(store)
	createResult, err := create.Execute(context.Background(), json.RawMessage(`{"name":"ops","prompt":"check status","schedule":"every:30m","session_target":"main","wake_mode":"agent_loop","delivery_mode":"session","payload":{"priority":"high"}}`))
	if err != nil {
		t.Fatalf("execute cron_create: %v", err)
	}
	if createResult.IsError {
		t.Fatalf("expected create success, got %s", createResult.Text())
	}
	var created cron.Job
	if err := json.Unmarshal([]byte(createResult.Text()), &created); err != nil {
		t.Fatalf("decode created job: %v", err)
	}
	if created.ID == "" || created.SessionTarget != "main" {
		t.Fatalf("unexpected created job: %+v", created)
	}

	update := NewCronUpdateTool(store)
	updateResult, err := update.Execute(context.Background(), json.RawMessage(`{"job_id":"`+created.ID+`","enabled":false}`))
	if err != nil {
		t.Fatalf("execute cron_update: %v", err)
	}
	if updateResult.IsError {
		t.Fatalf("expected update success, got %s", updateResult.Text())
	}
	var updated cron.Job
	if err := json.Unmarshal([]byte(updateResult.Text()), &updated); err != nil {
		t.Fatalf("decode updated job: %v", err)
	}
	if updated.Enabled {
		t.Fatalf("expected enabled=false after update")
	}

	del := NewCronDeleteTool(store)
	deleteResult, err := del.Execute(context.Background(), json.RawMessage(`{"job_id":"`+created.ID+`"}`))
	if err != nil {
		t.Fatalf("execute cron_delete: %v", err)
	}
	if deleteResult.IsError {
		t.Fatalf("expected delete success, got %s", deleteResult.Text())
	}
}

func TestCronRunTool_ExecutesAndRecordsRun(t *testing.T) {
	root := t.TempDir()
	store := cron.NewStore(root)
	job, err := store.CreateWithOptions(cron.CreateInput{
		Name:      "manual",
		Prompt:    "run now",
		Schedule:  "every:1m",
		Enabled:   true,
		HasEnable: true,
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	runCalled := 0
	tl := NewCronRunTool(store, func(_ context.Context, j cron.Job) (string, error) {
		if j.ID != job.ID {
			t.Fatalf("unexpected job id: %s", j.ID)
		}
		runCalled++
		return "cron tool ok", nil
	})

	result, err := tl.Execute(context.Background(), json.RawMessage(`{"job_id":"`+job.ID+`"}`))
	if err != nil {
		t.Fatalf("execute cron_run: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success result, got %s", result.Text())
	}
	if runCalled != 1 {
		t.Fatalf("expected run callback called once, got %d", runCalled)
	}
	runs, err := store.ListRuns(job.ID, 10)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected one run record, got %d", len(runs))
	}
	if runs[0].Response != "cron tool ok" {
		t.Fatalf("expected run response stored, got %q", runs[0].Response)
	}
}

func TestHeartbeatTools_StatusAndRunOnce(t *testing.T) {
	runCalled := 0
	statusTool := NewHeartbeatStatusTool(func(context.Context) (HeartbeatStatus, error) {
		return HeartbeatStatus{
			Configured:   true,
			ActiveHours:  "09:00-18:00",
			Timezone:     "UTC",
			LastRunAt:    "2026-02-16T10:00:00Z",
			LastSkipped:  false,
			LastLogged:   true,
			LastResponse: "next action",
		}, nil
	})
	statusResult, err := statusTool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("execute heartbeat_status: %v", err)
	}
	if statusResult.IsError {
		t.Fatalf("expected heartbeat status success, got %s", statusResult.Text())
	}

	runTool := NewHeartbeatRunOnceTool(func(context.Context) (HeartbeatRunResult, error) {
		runCalled++
		return HeartbeatRunResult{
			Response:     "done",
			Skipped:      false,
			SkipReason:   "",
			Logged:       true,
			Acknowledged: false,
			RanAt:        time.Date(2026, 2, 16, 10, 0, 0, 0, time.UTC),
		}, nil
	})
	runResult, err := runTool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("execute heartbeat_run_once: %v", err)
	}
	if runResult.IsError {
		t.Fatalf("expected heartbeat run success, got %s", runResult.Text())
	}
	if runCalled != 1 {
		t.Fatalf("expected run callback called once, got %d", runCalled)
	}
}

func TestCronTool_ActionRouting(t *testing.T) {
	root := t.TempDir()
	store := cron.NewStore(root)
	job, err := store.CreateWithOptions(cron.CreateInput{
		Name:      "daily",
		Prompt:    "check status",
		Schedule:  "every:1h",
		Enabled:   true,
		HasEnable: true,
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	runCalled := 0
	tl := NewCronTool(store, func(_ context.Context, j cron.Job) (string, error) {
		runCalled++
		return "ok:" + j.ID, nil
	})

	listRes, err := tl.Execute(context.Background(), json.RawMessage(`{"action":"list"}`))
	if err != nil {
		t.Fatalf("cron action list: %v", err)
	}
	if listRes.IsError {
		t.Fatalf("expected list success, got %s", listRes.Text())
	}

	runRes, err := tl.Execute(context.Background(), json.RawMessage(`{"action":"run","id":"`+job.ID+`"}`))
	if err != nil {
		t.Fatalf("cron action run: %v", err)
	}
	if runRes.IsError {
		t.Fatalf("expected run success, got %s", runRes.Text())
	}
	if runCalled != 1 {
		t.Fatalf("expected run callback called once, got %d", runCalled)
	}
}

func TestCronTool_ActionGetAndRuns(t *testing.T) {
	root := t.TempDir()
	store := cron.NewStore(root)
	job, err := store.CreateWithOptions(cron.CreateInput{
		Name:      "detail-target",
		Prompt:    "status check",
		Schedule:  "every:1h",
		Enabled:   true,
		HasEnable: true,
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	tl := NewCronTool(store, func(_ context.Context, j cron.Job) (string, error) {
		return "ok:" + j.ID, nil
	})

	getRes, err := tl.Execute(context.Background(), json.RawMessage(`{"action":"get","job_id":"`+job.ID+`"}`))
	if err != nil {
		t.Fatalf("cron action get: %v", err)
	}
	if getRes.IsError {
		t.Fatalf("expected get success, got %s", getRes.Text())
	}
	var gotJob cron.Job
	if err := json.Unmarshal([]byte(getRes.Text()), &gotJob); err != nil {
		t.Fatalf("decode cron get result: %v", err)
	}
	if gotJob.ID != job.ID {
		t.Fatalf("expected job id %q, got %q", job.ID, gotJob.ID)
	}

	if _, err := tl.Execute(context.Background(), json.RawMessage(`{"action":"run","job_id":"`+job.ID+`"}`)); err != nil {
		t.Fatalf("cron action run: %v", err)
	}
	runsRes, err := tl.Execute(context.Background(), json.RawMessage(`{"action":"runs","job_id":"`+job.ID+`","limit":5}`))
	if err != nil {
		t.Fatalf("cron action runs: %v", err)
	}
	if runsRes.IsError {
		t.Fatalf("expected runs success, got %s", runsRes.Text())
	}
	var runsBody struct {
		Count int              `json:"count"`
		Runs  []cron.RunRecord `json:"runs"`
	}
	if err := json.Unmarshal([]byte(runsRes.Text()), &runsBody); err != nil {
		t.Fatalf("decode runs result: %v", err)
	}
	if runsBody.Count < 1 || len(runsBody.Runs) < 1 {
		t.Fatalf("expected at least one run record, got %+v", runsBody)
	}
	if runsBody.Runs[0].JobID != job.ID {
		t.Fatalf("expected run job id %q, got %q", job.ID, runsBody.Runs[0].JobID)
	}
}

func TestCronCreateTool_RejectsNonNaturalPrompt(t *testing.T) {
	root := t.TempDir()
	store := cron.NewStore(root)
	create := NewCronCreateTool(store)

	result, err := create.Execute(context.Background(), json.RawMessage(`{"name":"ops","prompt":"rm -rf /tmp","schedule":"every:30m"}`))
	if err != nil {
		t.Fatalf("execute cron_create: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected error for non-natural prompt, got %s", result.Text())
	}
	if !strings.Contains(result.Text(), "prompt는 자연어 할일 문장이어야 합니다") {
		t.Fatalf("unexpected error message: %s", result.Text())
	}
}

func TestCronUpdateTool_RejectsNonNaturalPrompt(t *testing.T) {
	root := t.TempDir()
	store := cron.NewStore(root)
	job, err := store.CreateWithOptions(cron.CreateInput{
		Name:      "ops",
		Prompt:    "디스크 상태 확인하기",
		Schedule:  "every:30m",
		Enabled:   true,
		HasEnable: true,
	})
	if err != nil {
		t.Fatalf("create cron job: %v", err)
	}
	update := NewCronUpdateTool(store)

	result, err := update.Execute(context.Background(), json.RawMessage(`{"job_id":"`+job.ID+`","prompt":"sudo rm -rf /"}`))
	if err != nil {
		t.Fatalf("execute cron_update: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected error for non-natural prompt, got %s", result.Text())
	}
	if !strings.Contains(result.Text(), "prompt는 자연어 할일 문장이어야 합니다") {
		t.Fatalf("unexpected error message: %s", result.Text())
	}
}

func TestHeartbeatTool_ActionRouting(t *testing.T) {
	runCalled := 0
	tl := NewHeartbeatTool(
		func(context.Context) (HeartbeatStatus, error) {
			return HeartbeatStatus{Configured: true}, nil
		},
		func(context.Context) (HeartbeatRunResult, error) {
			runCalled++
			return HeartbeatRunResult{Response: "done"}, nil
		},
	)

	statusRes, err := tl.Execute(context.Background(), json.RawMessage(`{"action":"status"}`))
	if err != nil {
		t.Fatalf("heartbeat action status: %v", err)
	}
	if statusRes.IsError {
		t.Fatalf("expected status success, got %s", statusRes.Text())
	}

	runRes, err := tl.Execute(context.Background(), json.RawMessage(`{"action":"run_once"}`))
	if err != nil {
		t.Fatalf("heartbeat action run_once: %v", err)
	}
	if runRes.IsError {
		t.Fatalf("expected run_once success, got %s", runRes.Text())
	}
	if runCalled != 1 {
		t.Fatalf("expected run callback called once, got %d", runCalled)
	}
}
