import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { chatWorkbenchSource, readChatWorkbenchFile } from './helpers/chatWorkbenchSource.ts'

const chatSource = chatWorkbenchSource
const panelSource = readFileSync(new URL('../src/components/SessionConfigPanel.svelte', import.meta.url), 'utf8')

test('Chat exposes session config on the panel rail', () => {
  const railSource = readChatWorkbenchFile('../../src/components/ChatRail.svelte')
  assert.match(railSource, /\{ id: 'config', icon: [^}]*tooltip: 'configTooltip' \}/)
  assert.match(railSource, /class:active=\{isPanelOpen\(item\.id\)\}/)
  assert.match(railSource, /onclick=\{\(\) => togglePanel\(item\.id\)\}/)
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

test('Session config surfaces MCP servers as configurable chat entries', () => {
  assert.match(panelSource, /mcpServers = toolsResp\.mcp_servers \?\? \[\]/)
  assert.match(panelSource, /activeTab: 'tools' \| 'skills' \| 'commands' \| 'mcp' \| 'automation' \| 'style'/)
  assert.match(panelSource, /activeTab === 'mcp'/)
  assert.match(panelSource, /toggleMCP/)
  assert.match(panelSource, /mcp_custom = true/)
  assert.match(panelSource, /mcp_enabled = \[\.\.\.mcpEnabledSet\]/)
})
