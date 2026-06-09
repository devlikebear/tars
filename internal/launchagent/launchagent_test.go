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

func TestDefaultDomainForUID(t *testing.T) {
	if got := DefaultDomainForUID(501); got != "gui/501" {
		t.Fatalf("expected gui/501, got %q", got)
	}
}

func TestProgramArgumentsFromPlistDecodesEscapedStrings(t *testing.T) {
	plist := BuildPlist(Config{
		Label: "io.tars.server",
		ProgramArguments: []string{
			"/usr/local/bin/tars",
			"serve",
			"--api-addr",
			"127.0.0.1:43187",
			"--note",
			"a & b",
		},
	})
	args, err := ProgramArgumentsFromPlist([]byte(plist))
	if err != nil {
		t.Fatalf("parse ProgramArguments: %v", err)
	}
	if got, ok := ArgumentValue(args, "--api-addr"); !ok || got != "127.0.0.1:43187" {
		t.Fatalf("expected api addr, got %q ok=%v args=%#v", got, ok, args)
	}
	if got, ok := ArgumentValue(args, "--note"); !ok || got != "a & b" {
		t.Fatalf("expected decoded note, got %q ok=%v args=%#v", got, ok, args)
	}
}

func TestProgramArgumentsFromPlistMissingArrayReturnsEmpty(t *testing.T) {
	args, err := ProgramArgumentsFromPlist([]byte(`<plist><dict><key>Label</key><string>io.tars.server</string></dict></plist>`))
	if err != nil {
		t.Fatalf("parse plist: %v", err)
	}
	if len(args) != 0 {
		t.Fatalf("expected no args, got %#v", args)
	}
	if got, ok := ArgumentValue(args, "--api-addr"); ok {
		t.Fatalf("expected missing value, got %q", got)
	}
}

func TestProgramArgumentsFromPlistNonArrayValueReturnsEmpty(t *testing.T) {
	args, err := ProgramArgumentsFromPlist([]byte(`<plist><dict><key>ProgramArguments</key><dict/></dict></plist>`))
	if err != nil {
		t.Fatalf("parse plist: %v", err)
	}
	if len(args) != 0 {
		t.Fatalf("expected no args, got %#v", args)
	}
}

func TestProgramArgumentsFromPlistReturnsKeyDecodeError(t *testing.T) {
	_, err := ProgramArgumentsFromPlist([]byte(`<plist><dict><key>ProgramArguments`))
	if err == nil {
		t.Fatalf("expected malformed key error")
	}
}

func TestProgramArgumentsFromPlistReturnsStringDecodeError(t *testing.T) {
	_, err := ProgramArgumentsFromPlist([]byte(`<plist><dict><key>ProgramArguments</key><array><string>unterminated`))
	if err == nil {
		t.Fatalf("expected malformed string error")
	}
}

func TestArgumentValueFlagWithoutValueReturnsFalse(t *testing.T) {
	if got, ok := ArgumentValue([]string{"/usr/local/bin/tars", "serve", "--api-addr"}, "--api-addr"); ok {
		t.Fatalf("expected no value, got %q", got)
	}
}

func TestArgumentValueEmptyFlagReturnsFalse(t *testing.T) {
	if got, ok := ArgumentValue([]string{"--api-addr", "127.0.0.1:43180"}, " "); ok {
		t.Fatalf("expected empty flag to be ignored, got %q", got)
	}
}

func TestArgumentValueEmptyValueReturnsFalse(t *testing.T) {
	if got, ok := ArgumentValue([]string{"--api-addr", " "}, "--api-addr"); ok {
		t.Fatalf("expected empty value to be ignored, got %q", got)
	}
}
