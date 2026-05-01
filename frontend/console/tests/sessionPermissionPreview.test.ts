import test from 'node:test'
import assert from 'node:assert/strict'
import { buildSessionPermissionPreview } from '../src/lib/sessionPermissionPreview.ts'
import type { ChatToolInfo, SessionToolConfig } from '../src/lib/api.ts'

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
  })

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
    mcp_enabled: ['github'],
  }
  const after: SessionToolConfig = {
    skills_custom: true,
    skills_enabled: ['release-helper'],
    mcp_enabled: ['github', 'notion'],
  }

  const preview = buildSessionPermissionPreview(before, after, {
    tools,
    skills: ['daily-briefing', 'release-helper'],
    mcpServers: ['github', 'notion'],
  })

  assert.equal(preview.risk, 'medium')
  assert.deepEqual(preview.gainedSkills, ['release-helper'])
  assert.deepEqual(preview.lostSkills, ['daily-briefing'])
  assert.deepEqual(preview.gainedMCPServers, ['notion'])
  assert.match(preview.summary, /1 skill/)
  assert.match(preview.summary, /1 MCP/)
})
