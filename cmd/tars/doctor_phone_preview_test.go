package main

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/remoteaccess"
)

func TestDoctorPhonePreview(t *testing.T) {
	serving := remoteaccess.Status{
		Installed: true, LoggedIn: true, TailnetURL: "mac.ts.net",
		ServeActive: true, OwnedByTARS: true, ServePort: 443,
		Relay: &remoteaccess.RelayStatus{Active: true, OwnedByTARS: true, Port: 8443},
	}
	withRelay := func(r remoteaccess.RelayStatus) remoteaccess.Status {
		s := serving
		s.Relay = &r
		return s
	}
	cases := []struct {
		name      string
		enabled   bool
		status    remoteaccess.Status
		err       error
		listening bool
		want      string
		level     string
	}{
		{"off", false, serving, nil, true, "off (remote access is off)", "ok"},
		{"read error", true, serving, errors.New("boom"), true, "could not be read", "warn"},
		{"signed out", true, remoteaccess.Status{Installed: true}, nil, true, "not signed in", "warn"},
		{"serve not tars", true, remoteaccess.Status{Installed: true, LoggedIn: true, TailnetURL: "mac.ts.net"}, nil, true, "does not point at TARS", "warn"},
		{"renamed", true, remoteaccess.Status{Installed: true, LoggedIn: true, TailnetURL: "mac.ts.net", StaleHost: "old-mac.ts.net"}, nil, true, "old name old-mac.ts.net", "warn"},
		{"port taken", true, withRelay(remoteaccess.RelayStatus{Active: true, Port: 8443}), nil, true, "used by another target", "warn"},
		{"not mapped", true, withRelay(remoteaccess.RelayStatus{Port: 8443}), nil, true, "not mapped", "warn"},
		{"no listener", true, serving, nil, false, "nothing listens", "warn"},
		{"ok", true, serving, nil, true, "relay at https://mac.ts.net:8443", "ok"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var report doctorReport
			var gotOpts remoteaccess.Options
			probe := doctorPhonePreviewProbe{
				detect: func(_ context.Context, opts remoteaccess.Options) (remoteaccess.Status, error) {
					gotOpts = opts
					return tc.status, tc.err
				},
				listening: func(string) bool { return tc.listening },
			}
			cfg := config.Config{}
			cfg.RemoteAccessTailscaleServeEnabled = tc.enabled
			checkDoctorPhonePreviewWith(&report, cfg, probe)
			if report.lastStatusFor("phone preview") != tc.level {
				t.Fatalf("level = %q, want %q", report.lastStatusFor("phone preview"), tc.level)
			}
			if out := renderedDoctor(report); !strings.Contains(out, tc.want) {
				t.Fatalf("report %q lacks %q", out, tc.want)
			}
			if tc.enabled && (gotOpts.RelayTargetURL != remoteaccess.DefaultRelayTargetURL || gotOpts.HTTPSPort != 443) {
				t.Fatalf("detect options = %+v", gotOpts)
			}
		})
	}
}

func TestDoctorPhonePreviewDefaultProbeDials(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	probe := defaultDoctorPhonePreviewProbe()
	if !probe.listening(addr) {
		t.Fatal("listening address read as closed")
	}
	_ = ln.Close()
	if probe.listening(addr) {
		t.Fatal("closed address read as listening")
	}
}

func renderedDoctor(report doctorReport) string {
	var b strings.Builder
	renderDoctorReport(&b, report)
	return b.String()
}
