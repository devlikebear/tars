# Decision: Hand a session back and forth between TARS and Claude Code

Status: proposed (draft, not accepted). Since 2026-11-21 this is an expert-mode feature (`vibe-coder-identity.md` §4): it is for someone who also runs Claude Code, and its phase 1 is built in that record's phase 7. The console entry points named below (New chat, the session header, the focus New task screen) are expert-mode screens
Date: 2026-11-21
Builds on: `bot-mode.md` §5 and its phase 1 ("use alongside, Claude only"). This record makes that phase concrete, extends it to focus pipelines, and amends one sentence of it (§6 below).

## Context

The maintainer wants TARS to be the main tool while still opening Claude Code in a terminal: start a change in Claude Code, move to TARS and finish it in a focus pipeline, and go back to the terminal when that is quicker. Today there is no way to do this without retelling the whole context.

What already exists:

- The `claude-code-cli` provider stores the CLI's `session_id` on `Session.UpstreamSessionID` and passes it back as `--resume` on the next turn (`handler_chat_execution.go`). It records whatever id the CLI returns, including one carried by a failed call (`llm.UpstreamSessionError`).
- Claude Code saves every conversation as JSON lines in `<config dir>/projects/<encoded cwd>/<session id>.jsonl`, where the config dir is `$CLAUDE_CONFIG_DIR` or `~/.claude` (the model list already reads that dir the same way, `pkg/llm/claude_code_cli_models.go`).
- `claude --resume <id> --fork-session` (checked on 2.1.280) continues a conversation under a **new** id and leaves the original untouched.
- A resume works from another folder (CLAUDE.md, checked on 2.1.270).
- `POST /v1/admin/sessions {title, cwd, isolate, extra_dirs}` already creates a session in a folder, and `POST /v1/focus/pipelines` uses the same path.

What is in the way:

- The JSONL file is **not a documented format**. One file read for this record held 19 record types (`user`, `assistant`, `attachment`, `api-request`, `custom-title`, `file-history-snapshot`, …) and about 50 fields on a message record. It will change without notice.
- Two processes resuming the same upstream id write two different continuations of one conversation. Neither side can tell.
- A TARS transcript and a CLI conversation are different things. TARS sends only the new message on a resumed turn, so whatever the CLI did without TARS (in a terminal) is missing from the TARS transcript.

## Decision

### 1. The TARS transcript is the record; the CLI conversation is a cache

This is `bot-mode.md` §4 and it holds here too. Anything that comes back from Claude Code is copied **into** the TARS transcript. TARS never writes into Claude Code's files, and it never translates `.claude` settings into `.tars` settings or the other way. Both tools run in the same cwd, so each reads its own project config there.

### 2. One writer per upstream id: TARS always forks what it adopts

TARS never continues an upstream id that it did not create. The first TARS turn on an adopted conversation runs `--resume <id> --fork-session`. From then on TARS resumes its own fork. The terminal's conversation stays the terminal's, even if that Claude Code window is still open.

In the other direction, the command TARS offers is also a fork (§5). So every upstream id has exactly one writer, without locks or checking file times, and nothing has to guess whether a terminal is still open.

### 3. A read-only reader for Claude Code conversations

Add a new core package, `internal/claudecodelog`, that uses only the standard library and is classified in `internal/architecture/layers.go`. It does three things:

- **Find** a conversation by id. The id must parse as a UUID before it becomes part of a path, and only files directly under `<config dir>/projects/*/` are opened. The directory name encodes the cwd lossily (`/` and `.` both become `-`), so it is never decoded back into a path.
- **List** the conversations whose records carry a given `cwd`. The match uses the `cwd` field inside the file, and worktree paths count as their own cwd. Each entry has the id, the title (`custom-title`, otherwise the first user text), the last timestamp, `gitBranch`, and a message count. Newest first, capped at 20, reading only the head and tail of each file.
- **Read** the messages. Only `user` and `assistant` records count. Records with `isSidechain`, `isMeta` or `isCompactSummary` are skipped, and so are tool-result-only user records. Only text blocks are kept. One assistant reply is written as several records (one per content block, the first often empty), so records are grouped by `message.id` into one message. An assistant record's `tool_use` blocks are kept as a count, not as content. Unknown record types and fields are ignored. A line that does not parse is skipped and counted, so it never fails the read.

The reader is tolerant on purpose. When the format changes, the worst outcome should be a thinner imported transcript, not a failed handoff. A fixture taken from a real file (scrubbed: no absolute home paths, no keys, per `make security-scan`) pins the current shape.

### 4. Adopt in: a Claude Code conversation becomes a TARS session

`POST /v1/admin/sessions` gains `adopt: {provider: "claude-code-cli", session_id}`. It is admin only, the same right as starting a chat in an arbitrary folder, because it reads a local file chosen by the caller. The server:

