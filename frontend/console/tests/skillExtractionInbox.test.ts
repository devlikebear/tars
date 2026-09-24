import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { chatWorkbenchSource } from './helpers/chatWorkbenchSource.ts'
import { skillInboxEn } from '../src/i18n/sections/skillInbox.ts'

const apiSource = readFileSync(new URL('../src/lib/api/extensions.ts', import.meta.url), 'utf8')
const typesSource = readFileSync(new URL('../src/lib/types.ts', import.meta.url), 'utf8')
const panelSource = readFileSync(new URL('../src/components/SkillExtractionPanel.svelte', import.meta.url), 'utf8')
const chatSource = chatWorkbenchSource
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

test('skill extraction inbox exposes reviewed capability lifecycle controls and evidence', () => {
  assert.match(typesSource, /CapabilityState/)
  assert.match(typesSource, /EvaluationRun/)
  assert.match(typesSource, /CapabilityOutcome/)
  assert.match(typesSource, /'evaluate' \| 'approve' \| 'promote' \| 'rollback' \| 'reject'/)
  // The panel's text lives in the skillInbox i18n section.
  const sectionSource = readFileSync(new URL('../src/i18n/sections/skillInbox.ts', import.meta.url), 'utf8')
  assert.match(panelSource, /\$t\.skillInbox\.actions\.evaluate\b/)
  assert.equal(skillInboxEn.actions.evaluate, 'Evaluate draft')
  assert.match(panelSource, /\$t\.skillInbox\.actions\.approve\b/)
  assert.equal(skillInboxEn.actions.approve, 'Approve canary')
  assert.match(panelSource, /\$t\.skillInbox\.actions\.promote\b/)
  assert.equal(skillInboxEn.actions.promote, 'Promote 100%')
  assert.match(panelSource, /\$t\.skillInbox\.actions\.rollback\b/)
  assert.equal(skillInboxEn.actions.rollback, 'Roll back')
  assert.match(panelSource, /\$t\.skillInbox\.review\.permissionExpansion\b/)
  assert.equal(skillInboxEn.review.permissionExpansion, 'Permission expansion')
  assert.match(panelSource, /\$t\.skillInbox\.review\.evaluationDelta\b/)
  assert.equal(skillInboxEn.review.evaluationDelta, 'Evaluation delta')
  assert.match(panelSource, /\$t\.skillInbox\.review\.observedOutcomes\b/)
  assert.equal(skillInboxEn.review.observedOutcomes, 'Observed Work outcomes')
  assert.doesNotMatch(panelSource, /Saved skill draft/)
  assert.doesNotMatch(sectionSource, /Saved skill draft/)
})
