# Decision: Bots are the front of TARS and the workers of a pipeline

Status: proposed (draft, not accepted)
Date: 2026-11-21

## Context

The maintainer uses several coding CLIs side by side (Claude Code, Codex, Antigravity, Grok Build) and wants TARS to move between them quickly, keep each CLI's own settings, and let a TARS session be used together with the CLI it runs on.

Two products were read before writing this (2026-11-21):

- **Orca** (`stablyai/orca`) wraps any terminal agent in a worktree and puts many of them side by side. Its bots (beta) are named agents on the owner's machine that exchange three kinds of mail (`handoff`, `question`, `status`), act as their owner through connectors, and treat what they read as data, never as instructions. What its users value most is not orchestration (still experimental there) but "bring your own subscription" and seeing which agent waits on them.
- **Grok Bot** (SpaceXAI with Cursor, beta since 2026-08-11) is a team of named, always-on agents on one shared cloud computer. A bot has no settings page: it is changed by talking to it. A Primary Bot coordinates the others. Each bot has its own memory while facts about the user are shared. Its documentation says bots are not a security boundary, and reviewers criticise that the model cannot be chosen or seen and that there is no spend cap or audit view.

What TARS already has that points the same way: one persona (IDENTITY.md), CASE and its Bridge, the initiative loop that speaks first, Telegram and the phone's chat tab — all "talk to one character". What it also has that neither product does: a provider-neutral transcript per session, tiers the user picks, cost tracking, an automation audit, and a pipeline whose transitions are decided by facts the server reads.

What is in the way: the Advanced workbench exposes tiers, pins, docks and session config that a single user rarely needs to see, and three per-pipeline opt-ins (execution tier, second opinion, stage instructions) each answer the same question — *who does this stage* — in a different shape.

## Decision

### 1. A bot is a record, not a runtime

A bot is a small text record: `id`, `name`, a one-line `role`, an **engine** (a tier, or a provider alias and model), `instructions`, an optional permission mode **ceiling**, and a list of skills. It lives in `<workspace>/bots/*.yaml`, text only, no commands — the same rules as focus templates (`focuspipeline.Template`), including validation on load and built-in ids that cannot be overwritten.

Bots are never read from a repository's `.tars/`. A cloned repository must not choose the engine, widen a permission mode or add instructions that run on the owner's subscription; this is the reason `harness_proposal` and `exec_tier` live on the create request and not in `.tars/settings.json`.

No new process, registry or tool comes with a bot. Running a bot is running a chat turn with that engine and those instructions.

### 2. TARS is the primary bot

The main session is the primary bot and keeps IDENTITY.md as its voice. Other bots are picked directly (a new chat with a bot) or assigned to pipeline stages (§6). There is no coordinator model that decides which bot gets a job: the user picks, or the template says.

### 3. A session is bound to one bot at a time

