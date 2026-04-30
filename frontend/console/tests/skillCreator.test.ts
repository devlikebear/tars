import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const extensionsSource = readFileSync(new URL('../src/components/Extensions.svelte', import.meta.url), 'utf8')
const apiSource = readFileSync(new URL('../src/lib/api.ts', import.meta.url), 'utf8')

test('Extensions wires a Skill Creator wizard into the installed Skills section', () => {
  assert.match(extensionsSource, /import SkillCreator from '\.\/SkillCreator\.svelte'/)
  assert.match(extensionsSource, /let skillCreatorOpen = \$state\(false\)/)
  assert.match(extensionsSource, /\+ Create Skill/)
  assert.match(extensionsSource, /<SkillCreator/)
  assert.match(extensionsSource, /onsaved={handleSkillCreated}/)
})

test('Skill Creator API client exposes draft, save, and submit endpoints', () => {
  assert.match(apiSource, /export async function draftSkill/)
  assert.match(apiSource, /\/v1\/admin\/skills\/draft/)
  assert.match(apiSource, /export async function saveLocalSkill/)
  assert.match(apiSource, /\/v1\/admin\/skills\/save-local/)
  assert.match(apiSource, /export async function testSkillDraft/)
  assert.match(apiSource, /\/v1\/admin\/skills\/test/)
  assert.match(apiSource, /export async function submitSkillDraftPR/)
  assert.match(apiSource, /\/v1\/admin\/skills\/submit-pr/)
})

test('Skill Creator component exposes sandbox test controls and output', () => {
  const source = readFileSync(new URL('../src/components/SkillCreator.svelte', import.meta.url), 'utf8')
  assert.match(source, /testSkillDraft/)
  assert.match(source, /Test/)
  assert.match(source, /testResult/)
  assert.match(source, /stdout/i)
  assert.match(source, /Tool Trail/)
})
