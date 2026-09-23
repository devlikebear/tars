import test from 'node:test'
import assert from 'node:assert/strict'

import { compileSvelteModule } from './helpers/compileSvelteModule.ts'
import { emptyTaskProgressSummary, summarizeTasks } from '../src/lib/tasks.ts'
import type * as StoreModule from '../src/lib/stores/chatSessionStore.svelte.ts'

const mod = await compileSvelteModule<typeof StoreModule>('src/lib/stores/chatSessionStore.svelte.ts')
const { ChatSessionStore, goalEventFeedback, autoTitleFromHistory } = mod

type Deferred<T> = { promise: Promise<T>; resolve: (value: T) => void; reject: (err: unknown) => void }
function deferred<T>(): Deferred<T> {
  let resolve!: (value: T) => void
  let reject!: (err: unknown) => void
  const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}

const flush = () => new Promise((r) => setTimeout(r, 0))

function session(id: string, extra: Record<string, unknown> = {}) {
  return { id, title: `title-${id}`, kind: 'session', ...extra }
}

// A fake API that records calls. Individual methods can be overridden.
function fakeApi(overrides: Record<string, unknown> = {}) {
  const calls: string[] = []
  const api = {
    listSessions: async () => { calls.push('listSessions'); return [session('a'), session('b')] },
    getSession: async (id: string) => { calls.push(`getSession:${id}`); return session(id) },
    getSessionHistory: async (id: string) => { calls.push(`getSessionHistory:${id}`); return [] },
    getSessionTasks: async (id: string) => {
      calls.push(`getSessionTasks:${id}`)
      return { tasks: [{ id: 't1', title: 'x', status: 'completed' }], plan: { goal: `plan-${id}` } }
    },
    getSessionEffectiveConfig: async (id: string) => { calls.push(`getSessionEffectiveConfig:${id}`); return { effective: { tool_config: {} } } },
    listChatTools: async (id: string) => { calls.push(`listChatTools:${id}`); return { tools: [], skills: [] } },
    getSessionCwd: async (id: string) => { calls.push(`getSessionCwd:${id}`); return { current: `/w/${id}`, eligible: [`/w/${id}`] } },
    setSessionCwd: async (id: string, target: string) => { calls.push(`setSessionCwd:${id}:${target}`) },
    getSessionGoal: async (id: string) => { calls.push(`getSessionGoal:${id}`); return { goal: null } },
    renameSession: async (id: string, title: string) => { calls.push(`renameSession:${id}:${title}`) },
    compactSession: async (id: string) => {
      calls.push(`compactSession:${id}`)
      return { compacted: true, compacted_count: 3, original_count: 10, final_count: 7, tokens_before: 100, tokens_after: 40 }
    },
    ...overrides,
  }
  return { api, calls }
}

// Health helpers that make the report's inputs observable.
const helpers = {
  emptyReport: () => ({ status: 'healthy', empty: true, recommendations: [] }),
  buildReport: (input: { session: { id: string }; contextInfo?: Record<string, unknown> }) => ({
    status: 'healthy',
    empty: false,
    sessionId: input.session.id,
    contextInfo: input.contextInfo,
    recommendations: [],
  }),
  emptyTasks: emptyTaskProgressSummary,
  summarizeTasks,
}

function newStore(overrides: Record<string, unknown> = {}) {
  const { api, calls } = fakeApi(overrides)
  const store = new ChatSessionStore(api as never, helpers as never)
  return { store, calls }
}

test('setActive switches session, resets per-session slices, and remounts the thread', async () => {
  const { store, calls } = newStore()
  store.setActive('a')
  await flush()
  store.setArtifacts([{ path: 'x' }] as never)
  store.draft = 'half-typed'
  store.setContextInfo({ history_tokens: 10 })
  const before = store.threadVersion

  store.setActive('b')
  assert.equal(store.activeSessionId, 'b')
  assert.equal(store.threadVersion, before + 1)
  assert.deepEqual(store.artifacts, [])
  assert.equal(store.draft, '')
  assert.deepEqual(store.contextInfo, {})

  await flush()
  assert.equal(store.activeSession?.id, 'b')
  assert.equal(store.cwd?.current, '/w/b')
  assert.equal(store.tasksSummary.plan_goal, 'plan-b')
  assert.equal(store.tasksSummary.completed, 1)
  assert.ok(calls.includes('getSessionGoal:b'))
})

test('setActive with the current id is a no-op, so re-running the route effect is harmless', async () => {
  const { store } = newStore()
  store.setActive('a')
  await flush()
  store.draft = 'keep me'
  const version = store.threadVersion
  store.setActive('a')
  assert.equal(store.threadVersion, version)
  assert.equal(store.draft, 'keep me')
})

