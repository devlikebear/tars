import test from 'node:test'
import assert from 'node:assert/strict'
import { parseGoalArgs } from '../src/lib/goalCommand.ts'

test('parseGoalArgs keeps a plain description as is', () => {
  assert.deepEqual(parseGoalArgs('  ship the feature  '), { description: 'ship the feature' })
  assert.deepEqual(parseGoalArgs(''), { description: '' })
})

test('parseGoalArgs reads a leading permission flag', () => {
  assert.deepEqual(parseGoalArgs('--auto ship the feature'), { description: 'ship the feature', permissionMode: 'auto' })
  assert.deepEqual(parseGoalArgs('--accept-edits tidy up'), { description: 'tidy up', permissionMode: 'accept_edits' })
  // Only leading flags count: the rest is the user's own words.
  assert.deepEqual(parseGoalArgs('make --auto the default'), { description: 'make --auto the default' })
})

test('parseGoalArgs rejects an unknown leading flag', () => {
  assert.deepEqual(parseGoalArgs('--yolo ship it'), { description: '', unknownFlag: '--yolo' })
})
