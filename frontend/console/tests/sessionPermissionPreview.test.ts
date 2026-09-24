import test from 'node:test'
import assert from 'node:assert/strict'
import { buildSessionPermissionPreview } from '../src/lib/sessionPermissionPreview.ts'
import { sessionConfigEn } from '../src/i18n/sections/sessionConfig.ts'
import type { ChatToolInfo } from '../src/lib/api/chat.ts'
import type { SessionToolConfig } from '../src/lib/api/sessions.ts'

const summaryLabels = sessionConfigEn.permissionPreview.summary

const tools: ChatToolInfo[] = [
  { name: 'read_file', description: 'read files', high_risk: false, group: 'files' },
  { name: 'write_file', description: 'write files', high_risk: true, group: 'files' },
  { name: 'exec', description: 'run shell commands', high_risk: true, group: 'shell' },
  { name: 'git_status', description: 'read git status', high_risk: false, group: 'git' },
  { name: 'web_fetch', description: 'fetch urls', high_risk: false, group: 'network' },
]

test('permission preview reports gained risky tools and capabilities', () => {
  const before: SessionToolConfig = {
    tools_custom: true,
    tools_enabled: ['read_file'],
    tools_disabled: [],
  }
  const after: SessionToolConfig = {
    tools_custom: true,
    tools_enabled: ['read_file', 'write_file', 'exec', 'git_status', 'web_fetch'],
  }

  const preview = buildSessionPermissionPreview(before, after, {
    tools,
    skills: ['release-helper'],
    mcpServers: ['github'],
  }, summaryLabels)

  assert.equal(preview.risk, 'high')
  assert.deepEqual(preview.gainedTools, ['write_file', 'exec', 'git_status', 'web_fetch'])
  assert.match(preview.summary, /4 tool/)
  assert.ok(preview.capabilities.includes('shell'))
  assert.ok(preview.capabilities.includes('files'))
  assert.ok(preview.capabilities.includes('git'))
  assert.ok(preview.capabilities.includes('network'))
})

test('permission preview reports affected skills and MCP servers', () => {
  const before: SessionToolConfig = {
    skills_custom: true,
    skills_enabled: ['daily-briefing'],
    commands_custom: true,
    commands_enabled: ['memo'],
    mcp_enabled: ['github'],
  }
  const after: SessionToolConfig = {
    skills_custom: true,
    skills_enabled: ['release-helper'],
    commands_custom: true,
    commands_enabled: ['summarize'],
    mcp_enabled: ['github', 'notion'],
  }

  const preview = buildSessionPermissionPreview(before, after, {
    tools,
    skills: ['daily-briefing', 'release-helper'],
    commands: ['memo', 'summarize'],
    mcpServers: ['github', 'notion'],
  }, summaryLabels)

  assert.equal(preview.risk, 'medium')
  assert.deepEqual(preview.gainedSkills, ['release-helper'])
  assert.deepEqual(preview.lostSkills, ['daily-briefing'])
  assert.deepEqual(preview.gainedCommands, ['summarize'])
  assert.deepEqual(preview.lostCommands, ['memo'])
  assert.deepEqual(preview.gainedMCPServers, ['notion'])
  assert.match(preview.summary, /1 skill/)
  assert.match(preview.summary, /1 command/)
  assert.match(preview.summary, /1 MCP/)
})

test('permission preview treats mcp_custom empty allowlist as all MCP disabled', () => {
  const preview = buildSessionPermissionPreview({}, { mcp_custom: true, mcp_enabled: [] }, {
    tools,
    mcpServers: ['github', 'notion'],
  }, summaryLabels)

  assert.deepEqual(preview.lostMCPServers, ['github', 'notion'])
  assert.match(preview.summary, /2 MCP servers disabled/)
})

test('permission preview summary keeps the English wording and takes localized labels', () => {
  const unchanged = buildSessionPermissionPreview({}, {}, { tools }, summaryLabels)
  assert.equal(unchanged.summary, 'No effective permission change')

  const mixed = buildSessionPermissionPreview(
    { tools_custom: true, tools_enabled: ['read_file', 'git_status'] },
    { tools_custom: true, tools_enabled: ['read_file', 'exec'] },
    { tools },
    summaryLabels,
  )
  assert.equal(mixed.summary, '1 tool enabled, 1 tool disabled')

  const localized = buildSessionPermissionPreview(
    { tools_custom: true, tools_enabled: ['read_file', 'git_status'] },
    { tools_custom: true, tools_enabled: ['read_file', 'exec'] },
    { tools },
    { ...summaryLabels, toolsEnabled: (count) => `+${count}`, toolsDisabled: (count) => `-${count}` },
  )
  assert.equal(localized.summary, '+1, -1')
})
