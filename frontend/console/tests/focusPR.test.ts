import test from 'node:test'
import assert from 'node:assert/strict'

import { ciCounts, finishedText, finishOutcome, ghUnavailable, mergeSummary, prDraftOf, prStageChip } from '../src/lib/focusPR.ts'
import type { FocusCard, FocusPipeline, FocusStageId, FocusStageStatus } from '../src/lib/types.ts'

function pipeline(statuses: Partial<Record<FocusStageId, FocusStageStatus>>, current: FocusStageId, extra: Partial<FocusPipeline> = {}): FocusPipeline {
  const ids: FocusStageId[] = ['plan', 'build', 'review', 'pr', 'pr_review', 'merge']
  return {
    version: 1,
    session_id: 's1',
    goal: 'g',
    current,
    stages: ids.map((id) => ({ id, status: statuses[id] ?? 'done', iteration: 1 })),
    open_gate: '',
    cards: [],
    updated_at: '',
    plan: { goal: 'g', tasks: [], stages: ids, verify: [] },
    ...extra,
  }
}

const checks = [
  { name: 'test', state: 'fail' as const },
  { name: 'lint', state: 'pass' as const },
  { name: 'docs', state: 'skipped' as const },
  { name: 'e2e', state: 'pending' as const },
]

test('ciCounts counts skipped as passed', () => {
  assert.deepEqual(ciCounts(checks), { passed: 2, failed: 1, pending: 1 })
  assert.deepEqual(ciCounts(undefined), { passed: 0, failed: 0, pending: 0 })
})

test('prStageChip shows the PR on the PR stages it reached', () => {
  const p = pipeline({ pr: 'done', pr_review: 'active', merge: 'pending' }, 'pr_review', {
    pr: { number: 12, url: 'https://example.test/pr/12', state: 'OPEN', checks },
  })
  assert.deepEqual(prStageChip(p, 'pr'), { number: 12, url: 'https://example.test/pr/12', ci: null })
  assert.deepEqual(prStageChip(p, 'pr_review'), { number: 12, url: 'https://example.test/pr/12', ci: { passed: 2, failed: 1, pending: 1 } })
  assert.equal(prStageChip(p, 'merge'), null, 'a pending stage shows nothing')
  assert.equal(prStageChip(p, 'build'), null)
  assert.equal(prStageChip(pipeline({}, 'pr'), 'pr'), null, 'no PR yet')
  const noChecks = pipeline({ merge: 'active' }, 'merge', { pr: { number: 3, url: '', state: 'OPEN' } })
  assert.deepEqual(prStageChip(noChecks, 'merge'), { number: 3, url: '', ci: null })
})

function notice(stage: FocusStageId, state: FocusCard['state'] = 'unseen'): FocusCard {
  return { id: 'n1', kind: 'notice', stage, turn: 0, title: 'gh unavailable', state, created_at: '', payload: { error: 'gh not found' } }
}

test('ghUnavailable follows the latest probe, not the notice card', () => {
  const p = pipeline({ pr: 'active', pr_review: 'pending', merge: 'pending' }, 'pr', { cards: [notice('pr')], pr_unavailable: 'gh not found' })
  assert.equal(ghUnavailable(p), 'gh not found')
  assert.equal(ghUnavailable({ ...p, pr_unavailable: undefined }), null, 'a later probe ran: the notice stays, the banner goes')
  assert.equal(ghUnavailable({ ...p, open_gate: 'pr' }), null, 'a gate decides first')
  assert.equal(ghUnavailable({ ...p, pending_turn: 'Fix the accepted findings' }), null, 'a turn is owed: a pass now would be refused (409)')
  assert.equal(ghUnavailable({ ...p, stages: p.stages.map((s) => (s.id === 'pr' ? { ...s, status: 'blocked' as const } : s)) }), null)
})

test('finishOutcome reports the merge and what the server did with the worktree', () => {
  const merged = { number: 1, url: '', state: 'MERGED' }
  assert.deepEqual(finishOutcome(pipeline({}, 'merge', { pr: merged, worktree_end: { action: 'discard' } })), { merged: true, worktree: 'discard', reason: '' })
  assert.deepEqual(finishOutcome(pipeline({}, 'merge', { pr: merged, worktree_end: { action: 'keep', reason: 'the worktree has uncommitted changes' } })), { merged: true, worktree: 'keep', reason: 'the worktree has uncommitted changes' })
  assert.deepEqual(finishOutcome(pipeline({}, 'merge', { pr: merged, worktree_end: { action: 'none' } })), { merged: true, worktree: 'none', reason: '' })
  assert.deepEqual(finishOutcome(pipeline({}, 'merge')), { merged: false, worktree: null, reason: '' }, 'not recorded yet')
  assert.equal(finishOutcome(pipeline({ merge: 'active' }, 'merge')), null, 'still running')
  const noMerge = pipeline({ pr: 'skipped', pr_review: 'skipped', merge: 'skipped' }, 'review', { plan: { goal: 'g', tasks: [], stages: ['plan', 'build', 'review'], verify: [] } })
  assert.equal(finishOutcome(noMerge), null)
})

test('finishedText words each outcome', () => {
  const text = {
    finished: 'F.',
    finishedMerged: 'M, removed.',
    finishedMergedOnly: 'M.',
    finishedKept: 'K.',
    finishedKeptBecause: (r: string) => `K: ${r}.`,
    finishedLeft: (r: string) => `L: ${r}.`,
  }
  assert.equal(finishedText({ merged: true, worktree: 'discard', reason: '' }, text), 'M, removed.')
  assert.equal(finishedText({ merged: true, worktree: 'keep', reason: 'dirty' }, text), 'M. K: dirty.')
  assert.equal(finishedText({ merged: false, worktree: 'keep', reason: 'by hand' }, text), 'F. K: by hand.')
  assert.equal(finishedText({ merged: true, worktree: 'none', reason: '' }, text), 'M.')
  assert.equal(finishedText({ merged: false, worktree: 'none', reason: '' }, text), 'F.')
  assert.equal(finishedText({ merged: true, worktree: 'left', reason: 'a turn was running' }, text), 'M. L: a turn was running.')
  assert.equal(finishedText({ merged: false, worktree: null, reason: '' }, text), 'F.')
  assert.equal(finishedText(null, text), 'F.')
})

test('prDraftOf and mergeSummary read the PR gates', () => {
  const g3: FocusCard = { id: 'c1', kind: 'gate', stage: 'pr', turn: 3, title: 'Open pull request', state: 'unseen', created_at: '', payload: { title: 't', body: 'b' } }
  assert.deepEqual(prDraftOf(g3), { title: 't', body: 'b' })
  assert.equal(prDraftOf({ ...g3, stage: 'plan' }), null)
  assert.equal(prDraftOf({ ...g3, kind: 'report' }), null)
  const g4: FocusCard = { ...g3, id: 'c2', stage: 'merge', payload: { pr: { number: 9, url: 'u', state: 'OPEN' }, title: 't', checks: { passed: 3, failed: 0, pending: 0 }, fixed: 1, dismissed: 2, undecided: 0, merge_state: 'CLEAN' } }
  assert.deepEqual(mergeSummary(g4)?.checks, { passed: 3, failed: 0, pending: 0 })
  assert.equal(mergeSummary(g4)?.pr?.number, 9)
  assert.equal(mergeSummary(g3), null)
})
