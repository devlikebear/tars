import test from 'node:test'
import assert from 'node:assert/strict'
import { JSDOM } from 'jsdom'

import { getOwnConsoleBuild, shouldShowUpdateNotice } from '../src/lib/consoleUpdate.ts'

test('getOwnConsoleBuild reads the server-injected meta tag', () => {
  const dom = new JSDOM(`<!doctype html><html><head><meta name="tars-console-build" content="abc123def456"></head><body></body></html>`)
  assert.equal(getOwnConsoleBuild(dom.window.document as unknown as Document), 'abc123def456')
})

test('getOwnConsoleBuild returns "" when the meta tag is absent (dev proxy / unbuilt placeholder)', () => {
  const dom = new JSDOM(`<!doctype html><html><head></head><body></body></html>`)
  assert.equal(getOwnConsoleBuild(dom.window.document as unknown as Document), '')
})

test('shouldShowUpdateNotice: same build never shows', () => {
  assert.equal(shouldShowUpdateNotice('abc123', 'abc123', undefined, false), false)
})

test('shouldShowUpdateNotice: a genuinely different build shows', () => {
  assert.equal(shouldShowUpdateNotice('abc123', 'def456', undefined, false), true)
})

test('shouldShowUpdateNotice: empty own or served never shows (nothing to compare)', () => {
  assert.equal(shouldShowUpdateNotice('', 'def456', undefined, false), false)
  assert.equal(shouldShowUpdateNotice('abc123', '', undefined, false), false)
  assert.equal(shouldShowUpdateNotice('', '', undefined, false), false)
})

test('shouldShowUpdateNotice: the Vite dev server never shows it, even with differing values', () => {
  assert.equal(shouldShowUpdateNotice('abc123', 'def456', undefined, true), false)
})

test('shouldShowUpdateNotice: a dismissed build does not show again for that same build', () => {
  assert.equal(shouldShowUpdateNotice('abc123', 'def456', 'def456', false), false)
})

test('shouldShowUpdateNotice: a newer build than the one dismissed shows again', () => {
  assert.equal(shouldShowUpdateNotice('abc123', 'ghi789', 'def456', false), true)
})
