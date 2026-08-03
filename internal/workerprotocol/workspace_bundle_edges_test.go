package workerprotocol

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceBundleBuildEnforcesCancellationModesAndBounds(t *testing.T) {
	t.Parallel()

	for _, limits := range []WorkspaceBundleLimits{
		{}, {MaxFiles: -1, MaxFileBytes: 1, MaxBytes: 1},
		{MaxFiles: 1, MaxFileBytes: 0, MaxBytes: 1}, {MaxFiles: 1, MaxFileBytes: 2, MaxBytes: 1},
	} {
		if limits != (WorkspaceBundleLimits{}) {
			if err := limits.Validate(); !errors.Is(err, ErrBundleLimit) {
				t.Errorf("invalid limits %+v error=%v", limits, err)
			}
		}
	}
	if err := (WorkspaceBundleLimits{MaxFiles: 1, MaxFileBytes: 1, MaxBytes: 1}).Validate(); err != nil {
		t.Fatalf("valid limits: %v", err)
	}

	root := t.TempDir()
	writeWorkspaceTestFile(t, root, "a.txt", []byte("aa"), 0o600)
	writeWorkspaceTestFile(t, root, "b.txt", []byte("bb"), 0o600)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := BuildWorkspaceBundle(canceled, WorkspaceBundleOptions{RootDir: root, Mode: SyncModeDirectory}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled build error=%v", err)
	}
	if _, err := BuildWorkspaceBundle(context.Background(), WorkspaceBundleOptions{RootDir: "", Mode: SyncModeDirectory}); !errors.Is(err, ErrUnsafeWorkspace) {
		t.Fatalf("blank root error=%v", err)
	}
	if _, err := BuildWorkspaceBundle(context.Background(), WorkspaceBundleOptions{RootDir: root, Mode: "copy"}); !errors.Is(err, ErrUnsafeWorkspace) {
		t.Fatalf("unsupported mode error=%v", err)
	}
	if _, err := BuildWorkspaceBundle(context.Background(), WorkspaceBundleOptions{
		RootDir: root, Mode: SyncModeDirectory, Limits: WorkspaceBundleLimits{MaxFiles: 1, MaxFileBytes: 4, MaxBytes: 4},
	}); !errors.Is(err, ErrBundleLimit) {
		t.Fatalf("file count limit error=%v", err)
	}
	if _, err := BuildWorkspaceBundle(context.Background(), WorkspaceBundleOptions{
		RootDir: root, Mode: SyncModeDirectory, Limits: WorkspaceBundleLimits{MaxFiles: 2, MaxFileBytes: 1, MaxBytes: 2},
	}); !errors.Is(err, ErrBundleLimit) {
		t.Fatalf("file byte limit error=%v", err)
	}
	if _, err := BuildWorkspaceBundle(context.Background(), WorkspaceBundleOptions{
		RootDir: root, Mode: SyncModeDirectory, Limits: WorkspaceBundleLimits{MaxFiles: 2, MaxFileBytes: 2, MaxBytes: 3},
	}); !errors.Is(err, ErrBundleLimit) {
		t.Fatalf("total byte limit error=%v", err)
	}
}

