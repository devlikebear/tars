package tarsserver

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/devlikebear/tars/internal/config"
	"github.com/devlikebear/tars/internal/remoteaccess"
	"github.com/rs/zerolog"
)

// The remote access handler lives in internal/apihandlers; this test stays
// here because it drives the server runtime's reconcile-on-start.
func TestRemoteAccessReconcileOnStartUsesDesiredConfig(t *testing.T) {
	runner := newRemoteAccessTestRunner(map[string]string{
		"tailscale status --json":                                 `{"BackendState":"Running","Self":{"HostName":"mac","DNSName":"mac.tail.ts.net."}}`,
		"tailscale serve status --json":                           `{}`,
		"tailscale serve --https=443 --bg http://127.0.0.1:43180": ``,
	})
	runtime := &serveAPIRuntime{
		cfg: config.Config{
			RemoteAccessConfig: config.RemoteAccessConfig{
				RemoteAccessTailscaleServeEnabled:   true,
				RemoteAccessTailscaleServeHTTPSPort: 443,
			},
		},
		remoteAccessRunner:    runner,
		remoteAccessTargetURL: remoteaccess.DefaultTargetURL,
	}

	reconcileRemoteAccessOnStart(context.Background(), runtime, zerolog.Nop())

	if !containsCommand(runner.commands, "tailscale serve --https=443 --bg http://127.0.0.1:43180") {
		t.Fatalf("expected reconcile to enable Serve, commands=%v", runner.commands)
	}
}

func containsCommand(commands []string, want string) bool {
	for _, command := range commands {
		if command == want {
			return true
		}
	}
	return false
}

type remoteAccessTestRunner struct {
	outputs  map[string]string
	commands []string
}

func newRemoteAccessTestRunner(outputs map[string]string) *remoteAccessTestRunner {
	return &remoteAccessTestRunner{outputs: outputs}
}

func (r *remoteAccessTestRunner) Run(_ context.Context, name string, args ...string) (string, string, error) {
	key := strings.TrimSpace(name + " " + strings.Join(args, " "))
	r.commands = append(r.commands, key)
	out, ok := r.outputs[key]
	if !ok {
		return "", "", errors.New("unexpected command: " + key)
	}
	return out, "", nil
}
