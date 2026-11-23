package easyproject

import (
	"errors"
	"testing"
)

func TestReadinessOf(t *testing.T) {
	yes, no := true, false
	for _, tc := range []struct {
		name   string
		engine EngineFacts
		tools  error
		want   Readiness
	}{
		{"ready", EngineFacts{Kind: "claude-code-cli", Found: true, SignedIn: &yes}, nil,
			Readiness{Ready: true, Engine: CheckOK, EngineKind: "claude-code-cli", Tools: CheckOK}},
		{"no engine set", EngineFacts{}, ErrNodeMissing,
			Readiness{Next: NextChooseEngine, Engine: CheckNotSet, Tools: CheckMissing}},
		{"engine not installed comes first", EngineFacts{Kind: "claude-code-cli"}, ErrNodeMissing,
			Readiness{Next: NextInstallEngine, Engine: CheckMissing, EngineKind: "claude-code-cli", Tools: CheckMissing}},
		{"signed out", EngineFacts{Kind: "claude-code-cli", Found: true, SignedIn: &no}, nil,
			Readiness{Next: NextSignIn, Engine: CheckSignedOut, EngineKind: "claude-code-cli", Tools: CheckOK}},
		{"sign-in unknown does not stop", EngineFacts{Kind: "claude-code-cli", Found: true}, nil,
			Readiness{Ready: true, Engine: CheckOK, EngineKind: "claude-code-cli", Tools: CheckOK}},
		{"tools missing", EngineFacts{Kind: "claude-code-cli", Found: true, SignedIn: &yes}, errors.New("no npm"),
			Readiness{Next: NextInstallTools, Engine: CheckOK, EngineKind: "claude-code-cli", Tools: CheckMissing}},
		{"other cli needs only its program", EngineFacts{Kind: "antigravity-cli"}, nil,
			Readiness{Next: NextInstallEngine, Engine: CheckMissing, EngineKind: "antigravity-cli", Tools: CheckOK}},
		{"an API engine is ready once set", EngineFacts{Kind: "anthropic"}, nil,
			Readiness{Ready: true, Engine: CheckOK, EngineKind: "anthropic", Tools: CheckOK}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ReadinessOf(tc.engine, tc.tools); got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}
