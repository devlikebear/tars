import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const source = readFileSync(new URL('../src/components/Extensions.svelte', import.meta.url), 'utf8')
const apiSource = readFileSync(new URL('../src/lib/api/extensions.ts', import.meta.url), 'utf8')
const typesSource = readFileSync(new URL('../src/lib/types.ts', import.meta.url), 'utf8')

test('Extensions page removes legacy plugin management surfaces', () => {
  assert.doesNotMatch(source, /pluginsTitle/)
  assert.doesNotMatch(source, /hubPluginsOpen/)
  assert.doesNotMatch(source, /installedPluginsOpen/)
  assert.doesNotMatch(source, /listPlugins/)
  assert.doesNotMatch(source, /PluginDef/)
  assert.doesNotMatch(source, /handleInstall\('plugin'/)
  assert.doesNotMatch(source, /handleUninstall\('plugin'/)
})

test('Extensions page only loads skills, hub installs, disabled state, and MCP servers', () => {
  assert.match(source, /Promise\.all\(\[listSkills\(\), listMCPServers\(\), getHubInstalled\(\), getDisabledExtensions\(\)\]\)/)
  assert.doesNotMatch(source, /for \(const pl of p\)/)
  assert.doesNotMatch(source, /for \(const i of inst\.plugins\)/)
  assert.doesNotMatch(source, /registry\.plugins/)
  assert.doesNotMatch(apiSource, /export async function listPlugins/)
})

test('Extensions sections define Skills and MCP Servers inline', () => {
  assert.match(source, /\{\$t\.extensions\.skillsDefinition\}/)
  assert.match(source, /\{\$t\.extensions\.mcpDefinition\}/)
  assert.doesNotMatch(source, /Plugins are legacy Go extension packages/)
  assert.ok((source.match(/class="section-definition"/g) ?? []).length >= 4)
})

test('Extensions page exposes installed health checks and MCP repair actions', () => {
  assert.match(apiSource, /export async function getExtensionsHealth/)
  assert.match(apiSource, /export async function repairExtension/)
  assert.match(typesSource, /export type ExtensionHealthResponse/)
  assert.match(source, /handleRunDiagnostics/)
  assert.match(source, /handleRepair\('mcp'/)
  assert.match(source, /m\.tool_count/)
  assert.doesNotMatch(source, /m\.tools_count/)
})
