import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import { cliToolLabel, toolBaseDirs } from '../src/lib/cliToolLabels.ts'
import { formatToolInvocationPreview } from '../src/lib/toolCalls.ts'

const cwd = '/home/dev/tars'

test('Bash shows the command, then its description', () => {
  assert.equal(
    cliToolLabel('Bash', JSON.stringify({ command: 'rg -n "MCP 자동 주입" CLAUDE.md', description: 'Find the MCP note' })),
    'Bash(rg -n "MCP 자동 주입" CLAUDE.md) · Find the MCP note',
  )
  assert.equal(cliToolLabel('Bash', '{"command":"git status"}'), 'Bash(git status)')
  // Multi-line commands read as one line; long ones are shortened.
  const long = cliToolLabel('Bash', JSON.stringify({ command: `cat <<'EOF'\n${'x'.repeat(200)}\nEOF` }))!
  assert.ok(long.startsWith("Bash(cat <<'EOF' xxx"), long)
  assert.ok(long.endsWith('…)'), long)
  assert.ok(long.length < 110, long)
})

test('file tools show the path, relative to the working folder when inside it', () => {
  assert.equal(cliToolLabel('Read', JSON.stringify({ file_path: `${cwd}/internal/llm/router.go`, limit: 40 }), cwd), 'Read(internal/llm/router.go)')
  assert.equal(cliToolLabel('Edit', JSON.stringify({ file_path: `${cwd}/CLAUDE.md`, old_string: 'a', new_string: 'b' }), `${cwd}/`), 'Edit(CLAUDE.md)')
  assert.equal(cliToolLabel('Write', JSON.stringify({ content: 'package llm', file_path: '/tmp/out.go' }), cwd), 'Write(/tmp/out.go)')
  assert.equal(cliToolLabel('MultiEdit', JSON.stringify({ edits: [], file_path: `${cwd}-other/a.go` }), cwd), `MultiEdit(${cwd}-other/a.go)`)
  assert.equal(cliToolLabel('NotebookEdit', JSON.stringify({ notebook_path: `${cwd}/nb.ipynb`, new_source: 'x' }), cwd), 'NotebookEdit(nb.ipynb)')
  assert.equal(cliToolLabel('Read', JSON.stringify({ file_path: 'docs/a.md' })), 'Read(docs/a.md)')
})

test('an isolated session shows paths relative to its worktree and to its folder', () => {
  const wt = '/ws/_shared/session-worktrees/tars/s1'
  const bases = toolBaseDirs({ worktree: { dir: wt } }, cwd)
  assert.deepEqual(bases, [wt, cwd])
  assert.equal(cliToolLabel('Edit', JSON.stringify({ file_path: `${wt}/go.mod` }), bases), 'Edit(go.mod)')
  // Turns before the move ran in the session folder.
  assert.equal(cliToolLabel('Edit', JSON.stringify({ file_path: `${cwd}/go.sum` }), bases), 'Edit(go.sum)')
  assert.deepEqual(toolBaseDirs(null, ''), [])
  assert.deepEqual(toolBaseDirs({ worktree: null }, undefined), [])
})

test('search, web, and agent tools show what they look for', () => {
  assert.equal(cliToolLabel('Grep', JSON.stringify({ pattern: 'statusPreview', path: `${cwd}/internal`, output_mode: 'content' }), cwd), 'Grep(statusPreview in internal)')
  assert.equal(cliToolLabel('Grep', JSON.stringify({ pattern: 'TODO' }), cwd), 'Grep(TODO)')
  assert.equal(cliToolLabel('Glob', JSON.stringify({ pattern: '**/*.svelte', path: cwd }), cwd), 'Glob(**/*.svelte in .)')
  assert.equal(cliToolLabel('WebFetch', JSON.stringify({ url: 'https://example.com/a', prompt: 'summarize' })), 'WebFetch(https://example.com/a)')
  assert.equal(cliToolLabel('WebSearch', JSON.stringify({ query: 'svelte 5 runes' })), 'WebSearch(svelte 5 runes)')
  assert.equal(cliToolLabel('Task', JSON.stringify({ description: 'Explore tool cards', prompt: 'long…', subagent_type: 'Explore' })), 'Task(Explore tool cards)')
  assert.equal(cliToolLabel('Agent', JSON.stringify({ description: 'Review diff', prompt: 'p' })), 'Agent(Review diff)')
})

test('previews cut before the server kept them parseable still read cleanly', () => {
  // Transcripts written before the fix hold JSON cut mid-string.
  const cutBash = '{"command":"rg -n \\"MCP 자동 주입\\" CLAUDE.md; rg -n foo internal/tarsserver; rg...'
  const bash = cliToolLabel('Bash', cutBash)!
  assert.ok(bash.startsWith('Bash(rg -n "MCP 자동 주입" CLAUDE.md; rg -n foo'), bash)
  assert.ok(!bash.includes('\\"'), bash)
  const cutEdit = '{"file_path":"/home/dev/tars/CLAUDE.md","new_string":"line one\\nline two...'
  assert.equal(cliToolLabel('Edit', cutEdit, cwd), 'Edit(CLAUDE.md)')
  // The label argument was cut away entirely: name the tool, show no JSON.
  assert.equal(cliToolLabel('Write', '{"content":"package llm\\n\\nimport (\\n\\t\\"cont...'), 'Write(…)')
  assert.equal(cliToolLabel('Bash', ''), 'Bash()')
})

test('unknown and native tools keep the generic preview', () => {
  assert.equal(cliToolLabel('read_file', '{"path":"/tmp/a"}'), null)
  assert.equal(cliToolLabel('mcp__github__get_issue', '{"number":1}'), null)
  assert.equal(cliToolLabel('TodoWrite', '{"todos":[]}'), null)
  assert.equal(
    formatToolInvocationPreview('read_file', '{"path":"/tmp/example.txt","offset":10,"limit":20}'),
    'read_file(path=/tmp/example.txt, offset=10)',
  )
})

test('the card header uses the CLI label, relative to the session folder', () => {
  assert.equal(formatToolInvocationPreview('Read', JSON.stringify({ file_path: `${cwd}/go.mod` }), cwd), 'Read(go.mod)')
  const item = readFileSync(new URL('../src/components/ChatMessageItem.svelte', import.meta.url), 'utf8')
  assert.match(item, /formatToolInvocationPreview\(message\.toolName, message\.toolArgs, toolBaseDir\)/)
  const panel = readFileSync(new URL('../src/components/ChatPanel.svelte', import.meta.url), 'utf8')
  assert.match(panel, /toolBaseDir=\{toolBaseDirs\(chatSession\.activeSession, chatSession\.cwd\?\.current\)\}/)
  const side = readFileSync(new URL('../src/components/SideSessionPanel.svelte', import.meta.url), 'utf8')
  assert.match(side, /formatToolInvocationPreview\(message\.toolName, message\.toolArgs, toolBaseDirs\(sideSession, sideCwd\)\)/)
})
