package prompt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devlikebear/tars/internal/memory"
)

// BuildOptions configures system prompt generation.
type BuildOptions struct {
	WorkspaceDir string   // path to workspace root
	WorkDirs     []string // additional working directories the session can access
	CurrentDir   string   // session's current working directory (overrides WorkspaceDir for relative paths)
	SubAgent     bool     // if true, only inject AGENTS.md and TOOLS.md
	// PlanClarifyMode is one of "smart" / "auto" / "ask" — controls whether
	// the LLM asks clarifying questions before drafting a plan. Empty
	// defaults to "smart". Ignored for sub-agent prompts.
	PlanClarifyMode      string
	Query                string
	SessionID            string
	MemorySearcher       memory.Searcher
	ForceRelevantMemory  bool
	StaticBudgetTokens   int
	RelevantBudgetTokens int
	TotalBudgetTokens    int
	// PresetRelevant short-circuits the "## Prior Context" recall. When set,
	// the builder reuses this payload verbatim instead of querying
	// MemorySearcher. Callers that cache recall across turns must use this
	// rather than caching the assembled prompt: the static region depends on
	// live options (WorkDirs, CurrentDir, PlanClarifyMode, workspace files),
	// so replaying a stored prompt ships a stale one and defeats the very
	// provider prompt cache it was meant to help.
	PresetRelevant *PresetRelevantMemory
}

// PresetRelevantMemory is a previously computed "## Prior Context" section,
// handed back to the builder so a repeat semantic search can be skipped.
type PresetRelevantMemory struct {
	Section string
	Items   []RelevantMemoryItem
	Tokens  int
}

// RelevantMemoryItem is the structured form of one line injected into the
// "## Prior Context" prompt section.
type RelevantMemoryItem struct {
	Source    string `json:"source"`
	SourceTag string `json:"source_tag"`
	Snippet   string `json:"snippet"`
	Tokens    int    `json:"tokens"`
}

// BuildResult captures prompt assembly output and budget usage.
//
// Prompt is StaticPrompt+DynamicTail. The two are also exposed separately so
// the chat assembler can keep appending its own static sections (skills,
// session style, goal, critic) *before* the dynamic tail — see the ordering
// invariant on BuildResultFor.
type BuildResult struct {
	Prompt               string
	StaticPrompt         string
	DynamicTail          string
	StaticTokens         int
	RelevantTokens       int
	RelevantMemoryCount  int
	RelevantSection      string
	RelevantMemoryItems  []RelevantMemoryItem
	RelevantBudgetTokens int
	TotalTokens          int
}

// Build assembles a system prompt by reading workspace bootstrap files.
func Build(opts BuildOptions) string {
	return BuildResultFor(opts).Prompt
}

