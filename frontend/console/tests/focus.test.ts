import test from 'node:test'
import assert from 'node:assert/strict'

import {
  acknowledgeable,
  changeCards,
  deckFor,
  mustHandle,
  orderCards,
  pipelinePhase,
  planEdits,
  progressLine,
  stepperItems,
  stripFocusBlocks,
  stripFocusStage,
  turnIndex,
  turnSlice,
  turnStage,
} from '../src/lib/focus.ts'
import { userVisibleText } from '../src/lib/consoleContext.ts'
import { focusKo } from '../src/i18n/sections/focus.ts'
import type { ChatEvent, FocusCard, FocusPipeline, SessionMessage } from '../src/lib/types.ts'

function card(id: string, kind: FocusCard['kind'], extra: Partial<FocusCard> = {}): FocusCard {
  return { id, kind, stage: 'build', turn: 1, title: id, state: 'unseen', created_at: '2026-10-01T00:00:00Z', ...extra }
}

function pipeline(extra: Partial<FocusPipeline> = {}): FocusPipeline {
  return {
    version: 1,
    session_id: 's1',
    goal: 'g',
    current: 'plan',
    stages: [
      { id: 'plan', status: 'active', iteration: 1 },
      { id: 'build', status: 'pending', iteration: 0 },
      { id: 'review', status: 'pending', iteration: 0 },
      { id: 'pr', status: 'pending', iteration: 0 },
      { id: 'pr_review', status: 'pending', iteration: 0 },
      { id: 'merge', status: 'pending', iteration: 0 },
    ],
    cards: [],
    updated_at: '2026-10-01T00:00:00Z',
    ...extra,
  }
}

test('orderCards: kind priority, then unseen before seen, then created_at; decided last', () => {
  const cards = [
    card('change', 'change'),
    card('report-seen', 'report', { state: 'seen' }),
    card('report-new', 'report', { created_at: '2026-10-01T00:00:02Z' }),
    card('report-old', 'report', { created_at: '2026-10-01T00:00:01Z' }),
    card('failure', 'failure'),
    card('finding', 'finding'),
    card('gate-done', 'gate', { state: 'decided', decision: 'approve' }),
    card('decision', 'decision'),
    card('gate', 'gate'),
    card('notice', 'notice'),
  ]
  assert.deepEqual(orderCards(cards).map((c) => c.id), [
    'gate', 'decision', 'finding', 'failure', 'notice', 'report-old', 'report-new', 'report-seen', 'change', 'gate-done',
  ])
  // Never mutates its input.
  assert.equal(cards[0].id, 'change')
})

test('mustHandle: gates, decisions and findings until decided', () => {
  assert.equal(mustHandle(card('a', 'gate')), true)
  assert.equal(mustHandle(card('a', 'decision', { state: 'seen' })), true)
  assert.equal(mustHandle(card('a', 'finding')), true)
  assert.equal(mustHandle(card('a', 'finding', { state: 'decided' })), false)
  for (const kind of ['report', 'change', 'failure', 'notice'] as const) assert.equal(mustHandle(card('a', kind)), false)
})

test('acknowledgeable: undecided report, change, failure and notice cards', () => {
  const cards = [card('r', 'report'), card('c', 'change', { state: 'seen' }), card('f', 'failure'), card('n', 'notice'), card('d', 'decision'), card('x', 'report', { state: 'decided' })]
  assert.deepEqual(acknowledgeable(cards).map((c) => c.id), ['r', 'c', 'f', 'n'])
})

