import test from 'node:test'
import assert from 'node:assert/strict'

import { requestJSONAt, serveLocally } from '../src/lib/api/client.ts'
import { inboxAPI, desktopAPI } from '../src/lib/mobile/desktopApi.ts'
import { remoteChatAPI } from '../src/lib/mobile/remoteChat.ts'
import { createDemoDesk, demoBaseURL, demoDesktop, isDemoDesktop } from '../src/lib/mobile/demoDesk.ts'
import { targetOf } from '../src/lib/mobile/desktops.ts'
import { remoteFocusApi } from '../src/lib/mobile/focusRemote.ts'
import { inboxOf } from '../src/lib/mobile/inbox.ts'
import { openGateCardId, pipelinePhase } from '../src/lib/focus.ts'
import { mergeSummary, prDraftOf } from '../src/lib/focusPR.ts'
import type { ChatEvent, FocusPipeline } from '../src/lib/types.ts'
import { easyView, type EasyProject, type EasyReadiness, type EasyVersion } from '../src/lib/easy.ts'
import { remoteEasyApi } from '../src/lib/mobile/easyRemote.ts'
import { tryApp } from '../src/lib/mobile/tryApp.ts'
import type { FocusTemplate } from '../src/lib/types.ts'
import { compileSvelteModule } from './helpers/compileSvelteModule.ts'
import type * as EasyProjectStoreModule from '../src/lib/stores/easyProjectStore.svelte.ts'

const { EasyProjectStore } = await compileSvelteModule<typeof EasyProjectStoreModule>('src/lib/stores/easyProjectStore.svelte.ts')

// A desk on a clock the test moves: a turn ends when the clock passes it.
function desk(turnMs = 1000) {
  let at = Date.parse('2026-11-15T09:00:00Z')
  const demo = createDemoDesk({ now: () => at, turnMs })
  serveLocally(demoBaseURL, demo.handle)
  const desktop = demoDesktop(new Date(at))
  const target = targetOf(desktop)
  return {
    desktop,
    target,
    pass: (ms = turnMs) => { at += ms },
    api: (id: string) => remoteFocusApi(target, id),
  }
}

async function running(d: ReturnType<typeof desk>): Promise<string[]> {
  return (await inboxAPI.activity(d.target)).running.map((r) => r.session_id)
}

test('the demo desk is a desktop of its own address that answers like a paired one', async () => {
  const d = desk()
  assert.equal(isDemoDesktop(d.desktop), true)
  assert.equal(isDemoDesktop({ baseURL: 'https://mac.tail1.ts.net' }), false)
  assert.deepEqual(await desktopAPI.status(d.target), { state: 'online', version: 'demo' })
  assert.deepEqual((await desktopAPI.readiness!(d.target))?.map((c) => c.state), ['ok', 'ok', 'ok'])
})

test('it starts with one pipeline at each place a phone is asked something', async () => {
  const d = desk()
  const [pipelines, activity] = await Promise.all([inboxAPI.pipelines(d.target), inboxAPI.activity(d.target)])
  const entries = inboxOf({ desktopID: d.desktop.id, desktopName: d.desktop.name, pipelines, activity })
  assert.deepEqual(
    entries.map((e) => [e.sessionID, e.gate, e.cards, e.approvals.length]).sort(),
    [
      ['demo-dark-mode', 'plan', 0, 0],
      ['demo-search-index', 'merge', 0, 0],
      ['demo-upload-retry', 'triage', 2, 1],
    ],
  )
  assert.deepEqual(await running(d), [])

  // The queued tool call is answered once.
  await inboxAPI.reviewApproval(d.target, 'demo-approval-1', 'approve')
  assert.deepEqual((await inboxAPI.activity(d.target)).queued_approvals, [])
  await assert.rejects(inboxAPI.reviewApproval(d.target, 'demo-approval-1', 'approve'), /already answered/)
})

