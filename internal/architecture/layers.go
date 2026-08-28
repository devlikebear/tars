// Package architecture records the repository's layering and provides the
// data the layer test enforces it from.
//
// The layers, outermost first:
//
//	cmd/            entry points
//	app layer       orchestration: the server, schedulers, watchdogs, runtimes
//	core layer      primitives: llm, tools, session, memory, skill, mcp, prompt
//	pkg/            the public API surface
//
// Imports may only point inward. The rule that actually matters, and the one
// the test enforces, is the reverse: a core package must never import an app
// package, and neither must anything under pkg/.
//
// This exists because the alternative — splitting the repository into
// tars-app / tars-cli / tars-console — buys the same discipline at a far
// higher price for a single maintainer. See
// docs/decisions/repository-layering.md for that decision and the criteria
// for revisiting it.
//
// Membership is a list rather than a directory convention on purpose. Moving
// ~60 packages into internal/app/ and internal/core/ would be a rename large
// enough to bury the actual constraint, and it would break every import path
// in the tree for no additional enforcement.
package architecture

// CorePackages are the primitives. They may import each other, standard
// library, and third-party code — never an app package.
//
// This set is the transitive closure of what pkg/* depends on, which is what
// makes the public surface's dependency weight a property of the layering
// rather than an accident.
var CorePackages = []string{
	"agent",
	"atomicwrite",
	"auth",
	"config",
	"exepath",
	"git",
	"llm",
	"llmdefaults",
	"mcp",
	"memory",
	"prompt",
	"secrets",
	"session",
	"skill",
	"tool",
}

// AppPackages orchestrate: they own storage, scheduling, process supervision,
// HTTP surfaces, and the TARS-specific tool set. They may import core freely.
var AppPackages = []string{
	"a2a",
	"agentruntime",
	"apptool",
	"assistant",
	"consoleauth",
	"critic",
	"cron",
	"embodiment",
	"executionplane",
	"extensions",
	"goal",
	"launchagent",
	"onboarding",
	"ops",
	"plugin",
	"proofverifier",
	"pulse",
	"reflection",
	"release",
	"remoteaccess",
	"serverauth",
	"sessionoverride",
	"skillhub",
	"tarsserver",
	"workerprotocol",
	"workscheduler",
	"workstore",
}

// SharedPackages are leaf utilities with no layer opinion — small, dependency-
// light helpers either side may use. Listing them explicitly, rather than
// treating "unlisted" as shared, is what makes a newly added package fail the
// test until someone decides where it belongs.
var SharedPackages = []string{
	"agentharness",
	// architecture is this package: three string slices and a doc comment,
	// imported only by its own test.
	"architecture",
	"assetpath",
	"buildinfo",
	"cli",
	"envloader",
	"fileuri",
	"scheduleexpr",
	"shellexec",
	"sysprompt",
	"tarsclient",
	"textutil",
	"usage",
}
