import test from 'node:test'
import assert from 'node:assert/strict'

import { formatCodexStatusLines } from '../src/lib/codexStatus.ts'
import type { CodexUsageTier } from '../src/lib/types'

test('formatCodexStatusLines reports empty when no openai-codex tier', () => {
  const lines = formatCodexStatusLines([])
  assert.deepEqual(lines, ['Codex status: no openai-codex tiers configured.'])
})

test('formatCodexStatusLines drops non openai-codex providers', () => {
  const tiers: CodexUsageTier[] = [
    { tier: 'heavy', provider: 'anthropic', model: 'claude' },
  ]
  assert.deepEqual(formatCodexStatusLines(tiers), [
    'Codex status: no openai-codex tiers configured.',
  ])
})

test('formatCodexStatusLines renders awaiting-first-request when snapshot missing', () => {
  const tiers: CodexUsageTier[] = [
    { tier: 'heavy', provider: 'openai-codex', model: 'gpt-5.3-codex' },
  ]
  const lines = formatCodexStatusLines(tiers)
  assert.equal(lines[0], 'Codex status:')
  assert.equal(lines[1], '  [heavy] gpt-5.3-codex  Awaiting first request…')
})

test('formatCodexStatusLines renders bar + percent + reset/window for each window', () => {
  const tiers: CodexUsageTier[] = [
    {
      tier: 'standard',
      provider: 'openai-codex',
      model: 'gpt-5.5',
      snapshot: {
        captured_at: '',
        primary: { used_percent: 21.0, reset_after_seconds: 13500, window_minutes: 300 },
        secondary: { used_percent: 14.0, reset_after_seconds: 525060, window_minutes: 10080 },
      },
    },
  ]
  const lines = formatCodexStatusLines(tiers)
  assert.equal(lines.length, 4)
  assert.equal(lines[1], '  [standard] gpt-5.5')
  assert.equal(lines[2], '    primary  ██░░░░░░░░   21.0%  (resets 3h 45m / 5h)')
  assert.equal(lines[3], '    weekly   █░░░░░░░░░   14.0%  (resets 145h 51m / 7d)')
})

test('formatCodexStatusLines bar handles 0%, 100% and rounding', () => {
  const tiers: CodexUsageTier[] = [
    {
      tier: 'a',
      provider: 'openai-codex',
      model: 'm',
      snapshot: { captured_at: '', primary: { used_percent: 0 } },
    },
    {
      tier: 'b',
      provider: 'openai-codex',
      model: 'm',
      snapshot: { captured_at: '', primary: { used_percent: 100 } },
    },
    {
      tier: 'c',
      provider: 'openai-codex',
      model: 'm',
      snapshot: { captured_at: '', primary: { used_percent: 95.0 } },
    },
  ]
  const lines = formatCodexStatusLines(tiers)
  // tier blocks at lines 2, 4, 6 (each tier = head + 1 window line)
  assert.match(lines[2], /░░░░░░░░░░ {4}0\.0%/)
  assert.match(lines[4], /██████████ +100\.0%/)
  assert.match(lines[6], /██████████ +95\.0%/)
})

test('formatCodexStatusLines clamps out-of-range percentages on the bar', () => {
  const tiers: CodexUsageTier[] = [
    {
      tier: 'oob',
      provider: 'openai-codex',
      model: 'm',
      snapshot: { captured_at: '', primary: { used_percent: 150 } },
    },
  ]
  const lines = formatCodexStatusLines(tiers)
  assert.match(lines[2], /██████████/)
})

test('formatCodexStatusLines omits reset when not provided', () => {
  const tiers: CodexUsageTier[] = [
    {
      tier: 'a',
      provider: 'openai-codex',
      model: 'm',
      snapshot: {
        captured_at: '',
        primary: { used_percent: 50, window_minutes: 300 },
      },
    },
  ]
  const lines = formatCodexStatusLines(tiers)
  assert.equal(lines[2], '    primary  █████░░░░░   50.0%  (5h window)')
})

test('formatCodexStatusLines emits one block per tier in input order', () => {
  const tiers: CodexUsageTier[] = [
    {
      tier: 'heavy',
      provider: 'openai-codex',
      model: 'a',
      snapshot: { captured_at: '', primary: { used_percent: 50 } },
    },
    {
      tier: 'standard',
      provider: 'openai-codex',
      model: 'b',
      snapshot: { captured_at: '' },
    },
  ]
  const lines = formatCodexStatusLines(tiers)
  // header + heavy(head + 1 window) + standard(head + no-data) = 5
  assert.equal(lines.length, 5)
  assert.match(lines[1], /\[heavy\]/)
  assert.match(lines[3], /\[standard\]/)
  assert.match(lines[4], /no window data/)
})

test('formatCodexStatusLines case-insensitive provider match', () => {
  const tiers: CodexUsageTier[] = [
    {
      tier: 'light',
      provider: 'OpenAI-Codex',
      model: 'm',
      snapshot: { captured_at: '', primary: { used_percent: 1 } },
    },
  ]
  const lines = formatCodexStatusLines(tiers)
  assert.equal(lines.length, 3)
})
