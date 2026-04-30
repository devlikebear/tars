package launchagent

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildPlistIncludesEnvironmentVariables(t *testing.T) {
	plist := BuildPlist(Config{
		Label:            "io.tars.server",
		ProgramArguments: []string{"/usr/local/bin/tars", "serve"},
		WorkingDirectory: "/tmp/tars-workspace",
		StdoutPath:       "/tmp/tars.out.log",
		StderrPath:       "/tmp/tars.err.log",
		KeepAlive:        true,
		RunAtLoad:        true,
		Environment: map[string]string{
			"PATH":                "/opt/homebrew/bin:/usr/local/bin",
			"TARS_LAUNCHD_LABEL":  "io.tars.server",
			"TARS_LAUNCHD_DOMAIN": "gui/501",
		},
	})

	for _, token := range []string{
		"<key>Label</key>",
		"io.tars.server",
		"<key>EnvironmentVariables</key>",
		"<key>PATH</key>",
		"<key>TARS_LAUNCHD_LABEL</key>",
		"<key>TARS_LAUNCHD_DOMAIN</key>",
		"gui/501",
	} {
		if !strings.Contains(plist, token) {
			t.Fatalf("expected plist to contain %q, got:\n%s", token, plist)
		}
	}
}

func TestPathForHomeUsesDefaultLabel(t *testing.T) {
	got := PathForHome("/Users/tester", "", "io.tars.server")
	want := filepath.Join("/Users/tester", "Library", "LaunchAgents", "io.tars.server.plist")
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
