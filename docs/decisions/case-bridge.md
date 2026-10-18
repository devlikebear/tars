# Decision: CASE's Bridge answers from a server-built status, in its own session

Status: accepted
Plan: `docs/plans/case-companion/phase-2-bridge.md`
Date: 2026-10-18

## Context

CASE, the console companion, could already take a question in its bubble (tars#1236): the words became a normal chat turn on the **main session**. That is the right place for "talk to TARS", but not for "what is going on in TARS?" — the main session has no view of focus pipelines, other sessions' turns, the ops queue, cron, cost or pulse, and every such question would pile into its transcript.

The plan proposed a `tars_status` app tool plus a dedicated read-only session. Two facts changed the first half:

1. **CLI providers cannot call TARS tools.** claude-code-cli and antigravity-cli lack `CapToolCalling` (`pkg/llm/capabilities.go`); TARS never hands them tool schemas. A CASE on such a tier would never see the status.
2. **A registered tool rides on every turn.** `buildChatToolRegistry` (`handler_chat_policy.go`) builds every session's registry from the same list, and an allowlist only narrows sessions that have one. CLAUDE.md's extension rule — no new builtin tools for domain features — exists for exactly this.

## Decision

- **The status is computed in Go and handed to the turn, not fetched by the model.** `internal/casebridge` turns a `Snapshot` (pipelines, running turns, chat and queued approvals, cron failures in the last 24h, today's and this week's cost, pulse's latest warnings) into a short text with ids in brackets (`Render`, ≤ 4 KB). When a Bridge question starts, the server takes the snapshot and the turn of CASE's session gets it as a `<tars-status>` section of its turn context (`joinTurnContext` in `handler_chat_context.go`) — after the history, not saved in the transcript, and on no other session's turn. Works the same on every provider; nothing is added to any prompt but CASE's.
- **One hidden worker session per day.** `case_bridge.go` makes a `worker` session titled "CASE Bridge", hidden, isolation off, permission mode plan, tier pinned to light, and keeps its id with the local day in `<workspace>/_shared/case/bridge.json`. A new day starts a new session so a resuming CLI provider's context never grows past a day of questions. The main session never sees a Bridge question.
- **Read-only by construction.** The turn runs unattended with `readOnly` (no repository lease), and `case_bridge` is one of `unattendedReadOnlySources`: a call its mode would ask about is refused at once with a message that points back at the status — never queued for approval. The instructions tell CASE it cannot change anything and must point to the screen instead.
- **Cards are the server's.** CASE may end with `<case-cards>[{kind, id, title, sub, severity}]</case-cards>`. `casebridge.SplitCards` takes the block out of the answer and `ValidCards` keeps only cards whose id is in the snapshot the question was asked against (or whose screen has something to show), at most four; the link is always built by the server. A card the model invents is dropped.
- **API.** `POST /v1/case/ask {question, scope, view}` → 202 `{session_id}` (409 while a question is being answered, 503 before the chat handler is bound). The console streams the turn from `GET /v1/chat/stream?session_id=` like any turn, then reads `GET /v1/case/answer` for the answer without its block and the checked cards. User token, not admin — the Bridge only reads.
- **Console.** `CaseBridge.svelte`, a panel on the right of every screen: where the user is looking, a scope toggle (all of TARS / this screen, the latter only where a chat or pipeline is on screen), the conversation as plain text, cards, follow-up chips and the input. Mod+/ toggles it (Mod+J is the terminal's; Mod+Shift+J is the browser console on Windows), so do the palette and a link in CASE's bubble. It goes with CASE: off when the companion is off.
- **The bubble question stays.** tars#1236's main-session question is unchanged; the bubble gains a link to the Bridge. Moving the bubble question onto the Bridge is left for later — it would rewrite that feature's store, specs and in-thread forwarding for a change of destination only.

## Consequences

- A Bridge answer is only as fresh as the moment it was asked, and says so ("TARS status at …").
- Adding a source to the status means a field on `casebridge.Snapshot`, a line in `Render`, and filling it in `snapshotLocked`; the 4 KB cap and the five-per-section limit keep the prompt small.
- The status names session titles, tool names and cron errors (clipped) to the light tier's provider. That provider already receives chat text; the Bridge sends nothing a chat turn would not.

## Re-evaluate when

- A provider TARS uses gains tool calling for every tier and per-session tool registration becomes cheap — a tool would then let CASE look further than one snapshot.
- The Bridge needs history across days; the per-day session would have to become a summary instead.