test('adoptSession keeps state and does not remount a thread that is mid-stream', async () => {
  const { store, calls } = newStore()
  store.setActive(null)
  store.setArtifacts([{ path: 'streamed.txt' }] as never)
  store.setContextInfo({ history_tokens: 42 })
  const version = store.threadVersion

  store.adoptSession('lazy')
  assert.equal(store.activeSessionId, 'lazy')
  assert.equal(store.threadVersion, version)
  assert.equal(store.artifacts.length, 1)
  assert.equal(store.contextInfo.history_tokens, 42)

  await flush()
  assert.equal(store.activeSession?.id, 'lazy')
  assert.ok(calls.includes('listSessions'), 'the sidebar learns about the new session')
})

test('adoptSession never overrides a session the route already chose', () => {
  const { store } = newStore()
  store.setActive('routed')
  store.adoptSession('other')
  assert.equal(store.activeSessionId, 'routed')
})

test('a health refresh that resolves after a switch does not overwrite the new session', async () => {
  const slowTasks = deferred<unknown>()
  const { store } = newStore({
    getSessionTasks: (id: string) => (id === 'slow' ? slowTasks.promise : Promise.resolve({ tasks: [], plan: { goal: `plan-${id}` } })),
  })
  store.setActive('slow')
  store.setActive('fast')
  await flush()
  slowTasks.resolve({ tasks: [], plan: { goal: 'plan-slow' } })
  await flush()
  assert.equal(store.activeSessionId, 'fast')
  assert.equal(store.tasksSummary.plan_goal, 'plan-fast')
  assert.equal(store.activeSession?.id, 'fast')
})

test('setContextInfo rebuilds the health report with the latest context', async () => {
  const { store } = newStore()
  store.setActive('a')
  await flush()
  store.setContextInfo({ history_tokens: 999 })
  const report = store.health as unknown as { sessionId: string; contextInfo: { history_tokens: number } }
  assert.equal(report.sessionId, 'a')
  assert.equal(report.contextInfo.history_tokens, 999)
})

test('compact reloads the thread and returns the server result', async () => {
  const { store, calls } = newStore()
  store.setActive('a')
  await flush()
  const version = store.threadVersion
  const result = await store.compact()
  assert.equal(result?.compacted, true)
  assert.equal(store.threadVersion, version + 1)
  assert.ok(calls.includes('compactSession:a'))
})

test('a failed session list load keeps the previous list and reports the error', async () => {
  let fail = false
  const { store } = newStore({
    listSessions: async () => {
      if (fail) throw new Error('offline')
      return [session('a')]
    },
  })
  await store.refreshSessions()
  assert.equal(store.sessionsError, null)
  fail = true
  await store.refreshSessions()
  assert.equal(store.sessions.length, 1)
  assert.equal(store.sessionsError, 'offline')
  assert.equal(store.sessionsLoading, false)
})

test('setCwd writes, then re-reads the eligible directories', async () => {
  const { store, calls } = newStore()
  store.setActive('a')
  await flush()
  await store.setCwd('/w/elsewhere')
  assert.ok(calls.includes('setSessionCwd:a:/w/elsewhere'))
  assert.equal(calls.filter((c) => c === 'getSessionCwd:a').length, 2)
  assert.equal(store.cwdBusy, false)
})

test('applyGoalEvent updates the goal chip and surfaces feedback', () => {
  const { store } = newStore()
  const goal = { description: 'ship it', status: 'active', auto_continue_count: 1, max_auto_continues: 3 }
  store.applyGoalEvent({ phase: 'auto_continue', goal: goal as never })
  assert.equal(store.goal?.description, 'ship it')
  assert.equal(store.feedback, 'goal auto-continue 1/3')
})

test('goalEventFeedback covers each phase and stays quiet for the rest', () => {
  assert.equal(goalEventFeedback({ phase: 'satisfied', reason: 'done', goal: null }), 'goal satisfied: done')
  assert.equal(goalEventFeedback({ phase: 'exhausted', goal: null }), 'goal auto-continue budget exhausted')
  assert.equal(goalEventFeedback({ phase: 'judge_error', goal: null }), 'goal judge error: unknown')
  assert.equal(goalEventFeedback({ phase: 'auto_continue', goal: null }), '')
  assert.equal(goalEventFeedback({ phase: 'cleared', goal: null }), '')
})

test('autoTitleFromHistory prefers the first user message and clips long text', () => {
  assert.equal(autoTitleFromHistory([]), '')
  assert.equal(autoTitleFromHistory([{ role: 'assistant', content: 'hi there' }]), 'hi there')
  assert.equal(
    autoTitleFromHistory([
      { role: 'assistant', content: 'greeting' },
      { role: 'user', content: 'fix\nthe   build' },
    ]),
    'fix the build',
  )
  const long = autoTitleFromHistory([{ role: 'user', content: 'x'.repeat(80) }])
  assert.equal(long.length, 50)
  assert.ok(long.endsWith('...'))
})