test('stepperItems marks the current stage and carries iterations and labels', () => {
  const p = pipeline({
    current: 'build',
    stages: [
      { id: 'plan', status: 'done', iteration: 1 },
      { id: 'build', status: 'active', iteration: 2, limit: 3 },
      { id: 'review', status: 'skipped', iteration: 0 },
      { id: 'pr', status: 'pending', iteration: 0 },
      { id: 'pr_review', status: 'pending', iteration: 0 },
      { id: 'merge', status: 'pending', iteration: 0 },
    ],
  })
  const items = stepperItems(p)
  assert.deepEqual(items[1], { id: 'build', label: 'Build', status: 'active', iteration: 2, current: true })
  assert.deepEqual(items.map((i) => i.current), [false, true, false, false, false, false])
  assert.equal(items[2].status, 'skipped')
  assert.equal(stepperItems(p, focusKo.stages)[0].label, '계획')
})

test('pipelinePhase mirrors Pipeline.Active: active, finished or stopped', () => {
  assert.equal(pipelinePhase(pipeline()), 'active')
  const finished = pipeline({ current: 'merge', stages: pipeline().stages.map((s) => ({ ...s, status: s.id === 'plan' ? 'done' : 'skipped' })) })
  finished.stages[5] = { id: 'merge', status: 'done', iteration: 1 }
  assert.equal(pipelinePhase(finished), 'finished')
  const stopped = pipeline()
  stopped.stages[0] = { id: 'plan', status: 'blocked', iteration: 1 }
  assert.equal(pipelinePhase(stopped), 'stopped')
})

test('deckFor keeps the stage cards in deck order', () => {
  const cards = [card('r', 'report'), card('g', 'gate', { stage: 'plan' }), card('d', 'decision')]
  assert.deepEqual(deckFor(cards, 'build').map((c) => c.id), ['d', 'r'])
  assert.deepEqual(deckFor(cards, 'plan').map((c) => c.id), ['g'])
})

test('progressLine: stage verb, files changed, current activity', () => {
  const events: ChatEvent[] = [
    { type: 'turn_started' },
    { type: 'file_change', path: 'a.go', op: 'modify' },
    { type: 'file_change', path: 'b.go', op: 'create' },
    { type: 'file_change', path: 'a.go', op: 'modify' },
    { type: 'provider_tool', tool_name: 'Edit', tool_call_id: 't1', tool_args_preview: '{"file_path":"c.go"}' },
    { type: 'provider_tool_result', tool_call_id: 't1' },
    { type: 'status', phase: 'before_tool_call', tool_name: 'exec', tool_call_id: 't2', tool_args_preview: '{"command":"make test"}' },
  ]
  assert.equal(progressLine(events, { stage: 'build' }), 'Implementing · 3 files changed · running tests')
  // The finished test call leaves the reply being written.
  const after = [...events, { type: 'status', phase: 'after_tool_call', tool_call_id: 't2' }, { type: 'delta', text: 'ok' }]
  assert.equal(progressLine(after, { stage: 'build' }), 'Implementing · 3 files changed · writing')
  assert.equal(progressLine([], {}), 'Working')
  assert.equal(progressLine([{ type: 'provider_tool', tool_name: 'Read', tool_call_id: 'r', tool_args_preview: '{"file_path":"x"}' }], { stage: 'plan' }), 'Planning · reading code')
  assert.equal(progressLine([{ type: 'provider_tool', tool_name: 'Write', tool_call_id: 'w', tool_args_preview: '{"file_path":"src/x.ts"}' }]), 'Working · editing x.ts')
  assert.equal(progressLine([{ type: 'permission_request', request_id: 'p' }]), 'Working · waiting for approval')
  assert.equal(progressLine([{ type: 'reasoning_delta', text: 'hm' }]), 'Working · thinking')
  // A checkpoint's count wins when larger than the files seen streaming.
  assert.equal(progressLine([{ type: 'checkpoint', files: 4 }]), 'Working · 4 files changed')
  assert.equal(progressLine([], { stage: 'build', text: focusKo.progress }), '구현 중')
})

