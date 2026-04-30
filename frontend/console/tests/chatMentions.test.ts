import test from 'node:test'
import assert from 'node:assert/strict'

import {
  applyMentionCandidate,
  buildSubagentMentionCandidates,
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

const researcherCandidate: ChatMentionCandidate = {
  kind: 'subagent',
  name: 'researcher',
  path: 'researcher',
  root: 'agentruntime',
  root_label: 'subagent',
  token: '@researcher',
  description: 'Deep research subagent',
  tier: 'heavy',
  model: 'gpt-5.5',
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

test('applyMentionCandidate supports subagent mentions', () => {
  const trigger = findActiveMentionTrigger('ask @rese to inspect', 9)
  assert.ok(trigger)

  const applied = applyMentionCandidate('ask @rese to inspect', trigger, researcherCandidate)

  assert.equal(applied.value, 'ask @researcher to inspect')
  assert.equal(applied.caret, 'ask @researcher'.length)
  assert.deepEqual(applied.mention, {
    kind: 'subagent',
    root: 'agentruntime',
    path: 'researcher',
    label: 'researcher',
    token: '@researcher',
  })
})

test('filterSelectedMentionsForMessage keeps only mentions still present in text', () => {
  const selected: SelectedChatMention[] = [
    { kind: 'file', root: '/workspace/project', path: 'README.md', label: 'README.md', token: '@README.md' },
    { kind: 'directory', root: '/workspace/project', path: 'docs', label: 'docs', token: '@docs/' },
    { kind: 'subagent', root: 'agentruntime', path: 'researcher', label: 'researcher', token: '@researcher' },
  ]

  assert.deepEqual(filterSelectedMentionsForMessage(selected, 'ask @researcher to read @README.md'), [selected[0], selected[2]])
})

test('buildSubagentMentionCandidates filters enabled subagents by query', () => {
  const candidates = buildSubagentMentionCandidates('res', [
    {
      name: 'researcher',
      description: 'Deep research subagent',
      enabled: true,
      effective_tier: 'heavy',
      resolved_model: 'gpt-5.5',
      source: 'config',
    },
    {
      name: 'writer',
      description: 'Drafting helper',
      enabled: true,
      effective_tier: 'standard',
      resolved_model: 'gpt-5.4',
      source: 'config',
    },
    {
      name: 'disabled-researcher',
      description: 'Hidden helper',
      enabled: false,
      effective_tier: 'light',
      resolved_model: 'gpt-5.4-mini',
      source: 'config',
    },
  ])

  assert.deepEqual(candidates, [researcherCandidate])
})
