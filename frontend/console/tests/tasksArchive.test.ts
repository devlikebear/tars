import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const apiSource = readFileSync(new URL('../src/lib/api.ts', import.meta.url), 'utf8')
const typesSource = readFileSync(new URL('../src/lib/types.ts', import.meta.url), 'utf8')
const tasksPanelSource = readFileSync(new URL('../src/components/TasksPanel.svelte', import.meta.url), 'utf8')
const enSource = readFileSync(new URL('../src/i18n/en.ts', import.meta.url), 'utf8')
const koSource = readFileSync(new URL('../src/i18n/ko.ts', import.meta.url), 'utf8')

test('Tasks panel wires archived plan API and collapsible archive section', () => {
  assert.match(typesSource, /PlanArchiveItem/)
  assert.match(typesSource, /PlanArchiveResponse/)
  assert.match(apiSource, /getSessionPlanArchive/)
  assert.match(apiSource, /\/v1\/admin\/sessions\/.*\/plans\/archive/)
  assert.match(apiSource, /getPlanArchive/)
  assert.match(apiSource, /\/v1\/admin\/plans\/archive/)
  assert.match(tasksPanelSource, /getSessionPlanArchive/)
  assert.match(tasksPanelSource, /archiveExpanded/)
  assert.match(tasksPanelSource, /archive-items/)
  assert.match(enSource, /pastPlans/)
  assert.match(koSource, /pastPlans/)
})