test('stripFocusStage drops only the trailing stage guidance block', () => {
  const guided = 'add a flag\n\n<focus-stage>\nFocus mode — current stage: plan.\n</focus-stage>'
  assert.equal(stripFocusStage(guided), 'add a flag')
  assert.equal(stripFocusStage('plain'), 'plain')
  assert.equal(stripFocusStage('<focus-stage>\nonly guidance\n</focus-stage>'), '')
  // The user's own words mentioning the tag stay.
  const typed = 'why does <focus-stage> show up? and what is </focus-stage>?'
  assert.equal(stripFocusStage(typed), typed)
  assert.equal(stripFocusStage(`${typed}\n\n<focus-stage>\nx\n</focus-stage>`), typed)
  // A block that is not the trailing suffix stays.
  const quoted = 'see:\n\n<focus-stage>\nx\n</focus-stage>\n\nthen more words'
  assert.equal(stripFocusStage(quoted), quoted)
})

test('userVisibleText also drops <focus-stage>, between console context and review notes', () => {
  const ctx = '<console-context>\nCurrent console area: Focus.\n</console-context>'
  const stage = '<focus-stage>\nFocus mode — current stage: build.\n</focus-stage>'
  const notes = '<review-notes>\nNotes\n1. a.txt\nComment: x\n</review-notes>'
  assert.equal(userVisibleText(`fix it\n\n${ctx}\n\n${stage}\n\n${notes}`), 'fix it')
  assert.equal(userVisibleText(`fix it\n\n${stage}`), 'fix it')
})

test('stripFocusBlocks folds focus blocks out of assistant text, outside code fences only', () => {
  const reply = 'Here is the plan.\n\n<focus-plan>{"goal":"x","tasks":[]}</focus-plan>'
  assert.equal(stripFocusBlocks(reply), 'Here is the plan.')
  assert.equal(stripFocusBlocks('a <focus-report>{}</focus-report> b <focus-findings>[]</focus-findings>'), 'a  b')
  // Quoting the format inside a fence stays visible.
  const quoted = 'Format:\n```\n<focus-pr>{"title":"…"}</focus-pr>\n```'
  assert.equal(stripFocusBlocks(quoted), quoted)
  assert.equal(stripFocusBlocks('ok <focus-stage>x</focus-stage>'), 'ok')
  assert.equal(stripFocusBlocks('nothing here'), 'nothing here')
})

test('stripFocusBlocks pairs a close tag with the nearest open tag, like the server', () => {
  // An inline mention of the tag before the real block keeps the prose.
  const mention = 'I will end with a <focus-report> block as asked.\n\nDone.\n\n<focus-report>{"summary":"s"}</focus-report>'
  assert.equal(stripFocusBlocks(mention), 'I will end with a <focus-report> block as asked.\n\nDone.')
  // A mention after the block stays too.
  const after = 'Done.\n\n<focus-report>{"summary":"s"}</focus-report>\n\nThe <focus-report> above is the summary.'
  assert.equal(stripFocusBlocks(after), 'Done.\n\nThe <focus-report> above is the summary.')
  // A stray mention of an unfinished block in a finished message stays.
  assert.equal(stripFocusBlocks('Use <focus-plan> next time.'), 'Use <focus-plan> next time.')
})

test('stripFocusBlocks hides a block still streaming only while the message streams', () => {
  const partial = 'Done.\n<focus-report>{"summary":"ha'
  assert.equal(stripFocusBlocks(partial, { streaming: true }), 'Done.')
  assert.equal(stripFocusBlocks('Done.\n<focus-rep', { streaming: true }), 'Done.')
  // A finished message keeps it: the server found no block there either.
  assert.equal(stripFocusBlocks(partial), partial)
  // While streaming, a complete block before the open one is stripped too.
  assert.equal(stripFocusBlocks('a <focus-plan>{}</focus-plan> b <focus-report>{"s', { streaming: true }), 'a  b')
})