// BuildResultFor assembles a system prompt and returns budget usage details.
//
// ORDERING INVARIANT: static sections first, dynamic sections last.
//
// Provider prompt caching is prefix-matched — Anthropic against an explicit
// cache_control breakpoint, OpenAI and Gemini automatically. Anything that
// changes between two turns of the same session therefore has to sit behind
// everything that does not, or the cacheable prefix ends at the first byte
// that moved. Until LP-001 the wall-clock timestamp was the prompt's *first*
// line, so no prefix ever matched and the entire static body was re-charged
// at write rates on every single turn.
//
// Static (in order): Response Formatting, Planning, Long-running Commands,
// workspace bootstrap sections, Working Directories.
// Dynamic (BuildResult.DynamicTail, appended last): "## Prior Context"
// recall, then "## Current Time".
//
// The identity line (\"You are TARS, a personal AI assistant.\") was
// removed from the hardcoded header in ID-002(a). It now lives in the
// workspace IDENTITY.md default content, which is loaded as the
// \"## Identity\" bootstrap section below — that lets users override
// their assistant’s identity without recompiling. Response Formatting
// rules and the dynamic time line stay in code: they describe runtime
// constraints, not user-tunable persona.
func BuildResultFor(opts BuildOptions) BuildResult {
	var b strings.Builder

	b.WriteString("## Response Formatting\n\n")
	b.WriteString("Always use rich Markdown in your responses:\n")
	b.WriteString("- Use headings, bold, lists, and tables to structure information clearly.\n")
	b.WriteString("- Use fenced code blocks with language tags (```go, ```python, etc.) for any code.\n")
	b.WriteString("- When explaining architecture, flows, relationships, or processes, use Mermaid diagrams (```mermaid) proactively.\n")
	b.WriteString("- Prefer visual explanations (diagrams, tables) over long text when possible.\n")
	b.WriteString("\n")

	// Planning section is for the main agent only — sub-agents are spawned to
	// execute a single task and should not create their own plans.
	if !opts.SubAgent {
		mode := strings.ToLower(strings.TrimSpace(opts.PlanClarifyMode))
		switch mode {
		case "auto", "ask":
			// known modes; render below
		default:
			mode = "smart"
		}
		b.WriteString("## Planning\n\n")

		// Clarifying-questions stance varies by mode. Smart is the default
		// (LLM judges); auto skips the question step; ask always front-loads
		// 1–3 questions before drafting.
		switch mode {
		case "auto":
			b.WriteString("For multi-step requests (3+ steps), draft a plan immediately — do not ask clarifying questions.\n\n")
		case "ask":
			b.WriteString("For multi-step requests (3+ steps), ALWAYS ask 1–3 clarifying questions FIRST to disambiguate scope, success criteria, and constraints. Only after the user answers, start drafting the plan.\n\n")
		default:
			b.WriteString("For multi-step requests (3+ steps), evaluate ambiguity before drafting:\n")
			b.WriteString("- If success criteria, scope, or constraints are unclear → ask 1–3 clarifying questions FIRST, then draft after the user answers.\n")
			b.WriteString("- If clear → draft immediately.\n\n")
		}

		b.WriteString("Once you start drafting, use the `tasks` tool to propose the plan and then execute after the user approves:\n\n")
		b.WriteString("1. tasks(action=\"plan_set\", goal=..., scope=..., done_criteria=[...], verification_commands=[...], artifacts=[...]) — draft plan+contract\n")
		b.WriteString("2. tasks(action=\"add\", title=...) — one entry per step (still drafting)\n")
		b.WriteString("3. tasks(action=\"plan_propose\") — signal the plan is ready for the user to review (status=proposed)\n")
		b.WriteString("4. **STOP and wait** — say \"Plan and contract ready (N tasks). Reply 'go' to start, or describe changes.\"\n")
		b.WriteString("5. On changed criteria, tasks(action=\"contract_update\", ...). On `go`, tasks(action=\"contract_approve\"), tasks(action=\"plan_approve\"), then update one task to in_progress\n")
		b.WriteString("6. tasks(action=\"evidence_add\", task_id=..., type=..., summary=...) — attach test/log/image/PR/release proof before completing\n")
		b.WriteString("7. tasks(action=\"update\", id=..., status=\"completed\") — immediately on finish\n\n")
		b.WriteString("Mid-execution intervention:\n")
		b.WriteString("- If the user pauses, status flips to `paused` — stop and wait for instructions.\n")
		b.WriteString("- If the user edits tasks (different titles/order/added/removed), call tasks(action=\"list\") to re-read the plan before continuing.\n")
		b.WriteString("- If the user aborts, status flips to `aborted` — stop entirely.\n")
		b.WriteString("\n")
	}

	b.WriteString("## Long-running Commands\n\n")
	b.WriteString("For shell commands you expect to run longer than ~30s — builds, dependency installs, test suites, CI watchers like `gh pr checks --watch`, dev servers, deploys — DO NOT block the chat with foreground `exec`. Instead:\n\n")
	b.WriteString("1. Spawn with `exec` and `background:true` (returns a `session_id` immediately).\n")
	b.WriteString("2. Call the `process` tool's `wait` action with that `session_id` and a generous `timeout_ms`. This blocks server-side until the process exits, returning final stdout/stderr/exit_code in a single tool call — no polling loop needed.\n")
	b.WriteString("3. If the wait times out (`wait_timed_out:true`), decide: call `process wait` again with a longer timeout, `process log` to peek at output, or `process kill` to abort.\n\n")
	b.WriteString("Foreground `exec` is fine for fast commands (git status, ls, simple greps). Reach for `background:true` + `process wait` whenever the command might genuinely take more than half a minute.\n\n")

	totalBudgetTokens := opts.TotalBudgetTokens
	if totalBudgetTokens <= 0 {
		totalBudgetTokens = defaultTotalBudgetTokens
	}
	// The clock block is rendered last but charged here: it is always emitted,
	// so reserving it up front keeps the static sections clamped inside the
	// total budget exactly as they were when the timestamp led the prompt.
	timeSection := currentTimeSection()
	totalTokens := estimateTokens(b.String()) + estimateTokens(timeSection)
	remainingTotalTokens := max(0, totalBudgetTokens-totalTokens)

	remainingStaticTokens := opts.StaticBudgetTokens
	if remainingStaticTokens <= 0 {
		remainingStaticTokens = defaultStaticBudgetTokens
	}
	staticTokens := 0
	for _, section := range bootstrapSections {
		if opts.SubAgent && !section.subAgent {
			continue
		}
		if !opts.SubAgent && section.subAgent {
			continue
		}
		if remainingStaticTokens <= 0 || remainingTotalTokens <= 0 {
			break
		}
		content := readBootstrapSection(opts.WorkspaceDir, section)
		if content == "" {
			continue
		}
		content = trimToBudget(content, section.maxChars, max(0, min(remainingStaticTokens, remainingTotalTokens)-sectionHeaderTokenCost))
		if content == "" {
			continue
		}
		b.WriteString(fmt.Sprintf("## %s\n\n", section.name))
		b.WriteString(content)
		b.WriteString("\n\n")
		sectionTokens := estimateTokens(content) + sectionHeaderTokenCost
		staticTokens += sectionTokens
		totalTokens += sectionTokens
		remainingStaticTokens -= sectionTokens
		remainingTotalTokens -= sectionTokens
	}
	// Inject working directories section if session has work_dirs
	if len(opts.WorkDirs) > 0 {
		var dirSection strings.Builder
		dirSection.WriteString("## Working Directories\n\n")
		if opts.CurrentDir != "" {
			dirSection.WriteString(fmt.Sprintf("Current directory: `%s`\n", opts.CurrentDir))
		}
		dirSection.WriteString("Available directories:\n")
		for _, d := range opts.WorkDirs {
			if d == opts.CurrentDir {
				dirSection.WriteString(fmt.Sprintf("- `%s` (current)\n", d))
			} else {
				dirSection.WriteString(fmt.Sprintf("- `%s`\n", d))
			}
		}
		dirSection.WriteString("\nFile tool paths resolve relative to the current directory. Use absolute paths to access other directories.\n")
		// Add artifacts usage hint
		for _, d := range opts.WorkDirs {
			if strings.Contains(d, "/artifacts/") {
				dirSection.WriteString(fmt.Sprintf("Use `%s` for file outputs (reports, scripts, generated files).\n", d))
				break
			}
		}
		dirSection.WriteString("\n")
		content := dirSection.String()
		b.WriteString(content)
		sectionTokens := estimateTokens(content)
		staticTokens += sectionTokens
		totalTokens += sectionTokens
		remainingTotalTokens -= sectionTokens
	}

	// Everything written above is static for the lifetime of a session (it
	// only moves when a workspace bootstrap file or a session setting
	// changes). Everything below is per-turn and must stay behind it.
	staticPrompt := b.String()

	relevantTokens := 0
	relevantCount := 0
	relevantBudgetTokens := 0
	relevantSection := ""
	var relevantItems []RelevantMemoryItem
	usedTokens := 0
	var tail strings.Builder
	if !opts.SubAgent {
		relevantBudgetTokens = opts.RelevantBudgetTokens
		if relevantBudgetTokens <= 0 {
			relevantBudgetTokens = defaultRelevantBudgetTokens
		}
		relevantBudgetTokens = min(relevantBudgetTokens, remainingTotalTokens)
		if opts.PresetRelevant != nil {
			relevantSection = opts.PresetRelevant.Section
			relevantItems = append([]RelevantMemoryItem(nil), opts.PresetRelevant.Items...)
			usedTokens = opts.PresetRelevant.Tokens
		} else {
			relevantSection, relevantItems, usedTokens = buildRelevantMemorySection(opts, relevantBudgetTokens)
		}
		if relevantSection != "" {
			tail.WriteString(relevantSection)
			relevantTokens = usedTokens
			relevantCount = len(relevantItems)
			totalTokens += usedTokens
		}
	}

	// Recall changes with the user's query; the clock changes on its own. The
	// clock goes last so that re-running an identical query inside the same
	// minute still matches through the recall block.
	tail.WriteString(timeSection)

	dynamicTail := tail.String()
	return BuildResult{
		Prompt:               staticPrompt + dynamicTail,
		StaticPrompt:         staticPrompt,
		DynamicTail:          dynamicTail,
		StaticTokens:         staticTokens,
		RelevantTokens:       relevantTokens,
		RelevantMemoryCount:  relevantCount,
		RelevantSection:      relevantSection,
		RelevantMemoryItems:  relevantItems,
		RelevantBudgetTokens: relevantBudgetTokens,
		TotalTokens:          totalTokens,
	}
}

