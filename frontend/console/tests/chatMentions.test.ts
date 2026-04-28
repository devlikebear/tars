import test from 'node:test'
import assert from 'node:assert/strict'

import {
  applyMentionCandidate,
  filterSelectedMentionsForMessage,
  findActiveMentionTrigger,
  type ChatMentionCandidate,
  type SelectedChatMention,
} from '../src/lib/chatMentions.ts'

const readmeCandidate: ChatMentionCandidate = {
  kind: 'file',
  name: 'README.md',
  path: 'README.md',
  root: '/workspace/project',
  root_label: 'current',
  token: '@README.md',
}

test('findActiveMentionTrigger detects @ token at the caret', () => {
  assert.deepEqual(findActiveMentionTrigger('open @read', 10), {
    start: 5,
    end: 10,
    query: 'read',
  })
  assert.deepEqual(findActiveMentionTrigger('@docs/no', 8), {
    start: 0,
    end: 8,
    query: 'docs/no',
  })
})

test('findActiveMentionTrigger ignores email-like text and spaced mentions', () => {
  assert.equal(findActiveMentionTrigger('mail a@b', 8), null)
  assert.equal(findActiveMentionTrigger('open @read later', 16), null)
})

test('applyMentionCandidate replaces the active token with an insert token', () => {
  const trigger = findActiveMentionTrigger('summarize @read please', 15)
  assert.ok(trigger)

  const applied = applyMentionCandidate('summarize @read please', trigger, readmeCandidate)

  assert.equal(applied.value, 'summarize @README.md please')
  assert.equal(applied.caret, 'summarize @README.md'.length)
  assert.deepEqual(applied.mention, {
    kind: 'file',
    root: '/workspace/project',
    path: 'README.md',
    label: 'README.md',
    token: '@README.md',
  })
})

test('filterSelectedMentionsForMessage keeps only mentions still present in text', () => {
  const selected: SelectedChatMention[] = [
    { kind: 'file', root: '/workspace/project', path: 'README.md', label: 'README.md', token: '@README.md' },
    { kind: 'directory', root: '/workspace/project', path: 'docs', label: 'docs', token: '@docs/' },
  ]

  assert.deepEqual(filterSelectedMentionsForMessage(selected, 'read @README.md'), [selected[0]])
})
