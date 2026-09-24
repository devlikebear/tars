import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import { buildTierRecommendation, tierRecommendationPayload } from '../src/lib/tierRecommendation.ts'
import { chatThreadEn } from '../src/i18n/sections/chatThread.ts'

const chatPanelSource = readFileSync(new URL('../src/components/ChatPanel.svelte', import.meta.url), 'utf8')
const typesSource = readFileSync(new URL('../src/lib/types.ts', import.meta.url), 'utf8')

test('tier recommendation classifies first-turn work before chat starts', () => {
  const heavy = buildTierRecommendation('Implement the GitHub issue, run tests, push a PR, and verify the release.')
  assert.equal(heavy.recommended_tier, 'heavy')
  assert.equal(heavy.task_type, 'coding')
  assert.ok(heavy.should_prompt)

  const light = buildTierRecommendation('Translate this sentence to Korean.')
  assert.equal(light.recommended_tier, 'light')
  assert.equal(light.task_type, 'light_transform')

  const standard = buildTierRecommendation('Brainstorm a few UX ideas for a dashboard.')
  assert.equal(standard.recommended_tier, 'standard')
  assert.equal(standard.task_type, 'general')
})

test('tier recommendation payload records accepted and overridden choices', () => {
  const rec = buildTierRecommendation('Summarize this note.')
  const accepted = tierRecommendationPayload(rec, 'light')
  const overridden = tierRecommendationPayload(rec, 'standard')

  assert.equal(accepted.accepted, true)
  assert.equal(accepted.chosen_tier, 'light')
  assert.equal(overridden.accepted, false)
  assert.equal(overridden.chosen_tier, 'standard')
})

test('chat panel exposes first-turn tier recommendation controls', () => {
  assert.match(typesSource, /tier_recommendation\?: ChatTierRecommendationRequest/)
  assert.match(chatPanelSource, /pendingTierRecommendation/)
  assert.match(chatPanelSource, /tier-recommendation-card/)
  assert.match(chatPanelSource, /continueWithTier/)
})

test('the tier card shows the reason the payload carries, in English', () => {
  // The card reads the reason from the section by task_type; the payload keeps
  // the lib's reason. In English the two must stay the same words.
  assert.match(chatPanelSource, /\{tierReasonLabel\(pendingTierRecommendation\)\}/)
  const cases = [
    ['coding', 'Implement the GitHub issue and run tests.'],
    ['lightTransform', 'Summarize this note.'],
    ['general', 'Brainstorm a few UX ideas for a dashboard.'],
  ] as const
  for (const [key, message] of cases) {
    assert.equal(chatThreadEn.tierReason[key], buildTierRecommendation(message).reason)
  }
})
