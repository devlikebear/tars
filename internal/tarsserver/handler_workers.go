package tarsserver

import (
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/a2a"
	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/workerprotocol"
	"github.com/devlikebear/tars/internal/workstore"
	"github.com/rs/zerolog"
)

const maxWorkerControlPlaneEvents = 200

type workerControlPlaneResponse struct {
	Enabled         bool                       `json:"enabled"`
	ProtocolVersion string                     `json:"protocol_version"`
	A2A             workerA2AView              `json:"a2a"`
	Summary         workerControlPlaneSummary  `json:"summary"`
	Workers         []workerControlPlaneWorker `json:"workers"`
	Placements      []workerPlacementView      `json:"placements"`
	Events          []workerControlEventView   `json:"events"`
	UpdatedAt       *time.Time                 `json:"updated_at,omitempty"`
}

type workerA2AView struct {
	Enabled         bool   `json:"enabled"`
	Adapter         string `json:"adapter"`
	ProtocolVersion string `json:"protocol_version"`
}

type workerControlPlaneSummary struct {
	Workers       int `json:"workers"`
	ReadyWorkers  int `json:"ready_workers"`
	LostWorkers   int `json:"lost_workers"`
	Placements    int `json:"placements"`
	Active        int `json:"active_placements"`
	Recovering    int `json:"recovering_placements"`
	RecoveryCount int `json:"recovery_count"`
}

type workerControlPlaneWorker struct {
	ID              string                            `json:"id"`
	ProtocolVersion string                            `json:"protocol_version"`
	Transport       string                            `json:"transport"`
	State           workerprotocol.WorkerState        `json:"state"`
	Capabilities    workerprotocol.WorkerCapabilities `json:"capabilities"`
	LastSequence    int64                             `json:"last_sequence"`
	LastSeenAt      time.Time                         `json:"last_seen_at"`
	LeaseExpiresAt  *time.Time                        `json:"lease_expires_at,omitempty"`
	Version         int                               `json:"version"`
}

type workerPlacementView struct {
	ID             string                         `json:"id"`
	WorkspaceID    string                         `json:"workspace_id"`
	WorkID         string                         `json:"work_id"`
	StepID         string                         `json:"step_id"`
	AttemptID      string                         `json:"attempt_id"`
	WorkerID       string                         `json:"worker_id"`
	EnvironmentID  string                         `json:"environment_id,omitempty"`
	State          workerprotocol.PlacementState  `json:"state"`
	Policy         workerprotocol.ExecutionPolicy `json:"policy"`
	Sync           workerSyncView                 `json:"sync"`
	Checkpoint     *workerprotocol.Checkpoint     `json:"checkpoint,omitempty"`
	LeaseExpiresAt *time.Time                     `json:"lease_expires_at,omitempty"`
	LastSequence   int64                          `json:"last_sequence"`
	RecoveryCount  int                            `json:"recovery_count"`
	Version        int                            `json:"version"`
	UpdatedAt      time.Time                      `json:"updated_at"`
}

type workerSyncView struct {
	Mode           workerprotocol.SyncMode  `json:"mode"`
	SourceOwner    workerprotocol.Ownership `json:"source_owner"`
	WorkspaceOwner workerprotocol.Ownership `json:"workspace_owner"`
	ArtifactOwner  workerprotocol.Ownership `json:"artifact_owner"`
	ManifestDigest string                   `json:"manifest_digest,omitempty"`
	FileCount      int                      `json:"file_count,omitempty"`
	TotalBytes     int64                    `json:"total_bytes,omitempty"`
}

type workerControlEventView struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	Entity      string    `json:"entity"`
	WorkerID    string    `json:"worker_id,omitempty"`
	PlacementID string    `json:"placement_id,omitempty"`
	WorkID      string    `json:"work_id,omitempty"`
	StepID      string    `json:"step_id,omitempty"`
	AttemptID   string    `json:"attempt_id,omitempty"`
	Sequence    int64     `json:"sequence,omitempty"`
	FromState   string    `json:"from_state,omitempty"`
	ToState     string    `json:"to_state,omitempty"`
	Published   bool      `json:"published"`
	OccurredAt  time.Time `json:"occurred_at"`
}

func buildWorkerControllerIfEnabled(cfg config.Config, ledger *workstore.Store) (*workerprotocol.Controller, error) {
	if !cfg.WorkLedger.SchedulerRemoteWorkersEnabled {
		return nil, nil
	}
	dataDir := strings.TrimSpace(cfg.WorkLedger.SchedulerExecutionDataDir)
	if ledger == nil || dataDir == "" {
		return nil, fmt.Errorf("remote worker control plane requires the Work Ledger and execution data directory")
	}
	if pathsOverlap(cfg.WorkspaceDir, dataDir) {
		return nil, fmt.Errorf("remote worker control state must be outside the workspace")
	}
	sink, err := workerprotocol.NewWorkLedgerSink(ledger, "tars-worker-control")
	if err != nil {
		return nil, err
	}
	return workerprotocol.OpenController(workerprotocol.ControllerOptions{
		StatePath: filepath.Join(dataDir, "remote-workers", "controller.json"),
		EventSink: sink,
	})
}

