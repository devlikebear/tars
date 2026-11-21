import test from 'node:test'
import assert from 'node:assert/strict'

import { serveLocally } from '../src/lib/api/client.ts'
import { inboxAPI, desktopAPI, taskAPI } from '../src/lib/mobile/desktopApi.ts'
import { remoteChatAPI } from '../src/lib/mobile/remoteChat.ts'
import { createDemoDesk, demoBaseURL, demoDesktop, isDemoDesktop } from '../src/lib/mobile/demoDesk.ts'
import { targetOf } from '../src/lib/mobile/desktops.ts'
import { remoteFocusApi } from '../src/lib/mobile/focusRemote.ts'
import { inboxOf } from '../src/lib/mobile/inbox.ts'
import { openGateCardId, pipelinePhase } from '../src/lib/focus.ts'
import { mergeSummary, prDraftOf } from '../src/lib/focusPR.ts'
import type { ChatEvent, FocusPipeline } from '../src/lib/types.ts'

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

test('an idea sent as a task drafts a plan and waits at its gate', async () => {
  const d = desk()
  const folders = await taskAPI.folders(d.target)
  assert.equal(folders.length, 2)
  assert.deepEqual((await taskAPI.templates(d.target)).map((t) => t.id), ['dev'])

  await assert.rejects(taskAPI.createPipeline(d.target, { goal: 'x', cwd: '/somewhere/else' }), /not registered/)
  const { session_id } = await taskAPI.createPipeline(d.target, { goal: 'Add CSV export\nto the reports page', cwd: folders[0].path, template: 'dev' })
  assert.deepEqual(await running(d), [session_id])
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
