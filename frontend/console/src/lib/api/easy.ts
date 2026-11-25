import { APIRequestError, requestJSON } from './client.ts'
import { focusPipelinePath, postFocusAction, type FocusRequester } from './focus.ts'
import type { EasyPreview, EasyProject, EasyReadiness, EasyVersion } from '../easy.ts'
import { listQueryString, type EasyListed, type EasyListPage, type EasyListQuery } from '../easyBoard.ts'
import type { FocusActionResult, FocusCardState, FocusGateAction, FocusPipeline } from '../types.ts'

// Easy mode's first run (internal/apihandlers/setup_easy.go): Claude Code
// on every tier, then a restart. A server already set up (409) is fine:
// there is nothing left to do but wait for it. The desk does this itself;
// a phone never does (vibe redesign: the one thing that needs the desk).
export async function setUpEasy(): Promise<void> {
  try {
    await requestJSON('/v1/admin/setup/easy', { method: 'POST' })
  } catch (err) {
    if (err instanceof APIRequestError && err.status === 409) return
    throw err
  }
}

// EasyAPI is every call a vibe-mode screen makes: the easy routes
// (internal/tarsserver/easy_*.go) and the three pipeline calls a project's
// workspace needs (its pipeline, the plan and blocked gates, the leader's
// questions). The console uses localEasyApi; the phone aims the same
// object at one desktop (lib/mobile/easyRemote.ts), the way FocusPipeline
// takes a remote FocusStoreApi.
export type EasyAPI = {
  // What this computer still needs before a project can be made.
  readiness(): Promise<EasyReadiness>
  create(idea: string): Promise<{ project: EasyProject }>
  // The project board: a page of projects with the server's board state,
  // newest change first, and the count of each state.
  board(query?: EasyListQuery): Promise<EasyListPage>
  get(id: string): Promise<EasyProject>
  // Renaming, archiving and restoring answer the project and its board.
  update(id: string, change: { name?: string; archived?: boolean }): Promise<EasyListed>
  // Deleting moves the project's folder aside (into .deleted beside the
  // projects) and drops the record. A phone is refused.
  remove(id: string): Promise<void>
  // "Change it like this": opens the next run in the project's folder.
  change(id: string, request: string): Promise<EasyProject>
  // Saved versions, newest first; going back answers the list after it.
  versions(id: string): Promise<EasyVersion[]>
  restore(id: string, commit: string): Promise<EasyVersion[]>
  // The running app on the desk (reading it keeps it running). A phone is
  // refused (vibe redesign unit 7).
  preview(id: string): Promise<EasyPreview>
  startPreview(id: string): Promise<EasyPreview>
  // The project's run, by its session.
  pipeline(sessionId: string): Promise<FocusPipeline>
  gate(sessionId: string, gate: string, action: FocusGateAction, options?: { note?: string; card_id?: string }): Promise<FocusActionResult>
  card(sessionId: string, cardId: string, state: FocusCardState, decision?: string): Promise<FocusActionResult>
}

// easyApiOf builds the calls on one way of sending a JSON request: this
// console's own requestJSON, or the phone's request to a chosen desktop.
export function easyApiOf(request: FocusRequester): EasyAPI {
  const project = (id: string) => `/v1/easy/projects/${encodeURIComponent(id)}`
  const post = (body?: unknown): RequestInit =>
    body === undefined
      ? { method: 'POST' }
      : { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }
  return {
    readiness: async () => (await request<{ readiness: EasyReadiness }>('/v1/easy/readiness')).readiness,
    create: (idea) => request('/v1/easy/projects', post({ idea })),
    async board(query = {}) {
      const res = await request<Partial<EasyListPage>>(`/v1/easy/projects${listQueryString(query)}`)
      return { projects: res.projects ?? [], next_cursor: res.next_cursor, counts: res.counts ?? {}, archived: res.archived ?? 0 }
    },
    get: async (id) => (await request<{ project: EasyProject }>(project(id))).project,
    async update(id, change) {
      const res = await request<{ project: EasyProject; board: EasyListed['board'] }>(project(id), {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(change),
      })
      return { ...res.project, board: res.board }
    },
    remove: async (id) => {
      await request(project(id), { method: 'DELETE' })
    },
    change: async (id, text) => (await request<{ project: EasyProject }>(`${project(id)}/changes`, post({ request: text }))).project,
    versions: async (id) => (await request<{ versions: EasyVersion[] }>(`${project(id)}/versions`)).versions ?? [],
    restore: async (id, commit) =>
      (await request<{ versions: EasyVersion[] }>(`${project(id)}/versions/${encodeURIComponent(commit)}/restore`, post())).versions ?? [],
    preview: async (id) => (await request<{ preview: EasyPreview }>(`${project(id)}/preview`)).preview,
    startPreview: async (id) => (await request<{ preview: EasyPreview }>(`${project(id)}/preview`, post())).preview,
    pipeline: (sessionId) => request<FocusPipeline>(focusPipelinePath(sessionId)),
    gate: (sessionId, gate, action, options = {}) =>
      postFocusAction(request, `${focusPipelinePath(sessionId)}/gates/${encodeURIComponent(gate)}`, { action, ...options }),
    card: (sessionId, cardId, state, decision) =>
      postFocusAction(request, `${focusPipelinePath(sessionId)}/cards/${encodeURIComponent(cardId)}`, decision ? { state, decision } : { state }),
  }
}

// This console's own server.
export const localEasyApi: EasyAPI = easyApiOf(requestJSON)

// The console's screens that only ever talk to their own server call these.
export const getEasyReadiness = (): Promise<EasyReadiness> => localEasyApi.readiness()
export const createEasyProject = (idea: string): Promise<{ project: EasyProject }> => localEasyApi.create(idea)
export const listEasyBoard = (query: EasyListQuery = {}): Promise<EasyListPage> => localEasyApi.board(query)
export const updateEasyProject = (id: string, change: { name?: string; archived?: boolean }): Promise<EasyListed> =>
  localEasyApi.update(id, change)
export const deleteEasyProject = (id: string): Promise<void> => localEasyApi.remove(id)

// The error code of a refused request, or ''.
export function easyErrorCode(err: unknown): string {
  return err instanceof APIRequestError ? String((err.payload as { code?: string } | undefined)?.code ?? '') : ''
}
