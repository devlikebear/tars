import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import {
  countActiveSessions,
  derivePulseTone,
  deriveReflectionTone,
  formatRelativeStatusTime,
} from '../src/lib/statusStrip.ts'

const navSource = readFileSync(new URL('../src/components/Nav.svelte', import.meta.url), 'utf8')
const stripSource = readFileSync(new URL('../src/components/StatusStrip.svelte', import.meta.url), 'utf8')
const shellSource = readFileSync(new URL('../src/components/Shell.svelte', import.meta.url), 'utf8')
const appCssSource = readFileSync(new URL('../src/app.css', import.meta.url), 'utf8')

test('Nav mounts the persistent sidebar status strip', () => {
  assert.match(navSource, /import StatusStrip from '\.\/StatusStrip\.svelte'/)
  assert.match(navSource, /<StatusStrip \{onNavigate\} \/>/)
  assert.match(navSource, /nav-footer/)
})

test('StatusStrip polls existing status APIs and clears polling on destroy', () => {
  assert.match(stripSource, /getServerStatus/)
  assert.match(stripSource, /getPulseStatus/)
  assert.match(stripSource, /getReflectionStatus/)
  assert.match(stripSource, /listSessions/)
  assert.match(stripSource, /setInterval\(loadStatus, 30_000\)/)
  assert.match(stripSource, /onDestroy\(\(\) =>/)
  assert.match(stripSource, /clearInterval\(pollTimer\)/)
})

test('StatusStrip rows navigate to their detail surfaces', () => {
  assert.match(stripSource, /SERVER/)
  assert.match(stripSource, /PULSE/)
  assert.match(stripSource, /REFLECT/)
  assert.match(stripSource, /SESSIONS/)
  assert.match(stripSource, /navigate\('\/console'\)/)
  assert.match(stripSource, /navigate\('\/console\/pulse'\)/)
  assert.match(stripSource, /navigate\('\/console\/reflection'\)/)
  assert.match(stripSource, /navigate\('\/console\/chat'\)/)
})

test('sidebar status strip collapses before narrow desktop content can overlap', () => {
  assert.match(navSource, /@media \(max-width: 900px\)/)
  assert.match(shellSource, /@media \(max-width: 900px\)/)
  assert.match(appCssSource, /@media \(max-width: 900px\)/)
  assert.match(appCssSource, /--nav-width:\s*0px/)
})

test('status strip helpers format health and activity summaries', () => {
  const now = new Date('2026-05-01T00:05:00Z')

  assert.equal(formatRelativeStatusTime('2026-05-01T00:02:30Z', now), '2m ago')
  assert.equal(formatRelativeStatusTime('', now), 'never')
  assert.equal(derivePulseTone({ last_tick_at: '2026-05-01T00:04:00Z' }), 'ok')
  assert.equal(derivePulseTone({ last_tick_at: '0001-01-01T00:00:00Z' }), 'warn')
  assert.equal(derivePulseTone({ last_tick_at: '', last_err: 'boom' }), 'error')
  assert.equal(derivePulseTone(null), 'warn')
  assert.equal(deriveReflectionTone({ last_run_at: '2026-05-01T00:04:00Z', last_run_success: true }), 'ok')
  assert.equal(deriveReflectionTone({ last_run_at: '0001-01-01T00:00:00Z', last_run_success: true }), 'warn')
  assert.equal(
    deriveReflectionTone({ last_run_at: '2026-05-01T00:04:00Z', last_run_success: false, consecutive_failures: 1 }),
    'error',
  )
  assert.equal(deriveReflectionTone(null), 'warn')
  assert.equal(
    countActiveSessions([
      { id: 'one', title: 'One', created_at: '2026-05-01T00:00:00Z', updated_at: '2026-05-01T00:00:00Z' },
      {
        id: 'two',
        title: 'Hidden',
        hidden: true,
        created_at: '2026-05-01T00:00:00Z',
        updated_at: '2026-05-01T00:00:00Z',
      },
    ]),
    1,
  )
})
