package remoteaccess

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func TestDetectParsesTailscaleStatusAndOwnedServeStatus(t *testing.T) {
	runner := fakeRunner{
		outputs: map[string]string{
			"tailscale status --json": `{
				"BackendState": "Running",
				"Self": {"HostName": "mac-mini", "DNSName": "mac-mini.tailnet.ts.net."}
			}`,
			"tailscale serve status --json": `{
				"Web": {
					"mac-mini.tailnet.ts.net:443": {
						"Handlers": {
							"/": {"Proxy": "http://127.0.0.1:43180"}
						}
					}
				}
			}`,
		},
	}

	status, err := Detect(context.Background(), Options{
		Runner:    runner,
		HTTPSPort: 443,
		TargetURL: "http://127.0.0.1:43180",
	})
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if !status.Installed || !status.LoggedIn || status.HostName != "mac-mini" || status.TailnetURL != "mac-mini.tailnet.ts.net" {
		t.Fatalf("unexpected tailscale status: %+v", status)
	}
	if !status.ServeActive || !status.OwnedByTARS || status.ServePort != 443 {
		t.Fatalf("expected owned serve config, got %+v", status)
	}
}

func TestDetectPrefersServeStatusWhenGetConfigIsSparse(t *testing.T) {
	runner := &recordingRunner{
		outputs: map[string]string{
			"tailscale status --json": `{
				"BackendState": "Running",
				"Self": {"HostName": "mac-mini", "DNSName": "mac-mini.tailnet.ts.net."}
			}`,
			"tailscale serve status --json": `{
				"Web": {
					"mac-mini.tailnet.ts.net:443": {
						"Handlers": {"/": {"Proxy": "http://127.0.0.1:43180"}}
					}
				}
			}`,
			"tailscale serve get-config --all": `{"version":"0.0.1"}`,
		},
	}

	status, err := Detect(context.Background(), Options{Runner: runner})
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if !status.ServeActive || !status.OwnedByTARS || status.ServePort != 443 {
		t.Fatalf("expected owned serve status from serve status --json, got %+v", status)
	}
	for _, command := range runner.commands {
		if command == "tailscale serve get-config --all" {
			t.Fatalf("Detect should not call get-config when serve status succeeds, commands=%v", runner.commands)
		}
	}
}

func TestDetectFallsBackToGetConfigWhenServeStatusFails(t *testing.T) {
	runner := &recordingRunner{
		outputs: map[string]string{
			"tailscale status --json": `{
				"BackendState": "Running",
				"Self": {"HostName": "mac-mini", "DNSName": "mac-mini.tailnet.ts.net."}
			}`,
			"tailscale serve get-config --all": `{
				"Web": {
					"mac-mini.tailnet.ts.net:443": {
						"Handlers": {"/": {"Proxy": "http://127.0.0.1:43180"}}
					}
				}
			}`,
		},
		errors: map[string]error{
			"tailscale serve status --json": errors.New("unknown command"),
		},
	}

	status, err := Detect(context.Background(), Options{Runner: runner})
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if !status.ServeActive || !status.OwnedByTARS || status.ServePort != 443 {
		t.Fatalf("expected fallback get-config result, got %+v", status)
	}
	if got, want := strings.Join(runner.commands, "\n"), strings.Join([]string{
		"tailscale status --json",
		"tailscale serve status --json",
		"tailscale serve get-config --all",
	}, "\n"); got != want {
		t.Fatalf("unexpected commands:\n%s", got)
	}
}

func TestDetectMarksPortConflictWhenServeTargetDiffers(t *testing.T) {
	runner := fakeRunner{
		outputs: map[string]string{
			"tailscale status --json": `{"BackendState":"Running","Self":{"HostName":"mac","DNSName":"mac.tail.ts.net."}}`,
			"tailscale serve status --json": `{
				"Web": {
					"mac.tail.ts.net:443": {
						"Handlers": {"/": {"Proxy": "http://127.0.0.1:3000"}}
					}
				}
			}`,
		},
	}

	status, err := Detect(context.Background(), Options{
		Runner:    runner,
		HTTPSPort: 443,
		TargetURL: "http://127.0.0.1:43180",
	})
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if !status.ServeActive || status.OwnedByTARS {
		t.Fatalf("expected active non-owned serve config, got %+v", status)
	}
}

func TestDetectHandlesMissingTailscaleBinary(t *testing.T) {
	status, err := Detect(context.Background(), Options{
		Runner: fakeRunner{err: exec.ErrNotFound},
	})
	if err != nil {
		t.Fatalf("Detect should not error when tailscale is not installed: %v", err)
	}
	if status.Installed {
		t.Fatalf("expected Installed=false, got %+v", status)
	}
}

func TestDetectSkipsServeConfigWhenLoggedOut(t *testing.T) {
	runner := &recordingRunner{
		outputs: map[string]string{
			"tailscale status --json": `{"BackendState":"Stopped","Self":null}`,
		},
	}

	status, err := Detect(context.Background(), Options{Runner: runner})
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if !status.Installed || status.LoggedIn {
		t.Fatalf("expected installed/logged-out status, got %+v", status)
	}
	if len(runner.commands) != 1 {
		t.Fatalf("expected status-only detect while logged out, commands=%v", runner.commands)
	}
}