func TestWorkspaceBundleVerificationRejectsEveryManifestMismatch(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeWorkspaceTestFile(t, root, "a.txt", []byte("alpha"), 0o600)
	writeWorkspaceTestFile(t, root, "b.txt", []byte("bravo"), 0o700)
	valid, err := BuildWorkspaceBundle(context.Background(), WorkspaceBundleOptions{RootDir: root, Mode: SyncModeDirectory})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*WorkspaceBundle)
		limits WorkspaceBundleLimits
	}{
		{name: "schema", mutate: func(bundle *WorkspaceBundle) { bundle.Manifest.SchemaVersion = 2 }},
		{name: "mode", mutate: func(bundle *WorkspaceBundle) { bundle.Manifest.Mode = "copy" }},
		{name: "source owner", mutate: func(bundle *WorkspaceBundle) { bundle.Manifest.SourceOwner = OwnerWorker }},
		{name: "workspace owner", mutate: func(bundle *WorkspaceBundle) { bundle.Manifest.WorkspaceOwner = OwnerGateway }},
		{name: "artifact owner", mutate: func(bundle *WorkspaceBundle) { bundle.Manifest.ArtifactOwner = OwnerWorker }},
		{name: "file count", mutate: func(bundle *WorkspaceBundle) { bundle.Manifest.FileCount++ }},
		{name: "entry count", mutate: func(bundle *WorkspaceBundle) { bundle.Manifest.Entries = bundle.Manifest.Entries[:1] }},
		{name: "max files", limits: WorkspaceBundleLimits{MaxFiles: 1, MaxFileBytes: 10, MaxBytes: 20}},
		{name: "unsafe path", mutate: func(bundle *WorkspaceBundle) { bundle.Files[0].Path = "../escape" }},
		{name: "sensitive path", mutate: func(bundle *WorkspaceBundle) { bundle.Files[0].Path = ".env" }},
		{name: "excluded directory", mutate: func(bundle *WorkspaceBundle) { bundle.Files[0].Path = "node_modules/a.txt" }},
		{name: "unsorted", mutate: func(bundle *WorkspaceBundle) { bundle.Files[0], bundle.Files[1] = bundle.Files[1], bundle.Files[0] }},
		{name: "file bytes", limits: WorkspaceBundleLimits{MaxFiles: 2, MaxFileBytes: 4, MaxBytes: 20}},
		{name: "entry path", mutate: func(bundle *WorkspaceBundle) { bundle.Manifest.Entries[0].Path = "other.txt" }},
		{name: "entry digest", mutate: func(bundle *WorkspaceBundle) { bundle.Manifest.Entries[0].Digest = "sha256:bad" }},
		{name: "file digest", mutate: func(bundle *WorkspaceBundle) { bundle.Files[0].Digest = "sha256:bad" }},
		{name: "entry size", mutate: func(bundle *WorkspaceBundle) { bundle.Manifest.Entries[0].SizeBytes++ }},
		{name: "entry mode", mutate: func(bundle *WorkspaceBundle) { bundle.Manifest.Entries[0].Mode ^= 1 }},
		{name: "unsafe file mode", mutate: func(bundle *WorkspaceBundle) { bundle.Files[0].Mode = 0o1000; bundle.Manifest.Entries[0].Mode = 0o1000 }},
		{name: "total bytes", mutate: func(bundle *WorkspaceBundle) { bundle.Manifest.TotalBytes++ }},
		{name: "manifest digest", mutate: func(bundle *WorkspaceBundle) { bundle.Manifest.Digest = "sha256:bad" }},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			bundle := cloneWorkspaceBundle(valid)
			if tc.mutate != nil {
				tc.mutate(&bundle)
			}
			if err := VerifyWorkspaceBundle(bundle, tc.limits); err == nil {
				t.Fatalf("invalid bundle was accepted: %+v", bundle.Manifest)
			}
		})
	}
}

func TestWorkspaceBundleApplyRequiresEmptyPrivateDestination(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeWorkspaceTestFile(t, root, "run.sh", []byte("#!/bin/sh\n"), 0o755)
	bundle, err := BuildWorkspaceBundle(context.Background(), WorkspaceBundleOptions{RootDir: root, Mode: SyncModeDirectory})
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyWorkspaceBundle(context.Background(), "", bundle, WorkspaceBundleLimits{}); !errors.Is(err, ErrUnsafeWorkspace) {
		t.Fatalf("blank destination error=%v", err)
	}
	nonempty := t.TempDir()
	writeWorkspaceTestFile(t, nonempty, "existing", []byte("x"), 0o600)
	if err := ApplyWorkspaceBundle(context.Background(), nonempty, bundle, WorkspaceBundleLimits{}); !errors.Is(err, ErrUnsafeWorkspace) {
		t.Fatalf("nonempty destination error=%v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := ApplyWorkspaceBundle(canceled, t.TempDir(), bundle, WorkspaceBundleLimits{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled apply error=%v", err)
	}
	destination := filepath.Join(t.TempDir(), "new", "workspace")
	if err := ApplyWorkspaceBundle(context.Background(), destination, bundle, WorkspaceBundleLimits{}); err != nil {
		t.Fatalf("apply bundle: %v", err)
	}
	info, err := os.Stat(destination)
	if err != nil || info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("destination permissions=%v err=%v", info, err)
	}
}

