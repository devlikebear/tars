// Side session (#971): a second session shown next to the active one in a
// dock panel. It follows that session's running turn through the turn feed
// (GET /v1/chat/stream) and sends messages to it, so two sessions can be
// watched and answered side by side without switching. Pure helpers, tested
// under Node.
import type { ChatMessage } from './chatMessages.ts'
import { approvalFromEvent, resolveApproval } from './chatApproval.ts'
import { providerToolCard, settleProviderToolCard } from './providerToolCards.ts'
import type { ChatEvent, SessionMessage } from './types.ts'
import { userVisibleText } from './consoleContext.ts'

// historyMessages keeps what the side panel shows of a transcript: the
// user's and the assistant's words, and one line per tool call.
export function historyMessages(history: SessionMessage[]): ChatMessage[] {
  const out: ChatMessage[] = []
  for (const message of history) {
    const content = message.content ?? ''
    const text = (message.role === 'user' ? userVisibleText(content) : content).trim()
    if (message.role === 'user' || message.role === 'assistant') {
      if (text) out.push({ id: message.id || `h-${out.length}`, role: message.role, text })
    } else if (message.role === 'tool' && message.tool_name) {
      out.push({ id: message.id || `h-${out.length}`, role: 'tool', text: '', toolName: message.tool_name, toolIsError: !!message.tool_is_error, toolDone: true })
    }
  }
  return out
}

// applySideEvent folds one turn event into the panel's messages. replyId
// names the bubble the turn's reply streams into; it is added on the first
// text.
export function applySideEvent(messages: ChatMessage[], event: ChatEvent, replyId: string): ChatMessage[] {
  switch (event.type) {
    case 'delta': {
      const chunk = event.text ?? ''
      if (!chunk) return messages
      const idx = messages.findIndex((m) => m.id === replyId)
      if (idx < 0) return [...messages, { id: replyId, role: 'assistant', text: chunk }]
      const next = [...messages]
      next[idx] = { ...next[idx], text: next[idx].text + chunk }
      return next
    }
    case 'permission_request': {
      const approval = approvalFromEvent(event)
      if (!approval || messages.some((m) => m.approval?.requestId === approval.requestId)) return messages
      return [...messages, { id: `approval-${approval.requestId}`, role: 'approval', text: '', approval }]
    }
    case 'permission_resolved':
      return event.request_id ? resolveApproval(messages, event.request_id, event.outcome ?? '') : messages
    case 'status':
      if (event.phase === 'before_tool_call' && event.tool_name) {
        return [...messages, { id: `tool-${event.tool_call_id || messages.length}`, role: 'tool', text: '', toolName: event.tool_name }]
      }
      if (event.phase === 'provider_tool') {
        if (event.tool_call_id && messages.some((m) => m.role === 'tool' && m.toolCallId === event.tool_call_id)) return messages
        const card = providerToolCard(event, Date.now())
        return card ? [...messages, card] : messages
      }
      if (event.phase === 'provider_tool_result') {
        return settleProviderToolCard(messages, event, Date.now()) ?? messages
      }
      return messages
    case 'error':
      return [...messages, { id: `error-${messages.length}`, role: 'error', text: event.error || event.message || 'error' }]
  }
  return messages
}
