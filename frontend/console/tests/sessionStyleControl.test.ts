import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import { buildSessionStylePreview, sessionStylePayload } from '../src/lib/sessionStyle.ts'

const panelSource = readFileSync(new URL('../src/components/SessionConfigPanel.svelte', import.meta.url), 'utf8')
const apiSource = readFileSync(new URL('../src/lib/api/sessions.ts', import.meta.url), 'utf8')
const typesSource = readFileSync(new URL('../src/lib/types.ts', import.meta.url), 'utf8')

test('session style preview describes behavioral axes and consent limits', () => {
  const preview = buildSessionStylePreview({
    effective: {
      directness: 85,
      humor: 12,
      caution: 70,
      autonomy: 92,
    },
    defaults: {
      directness: 70,
      humor: 20,
      caution: 60,
      autonomy: 40,
    },
    preview: [],
  })

  assert.match(preview.join(' '), /direct/)
  assert.match(preview.join(' '), /rare humor/)
  assert.match(preview.join(' '), /verify/)
  assert.match(preview.join(' '), /consent/)
})

test('session style payload clamps slider values before save', () => {
  const payload = sessionStylePayload({
    directness: 120,
    humor: -10,
    caution: 55,
    autonomy: 90,
  })

  assert.deepEqual(payload, {
    directness: 100,
    humor: 0,
    caution: 55,
    autonomy: 90,
  })
})

test('session config panel exposes style sliders and API bindings', () => {
  assert.match(typesSource, /export type SessionStyleControl/)
  assert.match(apiSource, /getSessionStyle/)
  assert.match(apiSource, /updateSessionStyle/)
  assert.match(panelSource, /activeTab: 'tools' \\| 'skills' \\| 'automation' \\| 'style'/)
  assert.match(panelSource, /Style/)
  assert.match(panelSource, /style-slider/)
  assert.match(panelSource, /Directness/)
  assert.match(panelSource, /Autonomy/)
})
