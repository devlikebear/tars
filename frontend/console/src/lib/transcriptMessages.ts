// A saved transcript as the chat thread draws it. The server saves a turn in
// the order it streamed — text, tool calls, text, tool calls, reply — so a
// reopened turn reads the way it did live. Transcripts written before that
// hold every tool call first and all of the turn's text in one reply; they
// are drawn as they are. Pure, tested under Node.
import type { ChatMessage } from './chatMessages.ts'
import type { SessionMessage } from './types.ts'
import { stripFocusBlocks } from './focus.ts'

const hiddenSystemPrefixes = ['[HEARTBEAT]', '[COMPACTION SUMMARY]']

export function transcriptChatMessages(history: SessionMessage[]): ChatMessage[] {
  const out: ChatMessage[] = []
  for (const msg of history) {
    if (msg.role === 'system' && hiddenSystemPrefixes.some((prefix) => msg.content.startsWith(prefix))) continue
    if (msg.role === 'tool') {
      out.push({
        id: `tool-${msg.tool_call_id || msg.id || out.length}`,
        role: 'tool',
        text: '',
        toolName: msg.tool_name,
        toolCallId: msg.tool_call_id,
        toolArgs: msg.tool_args,
        toolResult: msg.content,
        toolDone: true,
        toolIsError: msg.tool_is_error,
      })
      continue
    }
    // A turn that ended on a tool call is saved with a blank reply, and a
    // focus turn's reply may be nothing but its <focus-*> block; neither has
    // anything to show.
    if (msg.role === 'assistant' && !stripFocusBlocks(msg.content).trim()) continue
    out.push({
      id: msg.id || `hist-${out.length}`,
      sourceMessageId: msg.id,
      role: msg.role as ChatMessage['role'],
      text: msg.content,
    })
  }
  return out
}
