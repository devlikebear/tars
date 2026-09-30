import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import { splitConsoleContext, userVisibleText } from '../src/lib/consoleContext.ts'

const block = '<console-context>\nAct as the TARS companion.\nCurrent console area: Board (board).\n</console-context>'

test('splitConsoleContext takes the server-added block off a stored user message', () => {
  assert.deepEqual(splitConsoleContext(`what now?\n\n${block}`), {
    text: 'what now?',
    context: 'Act as the TARS companion.\nCurrent console area: Board (board).',
  })
  assert.deepEqual(splitConsoleContext('plain words'), { text: 'plain words', context: '' })
  // Only a trailing block the server added counts, not a user quoting the tag.
  const quoted = 'what does <console-context> mean?'
  assert.deepEqual(splitConsoleContext(quoted), { text: quoted, context: '' })
})

test('userVisibleText drops the console context and the review notes', () => {
  const notes = '<review-notes>\nNotes\n1. a.txt\nComment: x\n</review-notes>'
  assert.equal(userVisibleText(`fix it\n\n${block}\n\n${notes}`), 'fix it')
  assert.equal(userVisibleText(`fix it\n\n${block}`), 'fix it')
  assert.equal(userVisibleText('fix it'), 'fix it')
})

test('the chat thread shows and titles only what the user typed', () => {
  const item = readFileSync(new URL('../src/components/ChatMessageItem.svelte', import.meta.url), 'utf8')
  assert.match(item, /splitConsoleContext\(/)
  const panel = readFileSync(new URL('../src/components/ChatPanel.svelte', import.meta.url), 'utf8')
  // The handoff's guidance goes to the server beside the message, once.
  assert.match(panel, /console_context:/)
  assert.match(panel, /initialContext/)
  const chat = readFileSync(new URL('../src/components/Chat.svelte', import.meta.url), 'utf8')
  assert.match(chat, /\{initialContext\}/)
})
