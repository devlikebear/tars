package tools

import (
	"context"
	"fmt"
	"os/exec"
	"path"
	"strings"

	"github.com/devlikebear/tars/internal/shellexec"
	"mvdan.cc/sh/v3/syntax"
)

// validateShellCommand checks every command position, including substitutions.
// This denylist is defense in depth, not a sandbox for arbitrary programs.
func validateShellCommand(command string) error {
	if strings.TrimSpace(command) == "" {
		return fmt.Errorf("command is required")
	}
	if strings.ContainsAny(command, "\n\r") {
		return fmt.Errorf("multi-line command is not allowed")
	}
	f, err := syntax.NewParser(syntax.Variant(syntax.LangPOSIX)).Parse(strings.NewReader(command), "")
	if err != nil {
		return fmt.Errorf("invalid shell command: %w", err)
	}
	syntax.Walk(f, func(n syntax.Node) bool {
		if err != nil {
			return false
		}
		call, ok := n.(*syntax.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		err = validateShellCall(call.Args)

		return err == nil
	})
	return err
}

func literalShellWord(w *syntax.Word) (string, bool) {
	var b strings.Builder
	for _, p := range w.Parts {
		switch p := p.(type) {
		case *syntax.Lit:
			b.WriteString(unescapeShellLiteral(p.Value))
		case *syntax.SglQuoted:
			b.WriteString(p.Value)
		case *syntax.DblQuoted:
			v, ok := literalShellWord(&syntax.Word{Parts: p.Parts})
			if !ok {
				return "", false
			}
			b.WriteString(v)
		default:
			return "", false
		}
	}

	return b.String(), true
}

func newShellCommand(ctx context.Context, dir, command string) (*exec.Cmd, error) {
	if err := validateShellCommand(command); err != nil {
		return nil, err
	}
	shell, err := shellexec.Executable()
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, shell, "-c", command)
	cmd.Dir = dir
	configureExecShellProcess(cmd)
	return cmd, nil
}

func unescapeShellLiteral(value string) string {
	var b strings.Builder
	escaped := false
	for _, r := range value {
		if !escaped && r == '\\' {
			escaped = true
			continue
		}
		b.WriteRune(r)
		escaped = false
	}
	if escaped {
		b.WriteRune('\\')
	}
	return b.String()
}

func validateShellCall(args []*syntax.Word) error {
	if len(args) == 0 {
		return nil
	}
	name, static := literalShellWord(args[0])
	if !static {
		return fmt.Errorf("dynamic command names are not allowed")
	}
	if strings.ContainsAny(name, "*?") || (strings.Contains(name, "[") && strings.Contains(name, "]")) {
		return fmt.Errorf("glob patterns in command names are not allowed")
	}
	name = strings.ToLower(path.Base(name))
	if _, blocked := blockedExecCommands[name]; blocked {
		return fmt.Errorf("blocked command: %s", name)
	}
	switch name {
	case "eval", ".", "source":
		return fmt.Errorf("indirect command execution is not allowed: %s", name)
	case "sh", "bash", "dash":
		for i := 1; i < len(args); i++ {
			flag, ok := literalShellWord(args[i])
			if !ok {
				return fmt.Errorf("dynamic shell options are not allowed")
			}
			// Script files are programs, like Python or Node entry points.
			// The command denylist is not a sandbox for their contents.
			if flag == "--" || !strings.HasPrefix(flag, "-") {
				return nil
			}
			if flag == "-o" || flag == "-O" {
				i++
				continue
			}
			if !strings.HasPrefix(flag, "--") && strings.Contains(flag, "c") {
				if i+1 >= len(args) {
					return fmt.Errorf("shell invocation requires a literal -c script")
				}
				script, static := literalShellWord(args[i+1])
				if !static {
					return fmt.Errorf("shell invocation requires a literal -c script")
				}
				return validateShellCommand(script)
			}
		}
	case "env", "command", "exec", "xargs":
		replacement := ""
		for i := 1; i < len(args); i++ {
			value, ok := literalShellWord(args[i])
			if !ok {
				return fmt.Errorf("dynamic %s command options are not allowed", name)
			}
			if value == "--" {
				return validateWrappedShellCall(args[i+1:], replacement)
			}
			if name == "env" && strings.Contains(value, "=") && !strings.HasPrefix(value, "-") {
				continue
			}
			if !strings.HasPrefix(value, "-") {
				return validateWrappedShellCall(args[i:], replacement)
			}
			switch name {
			case "command":
				if value == "-v" || value == "-V" {
					return nil
				}
				if value == "-p" {
					continue
				}
			case "exec":
				if value == "-c" || value == "-l" {
					continue
				}
				if value == "-a" && i+1 < len(args) {
					i++
					continue
				}
			case "env":
				if value == "-i" || value == "--ignore-environment" || value == "-0" || value == "--null" {
					continue
				}
				if (value == "-u" || value == "--unset" || value == "-C" || value == "--chdir") && i+1 < len(args) {
					i++
					continue
				}
				if strings.HasPrefix(value, "--unset=") || strings.HasPrefix(value, "--chdir=") {
					continue
				}
			case "xargs":
				if value == "-I" && i+1 < len(args) {
					i++
					var static bool
					replacement, static = literalShellWord(args[i])
					if !static || replacement == "" {
						return fmt.Errorf("xargs replacement must be literal and nonempty")
					}
					continue
				}
				if strings.HasPrefix(value, "-I") && len(value) > 2 {
					replacement = value[2:]
					continue
				}
				if value == "-0" || value == "--null" || value == "-r" || value == "--no-run-if-empty" || value == "-t" || value == "-p" || value == "-x" {
					continue
				}
				if strings.Contains("LEnPs", strings.TrimPrefix(value, "-")) && len(value) == 2 && i+1 < len(args) {
					i++
					continue
				}
				if len(value) > 2 && strings.ContainsRune("LEnPs", rune(value[1])) {
					continue
				}
			}
			return fmt.Errorf("unsupported %s option: %s", name, value)
		}
	}
	return nil
}

func validateWrappedShellCall(args []*syntax.Word, replacement string) error {
	if replacement == "" {
		return validateShellCall(args)
	}
	checked := append([]*syntax.Word(nil), args...)
	for i, word := range checked {
		value, static := literalShellWord(word)
		if static && strings.Contains(value, replacement) {
			// xargs substitutes these words later. Treat them as dynamic so
			// operands remain usable but command names and -c scripts fail.
			checked[i] = &syntax.Word{Parts: []syntax.WordPart{&syntax.ParamExp{}}}
		}
	}
	return validateShellCall(checked)
}
