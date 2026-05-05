package main

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestRunConsoleCommand_ProbesBeforeOpeningBrowser(t *testing.T) {
	origHealth := consoleHealthChecker
	origOpener := consoleURLOpener
	defer func() {
		consoleHealthChecker = origHealth
		consoleURLOpener = origOpener
	}()

	healthCalled := false
	openerCalled := false
	consoleHealthChecker = func(context.Context, string) error {
		healthCalled = true
		return nil
	}
	consoleURLOpener = func(context.Context, string) error {
		openerCalled = true
		return nil
	}

	var stdout strings.Builder
	err := runConsoleCommand(context.Background(), &stdout, io.Discard, defaultClientOptions())
	if err != nil {
		t.Fatalf("runConsoleCommand: %v", err)
	}
	if !healthCalled {
		t.Fatal("expected health probe to run before opening browser")
	}
	if !openerCalled {
		t.Fatal("expected browser opener to run when server is healthy")
	}
	if !strings.Contains(stdout.String(), "Open the console:") {
		t.Fatalf("expected console URL hint, got:\n%s", stdout.String())
	}
}

func TestRunConsoleCommand_PrintsOnboardingHintWhenServerDown(t *testing.T) {
	// Regression: `tars` (no args) used to fire the browser at the
	// default URL even when no server was listening. The result was
	// a confusing connection-refused page with no path to recovery.
	// Now the no-args invocation probes /v1/healthz first; if the
	// server is down, print the onboarding hint and return an error
	// without launching the browser.
	origHealth := consoleHealthChecker
	origOpener := consoleURLOpener
	defer func() {
		consoleHealthChecker = origHealth
		consoleURLOpener = origOpener
	}()

	openerCalled := false
	consoleHealthChecker = func(context.Context, string) error {
		return errors.New("connection refused")
	}
	consoleURLOpener = func(context.Context, string) error {
		openerCalled = true
		return nil
	}

	var stderr strings.Builder
	err := runConsoleCommand(context.Background(), io.Discard, &stderr, defaultClientOptions())
	if err == nil {
		t.Fatal("expected error when server is unreachable")
	}
	if openerCalled {
		t.Fatal("must not open browser when server is unreachable")
	}
	out := stderr.String()
	for _, want := range []string{
		"TARS server not reachable",
		"tars init",
		"tars service start",
		"tars serve",
		"TARS_SERVER_URL",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected hint to contain %q, got:\n%s", want, out)
		}
	}
}
