package tools

import "testing"

func TestValidateShellCommandCompatibility(t *testing.T) {
	for _, command := range []string{
		`env`,
		`env FOO=bar printenv FOO`,
		`command -v go`,
		`command printf '%s' hello`,
		`printf hello | xargs echo`,
		`printf hello | xargs -I {} echo {}`,
		`sh -c 'printf "hello\n"'`,
		`sh script.sh`,
		`bash -eu ./script.sh`,
		`sh -lc 'echo fine'`,
	} {
		t.Run(command, func(t *testing.T) {
			if err := validateShellCommand(command); err != nil {
				t.Fatalf("valid shell command rejected: %v", err)
			}
		})
	}
}

func TestValidateShellCommandWrappedRestrictions(t *testing.T) {
	for _, command := range []string{`env FOO=bar rm unused`, `command rm unused`, `echo unused | xargs rm`, `sh -c 'echo ok; rm unused'`, `sh -lc 'rm unused'`, `/bin/r[m] unused`, `printf 'rm unused' | xargs -I {} sh -c '{}'`} {
		t.Run(command, func(t *testing.T) {
			if err := validateShellCommand(command); err == nil {
				t.Fatal("wrapped blocked command accepted")
			}
		})
	}
}
