import test from 'node:test'
import assert from 'node:assert/strict'

import {
  answerShare,
  appHandoffLink,
  focusAppLink,
  onApplePhone,
  parseFocusLink,
  pipelineSessionOf,
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

test('the Phones card reads how much was answered on a phone', () => {
  const zero = { phone: 0, desk: 0, goal: 0 }
  assert.equal(answerShare(null), null)
  assert.equal(answerShare({ since: '', gates: zero, cards: zero, pipelines: 0, phone_pipelines: 0 }), null)
  assert.deepEqual(
    answerShare({
      since: '',
      gates: { phone: 2, desk: 1, goal: 3 },
      cards: { phone: 1, desk: 4, goal: 0 },
      pipelines: 4,
      phone_pipelines: 2,
    }),
    { phone: 3, total: 11, pipelines: 2 },
  )
})

test('a pipeline link round-trips the address and the session', () => {
  const link = focusAppLink('https://mac-mini.tail1.ts.net/console/focus/x', ' sess 1 ')
  assert.equal(link, 'tars-mobile://focus?url=https%3A%2F%2Fmac-mini.tail1.ts.net&session=sess+1')
  assert.deepEqual(parseFocusLink(link), { baseURL: 'https://mac-mini.tail1.ts.net', sessionID: 'sess 1' })
  assert.equal(focusAppLink('http://mac.tail1.ts.net', 's'), '')
  assert.equal(focusAppLink('https://mac.tail1.ts.net', ' '), '')
  assert.equal(parseFocusLink('tars-mobile://focus?url=http%3A%2F%2Fmac.tail1.ts.net&session=s'), null)
  assert.equal(parseFocusLink('tars-mobile://focus?url=https%3A%2F%2Fmac.tail1.ts.net'), null)
  assert.equal(parseFocusLink('tars-mobile://pair?url=https%3A%2F%2Fmac.tail1.ts.net&code=1'), null)
  assert.equal(parseFocusLink('https://mac.tail1.ts.net/console/focus/s'), null)
  assert.deepEqual(
    parseFocusLink('tars-mobile://focus?url=http%3A%2F%2F127.0.0.1%3A43180&session=s', { allowLoopbackHTTP: true }),
    { baseURL: 'http://127.0.0.1:43180', sessionID: 's' },
  )
})

const iPhone = { userAgent: 'Mozilla/5.0 (iPhone; CPU iPhone OS 26_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148', platform: 'iPhone', maxTouchPoints: 5 }
const iPad = { userAgent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/26.1 Safari/605.1.15', platform: 'MacIntel', maxTouchPoints: 5 }
const mac = { ...iPad, maxTouchPoints: 0 }
const android = { userAgent: 'Mozilla/5.0 (Linux; Android 15; Pixel 9) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140 Mobile Safari/537.36', platform: 'Linux armv8l', maxTouchPoints: 5 }

test('the app is offered on an iPhone or iPad only', () => {
  assert.equal(onApplePhone(iPhone), true)
  assert.equal(onApplePhone(iPad), true)
  assert.equal(onApplePhone(mac), false)
  assert.equal(onApplePhone(android), false)
})

test('only a pipeline page has a pipeline to hand off', () => {
  assert.equal(pipelineSessionOf('/console/focus/sess-1'), 'sess-1')
  assert.equal(pipelineSessionOf('/console/focus/sess%201/'), 'sess 1')
  assert.equal(pipelineSessionOf('/console/focus'), '')
  assert.equal(pipelineSessionOf('/console/focus/release'), '')
  assert.equal(pipelineSessionOf('/console/focus/templates'), '')
  assert.equal(pipelineSessionOf('/console/chat/sess-1'), '')
  assert.equal(pipelineSessionOf('/console/focus/%E0%A4%A'), '')
})

test('the hand-off link: an iPhone on a pipeline page at an https address', () => {
  const origin = 'https://mac-mini.tail1.ts.net'
  assert.equal(appHandoffLink(origin, '/console/focus/sess-1', iPhone), 'tars-mobile://focus?url=https%3A%2F%2Fmac-mini.tail1.ts.net&session=sess-1')
  assert.equal(appHandoffLink(origin, '/console/focus/sess-1', mac), '')
  assert.equal(appHandoffLink(origin, '/console/focus', iPhone), '')
  // The app pairs only over https, so a loopback console offers nothing.
  assert.equal(appHandoffLink('http://127.0.0.1:43180', '/console/focus/sess-1', iPhone), '')
})
