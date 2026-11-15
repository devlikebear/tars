package main

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

const (
	helpAPITokenSentinel   = "sentinel-user-token-7f3a"
	helpAdminTokenSentinel = "sentinel-admin-token-9c1e"
)

// The token flags used to take their environment value as the registered
// default, which pflag prints in --help: `tars --help` showed the live API
// tokens. Every command with a token flag is checked, so a new command that
// copies the old pattern fails here too.
func TestHelpNeverPrintsTokensFromTheEnvironment(t *testing.T) {
	t.Setenv("TARS_API_TOKEN", helpAPITokenSentinel)
	t.Setenv("TARS_ADMIN_API_TOKEN", helpAdminTokenSentinel)

	root := newRootCommand(strings.NewReader(""), io.Discard, io.Discard)
	checked := 0
	// Two levels reach every token flag today: root, its commands, and their
	// subcommands (assistant start/install).
	for _, cmd := range append(root.Commands(), root) {
		for _, c := range append(cmd.Commands(), cmd) {
			if c.Flags().Lookup("api-token") == nil && c.Flags().Lookup("admin-api-token") == nil {
				continue
			}
			checked++
			usage := c.UsageString()
			for _, secret := range []string{helpAPITokenSentinel, helpAdminTokenSentinel} {
				if strings.Contains(usage, secret) {
					t.Errorf("%q help prints a token from the environment:\n%s", c.CommandPath(), usage)
				}
			}
			for _, name := range []string{"api-token", "admin-api-token"} {
				f := c.Flags().Lookup(name)
				if f == nil {
					continue
				}
				if f.DefValue != "" {
					t.Errorf("%q --%s registers a non-empty default", c.CommandPath(), name)
				}
				if !strings.Contains(f.Usage, "TARS_") {
					t.Errorf("%q --%s usage does not name its environment variable: %q", c.CommandPath(), name, f.Usage)
				}
			}
		}
	}
	// root, status, health, update, assistant start/install.
	if checked < 6 {
		t.Fatalf("checked %d commands with token flags, want at least 6", checked)
	}
}

func TestSecretStringVarKeepsTheStartingValue(t *testing.T) {
	fs := pflag.NewFlagSet("t", pflag.ContinueOnError)
	token := "from-env"
	secretStringVar(fs, &token, "api-token", "TARS_API_TOKEN", "api token")
	if token != "from-env" {
		t.Fatalf("starting value lost: %q", token)
	}
	if err := fs.Parse(nil); err != nil {
		t.Fatal(err)
	}
	if token != "from-env" {
		t.Fatalf("value without the flag = %q, want the environment value", token)
	}
	var out bytes.Buffer
	fs.SetOutput(&out)
	fs.PrintDefaults()
	if strings.Contains(out.String(), "from-env") {
		t.Fatalf("defaults print the value: %s", out.String())
	}
	if !strings.Contains(out.String(), "(or TARS_API_TOKEN)") {
		t.Fatalf("defaults do not name the variable: %s", out.String())
	}

	if err := fs.Parse([]string{"--api-token", "given"}); err != nil {
		t.Fatal(err)
	}
	if token != "given" {
		t.Fatalf("flag value = %q, want given", token)
	}

	plain := "x"
	secretStringVar(fs, &plain, "other", "", "plain")
	if f := fs.Lookup("other"); f == nil || f.Usage != "plain" {
		t.Fatalf("usage without a variable changed: %+v", f)
	}
}
