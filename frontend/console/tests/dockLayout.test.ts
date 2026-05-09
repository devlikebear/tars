import test from 'node:test'
import assert from 'node:assert/strict'

import {
  createDockLayout,
  moveDockPanel,
  normalizeDockLayout,
  openDockPanel,
  panelIsOpen,
  resizeDock,
  serializeDockLayout,
  type DockPanelDefinition,
} from '../src/lib/dock/layout.ts'
import { sortStrings } from '../src/lib/sort.js'

const panels: DockPanelDefinition[] = [
  { id: 'sessions', title: 'Sessions', defaultZone: 'left', closeable: false },
  { id: 'files', title: 'Files', defaultZone: 'right' },
  { id: 'tasks', title: 'Tasks', defaultZone: 'right' },
  { id: 'terminal', title: 'Terminal', defaultZone: 'bottom' },
]

test('dock layout initializes panel placements and default active zones', () => {
  const layout = createDockLayout(panels)

  assert.equal(layout.placements.sessions, 'left')
  assert.equal(layout.placements.files, 'right')
  assert.equal(layout.placements.terminal, 'bottom')
  assert.equal(layout.active.left, 'sessions')
  assert.equal(layout.active.right, undefined)
  assert.equal(layout.active.bottom, undefined)
})

test('dock layout opens panels in their remembered zone and moves active panels', () => {
  let layout = createDockLayout(panels)

  layout = openDockPanel(layout, panels, 'files')
  assert.equal(layout.active.right, 'files')

  layout = moveDockPanel(layout, panels, 'files', 'bottom')
  assert.equal(layout.placements.files, 'bottom')
  assert.equal(layout.active.right, undefined)
  assert.equal(layout.active.bottom, 'files')

  layout = openDockPanel(layout, panels, 'tasks')
  assert.equal(layout.active.right, 'tasks')
  assert.equal(layout.active.bottom, 'files')
})

test('dock layout can restore a hidden non-closeable panel from its stable placement', () => {
  let layout = openDockPanel(createDockLayout(panels), panels, 'files')

  layout = moveDockPanel(layout, panels, 'files', 'left')
  assert.equal(layout.active.left, 'files')
  assert.equal(panelIsOpen(layout, 'sessions'), false)

  layout = openDockPanel(layout, panels, 'sessions')
  assert.equal(layout.placements.sessions, 'left')
  assert.equal(layout.active.left, 'sessions')
  assert.equal(panelIsOpen(layout, 'sessions'), true)
})

test('dock layout clamps resize values and drops unknown stored panels', () => {
  const raw = {
    placements: {
      sessions: 'left',
      files: 'moon',
      tasks: 'fullscreen',
      ghost: 'right',
    },
    active: {
      left: 'ghost',
      right: 'files',
      fullscreen: 'tasks',
    },
    sizes: {
      left: 10,
      right: 900,
      bottom: 42,
    },
  }

  const layout = normalizeDockLayout(raw, panels)

  assert.equal(layout.placements.sessions, 'left')
  assert.equal(layout.placements.files, 'right')
  assert.equal(layout.placements.tasks, 'fullscreen')
  assert.equal(layout.placements.ghost, undefined)
  assert.equal(layout.active.left, 'sessions')
  assert.equal(layout.active.right, 'files')
  assert.equal(layout.active.fullscreen, 'tasks')
  assert.equal(layout.sizes.left, 220)
  assert.equal(layout.sizes.right, 520)
  assert.equal(layout.sizes.bottom, 180)

  const resized = resizeDock(layout, 'bottom', 999)
  assert.equal(resized.sizes.bottom, 520)
})

test('dock layout serializes only stable placement, active, and size data', () => {
  const layout = moveDockPanel(openDockPanel(createDockLayout(panels), panels, 'files'), panels, 'files', 'fullscreen')
  const serialized = serializeDockLayout(layout)

  assert.deepEqual(sortStrings(Object.keys(serialized)), ['active', 'placements', 'sizes'])
  assert.equal(serialized.placements.files, 'fullscreen')
  assert.equal(serialized.active.fullscreen, 'files')
})
