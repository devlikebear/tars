//go:build !windows

// The stand-ins are #!/bin/sh scripts and the process check is a signal.

package tarsserver

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/easyproject"
	"github.com/devlikebear/tars/internal/serverauth"
	"github.com/rs/zerolog"
)

type easyPreviewResp struct {
	Preview easyproject.Preview `json:"preview"`
}

// previewScript points previews at a stand-in program with body.
func previewScript(t *testing.T, body string) {
	t.Helper()
	script := filepath.Join(t.TempDir(), "preview.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(easyproject.PreviewPathEnv, script)
}

func putEasyProject(t *testing.T, workspace string, p easyproject.Project) {
	t.Helper()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now()
	}
	if err := easyproject.NewStore(workspace).Put(p); err != nil {
		t.Fatal(err)
	}
}

func previewOf(t *testing.T, h http.Handler, method, id string, role string) (int, easyproject.Preview) {
	t.Helper()
	rec := focusRequestAs(t, h, method, "/v1/easy/projects/"+id+"/preview", "", role)
	var out easyPreviewResp
	if rec.Code == http.StatusOK {
		decodeInto(t, rec, &out)
	}
	return rec.Code, out.Preview
}

func TestEasyPreviewRoutes(t *testing.T) {
	previewScript(t, "exit 3")
	f := newWorktreeFixture(t)
	d, _ := createFocusDriver(t, f.store, focusPlanReply)
	h := newFocusPipelineHandler(f.store, f.c, d, zerolog.Nop())
	ready := easyproject.Project{ID: "0123456789abcdef", Name: "x", Folder: t.TempDir(), Status: easyproject.StatusReady}
	putEasyProject(t, f.store.WorkspaceDir(), ready)
	putEasyProject(t, f.store.WorkspaceDir(), easyproject.Project{ID: "fedcba9876543210", Name: "y", Status: easyproject.StatusFailed})

	if code, p := previewOf(t, h, http.MethodGet, ready.ID, serverauth.RoleUser); code != http.StatusOK || p.State != easyproject.PreviewStopped {
		t.Fatalf("before start: %d %+v", code, p)
	}
	if code, _ := previewOf(t, h, http.MethodGet, "0000000000000000", serverauth.RoleUser); code != http.StatusNotFound {
		t.Fatalf("unknown project: %d", code)
	}
	if code, _ := previewOf(t, h, http.MethodPost, "fedcba9876543210", serverauth.RoleUser); code != http.StatusConflict {
		t.Fatalf("project not ready: %d", code)
	}
	// A phone reads the preview (unit 7) but cannot stop it.
	if code, p := previewOf(t, h, http.MethodGet, ready.ID, serverauth.RoleFocus); code != http.StatusOK || p.State != easyproject.PreviewStopped {
		t.Fatalf("phone status: %d %+v", code, p)
	}
	if code, _ := previewOf(t, h, http.MethodDelete, ready.ID, serverauth.RoleFocus); code != http.StatusForbidden {
		t.Fatalf("phone stop: %d", code)
	}

	// The stand-in exits at once: the preview fails with a code.
	if code, p := previewOf(t, h, http.MethodPost, ready.ID, serverauth.RoleUser); code != http.StatusOK || p.State != easyproject.PreviewStarting {
		t.Fatalf("start: %d %+v", code, p)
	}
	waitFor(t, "the preview to fail", func() bool {
		_, p := previewOf(t, h, http.MethodGet, ready.ID, serverauth.RoleUser)
		return p.State == easyproject.PreviewFailed && p.ErrorCode == easyproject.ErrorPreviewFailed
	})
	if code, p := previewOf(t, h, http.MethodDelete, ready.ID, serverauth.RoleUser); code != http.StatusOK || p.State != easyproject.PreviewStopped {
		t.Fatalf("stop: %d %+v", code, p)
	}
}

// Closing the driver (server shutdown) stops a preview's process.
func TestEasyPreviewStopsWithTheServer(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "pid")
	previewScript(t, "echo $$ > '"+pidFile+"'\nexec sleep 60")
	f := newWorktreeFixture(t)
	d, _ := createFocusDriver(t, f.store, focusPlanReply)
	h := newFocusPipelineHandler(f.store, f.c, d, zerolog.Nop())
	p := easyproject.Project{ID: "0123456789abcdef", Name: "x", Folder: t.TempDir(), Status: easyproject.StatusReady}
	putEasyProject(t, f.store.WorkspaceDir(), p)

	previewOf(t, h, http.MethodPost, p.ID, serverauth.RoleUser)
	var pid int
	waitFor(t, "the preview to start", func() bool {
		raw, err := os.ReadFile(pidFile)
		if err != nil {
			return false
		}
		pid, err = strconv.Atoi(strings.TrimSpace(string(raw)))
		return err == nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	d.Close(ctx)
	waitFor(t, "the preview process to end", func() bool {
		return syscall.Kill(pid, 0) != nil
	})
}