Binding a session to a bot sets what a turn already reads today: the tier (as `tier_pin`), the instructions (as the session's prompt override layer) and the permission mode, never above the bot's ceiling and never above what the session's own rules allow. The session keeps its own cwd, work dirs, worktree and `.tars` overrides. Changing the bot of a session is a handoff (§4).

### 4. Changing engine is a handoff by transcript

The TARS transcript is the source of truth; an upstream CLI session is a cache of it. When a session moves to a bot whose engine is a different CLI kind, the server drops `UpstreamSessionID`, starts the new CLI with the transcript once, and writes a notice into the thread saying which bot took over. The new CLI runs in the session cwd and reads **its own** configuration there (`CLAUDE.md` and `.claude`, `AGENTS.md`, and so on) — TARS does not translate one CLI's settings into another's.

What does not cross: the previous CLI's internal context (tool results it kept, its own compaction). The handoff costs one full transcript and the notice says so.

### 5. Using the CLI alongside TARS: fork out, adopt in

- **Fork out.** A session on `claude-code-cli` offers the command `cd <cwd> && claude --resume <id> --fork-session`. The terminal gets a copy; the TARS session is untouched, and what happens in the terminal does not come back.
- **Adopt in.** A CLI session started in a terminal can become a TARS session: the server reads that CLI's saved conversation for display and sets it as the upstream session.
- **Never two writers on one upstream session.** Both directions are refused while a turn is running.

### 6. In a pipeline, bots are assigned to stages; transitions do not change

A template stage may name a bot. The turn of that stage runs with the bot's engine and instructions; the stage boundary is the handoff of §4, carried by the server through the stage guidance it already writes. Plan approval, verify exit codes, triage and the PR probe decide every transition exactly as now.

- Bots do not send each other messages inside a pipeline, and no bot is asked whether a stage is done. Goal mode's policy (`NextGoalStep`) is unchanged.
- Execution tier and second opinion become bot assignments (a builder bot; a reviewer bot for the first review round only). They stay **per-pipeline opt-ins, off by default**, with their measurement labels, until the measurements in `focus-mode.md` §5 are in. A pipeline that names no bots is byte-identical to today (`no_opt_in.golden`).
- A bot's permission ceiling can only narrow a stage. Goal mode's `auto` is still granted by the admin who turns goal mode on, not by a bot record.

### 7. Memory is shared in the first version

One user profile, one memory, one set of experiences for every bot. A bot differs by its instructions and engine only. Per-bot memory (Grok Bot's model: own memory, shared facts about the user) is left for later: with a single user it would mostly split what recall can find.

### 8. The console opens on bots; the workbench moves behind Advanced

The default screens become: talk to a bot, see what waits on you, and the focus pipeline. Tier pins, dock layout and session config stay where they are, under Advanced. **Nothing is removed.** A bot is created and edited by describing it, through the same draft → preview → approve flow as focus templates (`POST /v1/focus/templates/draft`): the model proposes a record, the server validates it, a person saves it.

### 9. What is not adopted

- A shared computer for bots, or connectors (Slack, Gmail, Linear) as built-in tools. External services stay skills with a companion CLI.
- Automatic model routing that the user cannot see. A bot's engine is written in its record and shown on every turn.
- Terminal (PTY) hosting and a catalogue of dozens of agents. An engine is added only when the maintainer uses it and it has a headless mode with resume.
- Bots as a security boundary. What a turn may do is decided by the session's permission mode and the unattended queue, as now.

## Relation to `product-focus.md`

This record does not fit the freeze as written, and it should not be accepted around it.

- The freeze runs to **2026-12-31**. `antigravity-cli` and `openai-codex` are Frozen, the workbench is Maintained, and a `feat:` outside Core needs a sentence naming the pipeline behaviour that requires it.
- §6 has that sentence: it replaces three per-pipeline opt-ins with one concept and makes "which engine did this stage run on" a first-class fact of a pipeline. §4 is what §6 stands on.
- §2, §8 and any new engine (a Codex CLI provider, Grok Build) are a change of product direction, not a pipeline need. They wait for the re-evaluation and are an input to it.

Proposed: phases 0 and 1 below may start before the review date because they touch only `claude-code-cli` and pipeline code; everything else is decided at the re-evaluation.

## Consequences

- One word for the user to learn (bot) in place of tier, pin, execution tier and second opinion. The cost is a layer of naming over things that already work, and a period in which both vocabularies are on screen.
- Handoffs make cross-engine cost visible: every switch replays a transcript. Long sessions will make that expensive, and the notice has to say how much.
- Console permission prompts exist only for `claude-code-cli`. A stage given to another CLI runs under that CLI's own policy, so an unattended pipeline is only as careful as its least careful engine. The bot picker must show this.
- Each engine is an adapter to maintain against a CLI that changes without notice. Orca absorbs this with thousands of commits a week; TARS cannot, which is why §9 keeps the list short.
- `pkg/session` and `pkg/llm` gain public fields (bot binding, adopt), so `make api-snapshot` changes.

## Open decisions

1. Whether a bot's engine is a **tier** (follows config changes) or a **provider alias and model** (stable, but breaks when the alias is renamed). Proposed: tier, with alias and model as an explicit override.
2. Whether a chat session may change bot mid-conversation, or only a new chat picks one. Proposed: allow it, as a visible handoff.
3. Whether adopt-in (§5) is worth building for more than `claude-code-cli`. Each CLI stores conversations differently.
4. Whether the subscription limit readout (the `rate_limit_event` in the Claude CLI stream, the `x-codex-*` snapshot that already exists) belongs next to the bot picker, so "this engine is at 95%" is seen where the switch is made. Proposed: yes; it is the commonest reason to switch.
5. Whether the primary bot may hand a question to another bot by itself. Proposed: no in the first version; a person picks.

## Phases

0. **Measure the handoff.** Verify what happens today when a session's tier pin moves between CLI kinds, and what one transcript replay costs on real sessions. No product change.
1. **Use alongside, Claude only.** Fork out and adopt in for `claude-code-cli` (§5). Small, and it tests whether using TARS and the CLI together is wanted at all.
2. **Bot records and stage assignment.** `<workspace>/bots/`, the handoff of §4 between the two CLI providers that exist, bots on template stages with execution tier and second opinion expressed through them (§6).
3. **The front.** Bot picker and bot chat as the default screens, the draft → preview → approve editor (§8). Only after the re-evaluation.
4. **More engines.** A Codex CLI provider, then Grok Build if its headless mode holds up. Only after the re-evaluation.

After phase 2, compare pipelines with and without assigned bots using the existing execution report (`GET /v1/focus/exec-report`) before going further.

## Re-evaluate when

- Phase 1 ships and fork out / adopt in go unused for a month. Then "use together with the CLI" was not the need, and §5 stops there.
- A handoff regularly costs more than the stage it hands to. Then engines should be chosen per pipeline, not per stage.
- A CLI the pipeline runs on ships named agents with stage assignment natively. This is `product-focus.md`'s second "wrong decision" criterion, seen from this side.