test('a pipeline runs from its plan gate to merged, a turn after each answer', async () => {
  const d = desk()
  const id = 'demo-dark-mode'
  const api = d.api(id)
  const read = () => api.getPipeline(id)

  let p: FocusPipeline = await read()
  assert.equal(p.open_gate, 'plan')
  assert.ok(openGateCardId(p))
  assert.equal(p.plan?.tasks.length, 3)

  // Approving the plan starts the build turn; nothing waits meanwhile.
  let result = await api.gate(id, 'plan', 'approve')
  assert.equal(result.conflict, undefined)
  assert.equal(result.pipeline.open_gate, undefined)
  assert.deepEqual(await running(d), [id])
  // The same gate again is a conflict that carries the pipeline.
  assert.equal((await api.gate(id, 'plan', 'approve')).conflict, true)

  d.pass()
  p = await read()
  assert.deepEqual(await running(d), [])
  assert.equal(p.current, 'review')
  assert.equal(p.open_gate, 'triage')
  const finding = p.cards.find((c) => c.kind === 'finding')!
  assert.deepEqual(p.review?.triage, [finding.id])

  // Asking about a card answers in the hidden Q&A session.
  const asked = await api.ask!(id, finding.id, 'Why does it flash?')
  assert.equal(asked.turn, 1)
  assert.equal((await api.getHistory(asked.qa_session_id)).length, 2)
  assert.deepEqual((await read()).qa_turns, { [finding.id]: [1] })

  // The last finding decided as fix starts the fix turn, then the PR gate.
  result = await api.card(id, finding.id, 'decided', 'fix')
  assert.equal(result.pipeline.cards.find((c) => c.id === finding.id)?.decided_by, 'phone')
  assert.deepEqual(await running(d), [id])
  d.pass()
  p = await read()
  assert.equal(p.open_gate, 'pr')
  const draftCard = p.cards.find((c) => c.id === openGateCardId(p))!
  assert.equal(prDraftOf(draftCard)?.title, 'feat(settings): dark mode toggle')

  // G3 with an edited title, then G4 with the server's facts.
  await api.gate(id, 'pr', 'approve', { pr: { title: 'feat: dark mode', body: 'b' } })
  d.pass()
  p = await read()
  assert.equal(p.open_gate, 'merge')
  assert.equal(p.pr?.number, 131)
  const summary = mergeSummary(p.cards.find((c) => c.id === openGateCardId(p))!)
  assert.deepEqual([summary?.title, summary?.checks.passed, summary?.fixed], ['feat: dark mode', 3, 1])

  await api.gate(id, 'merge', 'approve')
  d.pass()
  p = await read()
  assert.equal(pipelinePhase(p), 'finished')
  assert.equal(p.pr?.state, 'MERGED')
  assert.ok(p.finished_at)
  // Each change is newer than the last, so a screen takes it.
  assert.ok(Date.parse(p.updated_at) > Date.parse(result.pipeline.updated_at))
})

test('findings all dismissed go straight to the PR gate; a stop ends the pipeline', async () => {
  const d = desk()
  const id = 'demo-upload-retry'
  const api = d.api(id)
  let p = await api.getPipeline(id)
  const [first, second] = p.review!.triage!
  assert.equal((await api.card(id, first, 'decided', 'dismiss')).pipeline.open_gate, 'triage')
  p = (await api.card(id, second, 'decided', 'dismiss')).pipeline
  assert.equal(p.open_gate, 'pr')
  assert.deepEqual(await running(d), [])

  p = (await api.stop(id)).pipeline
  assert.equal(pipelinePhase(p), 'stopped')
  const item = (await inboxAPI.pipelines(d.target)).find((i) => i.session_id === id)!
  assert.deepEqual([item.phase, item.waiting], ['stopped', { cards: 0 }])

  // Resume opens the stopped PR gate again as a new card.
  const cards = p.cards.length
  p = (await api.resume!(id)).pipeline
  assert.equal(pipelinePhase(p), 'active')
  assert.equal(p.open_gate, 'pr')
  assert.equal(p.cards.length, cards + 1)
  assert.equal(p.cards.at(-1)!.state, 'unseen')
  assert.equal((await api.resume!(id)).conflict, true)

  // Stopped again, then cleaned up.
  p = (await api.stop(id)).pipeline
  assert.equal(pipelinePhase(p), 'stopped')
  // Cleaning up a stopped pipeline records how its worktree ended, the
  // same shape the real server answers (focus_pipeline.go's cleanup).
  assert.equal(p.worktree_end, undefined)
  p = (await api.cleanup!(id, 'discard')).pipeline
  assert.equal(p.worktree_end?.action, 'discard')
})

test('cleanup refuses an active pipeline, like the real server (409 conflict result)', async () => {
  const d = desk()
  const id = 'demo-upload-retry'
  const api = d.api(id)
  const result = await api.cleanup!(id, 'keep')
  assert.equal(result.conflict, true)
})

