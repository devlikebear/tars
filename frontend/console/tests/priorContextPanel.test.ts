import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { chatWorkbenchSource } from './helpers/chatWorkbenchSource.ts'

const chatSource = chatWorkbenchSource
const chatPanelSource = readFileSync(new URL('../src/components/ChatPanel.svelte', import.meta.url), 'utf8')
const apiSource = readFileSync(new URL('../src/lib/api/chat.ts', import.meta.url), 'utf8')
const panelSource = readFileSync(new URL('../src/components/PriorContextPanel.svelte', import.meta.url), 'utf8')

test('Chat surface exposes a Prior Context side panel wired to the draft message', () => {
  const i18nEnSource = readFileSync(new URL('../src/i18n/en.ts', import.meta.url), 'utf8')
  assert.match(chatSource, /import PriorContextPanel from '\.\/PriorContextPanel\.svelte'/)
  assert.match(chatSource, /type ChatDockPanelID = [^\n]*'prior'/)
  assert.match(chatSource, /panelID === 'prior'/)
  // The panel rail lists Prior with its own tooltip.
  assert.match(chatSource, /\{ id: 'prior', icon: [^}]*tooltip: 'priorTooltip' \}/)
  assert.match(i18nEnSource, /prior: 'Prior'/)
  assert.match(chatSource, /<PriorContextPanel[\s\S]*draftQuery=\{chatDraft\}/)
  // The draft reaches Chat through the shared session store, not a callback.
  assert.match(chatPanelSource, /chatSession\.draft = chatInput/)
  assert.match(chatSource, /let chatDraft = \$derived\(chatSession\.draft\)/)
})

test('Prior Context panel uses the preview API and renders source and budget fields', () => {
  assert.match(apiSource, /export type PriorContextPreview/)
  assert.match(apiSource, /getPriorContextPreview/)
  assert.match(apiSource, /\/v1\/chat\/prior-context\/preview/)
  assert.match(panelSource, /getPriorContextPreview/)
  assert.match(panelSource, /Refresh preview/)
  assert.match(panelSource, /source_tag/)
  assert.match(panelSource, /budget_percent/)
  assert.match(panelSource, /section/)
})
