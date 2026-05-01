import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const chatSource = readFileSync(new URL('../src/components/Chat.svelte', import.meta.url), 'utf8')

test('Chat keeps session config off the always-visible panel toggle row', () => {
  assert.doesNotMatch(chatSource, /title="Session tool config"/)
  assert.doesNotMatch(chatSource, /onclick=\{\(\) => togglePanel\('config'\)\}[^>]*>Config/)
})

test('Chat opens advanced session config only for an existing selected session', () => {
  assert.match(chatSource, /case 'config':[\s\S]*Select a session first[\s\S]*openPanel\('config'\)/)
  assert.match(chatSource, /panelID === 'config' && selectedSessionId/)
})