test('a focus task started on the desk drafts a plan and waits at its gate', async () => {
  const d = desk()
  const at = <T>(path: string, init?: RequestInit) => requestJSONAt<T>(d.target, path, init)
  const folders = (await at<{ folders: { path: string }[] }>('/v1/focus/folders')).folders
  assert.equal(folders.length, 2)
  assert.deepEqual((await at<{ templates: FocusTemplate[] }>('/v1/focus/templates')).templates.map((t) => t.id), ['dev'])
  const create = (body: unknown) => at<{ session_id: string }>('/v1/focus/pipelines', { method: 'POST', body: JSON.stringify(body) })

  await assert.rejects(create({ goal: 'x', cwd: '/somewhere/else' }), /not registered/)
  const { session_id } = await create({ goal: 'Add CSV export\nto the reports page', cwd: folders[0].path, template: 'dev' })
  const before = await running(d)
  assert.ok(before.includes(session_id))
  const api = d.api(session_id)
  assert.equal((await api.getPipeline(session_id)).pending_turn, 'Add CSV export\nto the reports page')

  d.pass()
  const p = await api.getPipeline(session_id)
  assert.equal(p.open_gate, 'plan')
  assert.equal(p.pending_turn, undefined)
  const item = (await inboxAPI.pipelines(d.target)).find((i) => i.session_id === session_id)!
  assert.deepEqual([item.title, item.waiting?.gate], ['Add CSV export', 'plan'])
  assert.equal((await api.getSession(session_id)).current_dir, folders[0].path)
})

test('a running turn streams into the pipeline screen and ends with the turn', async () => {
  const demo = createDemoDesk({ turnMs: 120 })
  serveLocally(demoBaseURL, demo.handle)
  const target = targetOf(demoDesktop(new Date()))
  const id = 'demo-dark-mode'
  const api = remoteFocusApi(target, id)

  assert.equal(await api.attachChatStream(id, () => {}), false)
  await api.gate(id, 'plan', 'approve')
  const events: ChatEvent[] = []
  assert.equal(await api.attachChatStream(id, (e) => events.push(e)), true)
  assert.equal(events[0].type, 'turn_started')
  assert.equal(events.at(-1)?.type, 'done')
  assert.equal(events.filter((e) => e.type === 'delta').length, 4)
  assert.equal((await api.getPipeline(id)).open_gate, 'triage')

  // An instruction typed on the screen is a turn of its own.
  const typed: ChatEvent[] = []
  await api.streamChat({ message: 'Use a switch, not a dropdown', session_id: id }, (e) => typed.push(e))
  assert.equal(typed.at(-1)?.type, 'done')
  const history = await api.getHistory(id)
  assert.equal(history.at(-2)?.content, 'Use a switch, not a dropdown')
  assert.equal(history.at(-1)?.role, 'assistant')
})

// The device's own chat session (§8): not a pipeline, so it never shows in
// /v1/focus/pipelines, and a round trip through it answers with fixed text.
test('the device chat session answers a plain question and settles into history', async () => {
  const demo = createDemoDesk({ turnMs: 100 })
  serveLocally(demoBaseURL, demo.handle)
  const target = targetOf(demoDesktop(new Date()))

  const sessionId = await remoteChatAPI.ensureSession(target)
  assert.equal(sessionId, 'demo-chat')
  // Asking again returns the same session, not a new one.
  assert.equal(await remoteChatAPI.ensureSession(target), sessionId)
  assert.ok(!(await inboxAPI.pipelines(target)).some((p) => p.session_id === sessionId))

  assert.deepEqual((await remoteChatAPI.history(target)).messages, [])
  assert.equal(await remoteChatAPI.attach(target, sessionId, () => {}), false)

  const events: ChatEvent[] = []
  await remoteChatAPI.send(target, sessionId, 'what can you do here?', (e) => events.push(e))
  assert.equal(events[0].type, 'turn_started')
  assert.equal(events.at(-1)?.type, 'done')
  assert.ok(events.some((e) => e.type === 'delta'))

  const after = await remoteChatAPI.history(target)
  assert.equal(after.sessionId, sessionId)
  assert.deepEqual(
    after.messages.map((m) => m.role),
    ['user', 'assistant'],
  )
  assert.equal(after.messages[0].content, 'what can you do here?')
  assert.match(after.messages[1].content, /demo desk/)
})

test('a device chat turn can be cancelled mid-stream, with no reply recorded', async () => {
  const demo = createDemoDesk({ turnMs: 300 })
  serveLocally(demoBaseURL, demo.handle)
  const target = targetOf(demoDesktop(new Date()))
  const sessionId = await remoteChatAPI.ensureSession(target)

  const events: ChatEvent[] = []
  const sent = remoteChatAPI.send(target, sessionId, 'hello', (e) => events.push(e))
  await new Promise((resolve) => setTimeout(resolve, 20))
  assert.equal(await remoteChatAPI.cancel(target, sessionId), true)
  await sent
  assert.equal(events.at(-1)?.type, 'done')

  const history = await remoteChatAPI.history(target)
  assert.deepEqual(
    history.messages.map((m) => m.role),
    ['user'],
  )
  // Cancelling again has nothing left to cancel.
  assert.equal(await remoteChatAPI.cancel(target, sessionId), false)
})