// timeNow is a seam so tests can build the prompt at two different wall-clock
// instants and compare the resulting prefixes.
var timeNow = time.Now

// currentTimeSection renders the dynamic clock block that closes every prompt.
//
// The timestamp is truncated to the minute rather than the second. Second
// resolution is more precision than any assistant answer needs, and it would
// re-break the prefix on every retry, tool-loop restart, or rapid follow-up —
// which is exactly the burst where a cache hit is worth the most.
func currentTimeSection() string {
	return fmt.Sprintf(
		"\n## Current Time\n\nCurrent time: %s\n",
		timeNow().UTC().Truncate(time.Minute).Format(time.RFC3339),
	)
}

func readBootstrapSection(workspaceDir string, section bootstrapSection) string {
	parts := make([]string, 0, len(section.files))
	for _, name := range section.files {
		content, err := readFileContent(filepath.Join(workspaceDir, name))
		if err != nil || strings.TrimSpace(content) == "" {
			continue
		}
		parts = append(parts, strings.TrimSpace(content))
	}
	if len(parts) == 0 {
		return ""
	}
	joined := strings.Join(parts, "\n\n")
	if section.maxChars > 0 && len(joined) > section.maxChars {
		joined = joined[:section.maxChars]
	}
	return joined
}

func readFileContent(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

func trimToBudget(content string, maxChars int, maxTokens int) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return ""
	}
	if maxChars > 0 && len(trimmed) > maxChars {
		trimmed = trimmed[:maxChars]
	}
	if maxTokens > 0 {
		maxCharsByTokens := maxTokens * 4
		if maxCharsByTokens <= 0 {
			return ""
		}
		if len(trimmed) > maxCharsByTokens {
			trimmed = trimmed[:maxCharsByTokens]
		}
	}
	return strings.TrimSpace(trimmed)
}

func estimateTokens(content string) int {
	if strings.TrimSpace(content) == "" {
		return 0
	}
	tokens := len(content) / 4
	if tokens < 1 {
		return 1
	}
	return tokens
}
