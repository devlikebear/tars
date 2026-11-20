// The device's one chat session for a plain question outside a focus task
// (docs/decisions/mobile-remote.md §8): ensure it exists, read its history,
// send/follow/cancel a turn and answer a permission question — all aimed at
// one desktop, never the page's own origin. No session id is ever chosen by
// the caller: the server always resolves "my session" from the device's own
// token, so ensureSession and history never take one.
//
// Sending and following reuse the console's own turn-feed plumbing
// (streamChatAt, attachChatStreamAt, cancelChatAt, answerChatPermissionAt),
// so a device chat session behaves exactly like any other chat session —
// detached turns, replayable feed included.
import { requestJSONAt, type APITarget } from '../api/client.ts'
import { answerChatPermissionAt, attachChatStreamAt, cancelChatAt, streamChatAt } from '../api/chat.ts'
import type { ChatEvent, SessionMessage } from '../types.ts'

export type RemoteChatHistory = { sessionId: string; messages: SessionMessage[] }

export type RemoteChatAPI = {
  ensureSession(target: APITarget): Promise<string>
  history(target: APITarget): Promise<RemoteChatHistory>
  send(
    target: APITarget,
    sessionId: string,
    message: string,
    onEvent: (event: ChatEvent) => void,
    signal?: AbortSignal,
  ): Promise<void>
  attach(
    target: APITarget,
    sessionId: string,
    onEvent: (event: ChatEvent) => void,
    signal?: AbortSignal,
  ): Promise<boolean>
  cancel(target: APITarget, sessionId: string): Promise<boolean>
  // decision is never allow_always and never carries a mode: the server
  // refuses both from a paired device (§4/§8), so the mobile UI never
  // offers them in the first place.
  answerPermission(
    target: APITarget,
    requestId: string,
    sessionId: string,
    decision: 'allow_once' | 'allow_session' | 'deny',
  ): Promise<void>
}

export const remoteChatAPI: RemoteChatAPI = {
  async ensureSession(target) {
    const answer = await requestJSONAt<{ session_id: string }>(target, '/v1/remote/chat/session', { method: 'POST' })
    return answer.session_id
  },

  async history(target) {
    const answer = await requestJSONAt<{ session_id: string; messages?: SessionMessage[] }>(target, '/v1/remote/chat/history')
    return { sessionId: answer.session_id, messages: answer.messages ?? [] }
  },

  send(target, sessionId, message, onEvent, signal) {
    return streamChatAt(target, { message, session_id: sessionId, interactive_permissions: true }, onEvent, signal)
  },

  attach(target, sessionId, onEvent, signal) {
    return attachChatStreamAt(target, sessionId, onEvent, signal)
  },

  cancel(target, sessionId) {
    return cancelChatAt(target, sessionId)
  },

  answerPermission(target, requestId, sessionId, decision) {
    return answerChatPermissionAt(target, requestId, sessionId, decision)
  },
}
