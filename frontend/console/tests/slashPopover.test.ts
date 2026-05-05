import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import { buildSlashCandidates, builtinSlashCommandId, parseLeadingSlashCommand } from '../src/lib/slash.ts'

const chatSource = readFileSync(new URL('../src/components/Chat.svelte', import.meta.url), 'utf8')
const panelSource = readFileSync(new URL('../src/components/ChatPanel.svelte', import.meta.url), 'utf8')
const appSource = readFileSync(new URL('../src/App.svelte', import.meta.url), 'utf8')
const memorySource = readFileSync(new URL('../src/components/MemoryCenter.svelte', import.meta.url), 'utf8')
const popoverSource = readFileSync(new URL('../src/components/SlashPopover.svelte', import.meta.url), 'utf8')

test('slash registry includes first-pass composer commands', () => {
  const commands = buildSlashCandidates('').map((candidate) => candidate.command)
  assert.ok(commands.includes('clear'))
  assert.ok(commands.includes('compact'))
  assert.ok(commands.includes('memory'))
  assert.ok(commands.includes('skill'))
  assert.ok(commands.includes('tasks'))
  assert.ok(commands.includes('sysprompt'))
  assert.ok(commands.includes('config'))
  assert.equal(builtinSlashCommandId('clear'), 'clear')
  assert.equal(builtinSlashCommandId('skill'), 'skill')
  assert.deepEqual(parseLeadingSlashCommand('/memory search token budget'), {
    command: 'memory',
    args: 'search token budget',
  })
})

test('ChatPanel delegates slash rendering to SlashPopover', () => {
  assert.match(panelSource, /import SlashPopover from '\.\/SlashPopover\.svelte'/)
  assert.match(panelSource, /<SlashPopover/)
  assert.match(panelSource, /onSelect=\{selectSlashCandidate\}/)
  assert.match(panelSource, /onExecute=\{\(candidate\) => void executeSlashCandidate\(candidate\)\}/)
  assert.match(panelSource, /listSkills\(sessionIdForSlash\)/)
  assert.match(panelSource, /listChatTools\(sessionIdForSlash\)/)
  assert.match(panelSource, /event\.tool_name \|\| ''\)\.trim\(\) === 'project_skill'/)
})

test('SlashPopover renders command and skill candidates with active state', () => {
  assert.match(popoverSource, /interface Props/)
  assert.match(popoverSource, /candidates: SlashCommandCandidate\[\]/)
  assert.match(popoverSource, /activeIndex: number/)
  assert.match(popoverSource, /sectionLabel\(candidate\.kind\)/)
  assert.match(popoverSource, /Commands/)
  assert.match(popoverSource, /class:active=\{i === activeIndex\}/)
  assert.match(popoverSource, /onmousedown=\{\(e\) => e\.preventDefault\(\)\}/)
})

test('Chat executes client-side slash commands without sending to the LLM', () => {
  assert.match(chatSource, /case 'clear':/)
  assert.match(chatSource, /chatPanelRef\?\.clearThread\(\)/)
  assert.match(chatSource, /case 'skill':/)
  assert.match(chatSource, /toggleSessionSkill\(args\)/)
  assert.match(chatSource, /getSessionEffectiveConfig\(selectedSessionId\)/)
  assert.match(chatSource, /updateSessionLocalConfig\(selectedSessionId, nextConfig\)/)
  assert.match(chatSource, /onNavigate\(`\/console\/memory\?tab=search&q=\$\{encodeURIComponent\(query\)\}`\)/)
})

test('Memory route preserves search query from slash navigation', () => {
  assert.match(appSource, /window\.location\.pathname \+ window\.location\.search/)
  assert.match(memorySource, /applyRouteSearchParams/)
  assert.match(memorySource, /new URLSearchParams\(window\.location\.search\)/)
  assert.match(memorySource, /searchQueryInput = query/)
  assert.match(memorySource, /activeTab = 'search'/)
})