1. Finds and reads the conversation (§3). The session's cwd is the conversation's `cwd`, and it must still exist. If the request also names a `cwd`, it must match, or the request is a 400.
2. Picks the tier. If the chat tier's provider is `claude-code-cli`, it uses that tier. Otherwise it uses the first tier bound to a `claude-code-cli` provider and saves it as `tier_pin`. If there is none, the request is a 400: a native provider cannot resume, and v1 does not summarise as a fallback.
3. Copies the messages into the transcript. User messages get `Origin: "claude_code"`, a new value, so initiative does not count them as the person typing now (CLAUDE.md, "사람이 친 메시지만 활동으로 센다"). They are display only: a resumed turn never sends them again.
4. Records `Session.Adopted {provider, session_id, at}` and sets `UpstreamSessionID` to the adopted id, marked "fork on next turn" (§2).
5. Writes a notice line into the thread, "Continued from a Claude Code conversation (N messages, M tool calls not shown)", so it is clear the history above is a copy.

`GET /v1/admin/claude-sessions?cwd=` exposes the list. The console shows it in two places: **New chat ▾ → From Claude Code…**, and a "Start from a Claude Code conversation" picker on the focus New task screen.

### 5. Fork out: continue a TARS session in a terminal

A session on `claude-code-cli` with an `UpstreamSessionID` shows **Open in Claude Code** in its header. The button copies `cd <cwd> && claude --resume <id> --fork-session`. The `<cwd>` is the session's active cwd, which is the worktree path for an isolated session. The TARS session is not changed. The button is disabled while a turn is running, because the id may change when that turn ends.

### 6. Bring back: the terminal's continuation returns to the same TARS session

This amends `bot-mode.md` §5, which says "what happens in the terminal does not come back". Going back and forth is the point of this record, so it does come back, under one rule.