const history: SessionMessage[] = [
  { id: 'u1', role: 'user', content: 'goal\n\n<focus-stage>\nFocus mode — current stage: plan.\n</focus-stage>', timestamp: '' },
  { id: 'a1', role: 'assistant', content: 'plan', timestamp: '' },
  { id: 'u2', role: 'user', content: 'Plan approved.\n\n<focus-stage>\nFocus mode — current stage: build (iteration 1 of 3).\n</focus-stage>', timestamp: '' },
  { id: 't2', role: 'tool', content: 'ok', timestamp: '', tool_name: 'exec' },
  { id: 'a2', role: 'assistant', content: 'built', timestamp: '' },
  { id: 'u3', role: 'user', content: 'thanks', timestamp: '' },
]

test('turnSlice and turnIndex find a card turn in the transcript', () => {
  assert.deepEqual(turnSlice(history, 2).map((m) => m.id), ['u2', 't2', 'a2'])
  assert.deepEqual(turnSlice(history, 3).map((m) => m.id), ['u3'])
  assert.deepEqual(turnSlice(history, 0), [])
  assert.deepEqual(turnSlice(history, 9), [])
  assert.equal(turnIndex(history, 'u2'), 2)
  assert.equal(turnIndex(history, 'nope'), 0)
})

test('turnStage reads the stage the guidance was written for', () => {
  assert.equal(turnStage(history[0].content), 'plan')
  assert.equal(turnStage(history[2].content), 'build')
  assert.equal(turnStage('no guidance'), null)
})

test('changeCards makes one card per changed file, acknowledged ones decided', () => {
  const cards = changeCards([
    {
      turnId: 'u2', turn: 2, stage: 'build', at: '2026-10-01T00:00:05Z',
      files: [
        { path: 'a.go', status: 'modified', additions: 2, deletions: 1, patch: '@@ -1 +1 @@\n-a\n+b\n' },
        { path: 'b.go', status: 'added', additions: 3, deletions: 0 },
      ],
    },
  ], new Set(['change:u2:b.go']))
  assert.deepEqual(cards.map((c) => [c.id, c.kind, c.stage, c.turn, c.title, c.state]), [
    ['change:u2:a.go', 'change', 'build', 2, 'a.go', 'unseen'],
    ['change:u2:b.go', 'change', 'build', 2, 'b.go', 'decided'],
  ])
  assert.deepEqual(cards[0].payload, { turn_id: 'u2', path: 'a.go', status: 'modified', additions: 2, deletions: 1, binary: undefined, patch: '@@ -1 +1 @@\n-a\n+b\n' })
})

test('planEdits applies stage skips and edited verification commands', () => {
  const plan = { goal: 'g', tasks: [{ title: 't', done: 'd' }], stages: ['plan', 'build', 'review', 'pr', 'merge'] as FocusPipeline['current'][], verify: ['make test'] }
  const edits = planEdits(plan, { skipped: new Set(['review', 'plan']), verify: ' make test \n\n go vet ./... \n' })
  assert.deepEqual(edits.stages, ['plan', 'build', 'pr', 'merge'])
  assert.deepEqual(edits.verify, ['make test', 'go vet ./...'])
  assert.deepEqual(plan.stages, ['plan', 'build', 'review', 'pr', 'merge'])
})

test('Advanced chat surfaces hide focus blocks: transcript, side panel, auto title', async () => {
  const { transcriptChatMessages } = await import('../src/lib/transcriptMessages.ts')
  const { historyMessages } = await import('../src/lib/sideSession.ts')
  const saved: SessionMessage[] = [
    { id: 'u1', role: 'user', content: 'ship it\n\n<focus-stage>\nFocus mode — current stage: plan.\n</focus-stage>', timestamp: '' },
    { id: 'a1', role: 'assistant', content: '<focus-plan>{"goal":"g"}</focus-plan>', timestamp: '' },
    { id: 'a2', role: 'assistant', content: 'Done.\n\n<focus-report>{"summary":"s"}</focus-report>', timestamp: '' },
  ]
  // The block-only reply has nothing to show; the user bubble strips itself.
  assert.deepEqual(transcriptChatMessages(saved).map((m) => m.id), ['u1', 'a2'])
  assert.deepEqual(historyMessages(saved).map((m) => m.text), ['ship it', 'Done.'])
})

