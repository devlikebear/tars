import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const extensionsSource = readFileSync(new URL('../src/components/Extensions.svelte', import.meta.url), 'utf8')
const apiSource = readFileSync(new URL('../src/lib/api.ts', import.meta.url), 'utf8')

test('Extensions wires an MCP Server Creator wizard into the MCP section', () => {
  assert.match(extensionsSource, /import MCPServerCreator from '\.\/MCPServerCreator\.svelte'/)
  assert.match(extensionsSource, /let mcpCreatorOpen = \$state\(false\)/)
  assert.match(extensionsSource, /\+ Create MCP Server/)
  assert.match(extensionsSource, /<MCPServerCreator/)
  assert.match(extensionsSource, /onsaved={handleMCPServerCreated}/)
})

test('MCP Server Creator API client exposes draft, save, test, and submit endpoints', () => {
  assert.match(apiSource, /export async function draftMCPServer/)
  assert.match(apiSource, /\/v1\/admin\/mcp-servers\/draft/)
  assert.match(apiSource, /export async function saveLocalMCPServer/)
  assert.match(apiSource, /\/v1\/admin\/mcp-servers\/save-local/)
  assert.match(apiSource, /export async function testMCPServerDraft/)
  assert.match(apiSource, /\/v1\/admin\/mcp-servers\/test/)
  assert.match(apiSource, /export async function submitMCPServerDraftPR/)
  assert.match(apiSource, /\/v1\/admin\/mcp-servers\/submit-pr/)
})

test('MCP Server Creator component exposes stdio validation results', () => {
  const source = readFileSync(new URL('../src/components/MCPServerCreator.svelte', import.meta.url), 'utf8')
  assert.match(source, /testMCPServerDraft/)
  assert.match(source, /Python FastMCP/)
  assert.match(source, /Node MCP SDK/)
  assert.match(source, /tools\/list/)
  assert.match(source, /tools\/call/)
  assert.match(source, /Call Result/)
})
