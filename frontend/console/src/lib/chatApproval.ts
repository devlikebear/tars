// Inline tool approvals in the chat thread (#970). The server streams a
// permission_request when Claude Code asks before a tool call and a
// permission_resolved when the prompt closes; the console answers with
// POST /v1/chat/permissions/{request_id}. Pure helpers, tested under Node.
import type { ChatMessage } from './chatMessages.ts'

export type ChatApprovalDecision = 'allow_once' | 'allow_session' | 'allow_always' | 'deny'

// sending: answered here, waiting for the server to confirm.
// withdrawn: the turn ended (or Claude Code dropped the question) first.
export type ChatApprovalState = 'pending' | 'sending' | 'allowed' | 'allowed_session' | 'allowed_always' | 'denied' | 'withdrawn'

export type ChatApproval = {
  requestId: string
  sessionId: string
  toolName: string
  toolUseId?: string
  input?: unknown
  title?: string
  description?: string
  reason?: string
  // Set when a subagent made the call.
  agentId?: string
  // The rule "allow for this session" adds, e.g. Bash(npm test:*). Absent
  // when the server offers none (compound commands, rm, sudo, ...).
  sessionRule?: string
  // The folder "always allow" would cover, remembered by TARS across
  // sessions and restarts. Absent when the server does not offer it.
  alwaysDir?: string
  state: ChatApprovalState
  error?: string
}

export type ChatApprovalPreview = {
  kind: 'command' | 'file' | 'url' | 'input'
  text: string
}

// The fields of a chat SSE event this module reads.
type PermissionEvent = {
  type?: string
  session_id?: string
  request_id?: string
  tool_name?: string
  tool_use_id?: string
  input?: unknown
  title?: string
  description?: string
  reason?: string
  agent_id?: string
  session_rule?: string
  always_dir?: string
}

function text(value: unknown): string | undefined {
  return typeof value === 'string' && value.trim() !== '' ? value.trim() : undefined
}

export function approvalFromEvent(event: PermissionEvent): ChatApproval | null {
  if (event.type !== 'permission_request') return null
  const requestId = text(event.request_id)
  const sessionId = text(event.session_id)
  if (!requestId || !sessionId) return null
  return {
    requestId,
    sessionId,
    toolName: text(event.tool_name) ?? 'tool',
    toolUseId: text(event.tool_use_id),
    input: event.input,
    title: text(event.title),
    description: text(event.description),
    reason: text(event.reason),
    agentId: text(event.agent_id),
    sessionRule: text(event.session_rule),
    alwaysDir: text(event.always_dir),
    state: 'pending',
  }
}

// approvalPreview picks what a person needs to judge the call: the command,
// the file, the URL, or failing those the whole input.
export function approvalPreview(approval: ChatApproval): ChatApprovalPreview {
  const input = approval.input && typeof approval.input === 'object' ? (approval.input as Record<string, unknown>) : {}
  const command = text(input.command)
  if (command) return { kind: 'command', text: command }
  const file = text(input.file_path) ?? text(input.notebook_path) ?? text(input.path)
  if (file) return { kind: 'file', text: file }
  const url = text(input.url)
  if (url) return { kind: 'url', text: url }
  return { kind: 'input', text: approval.input === undefined ? '' : JSON.stringify(approval.input, null, 2) }
}

const outcomes: ReadonlySet<string> = new Set(['allowed', 'allowed_session', 'allowed_always', 'denied', 'withdrawn'])

// resolveApproval returns messages with the card for requestId settled, or
// the same array when no card matches.
export function resolveApproval(messages: ChatMessage[], requestId: string, outcome: string): ChatMessage[] {
  const idx = messages.findIndex((m) => m.approval?.requestId === requestId)
  if (idx < 0) return messages
  const state = (outcomes.has(outcome) ? outcome : 'withdrawn') as ChatApprovalState
  const next = [...messages]
  next[idx] = { ...next[idx], approval: { ...(next[idx].approval as ChatApproval), state, error: undefined } }
  return next
}

// withdrawPendingApprovals closes the cards a finished stream left open: the
// turn is over, so nothing waits for them any more. Returns the same array
// when none are open.
export function withdrawPendingApprovals(messages: ChatMessage[]): ChatMessage[] {
  const open = (m: ChatMessage) => m.approval?.state === 'pending' || m.approval?.state === 'sending'
  if (!messages.some(open)) return messages
  return messages.map((m) => (open(m) ? { ...m, approval: { ...(m.approval as ChatApproval), state: 'withdrawn' } } : m))
}

// decisionForKey maps the card's single-key shortcuts (y, s, a, n). s needs a
// session rule, a an always folder, and a card that is not pending takes no
// keys.
export function decisionForKey(key: string, approval: ChatApproval): ChatApprovalDecision | null {
  if (approval.state !== 'pending') return null
  switch (key.toLowerCase()) {
    case 'y':
      return 'allow_once'
    case 's':
      return approval.sessionRule ? 'allow_session' : null
    case 'a':
      return approval.alwaysDir ? 'allow_always' : null
    case 'n':
      return 'deny'
  }
  return null
}