func TestEnableStartsServeWhenLoggedInAndIdle(t *testing.T) {
	runner := &recordingRunner{
		outputs: map[string]string{
			"tailscale status --json":                                 `{"BackendState":"Running","Self":{"HostName":"mac","DNSName":"mac.tail.ts.net."}}`,
			"tailscale serve status --json":                           `{}`,
			"tailscale serve --https=443 --bg http://127.0.0.1:43180": ``,
		},
	}

	if err := Enable(context.Background(), Options{Runner: runner}); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	if got := runner.commands[len(runner.commands)-1]; got != "tailscale serve --https=443 --bg http://127.0.0.1:43180" {
		t.Fatalf("expected serve enable command, got %q", got)
	}
}

func TestEnableRejectsPortOwnedByOtherTarget(t *testing.T) {
	runner := &recordingRunner{
		outputs: map[string]string{
			"tailscale status --json": `{"BackendState":"Running","Self":{"HostName":"mac","DNSName":"mac.tail.ts.net."}}`,
			"tailscale serve status --json": `{
				"Web": {"mac.tail.ts.net:443": {"Handlers": {"/": {"Proxy": "http://127.0.0.1:3000"}}}}
			}`,
		},
	}

	err := Enable(context.Background(), Options{Runner: runner})
	if err == nil || !strings.Contains(err.Error(), "different target") {
		t.Fatalf("expected port conflict error, got %v", err)
	}
	if len(runner.commands) != 2 {
		t.Fatalf("enable should not mutate on conflict, commands=%v", runner.commands)
	}
}

func TestEnableNoopsWhenAlreadyOwned(t *testing.T) {
	runner := &recordingRunner{
		outputs: map[string]string{
			"tailscale status --json": `{"BackendState":"Running","Self":{"HostName":"mac","DNSName":"mac.tail.ts.net."}}`,
			"tailscale serve status --json": `{
				"Web": {"mac.tail.ts.net:443": {"Handlers": {"/": {"Proxy": "http://127.0.0.1:43180"}}}}
			}`,
		},
	}

	if err := Enable(context.Background(), Options{Runner: runner}); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	if len(runner.commands) != 2 {
		t.Fatalf("expected detect-only noop, commands=%v", runner.commands)
	}
}

func TestDisableOnlyRemovesOwnedServeTarget(t *testing.T) {
	runner := &recordingRunner{
		outputs: map[string]string{
			"tailscale status --json":         `{"BackendState":"Running","Self":{"HostName":"mac","DNSName":"mac.tail.ts.net."}}`,
			"tailscale serve status --json":   `{"Web": {"mac.tail.ts.net:443": {"Handlers": {"/": {"Proxy": "http://127.0.0.1:43180"}}}}}`,
			"tailscale serve --https=443 off": ``,
		},
	}

	if err := Disable(context.Background(), Options{Runner: runner}); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	if got := runner.commands[len(runner.commands)-1]; got != "tailscale serve --https=443 off" {
		t.Fatalf("expected serve disable command, got %q", got)
	}
}

func TestDisableRejectsDifferentTarget(t *testing.T) {
	runner := &recordingRunner{
		outputs: map[string]string{
			"tailscale status --json": `{"BackendState":"Running","Self":{"HostName":"mac","DNSName":"mac.tail.ts.net."}}`,
			"tailscale serve status --json": `{
				"Web": {"mac.tail.ts.net:443": {"Handlers": {"/": {"Proxy": "http://127.0.0.1:3000"}}}}
			}`,
		},
	}

	err := Disable(context.Background(), Options{Runner: runner})
	if err == nil || !strings.Contains(err.Error(), "different target") {
		t.Fatalf("expected non-owned target error, got %v", err)
	}
	if len(runner.commands) != 2 {
		t.Fatalf("disable should not mutate non-owned config, commands=%v", runner.commands)
	}
}

func TestReconcileFollowsDesiredState(t *testing.T) {
	runner := &recordingRunner{
		outputs: map[string]string{
			"tailscale status --json":                                 `{"BackendState":"Running","Self":{"HostName":"mac","DNSName":"mac.tail.ts.net."}}`,
			"tailscale serve status --json":                           `{}`,
			"tailscale serve --https=443 --bg http://127.0.0.1:43180": ``,
		},
	}

	if err := Reconcile(context.Background(), Desired{Enabled: true}, Options{Runner: runner}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if got := runner.commands[len(runner.commands)-1]; got != "tailscale serve --https=443 --bg http://127.0.0.1:43180" {
		t.Fatalf("expected enable command, got %q", got)
	}
}

type fakeRunner struct {
	outputs map[string]string
	err     error
}

func (f fakeRunner) Run(_ context.Context, name string, args ...string) (string, string, error) {
	if f.err != nil {
		return "", "", f.err
	}
	key := strings.TrimSpace(name + " " + strings.Join(args, " "))
	out, ok := f.outputs[key]
	if !ok {
		return "", "", errors.New("unexpected command: " + key)
	}
	return out, "", nil
}

type recordingRunner struct {
	outputs  map[string]string
	errors   map[string]error
	stderr   map[string]string
	commands []string
}

func (r *recordingRunner) Run(_ context.Context, name string, args ...string) (string, string, error) {
	key := strings.TrimSpace(name + " " + strings.Join(args, " "))
	r.commands = append(r.commands, key)
	if err := r.errors[key]; err != nil {
		return "", r.stderr[key], err
	}
	out, ok := r.outputs[key]
	if !ok {
		return "", "", errors.New("unexpected command: " + key)
	}
	return out, "", nil
}
