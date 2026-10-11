// Pure helpers for CASE's bubble question-and-answer (tars#1236). The turn
// is a normal chat turn on the *main* session — no new server endpoint, no
// new session, same `console_context` guidance the old new-session handoff
// used (lib/companion.ts's `companionAskContext`). Everything here is a
// plain function of its inputs so it can run under plain Node tests; the
// impure shell (resolving the main session, calling streamChat) lives in
// lib/stores/companionAskStore.svelte.ts.
import type { ActivityMap } from './sessionBoard'
import type { AgentRuntimeTierOption, ChatRequest, ChatTierRecommendationRequest, Session } from './types'

// --- Building the turn ---

export type CompanionAskRequestInput = {
  message: string
  consoleContext: string
  tierRecommendation?: ChatTierRecommendationRequest
}

// companionAskRequest is the chat turn CASE's bubble sends: the main
// session (the server resolves "main" to the real id, same as every other
// console caller that does not yet know it — handler_chat.go
// resolveChatSession), the companion's console_context, and (usually) a
// light tier_recommendation with no `pin` — it applies to this turn only,
// the session's own tier_pin is left untouched (resolveChatTierRecommendation
// only pins when the request asks it to). Permission questions still wait
// for an answer via the console's existing approval path.
export function companionAskRequest(input: CompanionAskRequestInput): ChatRequest {
  return {
    message: input.message,
    session_id: 'main',
    console_context: input.consoleContext,
    tier_recommendation: input.tierRecommendation,
    interactive_permissions: true,
  }
}

const COMPANION_ASK_LIGHT_RECOMMENDATION: ChatTierRecommendationRequest = {
  task_type: 'companion_ask',
  recommended_tier: 'light',
  chosen_tier: 'light',
  reason: 'Companion bubble question (tars#1236): short check-ins run on the light tier.',
  confidence: 1,
  accepted: true,
  source: 'companion',
}

// CLI providers that resume an upstream session by id (internal CLAUDE.md
// "resume provider's hidden context" / tars#857, #1236 review). Every call
// already carries its own --model, so resuming the *same* kind on a
// different tier/model is safe; only a mismatched kind is not.
const CLI_RESUME_KINDS: ReadonlySet<string> = new Set(['claude-code-cli', 'antigravity-cli'])

function tierKindOf(tierName: string, tierOptions: readonly Pick<AgentRuntimeTierOption, 'name' | 'kind'>[]): string {
  const name = tierName.trim()
  if (!name) return ''
  return tierOptions.find((option) => option.name === name)?.kind?.trim() ?? ''
}

// companionAskTierRecommendation decides whether CASE's turn can safely use
// the light tier. It is skipped (the turn falls back to the session's own
// tier — tier_pin, or the console's default) only when the main session
// already resumes a CLI provider session (upstream_session_id set) and the
// light tier resolves to a *different* CLI kind: the resume id would then
// go to the wrong CLI. pkg/llm/claude_code_cli.go retries with a fresh
// session when its own resume id is unknown, but pkg/llm/antigravity_cli.go
// has no such fallback — that turn would fail outright. Same-kind CLI
// resume (a different model, same CLI) is unaffected: handler_chat_execution.go
// always passes the stored upstream id regardless of which tier/model this
// turn resolves to, and the CLI's own --resume/--conversation flag is what
// matters, not the model. A light tier that resolves to a native provider
// is safe too — native clients simply ignore ResumeSessionID.
export function companionAskTierRecommendation(
  mainSession: Pick<Session, 'tier_pin' | 'upstream_session_id'>,
  tierOptions: readonly Pick<AgentRuntimeTierOption, 'name' | 'kind'>[],
  defaultTier: string,
): ChatTierRecommendationRequest | undefined {
  const upstream = (mainSession.upstream_session_id ?? '').trim()
  if (upstream) {
    const mainTierName = (mainSession.tier_pin ?? '').trim() || defaultTier.trim()
    const mainKind = tierKindOf(mainTierName, tierOptions)
    const lightKind = tierKindOf('light', tierOptions)
    if (CLI_RESUME_KINDS.has(mainKind) && lightKind !== mainKind) {
      return undefined
    }
  }
  return COMPANION_ASK_LIGHT_RECOMMENDATION
}