test('routes: /console/focus is the focus home, /console/focus/<id> a pipeline', async () => {
  const { resolveRoute } = await import('../src/lib/router.ts')
  assert.deepEqual(resolveRoute('/console/focus'), { view: 'focus' })
  assert.deepEqual(resolveRoute('/console/focus/'), { view: 'focus' })
  assert.deepEqual(resolveRoute('/console/focus/abc%20d'), { view: 'focus', sessionId: 'abc d' })
  assert.deepEqual(resolveRoute('/console/focus/abc/x'), { view: 'focus', sessionId: 'abc' })
})

test('defaultModeRedirect lands /console on the focus home only when focus is the default', async () => {
  const { defaultModeRedirect } = await import('../src/lib/focus.ts')
  assert.equal(defaultModeRedirect('focus', { view: 'board' }), '/console/focus')
  assert.equal(defaultModeRedirect('advanced', { view: 'board' }), null)
  assert.equal(defaultModeRedirect('', { view: 'board' }), null)
  assert.equal(defaultModeRedirect('focus', { view: 'chat' }), null)
  assert.equal(defaultModeRedirect(undefined, { view: 'board' }), null)
})

test('deckCursor pins the card on screen and jumps to the first only when a card arrives or leaves', async () => {
  const { deckCursor } = await import('../src/lib/focus.ts')
  // Nothing known yet: the first card.
  assert.equal(deckCursor(new Set(), ['c4', 'c2'], null), 'c4')
  // c4 marked seen reorders the deck; the cursor stays on c4.
  assert.equal(deckCursor(new Set(['c4', 'c2']), ['c2', 'c4'], 'c4'), 'c4')
  // A new card arrives: go to the first (the highest priority).
  assert.equal(deckCursor(new Set(['c2', 'c4']), ['c5', 'c2', 'c4'], 'c2'), 'c5')
  // The pinned card left the deck (or none was pinned): the first.
  assert.equal(deckCursor(new Set(['c2', 'c4']), ['c2'], 'c4'), 'c2')
  assert.equal(deckCursor(new Set(['c2', 'c4']), ['c4', 'c2'], null), 'c4')
  assert.equal(deckCursor(new Set(['c2']), [], 'c2'), null)
})

test('onboardingModeUpdate writes focus only when no mode is set', async () => {
  const { onboardingModeUpdate } = await import('../src/lib/focus.ts')
  assert.deepEqual(onboardingModeUpdate({}), { console_default_mode: 'focus' })
  assert.deepEqual(onboardingModeUpdate({ console_default_mode: '' }), { console_default_mode: 'focus' })
  assert.deepEqual(onboardingModeUpdate({ console_default_mode: '  ' }), { console_default_mode: 'focus' })
  assert.equal(onboardingModeUpdate({ console_default_mode: 'advanced' }), null)
  assert.equal(onboardingModeUpdate({ console_default_mode: 'focus' }), null)
  // Config unreadable: the mode is unknown, so nothing is written.
  assert.equal(onboardingModeUpdate(null), null)
})

test('deckOrder keeps the order on screen until cards arrive or leave', async () => {
  const { deckOrder } = await import('../src/lib/focus.ts')
  // First look: the sorted order.
  assert.deepEqual(deckOrder([], ['a', 'b', 'c']), ['a', 'b', 'c'])
  // a was marked seen and sorts last now; the order on screen stays.
  assert.deepEqual(deckOrder(['a', 'b', 'c'], ['b', 'c', 'a']), ['a', 'b', 'c'])
  // A card arrives: the new sorted order.
  assert.deepEqual(deckOrder(['a', 'b', 'c'], ['d', 'b', 'c', 'a']), ['d', 'b', 'c', 'a'])
  // A card leaves: the new sorted order.
  assert.deepEqual(deckOrder(['a', 'b', 'c'], ['c', 'a']), ['c', 'a'])
})
