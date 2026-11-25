package main

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/previewrelay"
	"github.com/devlikebear/tars/internal/remoteaccess"
)

// doctorPhonePreviewProbe reads the two facts the phone preview relay
// stands on (docs/decisions/vibe-phone-preview.md): whether Tailscale serves
// the relay port beside TARS's own, and whether a server listens behind it.
type doctorPhonePreviewProbe struct {
	detect    func(ctx context.Context, opts remoteaccess.Options) (remoteaccess.Status, error)
	listening func(addr string) bool
}

func defaultDoctorPhonePreviewProbe() doctorPhonePreviewProbe {
	return doctorPhonePreviewProbe{
		detect: remoteaccess.Detect,
		listening: func(addr string) bool {
			conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
			if err != nil {
				return false
			}
			_ = conn.Close()
			return true
		},
	}
}

// checkDoctorPhonePreview reports in one line whether a paired phone can
// open a vibe project's app through the preview relay.
func checkDoctorPhonePreview(report *doctorReport, cfg config.Config) {
	checkDoctorPhonePreviewWith(report, cfg, defaultDoctorPhonePreviewProbe())
}

func checkDoctorPhonePreviewWith(report *doctorReport, cfg config.Config, probe doctorPhonePreviewProbe) {
	const name = "phone preview"
	if !cfg.RemoteAccessTailscaleServeEnabled {
		report.add("ok", name, "off (remote access is off)")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	port := cfg.RemoteAccessTailscaleServeHTTPSPort
	if port <= 0 {
		port = remoteaccess.DefaultHTTPSPort
	}
	status, err := probe.detect(ctx, remoteaccess.Options{HTTPSPort: port, RelayTargetURL: remoteaccess.DefaultRelayTargetURL})
	switch {
	case err != nil:
		report.add("warn", name, fmt.Sprintf("tailscale could not be read: %v", err))
	case !status.Installed || !status.LoggedIn:
		report.add("warn", name, "tailscale is not installed or not signed in")
	case status.URL() == "":
		report.add("warn", name, "remote access is on but Tailscale serve does not point at TARS")
	case status.RelayURL() == "" && status.Relay != nil && status.Relay.Active:
		report.add("warn", name, fmt.Sprintf("Tailscale serve port %d is used by another target; the phone cannot open vibe apps", status.Relay.Port))
	case status.RelayURL() == "":
		report.add("warn", name, fmt.Sprintf("Tailscale serve port %d is not mapped to the relay; restart tars serve", remoteaccess.DefaultRelayHTTPSPort))
	case !probe.listening(previewrelay.DefaultAddr):
		report.add("warn", name, fmt.Sprintf("%s is served but nothing listens on %s (is tars serve running?)", status.RelayURL(), previewrelay.DefaultAddr))
	default:
		report.add("ok", name, fmt.Sprintf("relay at %s → %s", status.RelayURL(), previewrelay.DefaultAddr))
	}
}
