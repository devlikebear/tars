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
  assert.equal(lines[1], '  [heavy] gpt-5.3-codex · Awaiting first request…')
})

test('formatCodexStatusLines renders primary and weekly windows with reset countdown', () => {
  const tiers: CodexUsageTier[] = [
    {
      tier: 'heavy',
      provider: 'openai-codex',
      model: 'gpt-5.3-codex',
      snapshot: {
        captured_at: '2026-05-10T11:00:00Z',
        primary: { used_percent: 37.5, reset_after_seconds: 1080 },
        secondary: { used_percent: 8 },
      },
    },
  ]
  const lines = formatCodexStatusLines(tiers)
  assert.equal(lines.length, 2)
  assert.equal(lines[1], '  [heavy] gpt-5.3-codex · primary 37.5% (resets 18m) · weekly 8.0%')
})

test('formatCodexStatusLines emits one line per tier in input order', () => {
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
  assert.equal(lines.length, 3)
  assert.match(lines[1], /\[heavy\]/)
  assert.match(lines[2], /\[standard\]/)
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
  assert.equal(lines.length, 2)
})
