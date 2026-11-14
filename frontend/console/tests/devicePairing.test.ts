import test from 'node:test'
import assert from 'node:assert/strict'

import {
  formatCountdown,
  newlyPaired,
  normalizeBaseURL,
  pairingBaseURL,
  pairingLink,
  parsePairingLink,
  qrPicture,
  secondsLeft,
  withFolder,
  withoutFolder,
} from '../src/lib/devicePairing.ts'

test('a pairing link round-trips the address and the code', () => {
  const link = pairingLink('https://mac-mini.tail1.ts.net:8443/console/', ' AB12-CD34 ')
  assert.equal(link, 'tars-mobile://pair?url=https%3A%2F%2Fmac-mini.tail1.ts.net%3A8443&code=AB12-CD34')
  assert.deepEqual(parsePairingLink(link), { baseURL: 'https://mac-mini.tail1.ts.net:8443', code: 'AB12-CD34' })
})

test('only an https address and a code make a link', () => {
  assert.equal(pairingLink('http://mac.tail1.ts.net', 'X'), '')
  assert.equal(pairingLink('https://mac.tail1.ts.net', '  '), '')
  assert.equal(pairingLink('not a url', 'X'), '')
  assert.equal(normalizeBaseURL('https://user:pw@mac.tail1.ts.net'), '')
  assert.equal(normalizeBaseURL('https://mac.tail1.ts.net:443/'), 'https://mac.tail1.ts.net')
})

test('anything that is not a pairing link is refused', () => {
  for (const text of [
    '',
    'https://mac.tail1.ts.net/?code=X',
    'tars://pair?url=https%3A%2F%2Fmac&code=X',
    'tars-mobile://open?url=https%3A%2F%2Fmac&code=X',
    'tars-mobile://pair?url=http%3A%2F%2Fmac&code=X',
    'tars-mobile://pair?url=https%3A%2F%2Fmac',
  ]) {
    assert.equal(parsePairingLink(text), null, text)
  }
})

test('the code carries the serve address only while serve points at TARS', () => {
  const url = 'https://mac.tail1.ts.net'
  assert.equal(pairingBaseURL({ url, serving: true, ownedByTARS: true }), url)
  assert.equal(pairingBaseURL({ url, serving: false, ownedByTARS: true }), '')
  assert.equal(pairingBaseURL({ url, serving: true, ownedByTARS: false }), '')
  assert.equal(pairingBaseURL({ serving: true, ownedByTARS: true }), '')
})

test('the QR picture is a square with a quiet zone and dark modules', () => {
  const pic = qrPicture('tars-mobile://pair?url=https%3A%2F%2Fmac.tail1.ts.net&code=AB12')
  assert.ok(pic.size >= 21 + 8)
  assert.match(pic.path, /^M\d+ \d+h1v1h-1z/)
  // Nothing is drawn inside the quiet zone.
  for (const [, x, y] of pic.path.matchAll(/M(\d+) (\d+)/g)) {
    assert.ok(Number(x) >= 4 && Number(x) < pic.size - 4)
    assert.ok(Number(y) >= 4 && Number(y) < pic.size - 4)
  }
})

test('countdown', () => {
  const now = Date.parse('2026-11-14T10:00:00Z')
  assert.equal(secondsLeft('2026-11-14T10:09:59Z', now), 599)
  assert.equal(secondsLeft('2026-11-14T09:59:00Z', now), 0)
  assert.equal(secondsLeft('garbage', now), 0)
  assert.equal(formatCountdown(599), '9:59')
  assert.equal(formatCountdown(5), '0:05')
})

test('the phone that used the code is the first new active device', () => {
  const devices = [
    { id: 'a', active: true },
    { id: 'b', active: false },
    { id: 'c', active: true },
  ]
  assert.deepEqual(newlyPaired(['a'], devices), { id: 'c', active: true })
  assert.equal(newlyPaired(['a', 'c'], devices), null)
})

test('folder list edits', () => {
  const list = ['/repo']
  assert.deepEqual(withFolder(list, ' /other '), ['/repo', '/other'])
  assert.equal(withFolder(list, '/repo'), list)
  assert.equal(withFolder(list, '  '), list)
  assert.deepEqual(withoutFolder(['/repo', '/other'], '/repo'), ['/other'])
})
