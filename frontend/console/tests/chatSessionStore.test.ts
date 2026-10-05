import test from 'node:test'
import assert from 'node:assert/strict'

import { compileSvelteModule } from './helpers/compileSvelteModule.ts'
import { emptyTaskProgressSummary, summarizeTasks } from '../src/lib/tasks.ts'
import { chatCommandsEn, chatCommandsKo } from '../src/i18n/sections/chatCommands.ts'
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
  // Tier pins the fake server has saved, returned on the session record.
  const pins: Record<string, string> = {}
  const withPin = (id: string) => session(id, pins[id] ? { tier_pin: pins[id] } : {})
  const api = {
    listSessions: async () => { calls.push('listSessions'); return [session('a'), session('b')] },
    getSession: async (id: string) => { calls.push(`getSession:${id}`); return withPin(id) },
    getSessionHistory: async (id: string) => { calls.push(`getSessionHistory:${id}`); return [] },
    // Not recorded in calls: it rides along with every health refresh.
    getChatContext: async () => ({}),
    getSessionTasks: async (id: string) => {
      calls.push(`getSessionTasks:${id}`)
      return { tasks: [{ id: 't1', title: 'x', status: 'completed' }], plan: { goal: `plan-${id}` } }
    },
    getSessionEffectiveConfig: async (id: string) => {
      calls.push(`getSessionEffectiveConfig:${id}`)
      return { effective: { tool_config: {}, claude_code_cli_permission_mode: id === 'plan-session' ? 'plan' : undefined } }
    },
    listChatTools: async (id: string) => { calls.push(`listChatTools:${id}`); return { tools: [], skills: [] } },
    getSessionCwd: async (id: string) => { calls.push(`getSessionCwd:${id}`); return { current: `/w/${id}`, eligible: [`/w/${id}`] } },
    setSessionCwd: async (id: string, target: string) => { calls.push(`setSessionCwd:${id}:${target}`) },
    getSessionGoal: async (id: string) => { calls.push(`getSessionGoal:${id}`); return { goal: null } },
    renameSession: async (id: string, title: string) => { calls.push(`renameSession:${id}:${title}`) },
    setSessionTierPin: async (id: string, tier: string | null) => {
      calls.push(`setSessionTierPin:${id}:${tier ?? ''}`)
      if (tier) pins[id] = tier
      else delete pins[id]
      return withPin(id)
    },
    compactSession: async (id: string) => {
      calls.push(`compactSession:${id}`)
      return { compacted: true, compacted_count: 3, original_count: 10, final_count: 7, tokens_before: 100, tokens_after: 40 }
    },
    getUsageSummary: async (params: { sessionId?: string }) => {
      calls.push(`getUsageSummary:${params.sessionId}`)
      return { period: 'month', group_by: 'provider', session_id: params.sessionId, total_calls: 2, total_cost_usd: 0.25, total_input_tokens: 300, total_output_tokens: 40, total_unpriced_calls: 1 }
    },
    listAgentRuntimeSubagents: async () => {
      calls.push('listAgentRuntimeSubagents')
      return { tiers: [{ name: 'heavy', kind: 'anthropic', model: 'big' }, { name: 'light', kind: 'claude-code-cli', model: 'small' }] }
    },
    ...overrides,
  }
  return { api, calls }
}

// The goal event lines the store reads; a test switches it to Korean.
let goalText = chatCommandsEn.goalEvents

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
  goalEventText: () => goalText,
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

test('opening a session loads its context usage for the health report', async () => {
  const { store } = newStore({ getChatContext: async () => ({ history_tokens: 5000, compaction_trigger_tokens: 100000 }) })
  store.setActive('a')
  await flush()
  const report = store.health as unknown as { contextInfo: { history_tokens: number } }
  assert.equal(report.contextInfo.history_tokens, 5000)

  // What a running turn reported is newer than the snapshot.
  store.setContextInfo({ history_tokens: 7000, compaction_trigger_tokens: 100000 })
  await store.refreshHealth()
  assert.equal(store.contextInfo.history_tokens, 7000)
})

