import test from 'node:test'
import assert from 'node:assert/strict'

import {
  applySlashCandidate,
  buildSlashCandidates,
  findActiveSlashTrigger,
  parseLeadingSlashCommand,
} from '../src/lib/slash.ts'
import type { SkillDef } from '../src/lib/types.ts'

const skills: SkillDef[] = [
  {
    name: 'source-analyzer',
    description: 'Analyze source code.',
    source: 'workspace',
    user_invocable: true,
  },
  {
    name: 'review',
    description: 'Review code changes.',
    source: 'user',
    slash: 'code-review',
    aliases: ['rev'],
    user_invocable: true,
  },
  {
    name: 'hidden',
    description: 'Hidden skill.',
    user_invocable: false,
  },
]

test('findActiveSlashTrigger detects a leading slash command token', () => {
  assert.deepEqual(findActiveSlashTrigger('/sou', 4), {
    start: 0,
    end: 4,
    query: 'sou',
  })
  assert.deepEqual(findActiveSlashTrigger('  /rev', 6), {
    start: 2,
    end: 6,
    query: 'rev',
  })
})

test('findActiveSlashTrigger ignores non-leading slashes and completed command args', () => {
  assert.equal(findActiveSlashTrigger('look at /tmp/file', 17), null)
  assert.equal(findActiveSlashTrigger('/review this diff', 17), null)
})

test('buildSlashCandidates merges built-ins and user invocable skills', () => {
  const candidates = buildSlashCandidates('rev', skills)

  assert.deepEqual(candidates.map((c) => `${c.kind}:${c.command}:${c.skillName ?? c.id}`), [
    'skill:rev:review',
    'skill:code-review:review',
  ])
})

test('buildSlashCandidates gives built-ins precedence on command conflicts', () => {
  const candidates = buildSlashCandidates('config', [
    ...skills,
    { name: 'config', description: 'Skill conflict.', user_invocable: true },
  ])

  assert.equal(candidates[0].kind, 'builtin')
  assert.equal(candidates[0].command, 'config')
  assert.equal(candidates.some((c) => c.kind === 'skill' && c.command === 'config'), false)
})

test('applySlashCandidate replaces the active command token and preserves args', () => {
  const trigger = findActiveSlashTrigger('/sou please inspect', 4)
  assert.ok(trigger)

  const candidate = buildSlashCandidates('sou', skills)[0]
  const applied = applySlashCandidate('/sou please inspect', trigger, candidate)

  assert.equal(applied.value, '/source-analyzer please inspect')
  assert.equal(applied.caret, '/source-analyzer'.length)
})

test('parseLeadingSlashCommand returns command and argument text', () => {
  assert.deepEqual(parseLeadingSlashCommand('/review inspect this'), {
    command: 'review',
    args: 'inspect this',
  })
  assert.equal(parseLeadingSlashCommand('please /review this'), null)
})
