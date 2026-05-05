import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const extensionsSource = readFileSync(new URL('../src/components/Extensions.svelte', import.meta.url), 'utf8')
const apiSource = readFileSync(new URL('../src/lib/api.ts', import.meta.url), 'utf8')

test('Extensions wires an MCP Server Creator wizard into the MCP section', () => {
  assert.match(extensionsSource, /import MCPServerCreator from '\.\/MCPServerCreator\.svelte'/)
  assert.match(extensionsSource, /let mcpCreatorOpen = \$state\(false\)/)
  assert.match(extensionsSource, /\{\$t\.extensions\.createMCP\}/)
  assert.match(extensionsSource, /<MCPServerCreator/)
  assert.match(extensionsSource, /onsaved={handleMCPServerCreated}/)
})

test('Extensions reloads runtime after saving a local MCP server draft', () => {
  assert.match(extensionsSource, /async function handleMCPServerCreated/)
  assert.match(extensionsSource, /async function handleMCPServerCreated[\s\S]*await reloadExtensions\(\)[\s\S]*await loadInstalled\(\)/)
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

test('MCP Server Creator supports natural-language LLM conversation draft flow', () => {
  const source = readFileSync(new URL('../src/components/MCPServerCreator.svelte', import.meta.url), 'utf8')
  assert.match(source, /let naturalPrompt = \$state/)
  assert.match(source, /let useLLM = \$state\(true\)/)
  assert.match(source, /let conversation: MCPServerCreatorConversationMessage\[\] = \$state/)
  assert.match(source, /Builder Prompt/)
  assert.match(source, /Generate with LLM/)
  assert.match(source, /appendConversation\('user'/)
  assert.match(source, /prompt: naturalPrompt\.trim\(\)/)
  assert.match(source, /conversation/)
  assert.match(source, /use_llm: useLLM/)
  assert.match(source, /draft\.draft_source/)
  assert.match(source, /draft\.assistant_message/)
})

test('MCP Server Creator validates draft input before calling the server', () => {
  const source = readFileSync(new URL('../src/components/MCPServerCreator.svelte', import.meta.url), 'utf8')
  assert.match(source, /function validateDraftInput/)
  assert.match(source, /let draftAttempted = \$state\(false\)/)
  assert.match(source, /let showDraftHint: boolean = \$derived\.by/)
  assert.match(source, /name must be kebab-case/i)
  assert.match(source, /const blockReason = validateDraftInput\(\)/)
  assert.match(source, /if \(blockReason\) \{/)
  assert.match(source, /draftAttempted = true/)
  assert.match(source, /if \(naturalPrompt\.trim\(\)\) return ''/)
  assert.match(source, /disabled=\{busy\}/)
  assert.doesNotMatch(source, /disabled=\{busy \|\| draftBlockedReason/)
})

test('MCP Server Creator modal keeps dense inputs and preview readable', () => {
  const source = readFileSync(new URL('../src/components/MCPServerCreator.svelte', import.meta.url), 'utf8')
  assert.match(source, /class="creator-body"/)
  assert.match(source, /class="creator-form-panel"/)
  assert.match(source, /class="creator-preview-panel"/)
  assert.match(source, /\.creator-modal\s*\{[\s\S]*height: min\(760px, calc\(100vh - var\(--space-8\) \* 2\)\)/)
  assert.match(source, /\.creator-form\s*\{[\s\S]*overflow-y: auto/)
  assert.match(source, /\.creator-actions\s*\{[\s\S]*position: sticky/)
})
