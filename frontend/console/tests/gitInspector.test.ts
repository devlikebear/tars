import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const chatSource = readFileSync(new URL('../src/components/Chat.svelte', import.meta.url), 'utf8')
const apiSource = readFileSync(new URL('../src/lib/api.ts', import.meta.url), 'utf8')
const typesSource = readFileSync(new URL('../src/lib/types.ts', import.meta.url), 'utf8')
const gitInspectorSource = readFileSync(new URL('../src/components/GitInspector.svelte', import.meta.url), 'utf8')

test('Chat mounts Git Inspector as a dockable read-only panel', () => {
  assert.match(chatSource, /import GitInspector from '\.\/GitInspector\.svelte'/)
  assert.match(chatSource, /type ChatDockPanelID = [^\n]*'git'/)
  assert.match(chatSource, /\{ id: 'git', title: 'Git', defaultZone: 'right' \}/)
  assert.match(chatSource, /title="Git Inspector"/)
  assert.match(chatSource, /<GitInspector[\s\S]*sessionId={selectedSessionId}/)
})

test('Git Inspector API client and view expose status, diff, log, and branches', () => {
  for (const name of ['GitStatus', 'GitDiff', 'GitLogResponse', 'GitBranchesResponse']) {
    assert.match(typesSource, new RegExp(`type ${name}`))
  }
  for (const fn of ['getGitStatus', 'getGitDiff', 'getGitLog', 'getGitBranches']) {
    assert.match(apiSource, new RegExp(`function ${fn}`))
  }
  assert.match(gitInspectorSource, /getGitStatus/)
  assert.match(gitInspectorSource, /getGitDiff/)
  assert.match(gitInspectorSource, /getGitLog/)
  assert.match(gitInspectorSource, /getGitBranches/)
  assert.match(gitInspectorSource, /side-by-side/i)
})