// Vibe redesign unit 6: the demo desk answers the vibe project routes a
// paired desk answers a phone, so the phone screens (unit 8) can run on it.
test('vibe projects: from one sentence through the questions, the blueprint, making and checks to a saved result', async () => {
  const d = desk()
  const at = <T>(path: string, init?: RequestInit) => requestJSONAt<T>(d.target, path, init)
  const post = (body: unknown, method = 'POST'): RequestInit => ({ method, body: JSON.stringify(body), headers: { 'Content-Type': 'application/json' } })
  type Listed = { projects: (EasyProject & { board: { state: string; waiting?: string } })[]; counts: Record<string, number>; archived: number }
  const easy = remoteEasyApi(d.target)

  assert.equal((await at<{ readiness: EasyReadiness }>('/v1/easy/readiness')).readiness.ready, true)
  let list = await at<Listed>('/v1/easy/projects')
  // One seeded project at each place the phone shows.
  assert.deepEqual(
    Object.fromEntries(list.projects.map((p) => [p.id, p.board.state])),
    { 'demo-recipe-box': 'done', 'demo-family-todo': 'paused', 'demo-club-dues': 'stuck' },
  )
  const seeded = await easy.versions('demo-recipe-box')
  assert.deepEqual(seeded.map((v) => [v.kind, !!v.current]), [['saved', true], ['start', false]])

  await assert.rejects(easy.create('  '), /idea_required/)
  const { project } = await easy.create('A page for my book club')
  const id = project.id
  const store = new EasyProjectStore({ api: easy, projectId: id, pollMs: 60_000 })
  const phase = async () => {
    await store.read()
    return store.view!.phase
  }
  try {
    assert.equal(await phase(), 'planning')
    assert.equal((await easy.get(id)).session_id, project.session_id)

    // The leader asks first; the person answers each question.
    d.pass()
    assert.equal(await phase(), 'questions')
    assert.deepEqual((await at<{ board: { state: string; waiting?: string } }>(`/v1/easy/projects/${id}`)).board, {
      state: 'your_turn', waiting: 'question', changed_at: (await easy.board()).projects.find((p) => p.id === id)!.board.changed_at,
    })
    const questions = store.view!.questions
    assert.equal(questions.length, 2)
    assert.ok(questions.every((q) => q.youDecide))
    await store.answer(questions[0].cardId, questions[0].options[0])
    assert.equal(store.view!.phase, 'questions')
    await store.answer(questions[1].cardId, 'You decide')
    assert.equal(store.view!.phase, 'planning')

    // The blueprint: screens and checks in plain words, then "tell the team".
    d.pass()
    assert.equal(await phase(), 'approve')
    const blueprint = store.view!.blueprint!
    assert.equal(blueprint.checks.length, 4)
    assert.equal(blueprint.screens.length, 1)
    assert.equal(await store.sayToTeam('  '), false)
    assert.equal(store.sayError, 'required')
    assert.equal(await store.sayToTeam('Show the meeting date too'), true)
    assert.equal(store.view!.phase, 'planning')
    d.pass()
    assert.equal(await phase(), 'approve')

    // Approved: the team makes it, then checks it; one check fails once.
    await store.approve()
    assert.equal(store.view!.phase, 'building')
    assert.equal(store.view!.step, 'make')
    await assert.rejects(easy.update(id, { archived: true }), /busy/)
    await assert.rejects(easy.change(id, 'Make it green'), /busy/)
    d.pass()
    await store.read()
    assert.equal(store.view!.step, 'check')
    d.pass()
    await store.read()
    assert.deepEqual(store.view!.checks.map((c) => c.state), ['passed', 'passed', 'failed', 'passed'])
    assert.equal(store.view!.checks[2].report?.expected, 'the newest entry at the top')
    assert.equal(store.view!.phase, 'building')
    for (let i = 0; i < 4 && (await phase()) !== 'finished'; i++) d.pass()
    assert.equal(store.view!.phase, 'finished')
    assert.equal(store.view!.saved, true)
    assert.ok(store.view!.checks.every((c) => c.state === 'passed'))
    assert.equal((await easy.board()).projects.find((p) => p.id === id)!.board.state, 'done')

    // Rename and archive; the archive holds it apart.
    const renamed = await easy.update(id, { name: '  Book   club ', archived: true })
    assert.equal(renamed.name, 'Book club')
    list = await at<Listed>('/v1/easy/projects')
    assert.equal(list.archived, 1)
    assert.ok(!list.projects.some((x) => x.id === id))
    assert.ok((await easy.board({ archived: true })).projects.some((x) => x.id === id))
    await easy.update(id, { archived: false })

    // A change runs the same flow again in the same project.
    assert.equal(await store.change('  '), false)
    assert.equal(store.changeError, 'change_required')
    assert.equal(await store.change('Make the button green'), true)
    assert.notEqual(store.project!.session_id, project.session_id)
    assert.equal(store.view!.change, 'Make the button green')
    const versions = await easy.versions(id)
    assert.deepEqual(versions.map((v) => v.kind), ['saved', 'start'])
    await assert.rejects(easy.restore(id, versions[1].commit), /busy/)
  } finally {
    store.stop()
  }

  // Going back on the finished seeded project adds a version, never removes one.
  const back = await easy.restore('demo-recipe-box', seeded[1].commit)
  assert.deepEqual(back.map((v) => v.kind), ['restored', 'saved', 'start'])

  // What a phone cannot do on a real desk, it cannot do here.
  await assert.rejects(easy.remove(id), /cannot delete/)
  await assert.rejects(easy.get('nope'), /no such project/)
})

