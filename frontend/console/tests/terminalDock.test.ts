import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { chatWorkbenchSource, readChatWorkbenchFile } from './helpers/chatWorkbenchSource.ts'
import { artifactPanelEn } from '../src/i18n/sections/artifactPanel.ts'

const chatSource = chatWorkbenchSource
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

// #667: the terminal renders once and moves between zones via data-zone, so
// xterm and its WebSocket survive a re-dock. That only holds while the
// terminal section and the zone panes share one parent component.
test('terminal pane stays in the same component as the dock zones', () => {
  const dockHostSource = readChatWorkbenchFile('../../src/components/ChatDockHost.svelte')
  assert.match(dockHostSource, /class="dock-pane dock-left"/)
  assert.match(dockHostSource, /class="dock-pane dock-bottom"/)
  assert.match(dockHostSource, /class="dock-pane dock-terminal" data-zone=\{terminalActiveZone\}/)
  assert.match(dockHostSource, /\.dock-terminal\[data-zone='bottom'\]/)
})

test('Files panel delegates Shell to the Chat dock while keeping external app fallback', () => {
  assert.doesNotMatch(artifactSource, /import IntegratedTerminal/)
  assert.match(artifactSource, /onOpenIntegratedTerminal: \(target: \{ cwd: string; label: string \}\) => void/)
  assert.match(artifactSource, /onOpenIntegratedTerminal\(\{ cwd: terminalCWDPath\(\), label: terminalTargetLabel\(\) \}\)/)
  assert.match(artifactSource, /title=\{\$t\.artifactPanel\.workspace\.openAppTitle\(terminalTargetLabel\(\)\)\}/)
  assert.equal(artifactPanelEn.workspace.openAppTitle('x'), 'Open macOS Terminal at x')
  // e2e/chat-workbench.spec.ts clicks button[title^="Open integrated terminal"].
  assert.match(artifactSource, /title=\{\$t\.artifactPanel\.workspace\.shellTitle\(terminalTargetLabel\(\)\)\}/)
  assert.equal(artifactPanelEn.workspace.shellTitle('x'), 'Open integrated terminal at x')
  assert.match(artifactSource, /onclick={openTerminalAtCurrentPath}/)
})

test('Integrated terminal can shrink with dock split resize', () => {
  assert.match(terminalSource, /\.terminal-frame \{[\s\S]*min-height: 0/)
  assert.match(terminalSource, /ResizeObserver\(\(\) => fitAndResize\(\)\)/)
})
