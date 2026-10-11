import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import { splitConsoleContext, splitInitiativeContext, userVisibleText } from '../src/lib/consoleContext.ts'

const block = '<console-context>\nAct as the TARS companion.\nCurrent console area: Board (board).\n</console-context>'
const initiativeBlock =
  '<initiative-context>\nYou spoke first, unprompted, at 2026-09-29T09:00Z — before the user\'s message below.\n</initiative-context>'

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

test('splitInitiativeContext takes the server-added hidden note off a stored user message', () => {
  assert.deepEqual(splitInitiativeContext(`thanks!\n\n${initiativeBlock}`), {
    text: 'thanks!',
    context: "You spoke first, unprompted, at 2026-09-29T09:00Z — before the user's message below.",
  })
  assert.deepEqual(splitInitiativeContext('plain words'), { text: 'plain words', context: '' })
  const quoted = 'what does <initiative-context> mean?'
  assert.deepEqual(splitInitiativeContext(quoted), { text: quoted, context: '' })
})

test('userVisibleText drops the initiative hidden note alongside console context and review notes', () => {
  assert.equal(userVisibleText(`thanks!\n\n${initiativeBlock}`), 'thanks!')
  assert.equal(userVisibleText(`thanks!\n\n${block}\n\n${initiativeBlock}`), 'thanks!')
  const notes = '<review-notes>\nNotes\n1. a.txt\nComment: x\n</review-notes>'
  // Server append order: message, console-context, initiative-context,
  // focus-stage, review-notes.
  assert.equal(userVisibleText(`thanks!\n\n${block}\n\n${initiativeBlock}\n\n${notes}`), 'thanks!')
})

test('the chat thread shows and titles only what the user typed', () => {
  const item = readFileSync(new URL('../src/components/ChatMessageItem.svelte', import.meta.url), 'utf8')
  assert.match(item, /splitConsoleContext\(/)
  const panel = readFileSync(new URL('../src/components/ChatPanel.svelte', import.meta.url), 'utf8')
  // CASE's bubble question (tars#1236, sendCompanionMessage) is the one
  // remaining console_context sender — the new-session handoff that used
  // to carry it through a composer-visible `initialContext` prop is gone,
  // along with its chip.
  assert.match(panel, /console_context:/)
  assert.doesNotMatch(panel, /initialContext/)
  assert.doesNotMatch(panel, /console-context-chip/)
  const chat = readFileSync(new URL('../src/components/Chat.svelte', import.meta.url), 'utf8')
  assert.doesNotMatch(chat, /initialContext/)
})