func TestWorkspacePathAndGitHelpersFailClosed(t *testing.T) {
	t.Parallel()

	validPaths := []string{"a", "dir/file.txt", ".env.example"}
	for _, path := range validPaths {
		if !safeWorkspaceRelativePath(path) {
			t.Errorf("valid workspace path %q rejected", path)
		}
	}
	invalidPaths := []string{"", ".", "..", "../escape", "dir/../file", "/absolute", `dir\file`, "bad\x00path"}
	for _, path := range invalidPaths {
		if valid := safeWorkspaceRelativePath(path); valid {
			t.Errorf("unsafe workspace path %q accepted", path)
		}
	}
	for _, path := range []string{".DS_Store", ".tars-result.json", ".env", ".env.production", "id_rsa", "id_ed25519", "credentials", "credentials.json", ".netrc", ".npmrc", ".pypirc", "client.pem", "private.key", "client.p12", "client.pfx", "my-credential.txt", "private-key.txt"} {
		if !sensitiveWorkspacePath(path) {
			t.Errorf("sensitive workspace path %q accepted", path)
		}
	}
	if sensitiveWorkspacePath(".env.example") || sensitiveWorkspacePath("README.md") {
		t.Fatal("safe workspace path marked sensitive")
	}
	for _, path := range []string{".git/config", "src/node_modules/a.js", ".tars/state.json"} {
		if !excludedWorkspaceDirectoryInPath(path) {
			t.Errorf("excluded workspace path %q accepted", path)
		}
	}
	values := []string{"a", "a", "b", "b"}
	if got := compactSortedStrings(values); strings.Join(got, ",") != "a,b" {
		t.Fatalf("compacted strings=%v", got)
	}
	if got := compactSortedStrings(nil); got == nil || len(got) != 0 {
		t.Fatalf("empty compact result=%v", got)
	}
	if string(bytesTrimSpace([]byte(" \n value \t"))) != "value" {
		t.Fatal("bytesTrimSpace did not trim")
	}

	root := t.TempDir()
	if _, _, _, _, err := gitWorkspacePaths(context.Background(), root, "/missing/git"); err == nil {
		t.Fatal("missing git executable was accepted")
	}
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is unavailable")
	}
	if _, _, _, _, err := gitWorkspacePaths(context.Background(), root, gitPath); err == nil {
		t.Fatal("non-Git workspace was accepted in git mode")
	}
	repo := t.TempDir()
	runGitTestCommand(t, gitPath, repo, "init")
	runGitTestCommand(t, gitPath, repo, "config", "user.email", "test@example.com")
	runGitTestCommand(t, gitPath, repo, "config", "user.name", "TARS Test")
	writeWorkspaceTestFile(t, repo, "tracked.txt", []byte("tracked"), 0o600)
	runGitTestCommand(t, gitPath, repo, "add", "tracked.txt")
	runGitTestCommand(t, gitPath, repo, "commit", "-m", "base")
	subdir := filepath.Join(repo, "subdir")
	if err := os.MkdirAll(subdir, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err := gitWorkspacePaths(context.Background(), subdir, gitPath); !errors.Is(err, ErrUnsafeWorkspace) {
		t.Fatalf("nested git root error=%v", err)
	}
}
