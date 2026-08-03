package tarsserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/a2a"
	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/workerprotocol"
	"github.com/devlikebear/tars/internal/workstore"
	"github.com/rs/zerolog"
)

func TestWorkerControlPlaneAPIExposesSanitizedCurrentState(t *testing.T) {
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	controller, err := workerprotocol.OpenController(workerprotocol.ControllerOptions{
		StatePath: filepath.Join(t.TempDir(), "controller.json"), Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("open controller: %v", err)
	}
	applyWorkerEnvelope(t, controller, workerprotocol.Envelope{
		ProtocolVersion: workerprotocol.ProtocolVersionV1, MessageID: "worker-a:register:1",
		IdempotencyKey: "worker-a:register:1", Type: workerprotocol.MessageRegister,
		WorkerID: "worker-a", Sequence: 1, SentAt: now,
		Payload: mustWorkerPayload(t, workerprotocol.RegisterPayload{
			Transport: "ssh", Endpoint: "worker.example.test", Capabilities: fullWorkerCapabilities(),
		}),
	})
	applyWorkerEnvelope(t, controller, workerprotocol.Envelope{
		ProtocolVersion: workerprotocol.ProtocolVersionV1, MessageID: "worker-a:heartbeat:2",
		IdempotencyKey: "worker-a:heartbeat:2", Type: workerprotocol.MessageHeartbeat,
		WorkerID: "worker-a", Sequence: 2, SentAt: now,
		Payload: mustWorkerPayload(t, workerprotocol.HeartbeatPayload{
			LeaseTTLMS: 60_000, Metadata: map[string]string{"credential": "must-not-persist-in-api"},
		}),
	})
	placement, err := controller.CreatePlacement(context.Background(), workerprotocol.CreatePlacementInput{
		ID: "placement-a", WorkspaceID: "workspace-a", WorkID: "work-a", StepID: "step-a", AttemptID: "attempt-a",
		WorkerID: "worker-a", Policy: workerprotocol.DefaultExecutionPolicy(), Sync: workerprotocol.SyncSpec{
			Mode: workerprotocol.SyncModeDirectory, SourceOwner: workerprotocol.OwnerGateway,
			WorkspaceOwner: workerprotocol.OwnerWorker, ArtifactOwner: workerprotocol.OwnerGateway,
		},
	})
	if err != nil {
		t.Fatalf("create placement: %v", err)
	}
	applyWorkerEnvelope(t, controller, placementEnvelope(t, placement, 1, workerprotocol.MessageProvision, workerprotocol.ProvisionPayload{
		EnvironmentID: "environment-a", Policy: workerprotocol.DefaultExecutionPolicy(),
	}))
	applyWorkerEnvelope(t, controller, placementEnvelope(t, placement, 2, workerprotocol.MessageSync, workerprotocol.SyncPayload{
		Mode: workerprotocol.SyncModeDirectory, Digest: "sha256:workspace", URI: "https://user:secret@files.example.test/bundle",
		FileCount: 12, TotalBytes: 4096,
	}))
	applyWorkerEnvelope(t, controller, placementEnvelope(t, placement, 3, workerprotocol.MessageLease, workerprotocol.LeasePayload{LeaseTTLMS: 60_000}))

	handler := newWorkerControlPlaneAPIHandler(controller, true, zerolog.Nop())
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/admin/workers", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "worker.example.test") || strings.Contains(recorder.Body.String(), "must-not-persist") ||
		strings.Contains(recorder.Body.String(), "files.example.test") || strings.Contains(recorder.Body.String(), "endpoint") {
		t.Fatalf("worker API leaked control-plane secrets: %s", recorder.Body.String())
	}
	var response workerControlPlaneResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !response.Enabled || !response.A2A.Enabled || response.A2A.Adapter != a2a.AdapterName ||
		response.Summary.Workers != 1 || response.Summary.Placements != 1 || len(response.Workers) != 1 || len(response.Placements) != 1 {
		t.Fatalf("unexpected response: %#v", response)
	}
	if response.Workers[0].State != workerprotocol.WorkerStateLeased || response.Placements[0].State != workerprotocol.PlacementStateReady ||
		response.Placements[0].Sync.FileCount != 12 || response.Placements[0].Sync.TotalBytes != 4096 {
		t.Fatalf("unexpected worker/placement state: %#v %#v", response.Workers[0], response.Placements[0])
	}
}

func TestWorkerControlPlaneAPIPreservesDisabledLocalInstall(t *testing.T) {
	handler := newWorkerControlPlaneAPIHandler(nil, false, zerolog.Nop())
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/admin/workers", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response workerControlPlaneResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode disabled response: %v", err)
	}
	if response.Enabled || response.A2A.Enabled || len(response.Workers) != 0 || len(response.Placements) != 0 || len(response.Events) != 0 {
		t.Fatalf("disabled response: %#v", response)
	}

	methodRecorder := httptest.NewRecorder()
	handler.ServeHTTP(methodRecorder, httptest.NewRequest(http.MethodPost, "/v1/admin/workers", nil))
	if methodRecorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status=%d", methodRecorder.Code)
	}
}

func TestBuildWorkerControllerIsOptInAndUsesPrivateExecutionState(t *testing.T) {
	cfg := config.Default()
	if controller, err := buildWorkerControllerIfEnabled(cfg, nil); err != nil || controller != nil {
		t.Fatalf("disabled controller=%#v err=%v", controller, err)
	}

	ledger, err := workstore.Open(context.Background(), filepath.Join(t.TempDir(), "ledger.db"), workstore.Options{})
	if err != nil {
		t.Fatalf("open work ledger: %v", err)
	}
	t.Cleanup(func() { _ = ledger.Close() })
	cfg.WorkLedger.SchedulerRemoteWorkersEnabled = true
	cfg.WorkLedger.SchedulerExecutionDataDir = filepath.Join(t.TempDir(), "execution")
	controller, err := buildWorkerControllerIfEnabled(cfg, ledger)
	if err != nil || controller == nil {
		t.Fatalf("enabled controller=%#v err=%v", controller, err)
	}
	if snapshot := controller.Snapshot(); len(snapshot.Workers) != 0 || len(snapshot.Placements) != 0 {
		t.Fatalf("new controller snapshot: %#v", snapshot)
	}
}

func applyWorkerEnvelope(t *testing.T, controller *workerprotocol.Controller, envelope workerprotocol.Envelope) {
	t.Helper()
	if _, err := controller.Apply(context.Background(), envelope); err != nil {
		t.Fatalf("apply %s: %v", envelope.Type, err)
	}
}

func placementEnvelope(t *testing.T, placement workerprotocol.Placement, sequence int64, messageType workerprotocol.MessageType, payload any) workerprotocol.Envelope {
	t.Helper()
	return workerprotocol.Envelope{
		ProtocolVersion: workerprotocol.ProtocolVersionV1,
		MessageID:       placement.ID + ":" + messageType.String() + ":" + strconv.FormatInt(sequence, 10),
		IdempotencyKey:  placement.ID + ":" + messageType.String() + ":" + strconv.FormatInt(sequence, 10),
		Type:            messageType, WorkerID: placement.WorkerID, PlacementID: placement.ID, Sequence: sequence,
		SentAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC), Payload: mustWorkerPayload(t, payload),
	}
}

func mustWorkerPayload(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal worker payload: %v", err)
	}
	return raw
}

func fullWorkerCapabilities() workerprotocol.WorkerCapabilities {
	return workerprotocol.WorkerCapabilities{
		Resume: true, Streaming: true, Checkpoints: true, EgressPolicy: true, ResourceLimits: true, ArtifactScan: true,
	}
}
