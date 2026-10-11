import test from 'node:test'
import assert from 'node:assert/strict'

import {
  classifyCompanionAskError,
  companionAskExpressionOverride,
  companionAskRequest,
  companionAskTierRecommendation,
  isMainSessionBusy,
  suppressRunningLine,
  truncateCompanionAnswer,
} from '../src/lib/companionAsk.ts'

test('companionAskRequest sends the main session, console_context and the recommendation unchanged', () => {
  const req = companionAskRequest({
    message: 'anything new?',
    consoleContext: 'guidance',
    tierRecommendation: {
      task_type: 'companion_ask',
      recommended_tier: 'light',
      chosen_tier: 'light',
      reason: 'r',
      confidence: 1,
      accepted: true,
      source: 'companion',
    },
  })
  assert.equal(req.session_id, 'main')
  assert.equal(req.message, 'anything new?')
  assert.equal(req.console_context, 'guidance')
  assert.equal(req.interactive_permissions, true)
  assert.equal(req.tier_recommendation?.chosen_tier, 'light')
  assert.equal(req.tier_recommendation?.pin, undefined)
})

test('companionAskTierRecommendation defaults to light with no upstream session', () => {
  const rec = companionAskTierRecommendation({}, [], 'standard')
  assert.equal(rec?.chosen_tier, 'light')
})

test('companionAskTierRecommendation keeps light when the upstream CLI matches', () => {
  const tiers = [
    { name: 'light', kind: 'claude-code-cli' },
    { name: 'standard', kind: 'claude-code-cli' },
  ]
  const rec = companionAskTierRecommendation({ tier_pin: 'standard', upstream_session_id: 'sess-1' }, tiers, 'standard')
  assert.equal(rec?.chosen_tier, 'light')
})

test('companionAskTierRecommendation keeps light when the main session is on a native provider', () => {
  const tiers = [
    { name: 'light', kind: 'openai' },
    { name: 'standard', kind: 'openai' },
  ]
  const rec = companionAskTierRecommendation({ upstream_session_id: 'sess-1' }, tiers, 'standard')
  assert.equal(rec?.chosen_tier, 'light')
})

test('companionAskTierRecommendation falls back to the session tier on a CLI kind mismatch', () => {
  const tiers = [
    { name: 'light', kind: 'antigravity-cli' },
    { name: 'standard', kind: 'claude-code-cli' },
  ]
  const rec = companionAskTierRecommendation({ tier_pin: 'standard', upstream_session_id: 'sess-1' }, tiers, 'standard')
  assert.equal(rec, undefined)
})

test('companionAskTierRecommendation reads the default tier when nothing is pinned', () => {
  const tiers = [
    { name: 'light', kind: 'antigravity-cli' },
    { name: 'standard', kind: 'claude-code-cli' },
  ]
  const rec = companionAskTierRecommendation({ upstream_session_id: 'sess-1' }, tiers, 'standard')
  assert.equal(rec, undefined)
})

test('isMainSessionBusy reads the activity snapshot by session id', () => {
  assert.equal(isMainSessionBusy({ m1: { running: true, pending: 0, queued: 0, title: '' } }, 'm1'), true)
  assert.equal(isMainSessionBusy({ m1: { running: false, pending: 0, queued: 0, title: '' } }, 'm1'), false)
  assert.equal(isMainSessionBusy({}, 'm1'), false)
  assert.equal(isMainSessionBusy({ m1: { running: true, pending: 0, queued: 0, title: '' } }, ''), false)
})

test('truncateCompanionAnswer leaves a short answer alone', () => {
  const result = truncateCompanionAnswer('short answer', 480)
  assert.deepEqual(result, { shown: 'short answer', truncated: false })
})

test('truncateCompanionAnswer cuts a long answer at a word boundary and marks it truncated', () => {
  const text = `${'word '.repeat(100)}tail`
  const { shown, truncated } = truncateCompanionAnswer(text, 50)
  assert.equal(truncated, true)
  assert.ok(shown.length <= 51)
  assert.ok(shown.endsWith('…'))
  assert.ok(!shown.slice(0, -1).endsWith(' '))
})

test('classifyCompanionAskError maps statuses and network failures', () => {
  assert.equal(classifyCompanionAskError({ status: 409 }), 'busy')
  assert.equal(classifyCompanionAskError({ status: 429 }), 'limit')
  assert.equal(classifyCompanionAskError({ status: 503 }), 'error')
  assert.equal(classifyCompanionAskError(new TypeError('fetch failed')), 'network')
  assert.equal(classifyCompanionAskError(new Error('boom')), 'error')
  assert.equal(classifyCompanionAskError(undefined), 'error')
})

test('suppressRunningLine drops only the main session running line', () => {
  const lines = [
    { kind: 'running', key: 'running:main-1', label: '' },
    { kind: 'pending', key: 'pending:main-1', label: '' },
    { kind: 'running', key: 'running:other', label: '' },
  ]
  const filtered = suppressRunningLine(lines, 'main-1')
  assert.deepEqual(filtered.map((l) => l.key), ['pending:main-1', 'running:other'])
})

test('suppressRunningLine is a no-op without a main session id', () => {
  const lines = [{ kind: 'running', key: 'running:main-1', label: '' }]
  assert.deepEqual(suppressRunningLine(lines, null), lines)
})

test('companionAskExpressionOverride forces working only while asking and nothing more urgent is waiting', () => {
  assert.equal(companionAskExpressionOverride([], false), null)
  assert.equal(companionAskExpressionOverride([], true), 'working')
  assert.equal(companionAskExpressionOverride([{ kind: 'failure' }], true), null)
  assert.equal(companionAskExpressionOverride([{ kind: 'pending' }], true), null)
  assert.equal(companionAskExpressionOverride([{ kind: 'queued' }], true), null)
})
