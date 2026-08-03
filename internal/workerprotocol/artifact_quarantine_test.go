package workerprotocol

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArtifactQuarantineReleasesOnlyVerifiedCleanArtifacts(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	quarantine, err := NewArtifactQuarantine(ArtifactQuarantineOptions{RootDir: root})
	if err != nil {
		t.Fatalf("new artifact quarantine: %v", err)
	}
	clean := []byte("verified result\n")
	report, err := quarantine.InspectAndRelease(context.Background(), "placement-a", []WireArtifact{
		{Name: "reports/result.txt", Digest: digestBytes(clean), MediaType: "text/plain", Data: clean},
	}, nil)
	if err != nil {
		t.Fatalf("inspect clean artifact: %v", err)
	}
	if len(report.Accepted) != 1 || len(report.Rejected) != 0 || report.Accepted[0].Digest != digestBytes(clean) {
		t.Fatalf("clean artifact report=%+v", report)
	}
	path, err := artifactFilePath(report.Accepted[0].URI)
	if err != nil {
		t.Fatalf("parse released artifact URI: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(raw, clean) {
		t.Fatalf("released artifact=%q error=%v", raw, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("released artifact permissions info=%v error=%v", info, err)
	}
	if !strings.Contains(path, filepath.Join("accepted", "placement-a")) {
		t.Fatalf("artifact was not released from quarantine: %s", path)
	}
}

func TestArtifactQuarantineRejectsSecretsTraversalAndDigestMismatchWithoutPersistingRawData(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	quarantine, err := NewArtifactQuarantine(ArtifactQuarantineOptions{RootDir: root})
	if err != nil {
		t.Fatalf("new artifact quarantine: %v", err)
	}
	secretAssignment := []byte("API_TOKEN=must-never-persist\n")
	untypedSecret := []byte("PASSWORD=untyped-secret\n")
	forbiddenValue := []byte("prefix runtime-credential-value suffix")
	tampered := []byte("tampered")
	report, err := quarantine.InspectAndRelease(context.Background(), "placement-a", []WireArtifact{
		{Name: "logs/output.txt", Digest: digestBytes(secretAssignment), MediaType: "text/plain", Data: secretAssignment},
		{Name: "logs/untyped.log", Digest: digestBytes(untypedSecret), Data: untypedSecret},
		{Name: "logs/runtime.txt", Digest: digestBytes(forbiddenValue), MediaType: "text/plain", Data: forbiddenValue},
		{Name: "../escape.txt", Digest: digestBytes([]byte("escape")), Data: []byte("escape")},
		{Name: "reports/tampered.txt", Digest: digestBytes([]byte("expected")), Data: tampered},
	}, []string{"runtime-credential-value"})
	if err != nil {
		t.Fatalf("inspect hostile artifacts: %v", err)
	}
	if len(report.Accepted) != 0 || len(report.Rejected) != 5 {
		t.Fatalf("hostile artifact report=%+v", report)
	}
	walkErr := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if bytes.Contains(raw, []byte("must-never-persist")) || bytes.Contains(raw, []byte("untyped-secret")) || bytes.Contains(raw, []byte("runtime-credential-value")) {
			t.Fatalf("rejected secret persisted at %s", path)
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("inspect quarantine root: %v", walkErr)
	}
}
