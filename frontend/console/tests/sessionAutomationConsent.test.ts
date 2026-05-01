import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const apiSource = readFileSync(new URL('../src/lib/api.ts', import.meta.url), 'utf8')
const panelSource = readFileSync(new URL('../src/components/SessionConfigPanel.svelte', import.meta.url), 'utf8')
const typesSource = readFileSync(new URL('../src/lib/types.ts', import.meta.url), 'utf8')

test('Session config exposes automation consent controls', () => {
  assert.match(typesSource, /SessionAutomationConsent/)
  assert.match(apiSource, /getSessionAutomationConsent/)
  assert.match(apiSource, /updateSessionAutomationConsent/)
  assert.match(apiSource, /\/automation-consent/)
  assert.match(panelSource, /activeTab: 'tools' \| 'skills' \| 'automation'/)
  assert.match(typesSource, /auto_resume_enabled/)
  assert.match(typesSource, /auto_resume_after_minutes/)
  assert.match(typesSource, /allowed_resume_modes/)
  assert.match(panelSource, /Auto-resume stalled chats/)
  assert.match(panelSource, /record_assumption_and_proceed/)
  assert.match(panelSource, /move_to_next_task/)
  assert.match(panelSource, /Approved git mutations/)
  assert.match(panelSource, /Autonomous workspace mutations/)
})
