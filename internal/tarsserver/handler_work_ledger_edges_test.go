package tarsserver

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/devlikebear/tars/internal/workscheduler"
	"github.com/devlikebear/tars/internal/workstore"
	"github.com/rs/zerolog"
)

func TestWorkLedgerControlRoutesRejectMissingSchedulerAndInvalidMethods(t *testing.T) {
	t.Parallel()

	store := openWorkLedgerHandlerTestStore(t)
	handler := newWorkLedgerAPIHandler(store, zerolog.Nop())
	for _, testCase := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/v1/work/works/work-a/wait"},
		{http.MethodGet, "/v1/work/works/work-a/watch"},
		{http.MethodPost, "/v1/admin/work/works/work-a/cancel"},
		{http.MethodPost, "/v1/admin/work/works/work-a/steps/step-a/resume"},
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(testCase.method, testCase.path, bytes.NewBufferString(`{"reason":"test"}`))
		request = request.WithContext(serverauth.WithWorkspaceID(request.Context(), "workspace-a"))
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusServiceUnavailable {
			t.Errorf("%s %s status=%d body=%s", testCase.method, testCase.path, recorder.Code, recorder.Body.String())
		}
	}

	for _, testCase := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/v1/work/works"},
		{http.MethodPost, "/v1/work/works/work-a/timeline"},
		{http.MethodPost, "/v1/work/works/work-a/wait"},
		{http.MethodPost, "/v1/work/works/work-a/watch"},
		{http.MethodPost, "/v1/work/legacy/sessions/session-a/tasks"},
		{http.MethodGet, "/v1/admin/work/works/work-a/cancel"},
		{http.MethodGet, "/v1/admin/work/works/work-a/steps/step-a/resume"},
	} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(testCase.method, testCase.path, nil))
		if recorder.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s %s status=%d", testCase.method, testCase.path, recorder.Code)
		}
	}
}