func newWorkerControlPlaneAPIHandler(controller *workerprotocol.Controller, a2aEnabled bool, logger zerolog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		if strings.TrimSuffix(r.URL.Path, "/") != "/v1/admin/workers" {
			http.NotFound(w, r)
			return
		}
		response := buildWorkerControlPlaneResponse(controller, a2aEnabled)
		logger.Debug().Int("workers", response.Summary.Workers).Int("placements", response.Summary.Placements).Msg("read worker control plane")
		writeJSON(w, http.StatusOK, response)
	})
}

func buildWorkerControlPlaneResponse(controller *workerprotocol.Controller, a2aEnabled bool) workerControlPlaneResponse {
	response := workerControlPlaneResponse{
		Enabled: controller != nil, ProtocolVersion: workerprotocol.ProtocolVersionV1,
		A2A:     workerA2AView{Enabled: a2aEnabled, Adapter: a2a.AdapterName, ProtocolVersion: a2a.ProtocolVersion},
		Workers: []workerControlPlaneWorker{}, Placements: []workerPlacementView{}, Events: []workerControlEventView{},
	}
	if controller == nil {
		return response
	}
	snapshot := controller.Snapshot()
	if !snapshot.UpdatedAt.IsZero() {
		updated := snapshot.UpdatedAt
		response.UpdatedAt = &updated
	}
	for _, worker := range snapshot.Workers {
		response.Workers = append(response.Workers, workerControlPlaneWorker{
			ID: worker.ID, ProtocolVersion: worker.ProtocolVersion, Transport: worker.Transport, State: worker.State,
			Capabilities: worker.Capabilities, LastSequence: worker.LastSequence, LastSeenAt: worker.LastSeenAt,
			LeaseExpiresAt: worker.LeaseExpiresAt, Version: worker.Version,
		})
		if worker.State == workerprotocol.WorkerStateReady {
			response.Summary.ReadyWorkers++
		}
		if worker.State == workerprotocol.WorkerStateLost || worker.State == workerprotocol.WorkerStateDisconnected {
			response.Summary.LostWorkers++
		}
	}
	for _, placement := range snapshot.Placements {
		response.Placements = append(response.Placements, workerPlacementView{
			ID: placement.ID, WorkspaceID: placement.WorkspaceID, WorkID: placement.WorkID,
			StepID: placement.StepID, AttemptID: placement.AttemptID, WorkerID: placement.WorkerID,
			EnvironmentID: placement.EnvironmentID, State: placement.State, Policy: placement.Policy,
			Sync: workerSyncView{
				Mode: placement.Sync.Mode, SourceOwner: placement.Sync.SourceOwner,
				WorkspaceOwner: placement.Sync.WorkspaceOwner, ArtifactOwner: placement.Sync.ArtifactOwner,
				ManifestDigest: placement.Sync.ManifestDigest, FileCount: placement.SyncFileCount, TotalBytes: placement.SyncTotalBytes,
			},
			Checkpoint: placement.Checkpoint, LeaseExpiresAt: placement.LeaseExpiresAt,
			LastSequence: placement.LastSequence, RecoveryCount: placement.RecoveryCount,
			Version: placement.Version, UpdatedAt: placement.UpdatedAt,
		})
		response.Summary.RecoveryCount += placement.RecoveryCount
		if activePlacementState(placement.State) {
			response.Summary.Active++
		}
		if recoveringPlacementState(placement.State) {
			response.Summary.Recovering++
		}
	}
	events := snapshot.Events
	if len(events) > maxWorkerControlPlaneEvents {
		events = events[len(events)-maxWorkerControlPlaneEvents:]
	}
	for _, event := range events {
		response.Events = append(response.Events, workerControlEventView{
			ID: event.ID, Type: event.Type, Entity: event.Entity, WorkerID: event.WorkerID,
			PlacementID: event.PlacementID, WorkID: event.WorkID, StepID: event.StepID, AttemptID: event.AttemptID,
			Sequence: event.Sequence, FromState: event.FromState, ToState: event.ToState,
			Published: event.Published, OccurredAt: event.OccurredAt,
		})
	}
	sort.Slice(response.Workers, func(i, j int) bool { return response.Workers[i].ID < response.Workers[j].ID })
	sort.Slice(response.Placements, func(i, j int) bool {
		if response.Placements[i].UpdatedAt.Equal(response.Placements[j].UpdatedAt) {
			return response.Placements[i].ID < response.Placements[j].ID
		}
		return response.Placements[i].UpdatedAt.After(response.Placements[j].UpdatedAt)
	})
	sort.Slice(response.Events, func(i, j int) bool {
		if response.Events[i].OccurredAt.Equal(response.Events[j].OccurredAt) {
			return response.Events[i].ID > response.Events[j].ID
		}
		return response.Events[i].OccurredAt.After(response.Events[j].OccurredAt)
	})
	response.Summary.Workers = len(response.Workers)
	response.Summary.Placements = len(response.Placements)
	return response
}

func activePlacementState(state workerprotocol.PlacementState) bool {
	switch state {
	case workerprotocol.PlacementStateCompleted, workerprotocol.PlacementStateFailed, workerprotocol.PlacementStateDestroyed:
		return false
	default:
		return true
	}
}

func recoveringPlacementState(state workerprotocol.PlacementState) bool {
	return state == workerprotocol.PlacementStateLost || state == workerprotocol.PlacementStateReclaiming || state == workerprotocol.PlacementStateRehydrating
}
