package remoteaccess

import (
	"context"
	"errors"
	"testing"
)

const relayTestStatus = `{"BackendState": "Running", "Self": {"HostName": "mac-mini", "DNSName": "mac-mini.tailnet.ts.net."}}`

func relayOpts(runner Runner) Options {
	return Options{Runner: runner, RelayTargetURL: DefaultRelayTargetURL}
}

func TestDetectReportsRelayPortBesideTARS(t *testing.T) {
	runner := &recordingRunner{outputs: map[string]string{
		"tailscale status --json": relayTestStatus,
		// The shape tailscale 1.104 printed with both ports on (2026-11-25).
		"tailscale serve status --json": `{
			"TCP": {"443": {"HTTPS": true}, "8443": {"HTTPS": true}},
			"Web": {
				"mac-mini.tailnet.ts.net:443": {"Handlers": {"/": {"Proxy": "http://127.0.0.1:43180"}}},
				"mac-mini.tailnet.ts.net:8443": {"Handlers": {"/": {"Proxy": "http://127.0.0.1:43181"}}}
			}
		}`,
	}}
	status, err := Detect(context.Background(), relayOpts(runner))
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if status.Relay == nil || !status.Relay.Active || !status.Relay.OwnedByTARS || status.Relay.Port != 8443 {
		t.Fatalf("relay = %+v", status.Relay)
	}
	if got := status.RelayURL(); got != "https://mac-mini.tailnet.ts.net:8443" {
		t.Fatalf("RelayURL = %q", got)
	}
	if got := status.URL(); got != "https://mac-mini.tailnet.ts.net" {
		t.Fatalf("URL = %q", got)
	}
}

func TestRelayURLNeedsBothPorts(t *testing.T) {
	base := Status{TailnetURL: "mac.ts.net", ServeActive: true, OwnedByTARS: true, ServePort: 443}
	cases := map[string]Status{
		"no relay":       base,
		"relay inactive": withRelay(base, RelayStatus{Port: 8443}),
		"relay foreign":  withRelay(base, RelayStatus{Active: true, Port: 8443}),
		"main off": withRelay(Status{TailnetURL: "mac.ts.net"},
			RelayStatus{Active: true, OwnedByTARS: true, Port: 8443}),
	}
	for name, status := range cases {
		if got := status.RelayURL(); got != "" {
			t.Errorf("%s: RelayURL = %q, want empty", name, got)
		}
	}
	on := withRelay(base, RelayStatus{Active: true, OwnedByTARS: true, Port: 443})
	if got := on.RelayURL(); got != "https://mac.ts.net" {
		t.Errorf("port 443 RelayURL = %q", got)
	}
}

func withRelay(s Status, r RelayStatus) Status {
	s.Relay = &r
	return s
}

func TestDetectWithoutRelayTargetLeavesRelayOut(t *testing.T) {
	runner := fakeRunner{outputs: map[string]string{
		"tailscale status --json":       relayTestStatus,
		"tailscale serve status --json": `{}`,
	}}
	status, err := Detect(context.Background(), Options{Runner: runner})
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if status.Relay != nil {
		t.Fatalf("Relay = %+v, want nil", status.Relay)
	}
}

func TestEnableRelayMapsThePort(t *testing.T) {
	runner := &recordingRunner{outputs: map[string]string{
		"tailscale status --json": relayTestStatus,
		"tailscale serve status --json": `{"Web": {
			"mac-mini.tailnet.ts.net:443": {"Handlers": {"/": {"Proxy": "http://127.0.0.1:43180"}}}
		}}`,
		"tailscale serve --https=8443 --bg http://127.0.0.1:43181": "",
	}}
	if err := EnableRelay(context.Background(), relayOpts(runner)); err != nil {
		t.Fatalf("EnableRelay: %v", err)
	}
	last := runner.commands[len(runner.commands)-1]
	if last != "tailscale serve --https=8443 --bg http://127.0.0.1:43181" {
		t.Fatalf("last command = %q", last)
	}
}

func TestEnableRelayLeavesAnotherTargetAlone(t *testing.T) {
	runner := &recordingRunner{outputs: map[string]string{
		"tailscale status --json": relayTestStatus,
		"tailscale serve status --json": `{"Web": {
			"mac-mini.tailnet.ts.net:8443": {"Handlers": {"/": {"Proxy": "http://127.0.0.1:9999"}}}
		}}`,
	}}
	if err := EnableRelay(context.Background(), relayOpts(runner)); !errors.Is(err, ErrRelayPortTaken) {
		t.Fatalf("EnableRelay = %v, want ErrRelayPortTaken", err)
	}
	if err := DisableRelay(context.Background(), relayOpts(runner)); err != nil {
		t.Fatalf("DisableRelay: %v", err)
	}
	for _, c := range runner.commands {
		if c == "tailscale serve --https=8443 off" || c == "tailscale serve --https=8443 --bg http://127.0.0.1:43181" {
			t.Fatalf("touched another target's port: %v", runner.commands)
		}
	}
}

func TestEnableRelayIsIdempotentAndDisableRemoves(t *testing.T) {
	runner := &recordingRunner{outputs: map[string]string{
		"tailscale status --json": relayTestStatus,
		"tailscale serve status --json": `{"Web": {
			"mac-mini.tailnet.ts.net:8443": {"Handlers": {"/": {"Proxy": "http://127.0.0.1:43181"}}}
		}}`,
		"tailscale serve --https=8443 off": "",
	}}
	if err := EnableRelay(context.Background(), relayOpts(runner)); err != nil {
		t.Fatalf("EnableRelay: %v", err)
	}
	if err := DisableRelay(context.Background(), relayOpts(runner)); err != nil {
		t.Fatalf("DisableRelay: %v", err)
	}
	if last := runner.commands[len(runner.commands)-1]; last != "tailscale serve --https=8443 off" {
		t.Fatalf("last command = %q", last)
	}
}

func TestRelaySamePortAsTARSIsRefused(t *testing.T) {
	opts := Options{Runner: fakeRunner{}, HTTPSPort: 8443, RelayTargetURL: DefaultRelayTargetURL}
	if err := EnableRelay(context.Background(), opts); !errors.Is(err, ErrRelayPortTaken) {
		t.Fatalf("EnableRelay = %v", err)
	}
	if err := DisableRelay(context.Background(), opts); err != nil {
		t.Fatalf("DisableRelay = %v", err)
	}
	if err := EnableRelay(context.Background(), Options{Runner: fakeRunner{}}); err != nil {
		t.Fatalf("no relay target: %v", err)
	}
}

func TestEnableRelayNeedsTailscale(t *testing.T) {
	runner := fakeRunner{outputs: map[string]string{
		"tailscale status --json": `{"BackendState": "NeedsLogin"}`,
	}}
	if err := EnableRelay(context.Background(), relayOpts(runner)); err == nil {
		t.Fatal("EnableRelay succeeded while logged out")
	}
}