// isMainSessionBusy reads the same chat-activity snapshot CASE's lines use
// (lib/sessionBoard.ts ActivityMap, polled every few seconds by
// lib/stores/sessionActivity.svelte.ts) so the console can refuse to send
// without an extra round trip — a 409 from the server is still the
// fallback for the race where the main session started a turn in between.
export function isMainSessionBusy(activity: ActivityMap, mainSessionId: string): boolean {
  if (!mainSessionId) return false
  return Boolean(activity[mainSessionId]?.running)
}

// --- Reading the answer ---

export const COMPANION_ASK_ANSWER_MAX_LEN = 480

// truncateCompanionAnswer caps what the bubble shows — the full answer
// already lives in the main session's transcript, which "Continue in chat"
// opens. Cuts on a word/line boundary when one is close enough to the
// limit so a long reply does not end mid-word.
export function truncateCompanionAnswer(text: string, maxLen = COMPANION_ASK_ANSWER_MAX_LEN): { shown: string; truncated: boolean } {
  if (text.length <= maxLen) return { shown: text, truncated: false }
  let cut = text.slice(0, maxLen)
  const lastBreak = Math.max(cut.lastIndexOf(' '), cut.lastIndexOf('\n'))
  if (lastBreak > maxLen * 0.6) cut = cut.slice(0, lastBreak)
  return { shown: `${cut.trimEnd()}…`, truncated: true }
}

// --- Failure classification ---

export type CompanionAskErrorKind = 'busy' | 'network' | 'limit' | 'error'

// classifyCompanionAskError turns a streamChat() rejection into a short,
// generic reason for the bubble (never the raw server message — it may
// name sessions, paths, or provider details that do not belong in a
// four-word bubble note). Duck-types the status like
// lib/stores/focusStore.svelte.ts's statusOf rather than importing
// APIRequestError, keeping this module framework/client-free.
export function classifyCompanionAskError(err: unknown): CompanionAskErrorKind {
  const status = (err as { status?: unknown } | null)?.status
  if (typeof status === 'number') {
    if (status === 409) return 'busy'
    if (status === 429) return 'limit'
    return 'error'
  }
  if (err instanceof TypeError) return 'network'
  return 'error'
}

// --- Keeping CASE's own turn out of its own waiting list ---

// suppressRunningLine drops the "running" line the main session's own
// chat-activity entry would otherwise add to CASE's waiting list while
// CASE's own bubble turn is in flight on that same session (tars#1236
// requirement 6) — the bubble already shows it is working. Approval
// ("pending"/"queued") lines for the main session are left alone: a tool
// permission question still needs the existing waiting-list treatment.
export function suppressRunningLine<T extends { kind: string; key: string }>(
  lines: T[],
  mainSessionId: string | null | undefined,
): T[] {
  if (!mainSessionId) return lines
  const runningKey = `running:${mainSessionId}`
  return lines.filter((line) => line.key !== runningKey)
}

// --- Expression while asking ---

// companionAskExpressionOverride forces CASE's `working` face while its
// own bubble turn streams, unless something more urgent is already
// showing in `lines` (a real failure, or an approval wait — the same
// order lib/companion.ts's companionExpression uses) — returns null in
// that case so the caller falls back to the normal companionExpression
// result instead of fighting it.
export function companionAskExpressionOverride(
  lines: readonly { kind: string }[],
  asking: boolean,
): 'working' | null {
  if (!asking) return null
  if (lines.some((line) => line.kind === 'failure' || line.kind === 'pending' || line.kind === 'queued')) return null
  return 'working'
}
