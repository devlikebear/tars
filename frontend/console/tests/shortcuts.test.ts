import test from 'node:test'
import assert from 'node:assert/strict'

import { formatShortcutKeys, isTypingTarget, matchShortcut, shortcutLabel, type ShortcutEvent } from '../src/lib/shortcuts.ts'

function key(k: string, mods: Partial<ShortcutEvent> = {}): ShortcutEvent {
  return {
    key: k,
    code: /^[0-9]$/.test(k) ? `Digit${k}` : `Key${k.toUpperCase()}`,
    metaKey: false,
    ctrlKey: false,
    altKey: false,
    shiftKey: false,
    defaultPrevented: false,
    target: null,
    ...mods,
  }
}

test('Mod combinations map to their actions with Ctrl or Meta', () => {
  assert.deepEqual(matchShortcut(key('k', { ctrlKey: true })), { action: 'palette' })
  assert.deepEqual(matchShortcut(key('k', { metaKey: true })), { action: 'palette' })
  assert.deepEqual(matchShortcut(key('O', { ctrlKey: true, shiftKey: true })), { action: 'new-session' })
  assert.deepEqual(matchShortcut(key('j', { ctrlKey: true })), { action: 'toggle-terminal' })
  assert.deepEqual(matchShortcut(key('b', { metaKey: true })), { action: 'toggle-sidebar' })
  assert.deepEqual(matchShortcut(key('.', { ctrlKey: true })), { action: 'toggle-zen' })
})

test('browser-reserved and near-miss combinations do not match', () => {
  assert.equal(matchShortcut(key('n', { ctrlKey: true })), null, 'Ctrl+N belongs to the browser')
  assert.equal(matchShortcut(key('1', { ctrlKey: true })), null, 'Ctrl+1 switches browser tabs')
  assert.equal(matchShortcut(key('k', { ctrlKey: true, altKey: true })), null)
  assert.equal(matchShortcut(key('k', { ctrlKey: true, shiftKey: true })), null)
  assert.equal(matchShortcut(key('k')), null)
  assert.equal(matchShortcut(key('k', { ctrlKey: true, defaultPrevented: true })), null)
})

test('Alt+1..9 switches sessions by physical digit, even when macOS changes the key', () => {
  assert.deepEqual(matchShortcut(key('1', { altKey: true })), { action: 'switch-session', index: 0 })
  assert.deepEqual(matchShortcut({ ...key('9', { altKey: true }), key: 'ª' }), { action: 'switch-session', index: 8 })
  assert.equal(matchShortcut(key('0', { altKey: true })), null)
  assert.equal(matchShortcut(key('1', { altKey: true, shiftKey: true })), null)
})

test('? opens help only outside text fields', () => {
  assert.deepEqual(matchShortcut(key('?', { shiftKey: true })), { action: 'help' })
  assert.equal(matchShortcut(key('?', { shiftKey: true, target: { tagName: 'TEXTAREA' } as unknown as EventTarget })), null)
  assert.equal(matchShortcut(key('?', { shiftKey: true, target: { tagName: 'DIV', isContentEditable: true } as unknown as EventTarget })), null)
})

test('isTypingTarget recognizes inputs, textareas, selects, and contenteditable', () => {
  for (const tagName of ['INPUT', 'textarea', 'SELECT']) {
    assert.equal(isTypingTarget({ tagName } as unknown as EventTarget), true, tagName)
  }
  assert.equal(isTypingTarget({ tagName: 'BUTTON' } as unknown as EventTarget), false)
  assert.equal(isTypingTarget(null), false)
})

test('labels use platform glyphs', () => {
  assert.equal(formatShortcutKeys(['Mod', 'Shift', 'O'], true), '⌘⇧O')
  assert.equal(formatShortcutKeys(['Mod', 'Shift', 'O'], false), 'Ctrl+Shift+O')
  assert.equal(shortcutLabel('palette', false), 'Ctrl+K')
  assert.equal(shortcutLabel('switch-session', true), '⌥1…9')
})
