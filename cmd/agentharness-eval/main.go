package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/agentharness/evalpack"
	"github.com/devlikebear/tars/internal/atomicwrite"
	"github.com/devlikebear/tars/internal/llm"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr, os.Getenv); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer, getenv func(string) string) error {
	fs := flag.NewFlagSet("agentharness-eval", flag.ContinueOnError)
	fs.SetOutput(stderr)
	modeValue := fs.String("mode", string(evalpack.ModeDeterministic), "evaluation mode: deterministic or live")
	packPath := fs.String("pack", "testdata/agent-harness/scenarios.json", "path to the versioned scenario pack")
	version := fs.String("version", "", "TARS version recorded in the report (defaults to VERSION.txt)")
	commit := fs.String("commit", "", "TARS commit recorded in the report (defaults to git HEAD)")
	jsonlPath := fs.String("jsonl", "-", "JSONL report path, '-' for stdout, or empty to disable")
	markdownPath := fs.String("markdown", "", "Markdown report path, or empty to disable")
	workspaceRoot := fs.String("workspace-root", "", "temporary deterministic-fixture parent directory")
	timeout := fs.Duration("timeout", 2*time.Minute, "overall evaluation timeout")
	inputCost := fs.Float64("input-cost-per-million", 0, "live provider input-token cost in USD per million")
	outputCost := fs.Float64("output-cost-per-million", 0, "live provider output-token cost in USD per million")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *timeout <= 0 {
		return fmt.Errorf("timeout must be positive")
	}

	pack, err := evalpack.LoadPack(*packPath)
	if err != nil {
		return err
	}
	resolvedVersion, err := resolveVersion(*version)
	if err != nil {
		return err
	}
	resolvedCommit := resolveCommit(*commit)
	mode := evalpack.Mode(strings.ToLower(strings.TrimSpace(*modeValue)))

	var executor evalpack.Executor
	switch mode {
	case evalpack.ModeDeterministic:
		executor = evalpack.NativeExecutor{RootDir: strings.TrimSpace(*workspaceRoot)}
	case evalpack.ModeLive:
		provider := strings.TrimSpace(getenv("TARS_AGENT_EVAL_PROVIDER"))
		if provider == "" {
			return fmt.Errorf("live mode requires TARS_AGENT_EVAL_PROVIDER")
		}
		client, err := llm.NewProvider(llm.ProviderOptions{
			Provider: provider,
			BaseURL:  strings.TrimSpace(getenv("TARS_AGENT_EVAL_BASE_URL")),
			Model:    strings.TrimSpace(getenv("TARS_AGENT_EVAL_MODEL")),
			APIKey:   resolveAPIKey(provider, getenv),
			WorkDir:  strings.TrimSpace(getenv("TARS_AGENT_EVAL_WORKDIR")),
		})
		if err != nil {
			return fmt.Errorf("configure live provider: %w", err)
		}
		executor = evalpack.LiveExecutor{
			Client: client, InputCostPerMillion: *inputCost, OutputCostPerMillion: *outputCost,
		}
	default:
		return fmt.Errorf("unsupported evaluation mode %q", mode)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	report, err := (evalpack.Runner{
		Executor: executor,
		Config: evalpack.RunConfig{
			Mode: mode, Version: resolvedVersion, Commit: resolvedCommit, Now: time.Now,
		},
	}).Run(ctx, pack)
	if err != nil {
		return err
	}
	if err := emitReports(stdout, *jsonlPath, *markdownPath, report); err != nil {
		return err
	}
	summaryWriter := stdout
	if strings.TrimSpace(*jsonlPath) == "-" {
		summaryWriter = stderr
	}
	if _, err := fmt.Fprintf(summaryWriter, "%d scenarios completed; %d baseline expectations met; task success %.1f%%; verifier pass %.1f%%\n",
		report.Summary.Completed, report.Summary.ExpectationsMet,
		report.Summary.TaskSuccessRate*100, report.Summary.VerifierPassRate*100); err != nil {
		return fmt.Errorf("write evaluation summary: %w", err)
	}
	if mode == evalpack.ModeDeterministic && (report.Summary.Errors > 0 || report.Summary.ExpectationsMet != report.Summary.Completed) {
		return fmt.Errorf("deterministic baseline drifted: %d errors, %d/%d expectations met",
			report.Summary.Errors, report.Summary.ExpectationsMet, report.Summary.Completed)
	}
	return nil
}

func resolveVersion(explicit string) (string, error) {
	if value := strings.TrimSpace(explicit); value != "" {
		return value, nil
	}
	data, err := os.ReadFile("VERSION.txt")
	if err != nil {
		return "", fmt.Errorf("read VERSION.txt (or pass --version): %w", err)
	}
	value := strings.TrimSpace(string(data))
	if value == "" {
		return "", fmt.Errorf("VERSION.txt is empty")
	}
	return value, nil
}

func resolveCommit(explicit string) string {
	if value := strings.TrimSpace(explicit); value != "" {
		return value
	}
	output, err := exec.Command("git", "rev-parse", "--short=12", "HEAD").Output()
	if err != nil || strings.TrimSpace(string(output)) == "" {
		return "unknown"
	}
	return strings.TrimSpace(string(output))
}

func resolveAPIKey(provider string, getenv func(string) string) string {
	if value := strings.TrimSpace(getenv("TARS_AGENT_EVAL_API_KEY")); value != "" {
		return value
	}
	var names []string
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "openai", "openai-codex":
		names = []string{"OPENAI_API_KEY"}
	case "anthropic", "claude-code-cli":
		names = []string{"ANTHROPIC_API_KEY", "CLAUDE_API_KEY"}
	case "gemini", "gemini-native":
		names = []string{"GEMINI_API_KEY"}
	case "kimi":
		names = []string{"KIMI_API_KEY"}
	}
	for _, name := range names {
		if value := strings.TrimSpace(getenv(name)); value != "" {
			return value
		}
	}
	return ""
}

func emitReports(stdout io.Writer, jsonlPath, markdownPath string, report evalpack.Report) error {
	jsonlPath = strings.TrimSpace(jsonlPath)
	markdownPath = strings.TrimSpace(markdownPath)
	if jsonlPath == "-" && markdownPath == "-" {
		return fmt.Errorf("JSONL and Markdown reports cannot both use stdout")
	}
	if jsonlPath != "" {
		var buffer bytes.Buffer
		if err := evalpack.WriteJSONL(&buffer, report); err != nil {
			return err
		}
		if err := writeOutput(stdout, jsonlPath, buffer.Bytes()); err != nil {
			return err
		}
	}
	if markdownPath != "" {
		var buffer bytes.Buffer
		if err := evalpack.WriteMarkdown(&buffer, report); err != nil {
			return err
		}
		if err := writeOutput(stdout, markdownPath, buffer.Bytes()); err != nil {
			return err
		}
	}
	return nil
}

func writeOutput(stdout io.Writer, path string, data []byte) error {
	if path == "-" {
		if _, err := stdout.Write(data); err != nil {
			return fmt.Errorf("write report to stdout: %w", err)
		}
		return nil
	}
	if err := atomicwrite.Write(path, data); err != nil {
		return fmt.Errorf("write report %q: %w", path, err)
	}
	return nil
}
