package remoteaccess

import (
	"context"
	"testing"
)

// renamedStatus is `tailscale status --json` after the machine was renamed
// in Tailscale from chang-macbookpro to mac (tailscale 1.104.7, 2026-11-28).
const renamedStatus = `{"BackendState": "Running", "Self": {"HostName": "mac", "DNSName": "mac.tailnet.ts.net."}}`

// renamedServe is the `tailscale serve status --json` the same machine
// printed: the 443 handler kept the old name; the relay was added after the
// rename, under the new one.
const renamedServe = `{
	"TCP": {"443": {"HTTPS": true}, "8443": {"HTTPS": true}},
	"Web": {
		"chang-macbookpro.tailnet.ts.net:443": {"Handlers": {"/": {"Proxy": "http://127.0.0.1:43180"}}},
		"mac.tailnet.ts.net:8443": {"Handlers": {"/": {"Proxy": "http://127.0.0.1:43181"}}}
	}
}`

func TestDetectServeHandlerHost(t *testing.T) {
	cases := []struct {
		name        string
		status      string
		serve       string
		wantActive  bool
		wantOwned   bool
		wantStale   string
		wantURL     string
		wantRelay   RelayStatus
		wantRelayOK bool
	}{
		{
			name:      "handler left under the old name",
			status:    renamedStatus,
			serve:     renamedServe,
			wantStale: "chang-macbookpro.tailnet.ts.net",
			wantRelay: RelayStatus{Active: true, Port: 8443, OwnedByTARS: true},
		},
		{
			name:   "both handlers under the old name",
			status: renamedStatus,
			serve: `{"Web": {
				"chang-macbookpro.tailnet.ts.net:443": {"Handlers": {"/": {"Proxy": "http://127.0.0.1:43180"}}},
				"chang-macbookpro.tailnet.ts.net:8443": {"Handlers": {"/": {"Proxy": "http://127.0.0.1:43181"}}}
			}}`,
			wantStale: "chang-macbookpro.tailnet.ts.net",
			wantRelay: RelayStatus{Port: 8443, StaleHost: "chang-macbookpro.tailnet.ts.net"},
		},
		{
			name:   "re-added under the new name beside the old one",
			status: renamedStatus,
			serve: `{"Web": {
				"chang-macbookpro.tailnet.ts.net:443": {"Handlers": {"/": {"Proxy": "http://127.0.0.1:43180"}}},
				"mac.tailnet.ts.net:443": {"Handlers": {"/": {"Proxy": "http://127.0.0.1:43180"}}},
				"mac.tailnet.ts.net:8443": {"Handlers": {"/": {"Proxy": "http://127.0.0.1:43181"}}}
			}}`,
			wantActive:  true,
			wantOwned:   true,
			wantURL:     "https://mac.tailnet.ts.net",
			wantRelay:   RelayStatus{Active: true, Port: 8443, OwnedByTARS: true},
			wantRelayOK: true,
		},
		{
			name:   "host differs only in case and trailing dot",
			status: renamedStatus,
			serve: `{"Web": {
				"MAC.tailnet.ts.net.:443": {"Handlers": {"/": {"Proxy": "http://127.0.0.1:43180"}}}
			}}`,
			wantActive: true,
			wantOwned:  true,
			wantURL:    "https://mac.tailnet.ts.net",
			wantRelay:  RelayStatus{Port: 8443},
		},
		{
			name:   "another target under the old name is not TARS's and not stale",
			status: renamedStatus,
			serve: `{"Web": {
				"chang-macbookpro.tailnet.ts.net:443": {"Handlers": {"/": {"Proxy": "http://127.0.0.1:3000"}}}
			}}`,
			wantRelay: RelayStatus{Port: 8443},
		},
		{
			name:   "another target under the new name, TARS under the old",
			status: renamedStatus,
			serve: `{"Web": {
				"chang-macbookpro.tailnet.ts.net:443": {"Handlers": {"/": {"Proxy": "http://127.0.0.1:43180"}}},
				"mac.tailnet.ts.net:443": {"Handlers": {"/": {"Proxy": "http://127.0.0.1:3000"}}}
			}}`,
			wantActive: true,
			wantRelay:  RelayStatus{Port: 8443},
		},
		{
			name:   "no MagicDNS name known counts any host",
			status: `{"BackendState": "Running", "Self": {"HostName": "mac", "DNSName": ""}}`,
			serve: `{"Web": {
				"chang-macbookpro.tailnet.ts.net:443": {"Handlers": {"/": {"Proxy": "http://127.0.0.1:43180"}}}
			}}`,
			wantActive: true,
			wantOwned:  true,
			wantRelay:  RelayStatus{Port: 8443},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := fakeRunner{outputs: map[string]string{
				"tailscale status --json":       tc.status,
				"tailscale serve status --json": tc.serve,
			}}
			status, err := Detect(context.Background(), relayOpts(runner))
			if err != nil {
				t.Fatalf("Detect: %v", err)
			}
			if status.ServeActive != tc.wantActive || status.OwnedByTARS != tc.wantOwned || status.StaleHost != tc.wantStale {
				t.Fatalf("serve = active %v owned %v stale %q, want %v %v %q", status.ServeActive, status.OwnedByTARS, status.StaleHost, tc.wantActive, tc.wantOwned, tc.wantStale)
			}
			if got := status.URL(); got != tc.wantURL {
				t.Fatalf("URL = %q, want %q", got, tc.wantURL)
			}
			if status.Relay == nil || *status.Relay != tc.wantRelay {
				t.Fatalf("relay = %+v, want %+v", status.Relay, tc.wantRelay)
			}
			if got := status.RelayURL() != ""; got != tc.wantRelayOK {
				t.Fatalf("RelayURL = %q", status.RelayURL())
			}
		})
	}
}

