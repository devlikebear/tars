import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import { buildSessionStylePreview, sessionStylePayload } from '../src/lib/sessionStyle.ts'
import { sessionConfigEn } from '../src/i18n/sections/sessionConfig.ts'

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
  }, sessionConfigEn.style.preview)

  assert.match(preview.join(' '), /direct/)
  assert.match(preview.join(' '), /rare humor/)
  assert.match(preview.join(' '), /verify/)
  assert.match(preview.join(' '), /consent/)
  assert.deepEqual(preview, [
    'direct answers first; rare humor.',
    'more verify-before-act behavior; autonomy stays bounded by explicit consent.',
  ])
})

test('session style preview prefers server preview lines over the localized fallback', () => {
  const serverLines = ['server line']
  const preview = buildSessionStylePreview({
    effective: { directness: 50, humor: 50, caution: 50, autonomy: 50 },
    defaults: { directness: 70, humor: 20, caution: 60, autonomy: 40 },
    preview: serverLines,
  }, sessionConfigEn.style.preview)

  assert.deepEqual(preview, serverLines)
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
  assert.match(panelSource, /\$t\.sessionConfig\.tabs\.style/)
  assert.equal(sessionConfigEn.tabs.style, 'Style')
  assert.match(panelSource, /style-slider/)
  assert.match(panelSource, /\$t\.sessionConfig\.style\.axes\[axis\]/)
  assert.match(panelSource, /styleAxes: Array<keyof SessionStyleValues> = \['directness', 'humor', 'caution', 'autonomy'\]/)
  assert.equal(sessionConfigEn.style.axes.directness, 'Directness')
  assert.equal(sessionConfigEn.style.axes.autonomy, 'Autonomy')
})
