package focusprobe

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/devlikebear/tars/internal/focuspipeline"
)

const (
	// repoVerifyMaxCommands caps the repository's default verify list.
	repoVerifyMaxCommands = 20
	// repoVerifyMaxCommandRunes caps one command of it.
	repoVerifyMaxCommandRunes = 300
	// promotionMaxTopLevel caps the top-level entries listed.
	promotionMaxTopLevel = 60
	// promotionMaxTargets caps the Makefile targets listed.
	promotionMaxTargets = 80
	// promotionMaxLintBytes is the largest linter configuration shown whole.
	promotionMaxLintBytes = 8 << 10
)

// ReadRepoVerify reads the repository's default verify list
// (focuspipeline.RepoVerifyFile at the top level of dir's repository): the
// commands every plan in it runs. A missing, unreadable or malformed file is
// no list; a command that is empty, spans lines or is too long is skipped,
// as is any past the cap. The file is read without following a symbolic
// link.
func ReadRepoVerify(ctx context.Context, dir string) []string {
	if strings.TrimSpace(dir) == "" {
		return nil
	}
	root, err := RunGit(ctx, GitTimeout, dir, nil, "rev-parse", "--show-toplevel")
	if err != nil || root == "" {
		return nil
	}
	text, ok := readRegular(filepath.Join(root, filepath.FromSlash(focuspipeline.RepoVerifyFile)))
	if !ok {
		return nil
	}
	var file struct {
		Verify []string `json:"verify"`
	}
	if err := json.Unmarshal([]byte(text), &file); err != nil {
		return nil
	}
	var out []string
	for _, cmd := range file.Verify {
		cmd = strings.TrimSpace(cmd)
		if cmd == "" || strings.ContainsAny(cmd, "\r\n") || utf8.RuneCountInString(cmd) > repoVerifyMaxCommandRunes {
			continue
		}
		if len(out) == repoVerifyMaxCommands {
			break
		}
		dup := false
		for _, have := range out {
			dup = dup || have == cmd
		}
		if !dup {
			out = append(out, cmd)
		}
	}
	return out
}

// lintConfigNames are the linter configurations a promotion prompt shows.
var lintConfigNames = []string{
	".golangci.yml", ".golangci.yaml", ".golangci.toml",
	"eslint.config.js", "eslint.config.mjs", "eslint.config.cjs", ".eslintrc.json", ".eslintrc.js", ".eslintrc.cjs", ".eslintrc.yml",
	"ruff.toml", ".ruff.toml", ".rubocop.yml", ".swiftlint.yml", "clippy.toml",
}

var makeTarget = regexp.MustCompile(`^([A-Za-z0-9][A-Za-z0-9_.-]*)\s*:([^=]|$)`)

// ReadPromotionContext reads what a verify promotion's prompt shows of the
// repository at repo: its top-level entries, its Makefile targets and its
// linter configurations (whole when small enough). Symbolic links are not
// followed.
func ReadPromotionContext(repo string) focuspipeline.PromotionContext {
	var out focuspipeline.PromotionContext
	if strings.TrimSpace(repo) == "" {
		return out
	}
	if entries, err := os.ReadDir(repo); err == nil {
		for _, e := range entries {
			if e.Name() == ".git" || len(out.TopLevel) == promotionMaxTopLevel {
				continue
			}
			name := e.Name()
			if e.IsDir() {
				name += "/"
			}
			out.TopLevel = append(out.TopLevel, name)
		}
	}
	if text, ok := readRegular(filepath.Join(repo, "Makefile")); ok {
		seen := map[string]bool{}
		for _, line := range strings.Split(text, "\n") {
			m := makeTarget.FindStringSubmatch(line)
			if m == nil || seen[m[1]] || strings.HasPrefix(m[1], ".") {
				continue
			}
			seen[m[1]] = true
			out.MakeTargets = append(out.MakeTargets, m[1])
			if len(out.MakeTargets) == promotionMaxTargets {
				break
			}
		}
		sort.Strings(out.MakeTargets)
	}
	for _, name := range lintConfigNames {
		text, ok := readRegular(filepath.Join(repo, name))
		if !ok || len(text) > promotionMaxLintBytes {
			continue
		}
		out.LintConfigs = append(out.LintConfigs, focuspipeline.PromotionFile{Path: name, Content: text})
	}
	return out
}