func TestWorkLedgerControlRoutesRejectMalformedMissingAndConflictingActions(t *testing.T) {
	t.Parallel()

	store := openWorkLedgerHandlerTestStore(t)
	scheduler, err := workscheduler.New(workscheduler.Options{
		Store: store, WorkspaceID: "workspace-a", WorkerID: "edge-scheduler", ActorID: "edge-scheduler",
		LeaseDuration: time.Minute, HeartbeatInterval: 20 * time.Second,
		PollInterval: time.Millisecond, MaxWorkers: 1,
		Executors: []workscheduler.Executor{workLedgerHandlerTestExecutor{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(scheduler.Close)
	handler := newWorkLedgerAPIHandler(store, zerolog.Nop(), scheduler)
	work := createWorkLedgerHandlerTestWork(t, store, "workspace-a", "edge-control", workstore.WorkStateReady)

	for _, target := range []string{
		"/v1/work/works/" + work.ID + "/wait?timeout_ms=0",
		"/v1/work/works/" + work.ID + "/wait?timeout_ms=invalid",
		"/v1/work/works/" + work.ID + "/wait?timeout_ms=300001",
		"/v1/work/works/" + work.ID + "/watch?after_sequence=-1",
		"/v1/work/works/" + work.ID + "/watch?after_sequence=invalid",
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, target, nil)
		request = request.WithContext(serverauth.WithWorkspaceID(request.Context(), "workspace-a"))
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("GET %s status=%d body=%s", target, recorder.Code, recorder.Body.String())
		}
	}

	for _, target := range []string{
		"/v1/work/works/missing/wait?timeout_ms=1",
		"/v1/work/works/missing/watch",
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, target, nil)
		request = request.WithContext(serverauth.WithWorkspaceID(request.Context(), "workspace-a"))
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusNotFound {
			t.Errorf("GET %s status=%d body=%s", target, recorder.Code, recorder.Body.String())
		}
	}

	streaming := &nonFlushingResponseWriter{header: make(http.Header)}
	watchRequest := httptest.NewRequest(http.MethodGet, "/v1/work/works/"+work.ID+"/watch", nil)
	watchRequest = watchRequest.WithContext(serverauth.WithWorkspaceID(watchRequest.Context(), "workspace-a"))
	handler.ServeHTTP(streaming, watchRequest)
	if streaming.status != http.StatusInternalServerError || !strings.Contains(streaming.body.String(), "streaming") {
		t.Fatalf("non-streaming writer status=%d body=%s", streaming.status, streaming.body.String())
	}

	for _, target := range []string{
		"/v1/work/works/work-a/nested/wait",
		"/v1/admin/work/works/work-a/nested/cancel",
	} {
		recorder := httptest.NewRecorder()
		method := http.MethodGet
		if strings.Contains(target, "/admin/") {
			method = http.MethodPost
		}
		request := httptest.NewRequest(method, target, bytes.NewBufferString(`{"reason":"test"}`))
		request = request.WithContext(serverauth.WithWorkspaceID(request.Context(), "workspace-a"))
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusNotFound {
			t.Errorf("%s %s status=%d", method, target, recorder.Code)
		}
	}

	for _, body := range []string{"", `{}`, `{`} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/v1/admin/work/works/"+work.ID+"/cancel", bytes.NewBufferString(body))
		request = request.WithContext(serverauth.WithWorkspaceID(request.Context(), "workspace-a"))
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("cancel body %q status=%d body=%s", body, recorder.Code, recorder.Body.String())
		}
	}
	missingCancel := httptest.NewRecorder()
	missingCancelRequest := httptest.NewRequest(http.MethodPost, "/v1/admin/work/works/missing/cancel", bytes.NewBufferString(`{"reason":"test"}`))
	missingCancelRequest = missingCancelRequest.WithContext(serverauth.WithWorkspaceID(missingCancelRequest.Context(), "workspace-a"))
	handler.ServeHTTP(missingCancel, missingCancelRequest)
	if missingCancel.Code != http.StatusNotFound {
		t.Fatalf("missing cancel status=%d body=%s", missingCancel.Code, missingCancel.Body.String())
	}

	for _, target := range []string{
		"/v1/admin/work/works/" + work.ID + "/resume",
		"/v1/admin/work/works/" + work.ID + "/steps//resume",
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, target, bytes.NewBufferString(`{"reason":"test"}`))
		request = request.WithContext(serverauth.WithWorkspaceID(request.Context(), "workspace-a"))
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusNotFound && recorder.Code != http.StatusBadRequest {
			t.Errorf("resume path %s status=%d body=%s", target, recorder.Code, recorder.Body.String())
		}
	}
	badResumeBody := httptest.NewRecorder()
	badResumeRequest := httptest.NewRequest(http.MethodPost, "/v1/admin/work/works/"+work.ID+"/steps/missing/resume", bytes.NewBufferString(`{}`))
	badResumeRequest = badResumeRequest.WithContext(serverauth.WithWorkspaceID(badResumeRequest.Context(), "workspace-a"))
	handler.ServeHTTP(badResumeBody, badResumeRequest)
	if badResumeBody.Code != http.StatusBadRequest {
		t.Fatalf("bad resume body status=%d body=%s", badResumeBody.Code, badResumeBody.Body.String())
	}
	missingResume := httptest.NewRecorder()
	missingResumeRequest := httptest.NewRequest(http.MethodPost, "/v1/admin/work/works/"+work.ID+"/steps/missing/resume", bytes.NewBufferString(`{"reason":"test"}`))
	missingResumeRequest = missingResumeRequest.WithContext(serverauth.WithWorkspaceID(missingResumeRequest.Context(), "workspace-a"))
	handler.ServeHTTP(missingResume, missingResumeRequest)
	if missingResume.Code != http.StatusNotFound {
		t.Fatalf("missing resume status=%d body=%s", missingResume.Code, missingResume.Body.String())
	}

	doneWork := createWorkLedgerHandlerTestWork(t, store, "workspace-a", "done-cancel-conflict", workstore.WorkStateDone)
	conflictCancel := httptest.NewRecorder()
	conflictCancelRequest := httptest.NewRequest(http.MethodPost, "/v1/admin/work/works/"+doneWork.ID+"/cancel", bytes.NewBufferString(`{"reason":"too late"}`))
	conflictCancelRequest = conflictCancelRequest.WithContext(serverauth.WithWorkspaceID(conflictCancelRequest.Context(), "workspace-a"))
	handler.ServeHTTP(conflictCancel, conflictCancelRequest)
	if conflictCancel.Code != http.StatusConflict {
		t.Fatalf("terminal cancel status=%d body=%s", conflictCancel.Code, conflictCancel.Body.String())
	}

	resumableWork, err := scheduler.Submit(context.Background(), workscheduler.SubmitInput{
		IdempotencyKey: "resume-conflict", Title: "Resume conflict", Adapter: "test", ActorID: "planner",
		Steps: []workscheduler.StepSpec{{Key: "run", Title: "Run", Policy: workstore.StepSchedulePolicy{MaxAttempts: 1}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	resumableProjection, err := store.GetWorkProjection(context.Background(), "workspace-a", resumableWork.ID)
	if err != nil || len(resumableProjection.Steps) != 1 {
		t.Fatalf("resume conflict projection=%+v err=%v", resumableProjection, err)
	}
	conflictResume := httptest.NewRecorder()
	conflictResumeRequest := httptest.NewRequest(http.MethodPost, "/v1/admin/work/works/"+resumableWork.ID+"/steps/"+resumableProjection.Steps[0].ID+"/resume", bytes.NewBufferString(`{"reason":"not gated"}`))
	conflictResumeRequest = conflictResumeRequest.WithContext(serverauth.WithWorkspaceID(conflictResumeRequest.Context(), "workspace-a"))
	handler.ServeHTTP(conflictResume, conflictResumeRequest)
	if conflictResume.Code != http.StatusConflict {
		t.Fatalf("ungated resume status=%d body=%s", conflictResume.Code, conflictResume.Body.String())
	}

	for _, path := range []string{
		"/v1/admin/work/works/%zz/steps/step-a/resume",
		"/v1/admin/work/works/work-a/steps/%zz/resume",
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"reason":"test"}`))
		request = request.WithContext(serverauth.WithWorkspaceID(request.Context(), "workspace-a"))
		handleWorkLedgerResume(recorder, request, scheduler, "workspace-a", path, zerolog.Nop())
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("malformed resume path %s status=%d body=%s", path, recorder.Code, recorder.Body.String())
		}
	}

	closedScheduler, err := workscheduler.New(workscheduler.Options{
		Store: store, WorkspaceID: "workspace-a", WorkerID: "closed-edge-scheduler", ActorID: "closed-edge-scheduler",
		LeaseDuration: time.Minute, HeartbeatInterval: 20 * time.Second,
		PollInterval: time.Millisecond, MaxWorkers: 1,
		Executors: []workscheduler.Executor{workLedgerHandlerTestExecutor{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	closedScheduler.Close()
	closedHandler := newWorkLedgerAPIHandler(store, zerolog.Nop(), closedScheduler)
	closedWait := httptest.NewRecorder()
	closedWaitRequest := httptest.NewRequest(http.MethodGet, "/v1/work/works/"+work.ID+"/wait?timeout_ms=100", nil)
	closedWaitRequest = closedWaitRequest.WithContext(serverauth.WithWorkspaceID(closedWaitRequest.Context(), "workspace-a"))
	closedHandler.ServeHTTP(closedWait, closedWaitRequest)
	if closedWait.Code != http.StatusInternalServerError {
		t.Fatalf("closed scheduler wait status=%d body=%s", closedWait.Code, closedWait.Body.String())
	}
}

func TestWorkLedgerControlPathAndDurationHelpersBoundUntrustedInput(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		query   url.Values
		want    time.Duration
		wantErr bool
	}{
		{url.Values{}, 30 * time.Second, false},
		{url.Values{"timeout_ms": []string{"25"}}, 25 * time.Millisecond, false},
		{url.Values{"timeout_ms": []string{"0"}}, 0, true},
		{url.Values{"timeout_ms": []string{"101"}}, 0, true},
		{url.Values{"timeout_ms": []string{"bad"}}, 0, true},
	} {
		got, err := boundedQueryDuration(testCase.query, "timeout_ms", 30000, 100)
		if got != testCase.want || (err != nil) != testCase.wantErr {
			t.Errorf("bounded duration query=%v got=%v err=%v", testCase.query, got, err)
		}
	}
	recorder := httptest.NewRecorder()
	if id, ok := workIDFromActionPath(recorder, "/v1/work/works/work%20a/wait", "/v1/work/works/", "/wait"); !ok || id != "work a" {
		t.Fatalf("decoded work id=%q ok=%v", id, ok)
	}
	if _, ok := workIDFromActionPath(recorder, "/v1/work/works/a/b/wait", "/v1/work/works/", "/wait"); ok {
		t.Fatal("nested work path accepted")
	}
	recorder = httptest.NewRecorder()
	if _, ok := workIDFromActionPath(recorder, "/v1/work/works/%zz/wait", "/v1/work/works/", "/wait"); ok {
		t.Fatal("malformed escaped work id accepted")
	}
	if states, err := parseWorkLedgerStates(url.Values{"state": []string{" , running, "}}); err != nil || len(states) != 1 || states[0] != workstore.WorkStateRunning {
		t.Fatalf("states with blanks=%v err=%v", states, err)
	}
	recorder = httptest.NewRecorder()
	handleWorkLedgerTimeline(recorder, httptest.NewRequest(http.MethodGet, "/", nil), nil, "workspace-a", "/v1/work/works/work-a/not-timeline", zerolog.Nop())
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("malformed timeline status=%d", recorder.Code)
	}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	if actor := workAPIActor(request); actor != "work-api:local" {
		t.Fatalf("default API actor=%q", actor)
	}
}

type nonFlushingResponseWriter struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func (writer *nonFlushingResponseWriter) Header() http.Header { return writer.header }

func (writer *nonFlushingResponseWriter) Write(raw []byte) (int, error) {
	if writer.status == 0 {
		writer.status = http.StatusOK
	}
	return writer.body.Write(raw)
}

func (writer *nonFlushingResponseWriter) WriteHeader(status int) { writer.status = status }

var _ http.ResponseWriter = (*nonFlushingResponseWriter)(nil)
