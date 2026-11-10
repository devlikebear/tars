import { test } from 'node:test'
import assert from 'node:assert/strict'
import { memoryQuestionState } from '../src/lib/memoryQuestion.ts'
import { transcriptChatMessages } from '../src/lib/transcriptMessages.ts'

test('a confirm_memory answer status maps to what the buttons show', () => {
  assert.equal(memoryQuestionState(undefined), 'loading')
  assert.equal(memoryQuestionState('pending'), 'open')
  assert.equal(memoryQuestionState('approved'), 'saved')
  assert.equal(memoryQuestionState('rejected'), 'skipped')
  assert.equal(memoryQuestionState('merged'), 'handled')
  assert.equal(memoryQuestionState('missing'), 'handled')
})

test('only a confirm_memory message with a candidate carries the answer entry', () => {
  const got = transcriptChatMessages([
    { id: 'q', role: 'assistant', content: 'Should I remember this?', timestamp: '', initiative: { intent: 'confirm_memory', entry_id: 'init_1', ref: 'cand' } },
    { id: 'r', role: 'assistant', content: 'Reminder: stretch', timestamp: '', initiative: { intent: 'remind', entry_id: 'init_2' } },
    { id: 'n', role: 'assistant', content: 'odd', timestamp: '', initiative: { intent: 'confirm_memory', entry_id: 'init_3' } },
  ])
  assert.equal(got[0].memoryQuestionEntryId, 'init_1')
  assert.equal(got[0].spokeFirst, true)
  assert.equal(got[1].memoryQuestionEntryId, undefined)
  assert.equal(got[1].spokeFirst, true)
  assert.equal(got[2].memoryQuestionEntryId, undefined)
})
