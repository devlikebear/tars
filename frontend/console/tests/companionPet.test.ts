import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import {
  companionAskHandoffReaction,
  companionPromptForAsk,
  companionReactionForStimulus,
  companionReactionFromEvent,
  companionUiText,
  shouldShowCompanion,
} from '../src/lib/companion.ts'

const appSource = readFileSync(new URL('../src/App.svelte', import.meta.url), 'utf8')
const componentSource = readFileSync(new URL('../src/components/CompanionPet.svelte', import.meta.url), 'utf8')
const helperSource = readFileSync(new URL('../src/lib/companion.ts', import.meta.url), 'utf8')

test('companion visibility is bounded by config and setup/auth state', () => {
  assert.equal(shouldShowCompanion({ enabled: true, needsSetup: false, loginRequired: false, zenActive: false }), true)
  assert.equal(shouldShowCompanion({ enabled: false, needsSetup: false, loginRequired: false, zenActive: false }), false)
  assert.equal(shouldShowCompanion({ enabled: true, needsSetup: true, loginRequired: false, zenActive: false }), false)
  assert.equal(shouldShowCompanion({ enabled: true, needsSetup: false, loginRequired: true, zenActive: false }), false)
  assert.equal(shouldShowCompanion({ enabled: true, needsSetup: false, loginRequired: false, zenActive: true }), false)
})

test('console app wires the floating companion to config schema', () => {
  assert.match(appSource, /CompanionPet/)
  assert.match(appSource, /getConfigSchema/)
  assert.match(helperSource, /companion_enabled/)
  assert.match(appSource, /shouldShowCompanion/)
  assert.match(appSource, /companionReactionFromEvent/)
  assert.match(appSource, /onStimulus=\{handleCompanionStimulus\}/)
  assert.match(appSource, /onAsk=\{handleCompanionAsk\}/)
  assert.match(appSource, /locale=\{\$locale\}/)
  assert.match(appSource, /routeView=\{route\.view\}/)
})

test('companion pet is an interactive floating console presence', () => {
  assert.match(componentSource, /aria-label=\{labels\.buttonAria\}/)
  assert.match(componentSource, /companion-bubble/)
  assert.match(componentSource, /manualPriority/)
  assert.match(componentSource, /activeAction/)
  assert.match(componentSource, /companion-feedback-strip/)
  assert.match(componentSource, /aria-pressed=\{activeAction === 'poke'\}/)
  assert.match(componentSource, /position:\s*fixed;/)
  assert.match(componentSource, /bottom:\s*var\(--space-5\);/)
  assert.match(componentSource, /right:\s*var\(--space-5\);/)
  assert.match(componentSource, /pointer-events:\s*auto;/)
  assert.match(componentSource, /@keyframes companionBlink/)
})

test('companion creates short feedback from user stimuli and runtime events', () => {
  assert.equal(companionReactionForStimulus('poke', 'home').mood, 'spark')
  assert.match(companionReactionForStimulus('suggest', 'pulse').message, /Pulse/i)
  assert.match(companionReactionForStimulus('feedback', 'chat').message, /review/i)
  assert.match(companionReactionForStimulus('poke', 'home', 'ko').message, /같이 보고/)
  assert.match(companionReactionForStimulus('suggest', 'pulse', 'ko').message, /펄스/)

  const embodiment = companionReactionFromEvent({
    type: 'notification',
    category: 'embodiment',
    severity: 'info',
    title: 'Embodiment percept: host',
    message: 'audio owner: Owner asked a question.',
    timestamp: '2026-05-19T00:00:00Z',
  })
  assert.equal(embodiment?.mood, 'focus')
  assert.match(embodiment?.message || '', /heard/i)

  const koreanEmbodiment = companionReactionFromEvent({
    type: 'notification',
    category: 'embodiment',
    severity: 'info',
    title: 'Embodiment percept: host',
    message: 'audio owner: Owner asked a question.',
    timestamp: '2026-05-19T00:00:00Z',
  }, 'ko')
  assert.match(koreanEmbodiment?.message || '', /들었어요/)

  const error = companionReactionFromEvent({
    type: 'notification',
    category: 'cron',
    severity: 'error',
    title: 'Cron failed',
    message: 'nightly job failed',
    timestamp: '2026-05-19T00:00:00Z',
  })
  assert.equal(error?.mood, 'error')
})

test('companion ask prompt hands off to normal chat with bounded context', () => {
  const prompt = companionPromptForAsk('what should I inspect?', 'agentruntime')
  assert.match(prompt, /TARS companion/i)
  assert.match(prompt, /agentruntime/i)
  assert.match(prompt, /what should I inspect/)

  const koreanPrompt = companionPromptForAsk('어디를 보면 돼?', 'pulse', 'ko')
  assert.match(koreanPrompt, /TARS 콘솔/)
  assert.match(koreanPrompt, /사용자 자극: 어디를 보면 돼\?/)
  assert.match(companionAskHandoffReaction('ko').message, /채팅/)
})

test('companion labels follow console locale', () => {
  assert.equal(companionUiText('en').actions.poke, 'Poke')
  assert.equal(companionUiText('ko-KR').actions.poke, '콕 찌르기')
  assert.equal(companionUiText('ko').send, '묻기')
})
