import test from 'node:test'
import assert from 'node:assert/strict'

import { formatResetCountdown } from '../src/lib/formatDuration.ts'

test('formatResetCountdown returns <1m for sub-minute durations', () => {
  assert.equal(formatResetCountdown(0), '<1m')
  assert.equal(formatResetCountdown(30), '<1m')
  assert.equal(formatResetCountdown(59), '<1m')
})

test('formatResetCountdown renders whole minutes', () => {
  assert.equal(formatResetCountdown(60), '1m')
  assert.equal(formatResetCountdown(599), '9m')
  assert.equal(formatResetCountdown(3599), '59m')
})

test('formatResetCountdown renders hours and minutes', () => {
  assert.equal(formatResetCountdown(3600), '1h')
  assert.equal(formatResetCountdown(3660), '1h 1m')
  assert.equal(formatResetCountdown(8040), '2h 14m')
})

test('formatResetCountdown returns empty string for missing or invalid input', () => {
  assert.equal(formatResetCountdown(undefined), '')
  assert.equal(formatResetCountdown(null), '')
  assert.equal(formatResetCountdown(-1), '')
  assert.equal(formatResetCountdown(NaN), '')
})
