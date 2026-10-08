import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import { canSignOut, loginRequiredFor } from '../src/lib/authGate.ts'

const off = { authenticated: true, auth_role: 'admin', is_admin: true, auth_mode: 'off' }
const signedIn = { authenticated: true, auth_role: 'user', is_admin: false, auth_mode: 'required' }
const signedOut = { authenticated: false, auth_role: '', is_admin: false, auth_mode: 'required' }

test('loginRequiredFor asks for login only on a required server without a session', () => {
  assert.equal(loginRequiredFor(signedOut), true)
  assert.equal(loginRequiredFor(signedIn), false)
  assert.equal(loginRequiredFor(off), false)
  assert.equal(loginRequiredFor({ ...off, authenticated: false }), false)
  assert.equal(loginRequiredFor(null), false)
})

test('canSignOut is false when auth is off', () => {
  assert.equal(canSignOut(off), false)
  assert.equal(canSignOut(signedIn), true)
  assert.equal(canSignOut(null), false)
})

test('App decides the login screen after sign-out from the server answer', () => {
  const app = readFileSync(new URL('../src/App.svelte', import.meta.url), 'utf8')
  const logout = app.slice(app.indexOf('async function handleLogout()'), app.indexOf('// ⌘K palette'))
  assert.match(logout, /getAuthWhoami\(\)/)
  assert.match(logout, /loginRequiredFor\(/)
  assert.ok(logout.indexOf('loginRequiredFor(') < logout.indexOf('loginRequired = true'))
  assert.match(app, /onLogout=\{canSignOut\(authInfo\) \? handleLogout : undefined\}/)
})
