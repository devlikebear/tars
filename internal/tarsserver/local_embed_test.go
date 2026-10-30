package tarsserver

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/rs/zerolog"
)

func TestNewLocalEmbedManager_OnlyForSemanticLocal(t *testing.T) {
	var base config.Config
	base.WorkspaceDir = t.TempDir()
	base.MemoryEmbedProvider = "local"
	base.MemoryEmbedModel = config.DefaultLocalEmbedModel
	base.MemoryEmbedBaseURL = config.DefaultLocalEmbedBaseURL

	if mgr, err := newLocalEmbedManager(base); mgr != nil || err != nil {
		t.Fatalf("expected no manager while semantic memory is off, got %v %v", mgr, err)
	}
	on := base
	on.MemorySemanticEnabled = true
	if mgr, err := newLocalEmbedManager(on); mgr == nil || err != nil {
		t.Fatalf("expected manager for semantic local, got %v %v", mgr, err)
	}
	gemini := on
	gemini.MemoryEmbedProvider = "gemini"
	if mgr, err := newLocalEmbedManager(gemini); mgr != nil || err != nil {
		t.Fatalf("expected no manager for gemini, got %v %v", mgr, err)
	}
	remote := on
	remote.MemoryEmbedBaseURL = "http://10.1.2.3:43181/v1"
	if _, err := newLocalEmbedManager(remote); err == nil {
		t.Fatal("expected a non-loopback local base_url to be rejected")
	}
}

func TestStartLocalEmbed_WarnsWhenNotInstalledAndStopIsSafe(t *testing.T) {
	var cfg config.Config
	cfg.WorkspaceDir = t.TempDir()
	cfg.MemorySemanticEnabled = true
	cfg.MemoryEmbedProvider = "local"
	cfg.MemoryEmbedModel = config.DefaultLocalEmbedModel
	cfg.MemoryEmbedBaseURL = "http://127.0.0.1:1/v1"
	runtime := &serveAPIRuntime{cfg: cfg}
	var logs syncBuffer
	startLocalEmbed(context.Background(), runtime, zerolog.New(&logs))
	if runtime.localEmbed == nil {
		t.Fatal("expected manager on the runtime")
	}
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(logs.String(), "keywords only") {
		if time.Now().After(deadline) {
			t.Fatalf("expected not-installed warning, got %q", logs.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
	runtime.localEmbed.Stop()

	bad := &serveAPIRuntime{cfg: cfg}
	bad.cfg.MemoryEmbedBaseURL = "http://10.1.2.3:43181/v1"
	logs.Reset()
	startLocalEmbed(context.Background(), bad, zerolog.New(&logs))
	if bad.localEmbed != nil || !strings.Contains(logs.String(), "misconfigured") {
		t.Fatalf("expected misconfiguration warning, got %q", logs.String())
	}
	startLocalEmbed(context.Background(), &serveAPIRuntime{}, zerolog.New(&bytes.Buffer{}))
}

// syncBuffer is a bytes.Buffer safe to write from the sidecar goroutine
// while the test reads it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (b *syncBuffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf.Reset()
}
