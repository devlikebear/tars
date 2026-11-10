package focusprobe

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestReadRepoVerify(t *testing.T) {
	repo, _ := focusReviewRepo(t)
	if got := ReadRepoVerify(context.Background(), repo); got != nil {
		t.Fatalf("no file: %v", got)
	}
	if ReadRepoVerify(context.Background(), "") != nil || ReadRepoVerify(context.Background(), t.TempDir()) != nil {
		t.Fatal("outside a repository")
	}
	cmds := []string{"make test", " make test ", "", "a\nb", strings.Repeat("x", repoVerifyMaxCommandRunes+1), "make lint"}
	for i := 0; i < repoVerifyMaxCommands+5; i++ {
		cmds = append(cmds, fmt.Sprintf("echo %d", i))
	}
	body := `{"verify":["` + strings.Join(cmds[:1], `","`) + `"`
	for _, c := range cmds[1:] {
		body += fmt.Sprintf(",%q", c)
	}
	body += "]}"
	writeRepoFile(t, repo, ".tars/verify.json", body)
	sub := filepath.Join(repo, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	got := ReadRepoVerify(context.Background(), sub)
	if len(got) != repoVerifyMaxCommands || !slices.Equal(got[:3], []string{"make test", "make lint", "echo 0"}) {
		t.Fatalf("read = %v", got)
	}
	writeRepoFile(t, repo, ".tars/verify.json", "{")
	if got := ReadRepoVerify(context.Background(), repo); got != nil {
		t.Fatalf("malformed: %v", got)
	}
}

func TestReadPromotionContext(t *testing.T) {
	if got := ReadPromotionContext(" "); got.TopLevel != nil {
		t.Fatal("no repository")
	}
	repo, _ := focusReviewRepo(t)
	writeRepoFile(t, repo, "Makefile", "VAR := 1\n.PHONY: test\ntest: build\n\tgo test\nbuild:\n\tgo build\ntest: other\n")
	writeRepoFile(t, repo, ".golangci.yml", "linters: {}\n")
	writeRepoFile(t, repo, "eslint.config.js", strings.Repeat("x", promotionMaxLintBytes+1))
	if err := os.MkdirAll(filepath.Join(repo, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := ReadPromotionContext(repo)
	if !slices.Contains(got.TopLevel, "scripts/") || slices.Contains(got.TopLevel, ".git/") || !slices.Contains(got.TopLevel, "Makefile") {
		t.Fatalf("top level = %v", got.TopLevel)
	}
	if !slices.Equal(got.MakeTargets, []string{"build", "test"}) {
		t.Fatalf("targets = %v", got.MakeTargets)
	}
	if len(got.LintConfigs) != 1 || got.LintConfigs[0].Path != ".golangci.yml" {
		t.Fatalf("lint = %+v", got.LintConfigs)
	}
}
