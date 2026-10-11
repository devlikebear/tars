import type { ChatApproval } from './chatApproval.ts'
import type { ToolFileChange } from './toolFileChanges.ts'

export type ToolOutputLine = {
  stream: 'stdout' | 'stderr' | string
  text: string
}

export type ChatMessage = {
  id: string
  sourceMessageId?: string
  role: 'user' | 'assistant' | 'system' | 'error' | 'tool' | 'approval'
  text: string
  // role 'approval': a tool call waiting on the user's decision (#970).
  approval?: ChatApproval
  reasoningText?: string
  toolName?: string
  toolCallId?: string
  toolArgs?: string
  toolResult?: string
  toolDone?: boolean
  toolIsError?: boolean
  toolStartedAt?: number
  toolFinishedAt?: number
  // A tool a CLI provider ran itself (SSE `provider_tool`), not TARS.
  toolUpstream?: boolean
  // Streaming stdout/stderr lines emitted while the tool runs.
  // Currently populated by exec via SSE `tool_output_line` events.
  toolOutputLines?: ToolOutputLine[]
  // Files the call changed, from SSE `file_change` events (#1032). Only
  // native providers send them; absent for CLI providers and reloaded history.
  toolFileChanges?: ToolFileChange[]
  usage?: {
    input_tokens: number
    output_tokens: number
    cached_tokens: number
    cache_read_tokens: number
    cache_write_tokens: number
  }
  // Set when TARS wrote this message on its own, not in reply to a chat
  // turn (tars#1220's live mode). Only the "spoke first" badge reads it —
  // everything else treats the message like any other assistant reply.
  spokeFirst?: boolean
}
