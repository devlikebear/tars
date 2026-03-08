package tool

import (
	"testing"

	"github.com/devlikebear/tars/internal/gateway"
)

func TestResolveBrowserStartProfile(t *testing.T) {
	tests := []struct {
		name            string
		requested       string
		status          gateway.BrowserState
		expectedProfile string
	}{
		{
			name:      "extension connected empty profile defaults to chrome",
			requested: "",
			status: gateway.BrowserState{
				ExtensionConnected: true,
			},
			expectedProfile: "chrome",
		},
		{
			name:      "extension connected managed profile coerces to chrome",
			requested: "managed",
			status: gateway.BrowserState{
				ExtensionConnected: true,
			},
			expectedProfile: "chrome",
		},
		{
			name:      "extension connected chrome profile stays chrome",
			requested: "chrome",
			status: gateway.BrowserState{
				ExtensionConnected: true,
			},
			expectedProfile: "chrome",
		},
		{
			name:      "extension disconnected keeps requested profile",
			requested: "managed",
			status: gateway.BrowserState{
				ExtensionConnected: false,
			},
			expectedProfile: "managed",
		},
		{
			name:      "extension disconnected keeps empty profile for service default",
			requested: "",
			status: gateway.BrowserState{
				ExtensionConnected: false,
			},
			expectedProfile: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveBrowserStartProfile(tc.requested, tc.status)
			if got != tc.expectedProfile {
				t.Fatalf("expected %q, got %q", tc.expectedProfile, got)
			}
		})
	}
}
