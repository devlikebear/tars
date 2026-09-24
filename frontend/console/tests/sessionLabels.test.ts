import test from 'node:test'
import assert from 'node:assert/strict'

import { displaySessionTitle, untitledSessionTitle } from '../src/lib/sessionLabels.ts'

test('an untitled session shows the console language placeholder', () => {
  assert.equal(untitledSessionTitle, 'New Chat')
  assert.equal(displaySessionTitle('New Chat', '새 대화'), '새 대화')
  assert.equal(displaySessionTitle('  New Chat ', '새 대화'), '새 대화')
  assert.equal(displaySessionTitle('New Chat', 'New Chat'), 'New Chat')
})

test('a real title and an empty one pass through', () => {
  assert.equal(displaySessionTitle('Refactor the dock', '새 대화'), 'Refactor the dock')
  assert.equal(displaySessionTitle('new chat about X', '새 대화'), 'new chat about X')
  assert.equal(displaySessionTitle('', '새 대화'), '')
  assert.equal(displaySessionTitle(undefined, '새 대화'), '')
})
