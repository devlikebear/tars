package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
)

func TestRootCommand_IncludesRemoteSubcommand(t *testing.T) {
	cmd := newRootCommand(strings.NewReader(""), io.Discard, io.Discard)
	if subcmd, _, err := cmd.Find([]string{"remote"}); err != nil || subcmd == nil || subcmd.Name() != "remote" {
		t.Fatalf("expected remote subcommand, got subcmd=%v err=%v", subcmd, err)
	}
}

func TestRemoteStatusSubcommandInvokesRunner(t *testing.T) {
	original := remoteStatusRunner
	defer func() { remoteStatusRunner = original }()
	var got remoteStatusOptions
	remoteStatusRunner = func(_ context.Context, opts remoteStatusOptions, stdout, _ io.Writer) error {
		got = opts
		_, err := fmt.Fprintln(stdout, "remote ok")
		return err
	}

	var stdout strings.Builder
	cmd := newRootCommand(strings.NewReader(""), &stdout, io.Discard)
	cmd.SetArgs([]string{"remote", "status", "--json", "--port", "9443"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("remote status: %v", err)
	}
	if !got.json || got.httpsPort != 9443 {
		t.Fatalf("unexpected remote status options: %+v", got)
	}
	if strings.TrimSpace(stdout.String()) != "remote ok" {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
}

func TestRemoteEnableDisableURLSubcommandsInvokeRunners(t *testing.T) {
	origEnable := remoteEnableRunner
	origDisable := remoteDisableRunner
	origURL := remoteURLRunner
	defer func() {
		remoteEnableRunner = origEnable
		remoteDisableRunner = origDisable
		remoteURLRunner = origURL
	}()

	var enabled remoteAccessOptions
	remoteEnableRunner = func(_ context.Context, opts remoteAccessOptions, stdout, _ io.Writer) error {
		enabled = opts
		_, err := fmt.Fprintln(stdout, "enabled")
		return err
	}
	var disabled remoteAccessOptions
	remoteDisableRunner = func(_ context.Context, opts remoteAccessOptions, stdout, _ io.Writer) error {
		disabled = opts
		_, err := fmt.Fprintln(stdout, "disabled")
		return err
	}
	var urlOpts remoteAccessOptions
	remoteURLRunner = func(_ context.Context, opts remoteAccessOptions, stdout, _ io.Writer) error {
		urlOpts = opts
		_, err := fmt.Fprintln(stdout, "https://mac.tail.ts.net:9443")
		return err
	}

	var stdout strings.Builder
	cmd := newRootCommand(strings.NewReader(""), &stdout, io.Discard)
	cmd.SetArgs([]string{"remote", "enable", "--port", "9443"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("remote enable: %v", err)
	}
	if enabled.httpsPort != 9443 || !strings.Contains(stdout.String(), "enabled") {
		t.Fatalf("unexpected enable result opts=%+v stdout=%q", enabled, stdout.String())
	}

	stdout.Reset()
	cmd = newRootCommand(strings.NewReader(""), &stdout, io.Discard)
	cmd.SetArgs([]string{"remote", "disable", "--port", "9443"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("remote disable: %v", err)
	}
	if disabled.httpsPort != 9443 || !strings.Contains(stdout.String(), "disabled") {
		t.Fatalf("unexpected disable result opts=%+v stdout=%q", disabled, stdout.String())
	}

	stdout.Reset()
	cmd = newRootCommand(strings.NewReader(""), &stdout, io.Discard)
	cmd.SetArgs([]string{"remote", "url", "--port", "9443"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("remote url: %v", err)
	}
	if urlOpts.httpsPort != 9443 || strings.TrimSpace(stdout.String()) != "https://mac.tail.ts.net:9443" {
		t.Fatalf("unexpected url result opts=%+v stdout=%q", urlOpts, stdout.String())
	}
}