test('a failed context lookup leaves the health report without usage', async () => {
  const { store } = newStore({ getChatContext: async () => { throw new Error('503') } })
  store.setActive('a')
  await flush()
  assert.deepEqual(store.contextInfo, {})
  assert.equal((store.health as unknown as { sessionId: string }).sessionId, 'a')
})

test('rebuildHealth words the report again after a language switch', async () => {
  let language = 'en'
  const { api } = fakeApi()
  const store = new ChatSessionStore(api as never, {
    ...helpers,
    buildReport: (input: { session: { id: string } }) => ({ status: 'healthy', summary: `${language}:${input.session.id}`, recommendations: [] }),
  } as never)
  store.setActive('a')
  await flush()
  assert.equal((store.health as unknown as { summary: string }).summary, 'en:a')
  language = 'ko'
  store.rebuildHealth()
  assert.equal((store.health as unknown as { summary: string }).summary, 'ko:a')
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

test('an older session list response never overwrites a newer one', async () => {
  const first = deferred<unknown>()
  const second = deferred<unknown>()
  const responses = [first, second]
  const { store } = newStore({ listSessions: () => responses.shift()!.promise })
  const older = store.refreshSessions()
  const newer = store.refreshSessions()
  second.resolve([session('b'), session('a')])
  await newer
  first.resolve([session('a')])
  await older
  assert.deepEqual(store.sessions.map((s) => s.id), ['b', 'a'])
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

test('setCwd to a new folder shows it as current and as a candidate', async () => {
  // The server registers a folder that is not a candidate yet.
  let state = { current: '/w/a', eligible: ['/w/a'] }
  const { store } = newStore({
    getSessionCwd: async () => state,
    setSessionCwd: async (_id: string, target: string) => {
      state = { current: target, eligible: [...state.eligible, target] }
    },
  })
  store.setActive('a')
  await flush()
  await store.setCwd('/w/some/repo')
  assert.equal(store.cwd?.current, '/w/some/repo')
  assert.deepEqual(store.cwd?.eligible, ['/w/a', '/w/some/repo'])
})

test('setCwd rejects with the server error and keeps the previous cwd', async () => {
  const { store } = newStore({
    setSessionCwd: async () => { throw new Error('session: cwd does not exist: /nope') },
  })
  store.setActive('a')
  await flush()
  await assert.rejects(store.setCwd('/nope'), /does not exist/)
  assert.equal(store.cwd?.current, '/w/a')
  assert.equal(store.cwdBusy, false)
})

test('applyGoalEvent updates the goal chip and surfaces feedback', () => {
  const { store } = newStore()
  const goal = { description: 'ship it', status: 'active', auto_continue_count: 1, max_auto_continues: 3 }
  store.applyGoalEvent({ phase: 'auto_continue', goal: goal as never })
  assert.equal(store.goal?.description, 'ship it')
  assert.equal(store.feedback, 'goal auto-continue 1/3')
})

test('applyGoalEvent reads the goal lines when the event arrives, so a locale switch applies', () => {
  const { store } = newStore()
  const goal = { description: 'ship it', status: 'active', auto_continue_count: 2, max_auto_continues: 3 }
  goalText = chatCommandsKo.goalEvents
  try {
    store.applyGoalEvent({ phase: 'auto_continue', goal: goal as never })
  } finally {
    goalText = chatCommandsEn.goalEvents
  }
  assert.equal(store.feedback, chatCommandsKo.goalEvents.autoContinue(2, 3))
  assert.notEqual(store.feedback, 'goal auto-continue 2/3')
})

test('goalEventFeedback covers each phase and stays quiet for the rest', () => {
  const text = chatCommandsEn.goalEvents
  assert.equal(goalEventFeedback({ phase: 'satisfied', reason: 'done', goal: null }, text), 'goal satisfied: done')
  assert.equal(goalEventFeedback({ phase: 'satisfied', goal: null }, text), 'goal satisfied')
  assert.equal(goalEventFeedback({ phase: 'exhausted', goal: null }, text), 'goal auto-continue budget exhausted')
  assert.equal(
    goalEventFeedback({ phase: 'exhausted', reason: 'still failing', goal: null }, text),
    'goal auto-continue budget exhausted (last: still failing)',
  )
  assert.equal(goalEventFeedback({ phase: 'judge_error', goal: null }, text), 'goal judge error: unknown')
  assert.equal(goalEventFeedback({ phase: 'auto_continue', goal: null }, text), '')
  assert.equal(goalEventFeedback({ phase: 'cleared', goal: null }, text), '')
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
  // A companion handoff's guidance is not part of the title.
  assert.equal(
    autoTitleFromHistory([{ role: 'user', content: '어디를 보면 돼?\n\n<console-context>\nTARS 콘솔 안의 컴패니언처럼 답해줘.\n</console-context>' }]),
    '어디를 보면 돼?',
  )
})

test('a pinned tier belongs to its session and survives switching away and back', async () => {
  const { store } = newStore()
  store.setActive('a')
  store.setPinnedTier('heavy')
  assert.equal(store.pinnedTier, 'heavy')
  store.setActive('b')
  assert.equal(store.pinnedTier, null)
  store.setActive('a')
  assert.equal(store.pinnedTier, 'heavy')
  store.setPinnedTier(null)
  assert.equal(store.pinnedTier, null)
})

test('a tier pinned before the first send moves to the adopted session', () => {
  const { store } = newStore()
  store.setActive(null)
  store.setPinnedTier('light')
  store.adoptSession('fresh')
  assert.equal(store.pinnedTier, 'light')
  assert.equal(store.pinnedTiers[''], undefined)
})

test('a pinned tier is saved on the session and cleared by Auto', async () => {
  const { store, calls } = newStore()
  store.setActive('a')
  await flush()
  assert.equal(await store.setPinnedTier('heavy'), true)
  assert.ok(calls.includes('setSessionTierPin:a:heavy'))
  assert.equal(store.pinnedTier, 'heavy')
  assert.equal(await store.setPinnedTier(null), true)
  assert.ok(calls.includes('setSessionTierPin:a:'))
  assert.equal(store.pinnedTier, null)
})

test('the pin comes back from the session record after a reload', async () => {
  const { store } = newStore({
    getSession: async (id: string) => session(id, id === 'a' ? { tier_pin: 'heavy' } : {}),
  })
  store.setActive('a')
  await flush()
  assert.equal(store.pinnedTier, 'heavy')
  store.setActive('b')
  await flush()
  assert.equal(store.pinnedTier, null)
})

test('a session record without a pin clears a stale one', async () => {
  const { store } = newStore()
  store.pinnedTiers = { a: 'light' }
  store.setActive('a')
  await flush()
  assert.equal(store.pinnedTier, null)
})

test('a failed save puts the previous pick back', async () => {
  const { store } = newStore({
    getSession: async (id: string) => session(id, { tier_pin: 'light' }),
    setSessionTierPin: async () => { throw new Error('offline') },
  })
  store.setActive('a')
  await flush()
  assert.equal(await store.setPinnedTier('heavy'), false)
  assert.equal(store.pinnedTier, 'light')
  assert.equal(store.tierPinError, 'offline')
})

test('a session load that started before a pick does not undo it', async () => {
  const load = deferred<ReturnType<typeof session>>()
  const { store } = newStore({ getSession: () => load.promise })
  store.setActive('a')
  const saved = store.setPinnedTier('heavy')
  load.resolve(session('a'))
  await saved
  await flush()
  assert.equal(store.pinnedTier, 'heavy')
})

test('a draft pin is saved on the session the first turn creates', async () => {
  const { store, calls } = newStore()
  store.setActive(null)
  await store.setPinnedTier('heavy')
  assert.ok(!calls.some((call) => call.startsWith('setSessionTierPin')), 'no session to save on yet')
  store.adoptSession('fresh')
  await flush()
  assert.ok(calls.includes('setSessionTierPin:fresh:heavy'))
  assert.equal(store.pinnedTier, 'heavy')
  // A reload reads it back from the session.
  store.pinnedTiers = {}
  store.setActive('other')
  store.setActive('fresh')
  await flush()
  assert.equal(store.pinnedTier, 'heavy')
})

test('usage and the permission override load for the active session', async () => {
  const { store, calls } = newStore()
  store.setActive('plan-session')
  await flush()
  assert.deepEqual(store.usage, { costUSD: 0.25, calls: 2, inputTokens: 300, outputTokens: 40, unpricedCalls: 1 })
  assert.equal(store.permissionModeOverride, 'plan')
  assert.ok(calls.includes('getUsageSummary:plan-session'))
  store.setActive('other')
  assert.equal(store.usage, null, 'usage resets with the session')
  assert.equal(store.permissionModeOverride, '')
})

test('a settled turn re-reads the session cost', async () => {
  let cost = 0
  const { store } = newStore({
    getUsageSummary: async (params: { sessionId?: string }) => ({
      period: 'month', group_by: 'provider', session_id: params.sessionId,
      total_calls: cost ? 1 : 0, total_cost_usd: cost, total_input_tokens: cost ? 10 : 0, total_output_tokens: cost ? 90 : 0,
    }),
  })
  store.setActive('isolated')
  await flush()
  assert.equal(store.usage?.costUSD, 0, 'nothing recorded before the first turn ends')
  cost = 1.5
  await store.turnSettled()
  assert.deepEqual(store.usage, { costUSD: 1.5, calls: 1, inputTokens: 10, outputTokens: 90, unpricedCalls: 0 })
})

// The store outlives a settings save, so each load reads the tiers again;
// only calls that overlap share one request.
test('tier options are read again on each load', async () => {
  let model = 'old'
  const { api, calls } = fakeApi({
    listAgentRuntimeSubagents: async () => {
      calls.push('listAgentRuntimeSubagents')
      return { tiers: [{ name: 'heavy', kind: 'openai-codex', model }] }
    },
  })
  const store = new ChatSessionStore(api as never, helpers as never)
  await Promise.all([store.loadTierOptions(), store.loadTierOptions()])
  assert.equal(calls.filter((c) => c === 'listAgentRuntimeSubagents').length, 1, 'overlapping loads share a request')
  assert.equal(store.tierOptions[0].model, 'old')

  model = 'new'
  await store.loadTierOptions()
  assert.equal(store.tierOptions[0].model, 'new')

  api.listAgentRuntimeSubagents = async () => { throw new Error('offline') }
  await store.loadTierOptions()
  assert.equal(store.tierOptions[0].model, 'new', 'a failed load keeps the last tiers')
})

function modeApi(initial: { mode: string; effective: string; source: string }, fail = false) {
  const state = { ...initial }
  const log: string[] = []
  const view = () => ({ ...state, claude_code_effective: state.effective, claude_code_flag: '', modes: ['manual', 'accept_edits', 'plan', 'auto'] })
  return {
    log,
    overrides: {
      getPermissionMode: async (id: string) => { log.push(`get:${id}`); return view() },
      setPermissionMode: async (id: string, mode: string) => {
        log.push(`set:${id}:${mode}`)
        if (fail) throw new Error('nope')
        state.mode = mode
        state.effective = mode || 'manual'
        state.source = mode ? 'session' : 'config'
        return view()
      },
    },
  }
}

test('the permission mode loads, switches, cycles, and follows the server (#970)', async () => {
  const api = modeApi({ mode: '', effective: 'auto', source: 'config' })
  const { store } = newStore(api.overrides)
  store.setActive('m1')
  await flush()
  assert.equal(store.permission?.effective, 'auto')
  assert.equal(store.permission?.mode, '')

  assert.equal(await store.setPermissionMode('plan'), true)
  assert.deepEqual([store.permission?.mode, store.permission?.effective, store.permission?.source], ['plan', 'plan', 'session'])

  await store.cyclePermissionMode()
  assert.equal(store.permission?.effective, 'auto', 'plan → auto')
  await store.cyclePermissionMode('plan')
  assert.equal(store.permission?.effective, 'auto', 'the shown mode can be passed in')
  await store.cyclePermissionMode()
  assert.equal(store.permission?.effective, 'manual', 'auto wraps to manual')

  store.applyPermissionMode('accept_edits')
  assert.equal(store.permission?.effective, 'accept_edits')
  store.applyPermissionMode('bogus')
  assert.equal(store.permission?.effective, 'accept_edits', 'unknown modes are ignored')

  await store.setPermissionMode('')
  assert.equal(store.permission?.source, 'config')
  assert.deepEqual(api.log, ['get:m1', 'set:m1:plan', 'set:m1:auto', 'set:m1:auto', 'set:m1:manual', 'set:m1:'])

  store.setActive('m2')
  assert.equal(store.permission, null, 'the mode resets with the session')
})

test('a failed mode switch falls back and reports it', async () => {
  const api = modeApi({ mode: '', effective: 'manual', source: 'config' }, true)
  const { store } = newStore(api.overrides)
  store.setActive('m1')
  await flush()
  assert.equal(await store.setPermissionMode('auto'), false)
  assert.equal(store.permission?.effective, 'manual')
  assert.equal(store.permissionError, 'nope')
  store.setActive(null)
  assert.equal(await store.setPermissionMode('auto'), false, 'no session, nothing to switch')
  await store.refreshPermission()
  assert.equal(store.permission, null)
})

// The health report learns which provider runs the session's turns (#857):
// the pinned tier's kind, else the last turn's provider, else the default
// tier's kind — and the Claude Code permission flag.
test('the health report is told the provider and Claude Code flag of the next turn', async () => {
  let built: { provider?: { kind?: string; permissionMode?: string; claudeCodeFlag?: string } } = {}
  const { api } = fakeApi({
    listAgentRuntimeSubagents: async () => ({
      default_tier: 'standard',
      tiers: [
        { name: 'heavy', kind: 'anthropic', model: 'big' },
        { name: 'standard', kind: 'claude-code-cli', model: 'mid' },
        { name: 'light', kind: 'openai', model: 'small' },
      ],
    }),
    getPermissionMode: async () => ({ mode: '', effective: 'manual', claude_code_effective: 'auto', claude_code_flag: 'auto', source: 'config', modes: [] }),
  })
  const store = new ChatSessionStore(api as never, {
    ...helpers,
    buildReport: (input: typeof built) => {
      built = input
      return { status: 'healthy', recommendations: [] }
    },
  } as never)
  store.setActive('a')
  await store.loadTierOptions()
  await flush()
  assert.deepEqual(built.provider, { kind: 'claude-code-cli', permissionMode: 'manual', claudeCodeFlag: 'auto' }, 'default tier')

  store.setContextInfo({ llm_tier: 'light', llm_provider: 'openai' })
  assert.equal(built.provider?.kind, 'openai', 'the last turn wins over the default tier')

  await store.setPinnedTier('heavy')
  assert.equal(built.provider?.kind, 'anthropic', 'a pin wins over the last turn')
})

test('the health report follows a permission mode switch', async () => {
  let built: { provider?: { permissionMode?: string; claudeCodeFlag?: string } } = {}
  let flag = 'bypassPermissions'
  const { api } = fakeApi({
    getPermissionMode: async () => ({ mode: '', effective: 'manual', claude_code_effective: 'auto', claude_code_flag: flag, source: 'config', modes: [] }),
    setPermissionMode: async (_id: string, mode: string) => {
      flag = 'default'
      return { mode, effective: mode, claude_code_effective: mode, claude_code_flag: flag, source: 'session', modes: [] }
    },
  })
  const store = new ChatSessionStore(api as never, {
    ...helpers,
    buildReport: (input: typeof built) => {
      built = input
      return { status: 'healthy', recommendations: [] }
    },
  } as never)
  store.setActive('a')
  await flush()
  assert.equal(built.provider?.claudeCodeFlag, 'bypassPermissions')
  assert.equal(built.provider?.permissionMode, 'manual')
  await store.setPermissionMode('auto')
  assert.equal(built.provider?.permissionMode, 'auto')
  await store.setPermissionMode('manual')
  assert.equal(built.provider?.claudeCodeFlag, 'default')
  assert.equal(built.provider?.permissionMode, 'manual')
})