test('vibe projects: a pause on the usage limit continues, a stuck project tries again with the person\'s words', async () => {
  const d = desk()
  const easy = remoteEasyApi(d.target)

  const family = new EasyProjectStore({ api: easy, projectId: 'demo-family-todo', pollMs: 60_000 })
  const dues = new EasyProjectStore({ api: easy, projectId: 'demo-club-dues', pollMs: 60_000 })
  try {
    await family.read()
    assert.equal(family.view!.phase, 'limit')
    assert.ok(family.view!.limit?.resetsAt)
    await family.continueAfterLimit()
    assert.equal(family.view!.phase, 'building')
    assert.equal(family.view!.step, 'make')

    await dues.read()
    assert.equal(dues.view!.phase, 'stuck')
    assert.ok(dues.view!.retry)
    assert.equal(dues.view!.checks.filter((c) => c.state === 'failed').length, 1)
    assert.equal(await dues.retryStuck('The newest payment goes at the top'), true)
    assert.equal(dues.view!.phase, 'building')
    for (let i = 0; i < 4 && dues.view!.phase !== 'finished'; i++) {
      d.pass()
      await dues.read()
    }
    assert.equal(dues.view!.phase, 'finished')
  } finally {
    family.stop()
    dues.stop()
  }
  // The board reads the same.
  const states = Object.fromEntries((await easy.board()).projects.map((p) => [p.id, p.board.state]))
  assert.equal(states['demo-club-dues'], 'done')
  // A finished seeded run reads as finished and saved in the workspace too.
  const recipe = await easy.get('demo-recipe-box')
  assert.equal(easyView(recipe, await easy.pipeline(recipe.session_id!)).phase, 'finished')
})

test('vibe projects: Try it on the demo desk answers in place and never leaves the app', async () => {
  const d = desk()
  const easy = remoteEasyApi(d.target)
  // The finished seeded project "runs"; its ticket is an address on the demo
  // desk itself, new on every press.
  assert.deepEqual(await easy.startPreview('demo-recipe-box'), { state: 'running' })
  assert.deepEqual(await easy.preview('demo-recipe-box'), { state: 'running' })
  const first = await easy.previewTicket('demo-recipe-box')
  const second = await easy.previewTicket('demo-recipe-box')
  assert.ok(first.url.startsWith(`${demoBaseURL}/__tars/enter?t=`))
  assert.notEqual(first.url, second.url)
  assert.equal(Date.parse(first.expires_at) - Date.parse('2026-11-15T09:00:00Z'), 60_000)
  assert.deepEqual(await tryApp(easy, 'demo-recipe-box'), { kind: 'demo' })

  // A project still being made is not ready; stopping the app stays the desk's.
  await assert.rejects(easy.previewTicket('demo-family-todo'), /not ready/)
  assert.deepEqual(await tryApp(easy, 'demo-family-todo'), { kind: 'failed', reason: 'not_ready' })
  await assert.rejects(requestJSONAt(d.target, '/v1/easy/projects/demo-recipe-box/preview', { method: 'DELETE' }), /cannot stop/)
  assert.deepEqual(await tryApp(easy, 'nope'), { kind: 'failed', reason: 'not_ready' })
})
