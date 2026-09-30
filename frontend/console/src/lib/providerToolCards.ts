// Tool cards for tools a CLI provider (claude-code-cli) runs itself. The
// server streams `provider_tool` when the CLI starts a tool and
// `provider_tool_result` when its result comes back — separate from the
// native before/after_tool_call phases so TARS never counts them as its
// own tool calls — and the console draws both into the same card a native
// tool gets. Pure helpers, tested under Node.
import type { ChatMessage } from './chatMessages.ts'
import type { ChatEvent } from './types.ts'

export function isProviderToolPhase(phase?: string): boolean {
  return phase === 'provider_tool' || phase === 'provider_tool_result'
}

// providerToolCard is the running card for a `provider_tool` event, or null
// when the event names no tool. The id matches the one a reloaded
// transcript gives the same call, so a replayed turn and the history agree.
export function providerToolCard(event: ChatEvent, now: number): ChatMessage | null {
  const name = event.tool_name?.trim()
  if (!name) return null
  return {
    id: `tool-${event.tool_call_id || now}`,
    role: 'tool',
    text: '',
    toolName: name,
    toolCallId: event.tool_call_id,
    toolArgs: event.tool_args_preview,
    toolDone: false,
    toolStartedAt: now,
    toolUpstream: true,
  }
}

// settleProviderToolCard marks the card of a `provider_tool_result` event
// done (or failed). Null when no card has that call id.
export function settleProviderToolCard(messages: ChatMessage[], event: ChatEvent, now: number): ChatMessage[] | null {
  const id = event.tool_call_id
  if (!id) return null
  const idx = messages.findIndex((m) => m.role === 'tool' && m.toolCallId === id)
  if (idx < 0) return null
  const next = [...messages]
  next[idx] = {
    ...next[idx],
    toolArgs: event.tool_args_preview || next[idx].toolArgs,
    toolResult: event.tool_result_preview,
    toolDone: true,
    toolIsError: !!event.tool_is_error,
    toolFinishedAt: now,
  }
  return next
}

// settleInterruptedProviderTools stops the provider cards still running when
// a turn ends (timeout, error, cancel): the CLI is gone and no result will
// come. The cards stay; native cards are left as they are.
export function settleInterruptedProviderTools(messages: ChatMessage[], now: number): ChatMessage[] {
  if (!messages.some((m) => m.toolUpstream && !m.toolDone)) return messages
  return messages.map((m) => (m.toolUpstream && !m.toolDone ? { ...m, toolDone: true, toolFinishedAt: now } : m))
}
