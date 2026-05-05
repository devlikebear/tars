import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const chatSource = readFileSync(new URL('../src/components/Chat.svelte', import.meta.url), 'utf8')
const panelSource = readFileSync(new URL('../src/components/SessionConfigPanel.svelte', import.meta.url), 'utf8')

test('Chat exposes session config in the panel toggle row', () => {
  assert.match(chatSource, /class:active=\{isPanelOpen\('config'\)\}/)
  assert.match(chatSource, /onclick=\{\(\) => togglePanel\('config'\)\}/)
  assert.match(chatSource, /\$t\.chat\.panels\.configTooltip/)
})

test('Chat opens advanced session config only for an existing selected session', () => {
  assert.match(chatSource, /case 'config':[\s\S]*Select a session first[\s\S]*openPanel\('config'\)/)
  assert.match(chatSource, /panelID === 'config' && selectedSessionId/)
})

test('Session config can reload session skills and commands separately', () => {
  assert.match(panelSource, /listSkills\(sessionId \|\| undefined\)/)
  assert.match(panelSource, /toolsResp\.commands/)
  assert.match(panelSource, /onclick=\{\(\) => \{ void load\(\) \}\}/)
  assert.match(panelSource, /skillSourceFilter/)
  assert.match(panelSource, /Session only/)
  assert.match(panelSource, /activeTab === 'commands'/)
  assert.match(panelSource, /commands_enabled/)
  assert.match(panelSource, /source-session/)
  assert.match(panelSource, /source-command/)
})