// A handler under the old name must not make Enable think TARS is already
// served: it adds the handler under the current name, and never removes the
// old one.
func TestEnableReaddsHandlerUnderCurrentName(t *testing.T) {
	runner := &recordingRunner{outputs: map[string]string{
		"tailscale status --json":                                 renamedStatus,
		"tailscale serve status --json":                           renamedServe,
		"tailscale serve --https=443 --bg http://127.0.0.1:43180": ``,
	}}
	if err := Reconcile(context.Background(), Desired{Enabled: true}, Options{Runner: runner}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	want := []string{"tailscale status --json", "tailscale serve status --json", "tailscale serve --https=443 --bg http://127.0.0.1:43180"}
	if len(runner.commands) != len(want) {
		t.Fatalf("commands = %v, want %v", runner.commands, want)
	}
	for i := range want {
		if runner.commands[i] != want[i] {
			t.Fatalf("commands = %v, want %v", runner.commands, want)
		}
	}
}

func TestEnableRelayReaddsHandlerUnderCurrentName(t *testing.T) {
	runner := &recordingRunner{outputs: map[string]string{
		"tailscale status --json": renamedStatus,
		"tailscale serve status --json": `{"Web": {
			"chang-macbookpro.tailnet.ts.net:8443": {"Handlers": {"/": {"Proxy": "http://127.0.0.1:43181"}}}
		}}`,
		"tailscale serve --https=8443 --bg http://127.0.0.1:43181": ``,
	}}
	if err := EnableRelay(context.Background(), relayOpts(runner)); err != nil {
		t.Fatalf("EnableRelay: %v", err)
	}
	if got := runner.commands[len(runner.commands)-1]; got != "tailscale serve --https=8443 --bg http://127.0.0.1:43181" {
		t.Fatalf("last command = %q", got)
	}
}

// Disable acts only on the current name's handler; one left under the old
// name is not something serve's CLI can address, so it is left as is.
func TestDisableLeavesOldNameHandlerAlone(t *testing.T) {
	runner := &recordingRunner{outputs: map[string]string{
		"tailscale status --json":       renamedStatus,
		"tailscale serve status --json": renamedServe,
	}}
	if err := Disable(context.Background(), Options{Runner: runner}); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	if len(runner.commands) != 2 {
		t.Fatalf("Disable should not run serve off, commands = %v", runner.commands)
	}
}
