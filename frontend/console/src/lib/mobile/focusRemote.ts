// The pipeline screen's calls, aimed at one desktop (docs/decisions/
// mobile-remote.md §1): the same FocusStore the desktop console uses, given
// an API that sends each request to the desktop that owns the pipeline with
// the phone's token. Reads the console makes on /v1/admin/sessions (the
// session, its transcript, its checkpoints) go to the pipeline's own read
// routes, the only ones a paired phone reaches.
import { requestJSONAt, type APITarget } from '../api/client.ts'
import { attachChatStreamAt, streamChatAt } from '../api/chat.ts'
import { focusPipelinePath, postFocusAction, type FocusRequester } from '../api/focus.ts'
import type { CheckpointDiff, CheckpointList } from '../api/checkpoints.ts'
import type { FocusStoreApi } from '../stores/focusStore.svelte.ts'
import type { FocusPipeline, FocusQAResult, Session, SessionMessage } from '../types.ts'
import type { ActivitySnapshot } from '../sessionBoard.ts'

// remoteFocusApi is the API of one pipeline screen: pipelineID is the
// pipeline's session. Its Q&A session's transcript is read through the
// pipeline (`?session=qa`), the only way a phone reaches it.
export function remoteFocusApi(target: APITarget, pipelineID: string): FocusStoreApi {
  const at: FocusRequester = <T>(path: string, init?: RequestInit) => requestJSONAt<T>(target, path, init)
  const path = focusPipelinePath
  const json = (body: unknown): RequestInit => ({
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  return {
    getPipeline: (id) => at<FocusPipeline>(path(id)),
    getSession: (id) => at<Session>(`${path(id)}/session`),
    getHistory: (id) => at<SessionMessage[]>(`${path(pipelineID)}/history${id === pipelineID ? '' : '?session=qa'}`),
    listCheckpoints: (id) => at<CheckpointList>(`${path(id)}/checkpoints`),
    getCheckpointDiff: (id, turnId, options) => {
      const query = options?.scope ? `?scope=${encodeURIComponent(options.scope)}` : ''
      return at<CheckpointDiff>(`${path(id)}/checkpoints/${encodeURIComponent(turnId)}/diff${query}`)
    },
    streamChat: (request, onEvent, signal) => streamChatAt(target, request, onEvent, signal),
    attachChatStream: (id, onEvent, signal) => attachChatStreamAt(target, id, onEvent, signal),
    gate: (id, gate, action, options) => postFocusAction(at, `${path(id)}/gates/${encodeURIComponent(gate)}`, { action, ...options }),
    card: (id, cardId, state, decision) => postFocusAction(at, `${path(id)}/cards/${encodeURIComponent(cardId)}`, decision ? { state, decision } : { state }),
    advance: (id, stage) => postFocusAction(at, `${path(id)}/advance`, { stage }),
    addFinding: (id, finding) => postFocusAction(at, `${path(id)}/findings`, finding),
    stop: (id) => postFocusAction(at, `${path(id)}/stop`, {}),
    goal: (id, enabled) => postFocusAction(at, `${path(id)}/goal`, { enabled }),
    ask: (id, cardId, question) => at<FocusQAResult>(`${path(id)}/qa`, json({ card_id: cardId, question })),
    activity: () => at<ActivitySnapshot>('/v1/chat/activity'),
  }
}