`POST /v1/admin/sessions/{id}/adopt {session_id}` (admin, 409 while a turn runs) appends the terminal conversation's new messages to an existing TARS session. It is allowed only when the terminal conversation **contains the record `uuid` of the last message in TARS's own upstream conversation** (the file of the session's current `UpstreamSessionID`, read at bring-back time). A fork copies earlier records with their `uuid`s unchanged (phase 0). So this proves the fork was taken from where TARS stopped and that TARS has not moved on since: a later TARS turn would have a newer last `uuid` that the fork does not have. TARS does no per-turn bookkeeping for this. In that case:

- The server appends only the records after that `uuid`, with the same marking as §4.
- `UpstreamSessionID` becomes the terminal's id, marked "fork on next turn" (§2).
- The session keeps its tier, cwd, worktree, permission mode and, for a pipeline, its stage.

If TARS has taken a turn since the fork, the two sides have diverged. The server refuses with a 409 and does not try to merge. The console then offers to adopt the terminal conversation as a new session (§4).

The console finds candidates by listing conversations in the session's cwd (§3) and keeping the ones that pass the rule. It offers them as **Bring back from Claude Code** in the session header.

### 7. Focus: start a pipeline from a Claude Code conversation

`POST /v1/focus/pipelines` gains `adopt: {session_id}`. It is admin only, and 403 for the `focus` role, because a phone must not name a local file. The pipeline session is created by §4. Then:

- **Folder and isolation.** The work so far lives in the conversation's cwd: commits on its branch, and often uncommitted changes. The adopted pipeline therefore runs there by default (`isolate: false`). With `isolate: true`:
  - If the tree has uncommitted changes, the request is a 400, because they would not move.
  - If the tree is clean, the worktree branches from the cwd's **HEAD**, not from the remote default branch as other feature pipelines do (`focus-mode.md` §4). That way the commits made in the terminal come along. The response's `worktree_base` says which.
- **Plan stage.** The plan stage starts as usual. Its kickoff says that the conversation so far happened in Claude Code, that work already done goes in the plan as done tasks, and that the plan covers what remains. G1 is unchanged, and so is goal mode's policy (`NextGoalStep`).
- **Opt-ins.** All other per-pipeline opt-ins mean what they mean today. An `exec_tier` of another CLI kind is refused by the existing `ExecTierSwitchSafe` notice.
- **Bring back.** Bring back (§6) works on a pipeline's session as on any session, and it changes no stage. The next verify decides, as always.

### 8. What is not done

- **No live mirror.** TARS does not watch Claude Code's files. Copying happens only on adopt and bring back, at a person's request.
- **No other CLIs.** `antigravity-cli` and any later engine stay out (`bot-mode.md` open decision 3).
- **No summary fallback** for native providers in v1.
- **No `tars-skills` command inside Claude Code** that pushes the conversation to TARS. Claude Code hooks receive `session_id`, so it is possible later as a skill with a companion CLI, never as a TARS builtin tool.
- **No merging** of diverged conversations.

## Relation to `product-focus.md`

`claude-code-cli`, sessions and the focus pipeline are Core. §7 is the pipeline need: a pipeline can pick up work that was already started in the CLI instead of starting over from a typed goal. §4–§6 are what §7 stands on. Like `bot-mode.md` phase 1, this touches only `claude-code-cli`, sessions and pipeline code.

## Consequences

- **The format can break.** TARS now depends on an undocumented file format. A change in it degrades adopt and bring back (fewer messages, or none). Chat and pipelines are not affected. `tars doctor` reports whether the reader understood the newest local conversation, so the break shows up there before it surprises anyone.
- **Every adopt and bring back costs a fork.** A fork's `total_cost_usd` is the parent's running total plus its own call (phase 0). The existing rule records the first call of an unknown id as 0 (`claude_code_cli_cost.go`), which is right here: recording the reported value would bill the parent's whole history again. Usage readouts undercount by that one call. This is accepted, as for the first resume after a restart.
- **Imported history is display only.** Tool calls made in Claude Code show as a count, not as cards. That is less than a native TARS turn shows, and the notice line says so.
- **Public API changes.** `pkg/session` gains `Adopted` and the `claude_code` origin value. `pkg/llm` gains a fork flag on `ChatOptions`. `make api-snapshot` changes.
- **New behaviour on adopted conversations.** A TARS session adopted from Claude Code never continues the terminal's id. Someone who expects the two windows to share one live conversation gets two forks instead. Both buttons say "fork" for that reason.

## Open decisions

1. ~~Whether bring back's rule (§6) should compare record `uuid`s instead of message text.~~ Settled by phase 0: a fork keeps them, so §6 compares `uuid`s.
2. Whether adopt should also import Claude Code's `/compact` summaries as a collapsed block. Proposed: no. TARS's own compaction does not apply to a resumed CLI session anyway (`sessionHealth.ts`).
3. Whether an adopted pipeline should default to `isolate: true` when the tree is clean. Proposed: no. Starting where the terminal was is less surprising.

## Phases

0. **Measure.** Done 2026-11-21; results below. No product change.
1. **Reader, adopt in, fork out.** `internal/claudecodelog`, §2, §4 and §5, in chat only.
2. **Bring back.** §6.
3. **Focus adopt.** §7, including the New task picker.

## Phase 0 results

Measured on 2026-11-21 with Claude Code 2.1.280, model `haiku`, in an empty git repository. The run was 7 headless calls costing about $0.06 in total.

| Question | Result | Effect on this record |
| --- | --- | --- |
| Does `-p --resume <id>` keep `<id>`? | Yes. Two resumes of one id returned the same id, and the file grew in place. | TARS's own conversation keeps one id across turns, so §6 can read "TARS's own upstream file" from `UpstreamSessionID`. |
| Does `--fork-session` work with `-p`? | Yes. It returned a new id, and the reply showed the earlier turns were in context. | §2 and §4 work as written. |
| Does it work with the interactive permission path (`--input-format stream-json --permission-prompt-tool stdio`)? | Yes. The new id already appears in the `system`/`init` event, before the result. | A fork that fails mid-turn still reports its id, so the existing `UpstreamSessionError` path keeps it. |
| Does forking change the original? | No. A resume of the original after the fork did not see the forked turn. | One writer per id holds (§2). |
| Does a fork copy earlier records? | Yes, all of them, with the same `uuid` and `timestamp`. `sessionId` is rewritten to the new id. | §6 compares `uuid`s (open decision 1). |
| Where does a fork from another folder land? | Under the **new** cwd's project dir. Every copied record's `cwd` is rewritten to the new cwd. | Find-by-id must scan all project dirs (§3). A terminal fork taken in the session's cwd is listed under that cwd. |
| What does a fork report as `total_cost_usd`? | The parent's running total plus its own call, e.g. 0.0346 then 0.0393 for a call of about 0.0047. | The existing "unknown id → 0" rule is the right one (Consequences). |
| How is an assistant reply stored? | As several records sharing `message.id`: an empty one, then the text. | The reader groups by `message.id` (§3). |

Not measured: the interactive TUI (`claude --resume <id> --fork-session` without `-p`). It uses the same flags, and it is checked by hand in phase 1.

## Re-evaluate when

- Phase 2 ships and adopt or bring back goes unused for a month. This is the same test as `bot-mode.md` phase 1. Then going back and forth was not the need, and the feature stays as it is.
- Claude Code documents a session export or import format. Switch the reader to it.
- The reader breaks twice in a quarter on a CLI update. Then drop the transcript import and keep only the fork commands, which depend on documented flags alone.
