import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const apiSource = readFileSync(new URL('../src/lib/api.ts', import.meta.url), 'utf8')
const typesSource = readFileSync(new URL('../src/lib/types.ts', import.meta.url), 'utf8')
const panelSource = readFileSync(new URL('../src/components/SkillExtractionPanel.svelte', import.meta.url), 'utf8')
const chatSource = readFileSync(new URL('../src/components/Chat.svelte', import.meta.url), 'utf8')
const slashSource = readFileSync(new URL('../src/lib/slash.ts', import.meta.url), 'utf8')

test('skill extraction inbox is wired into console chat', () => {
  const i18nEnSource = readFileSync(new URL('../src/i18n/en.ts', import.meta.url), 'utf8')
  assert.match(typesSource, /SkillExtractionCandidate/)
  assert.match(apiSource, /extractSkillsFromSession/)
  assert.match(apiSource, /\/v1\/admin\/skills\/extractions\/extract/)
  assert.match(apiSource, /reviewSkillExtractionCandidate/)
  assert.match(chatSource, /SkillExtractionPanel/)
  assert.match(i18nEnSource, /extractSkill: 'Extract Skill'/)
  assert.match(chatSource, /\$t\.chat\.session\.actions\.extractSkill/)
  assert.match(slashSource, /extract-skill/)
})
