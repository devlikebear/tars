package workerprotocol

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestControllerRejectsSemanticallyCorruptPersistedPlacement(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "controller.json")
	controller, err := OpenController(ControllerOptions{StatePath: path})
	if err != nil {
		t.Fatal(err)
	}
	registerReadyWorker(t, controller, "worker-a", 0)
	if _, err := controller.CreatePlacement(context.Background(), CreatePlacementInput{
		ID: "placement-a", WorkspaceID: "workspace-a", WorkID: "work-a", StepID: "step-a", AttemptID: "attempt-a",
		WorkerID: "worker-a", Policy: DefaultExecutionPolicy(),
		Sync: SyncSpec{Mode: SyncModeDirectory, SourceOwner: OwnerGateway, WorkspaceOwner: OwnerWorker, ArtifactOwner: OwnerGateway},
	}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var state controllerState
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatal(err)
	}
	placement := state.Placements["placement-a"]
	placement.WorkerID = "worker-does-not-exist"
	state.Placements["placement-a"] = placement
	raw, err = json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenController(ControllerOptions{StatePath: path}); !errors.Is(err, ErrWireContract) {
		t.Fatalf("corrupt controller error=%v want ErrWireContract", err)
	}
}
