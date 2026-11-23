package easyproject

// Readiness (vibe-coder-identity.md §4, phase 4): what the Start screen
// shows before anything can be made — whether this computer has what a
// project needs, and the one next thing to do when it does not. Two facts,
// each a fixed state, never a tool's own output:
//
//   - engine: the AI that does the work. A CLI engine (claude-code-cli,
//     antigravity-cli) must be installed and, for Claude Code, signed in.
//   - tools: what builds and runs the app (Node and npm).
//
// The decision is the order the person has to fix them in.

// Check states.
const (
	CheckOK = "ok"
	// CheckMissing: not installed on this computer.
	CheckMissing = "missing"
	// CheckSignedOut: installed, but nobody is signed in.
	CheckSignedOut = "signed_out"
	// CheckNotSet: no engine is chosen in this server's settings.
	CheckNotSet = "not_set"
)

// Next steps, in the order they are asked for.
const (
	NextChooseEngine  = "choose_engine"
	NextInstallEngine = "install_engine"
	NextSignIn        = "sign_in"
	NextInstallTools  = "install_tools"
)

// EngineFacts is what the server read about the engine.
type EngineFacts struct {
	// Kind is the chat tier's provider kind; "" when none resolves.
	Kind string
	// Found reports whether a CLI engine's program is on this computer.
	Found bool
	// SignedIn is Claude Code's own answer; nil when it could not say
	// (an older CLI, a timeout), which does not stop the person.
	SignedIn *bool
}

// Readiness is the Start screen's answer.
type Readiness struct {
	Ready bool `json:"ready"`
	// Next is the one thing to do, "" when ready.
	Next   string `json:"next,omitempty"`
	Engine string `json:"engine"`
	// EngineKind lets the screen name the engine to install or sign in to.
	EngineKind string `json:"engine_kind,omitempty"`
	Tools      string `json:"tools"`
}

const (
	kindClaudeCodeCLI  = "claude-code-cli"
	kindAntigravityCLI = "antigravity-cli"
)

// IsCLIEngine reports whether the engine kind runs a program on this computer.
func IsCLIEngine(kind string) bool {
	return kind == kindClaudeCodeCLI || kind == kindAntigravityCLI
}

// ReadinessOf decides from the facts. toolsErr is the scaffolder's Ready().
func ReadinessOf(engine EngineFacts, toolsErr error) Readiness {
	r := Readiness{Engine: CheckOK, EngineKind: engine.Kind, Tools: CheckOK}
	switch {
	case engine.Kind == "":
		r.Engine, r.Next = CheckNotSet, NextChooseEngine
	case IsCLIEngine(engine.Kind) && !engine.Found:
		r.Engine, r.Next = CheckMissing, NextInstallEngine
	case engine.Kind == kindClaudeCodeCLI && engine.SignedIn != nil && !*engine.SignedIn:
		r.Engine, r.Next = CheckSignedOut, NextSignIn
	}
	if toolsErr != nil {
		r.Tools = CheckMissing
		if r.Next == "" {
			r.Next = NextInstallTools
		}
	}
	r.Ready = r.Next == ""
	return r
}
