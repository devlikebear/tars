import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const apiSource = readFileSync(new URL('../src/lib/api.ts', import.meta.url), 'utf8')
const typesSource = readFileSync(new URL('../src/lib/types.ts', import.meta.url), 'utf8')
const panelSource = readFileSync(new URL('../src/components/SkillExtractionPanel.svelte', import.meta.url), 'utf8')
const chatSource = readFileSync(new URL('../src/components/Chat.svelte', import.meta.url), 'utf8')
const slashSource = readFileSync(new URL('../src/lib/slash.ts', import.meta.url), 'utf8')

test('skill extraction inbox is wired into console chat', () => {
  assert.match(typesSource, /SkillExtractionCandidate/)
  assert.match(apiSource, /extractSkillsFromSession/)
  assert.match(apiSource, /\/v1\/admin\/skills\/extractions\/extract/)
  assert.match(apiSource, /reviewSkillExtractionCandidate/)
  assert.match(panelSource, /Extract from session/)
  assert.match(panelSource, /Approve draft/)
  assert.match(chatSource, /SkillExtractionPanel/)
  assert.match(chatSource, /Extract Skill/)
  assert.match(slashSource, /extract-skill/)
})
