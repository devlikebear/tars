package main

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"testing"
)

func TestWorkerServeCommandRequiresExplicitStdioProtocol(t *testing.T) {
	original := workerServeRunner
	defer func() { workerServeRunner = original }()

	var got workerServeOptions
	called := false
	workerServeRunner = func(_ context.Context, opts workerServeOptions, _ io.Reader, _ io.Writer) error {
		called = true
		got = opts
		return nil
	}
	configPath := filepath.Join(t.TempDir(), "worker.json")
	command := newRootCommand(bytes.NewBufferString("{}\n"), &bytes.Buffer{}, &bytes.Buffer{})
	command.SetArgs([]string{"worker", "serve", "--stdio", "--protocol", "1.0", "--config", configPath})
	if err := command.Execute(); err != nil {
		t.Fatalf("worker serve command: %v", err)
	}
	if !called || !got.stdio || got.protocol != "1.0" || got.configPath != configPath {
		t.Fatalf("worker serve options=%+v called=%v", got, called)
	}

	called = false
	command = newRootCommand(bytes.NewBufferString("{}\n"), &bytes.Buffer{}, &bytes.Buffer{})
	command.SetArgs([]string{"worker", "serve", "--protocol", "1.0", "--config", configPath})
	if err := command.Execute(); err == nil || called {
		t.Fatalf("worker serve without --stdio error=%v called=%v", err, called)
	}
}

func TestWorkerServeSkipsWorkspaceDotEnvBootstrap(t *testing.T) {
	t.Parallel()

	if shouldBootstrapEnv([]string{"worker", "serve", "--stdio"}) {
		t.Fatal("worker stdio process would load workspace .env credentials")
	}
	if !shouldBootstrapEnv([]string{"serve"}) || !shouldBootstrapEnv([]string{"worker", "status"}) {
		t.Fatal("normal commands unexpectedly skipped environment bootstrap")
	}
}
