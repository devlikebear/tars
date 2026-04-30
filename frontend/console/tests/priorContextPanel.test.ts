import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const chatSource = readFileSync(new URL('../src/components/Chat.svelte', import.meta.url), 'utf8')
const chatPanelSource = readFileSync(new URL('../src/components/ChatPanel.svelte', import.meta.url), 'utf8')
const apiSource = readFileSync(new URL('../src/lib/api.ts', import.meta.url), 'utf8')
const panelSource = readFileSync(new URL('../src/components/PriorContextPanel.svelte', import.meta.url), 'utf8')

test('Chat surface exposes a Prior Context side panel wired to the draft message', () => {
  assert.match(chatSource, /import PriorContextPanel from '\.\/PriorContextPanel\.svelte'/)
  assert.match(chatSource, /type RightPanel = [^\n]*'prior'/)
  assert.match(chatSource, /rightPanel === 'prior'/)
  assert.match(chatSource, />Prior</)
  assert.match(chatSource, /<PriorContextPanel[\s\S]*draftQuery={chatDraft}/)
  assert.match(chatPanelSource, /onDraftChange\?: \(draft: string\) => void/)
  assert.match(chatPanelSource, /onDraftChange\?\.\(chatInput\)/)
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
