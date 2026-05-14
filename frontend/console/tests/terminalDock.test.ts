import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const chatSource = readFileSync(new URL('../src/components/Chat.svelte', import.meta.url), 'utf8')
const artifactSource = readFileSync(new URL('../src/components/ArtifactPanel.svelte', import.meta.url), 'utf8')
const terminalSource = readFileSync(new URL('../src/components/IntegratedTerminal.svelte', import.meta.url), 'utf8')
const terminalTabsSource = readFileSync(new URL('../src/components/TerminalTabs.svelte', import.meta.url), 'utf8')

test('Chat owns the integrated terminal as a bottom dock panel', () => {
  assert.match(terminalTabsSource, /import IntegratedTerminal from '\.\/IntegratedTerminal\.svelte'/)
  assert.match(chatSource, /type ChatDockPanelID = [^\n]*'terminal'/)
  assert.match(chatSource, /id: 'terminal'[^}]*defaultZone: 'bottom'/)
  assert.match(chatSource, /terminalDockSessionId/)
  assert.match(chatSource, /<TerminalTabsRoute\b[\s\S]*sessionId=\{terminalDockSessionId\}/)
  assert.match(chatSource, /onOpenIntegratedTerminal=\{openIntegratedTerminalDock\}/)
})

test('Files panel delegates Shell to the Chat dock while keeping external app fallback', () => {
  assert.doesNotMatch(artifactSource, /import IntegratedTerminal/)
  assert.match(artifactSource, /onOpenIntegratedTerminal: \(target: \{ cwd: string; label: string \}\) => void/)
  assert.match(artifactSource, /onOpenIntegratedTerminal\(\{ cwd: terminalCWDPath\(\), label: terminalTargetLabel\(\) \}\)/)
  assert.match(artifactSource, /Open macOS Terminal at/)
  assert.match(artifactSource, /onclick={openTerminalAtCurrentPath}/)
})

test('Integrated terminal can shrink with dock split resize', () => {
  assert.match(terminalSource, /\.terminal-frame \{[\s\S]*min-height: 0/)
  assert.match(terminalSource, /ResizeObserver\(\(\) => fitAndResize\(\)\)/)
})
