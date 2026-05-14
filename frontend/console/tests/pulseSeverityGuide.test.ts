import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const source = readFileSync(new URL('../src/components/Pulse.svelte', import.meta.url), 'utf8')
const i18nEnSource = readFileSync(new URL('../src/i18n/en.ts', import.meta.url), 'utf8')

test('Pulse explains Min Severity and signal severity mappings inline', () => {
  assert.match(i18nEnSource, /severityGuideTitle: 'Min Severity guide'/)
  assert.match(i18nEnSource, /pulse_\* config fields/)
  assert.match(i18nEnSource, /Notifications below this floor are dropped/)
  assert.match(source, /cron_failures/)
  assert.match(source, /disk_usage/)
  assert.match(source, /stuck_agentruntime_run/)
  assert.match(source, /delivery_failures/)
  assert.match(source, /reflection_failure/)
  assert.match(source, /lastSignalSeen/)
  assert.match(source, /class="pulse-severity-guide"/)
})

test('Pulse compresses all-clear recent ticks and highlights signal ticks', () => {
  assert.match(source, /recentTickSummary/)
  assert.match(i18nEnSource, /allClear: 'all clear \(no signals\)'/)
  assert.match(source, /warningCount/)
  assert.match(source, /errorCount/)
  assert.match(source, /autofixCount/)
  assert.match(source, /pulse-recent-dots/)
  assert.match(source, /pulse-signal-ticks/)
})

test('Pulse introduces the system watchdog scope before status readouts', () => {
  assert.match(i18nEnSource, /title: 'Pulse - System Watchdog'/)
  assert.match(source, /pulseWatchItems/)
  assert.match(source, /class="pulse-intro/)
})
