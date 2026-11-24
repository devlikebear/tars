import test from 'node:test'
import assert from 'node:assert/strict'

import {
  BOARD_COLUMNS,
  COLUMN_STATES,
  boardTotal,
  cardLine,
  columnCount,
  columnNeeds,
  columnOf,
  columnView,
  easyAgo,
  emptyColumn,
  groupByColumn,
  homeRecent,
  listQueryString,
  manageErrorKey,
  manageable,
  matchesName,
  renamedIn,
  showMore,
  withPage,
  withoutProject,
  type EasyBoardState,
  type EasyListed,
} from '../src/lib/easyBoard.ts'
import { easyEn, easyKo } from '../src/i18n/sections/easy.ts'
import { hiddenWordsIn } from '../src/lib/easy.ts'

function project(id: string, state: EasyBoardState, minutesAgo: number, extra: Partial<EasyListed['board']> = {}, name = `Project ${id}`): EasyListed {
  const at = new Date(Date.UTC(2026, 10, 25, 12, 0, 0) - minutesAgo * 60_000).toISOString()
  return {
    id, name, idea: name, folder: `/p/${id}`, status: 'ready', created_at: at, updated_at: at,
    board: { state, changed_at: at, ...extra },
  }
}

test('every server state has exactly one column', () => {
  const states: EasyBoardState[] = ['starting', 'failed', 'your_turn', 'working', 'paused', 'stuck', 'done', 'published']
  for (const s of states) {
    const holders = BOARD_COLUMNS.filter((c) => COLUMN_STATES[c].includes(s))
    assert.equal(holders.length, 1, s)
    assert.equal(columnOf(s), holders[0])
  }
  assert.equal(columnOf('starting'), 'working')
  assert.equal(columnOf('failed'), 'stopped')
  assert.equal(columnOf('paused'), 'stopped')
})

test('counts add up per column and over the board', () => {
  const counts = { your_turn: 1, working: 2, starting: 1, paused: 1, stuck: 2, failed: 1, done: 7, published: 0 }
  assert.equal(columnCount('working', counts), 3)
  assert.equal(columnCount('stopped', counts), 4)
  assert.equal(columnCount('published', {}), 0)
  assert.equal(boardTotal(counts), 15)
})

test('the list query names only what is asked', () => {
  assert.equal(listQueryString({}), '')
  assert.equal(listQueryString({ state: 'done', limit: 5, cursor: 'abc', archived: true }), '?state=done&limit=5&cursor=abc&archived=1')
})

test('a one-state column reads its first page, then more only when asked', () => {
  let feed = emptyColumn('done')
  assert.deepEqual(columnNeeds(feed), ['done'])
  const first = [1, 2, 3, 4, 5].map((n) => project(`d${n}`, 'done', n))
  feed = withPage(feed, 'done', { projects: first, next_cursor: 'c1' })
  assert.deepEqual(columnNeeds(feed), [], 'five cards are shown in order; the next page waits for "more"')
  let view = columnView(feed)
  assert.equal(view.cards.length, 5)
  assert.equal(view.more, true)

  feed = showMore(feed)
  assert.deepEqual(columnNeeds(feed), ['done'])
  feed = withPage(feed, 'done', { projects: [6, 7].map((n) => project(`d${n}`, 'done', n)) })
  view = columnView(feed)
  assert.equal(view.cards.length, 7)
  assert.equal(view.more, false)
})

test('a column of several states merges them newest first and reads the one that could still be newer', () => {
  let feed = emptyColumn('stopped')
  assert.deepEqual(columnNeeds(feed).sort(), ['failed', 'paused', 'stuck'])
  // paused: 5 recent ones with more behind; stuck: two old ones; failed: none.
  feed = withPage(feed, 'paused', { projects: [1, 2, 3, 10, 11].map((n) => project(`p${n}`, 'paused', n)), next_cursor: 'pc' })
  feed = withPage(feed, 'stuck', { projects: [4, 50].map((n) => project(`s${n}`, 'stuck', n)) })
  feed = withPage(feed, 'failed', { projects: [] })
  // The fifth card is p10 (10 minutes); paused's last read is p11, older — no read needed.
  assert.deepEqual(columnNeeds(feed), [])
  assert.deepEqual(columnView(feed).cards.map((p) => p.id), ['p1', 'p2', 'p3', 's4', 'p10'])

  feed = showMore(feed)
  // Ten to show, seven read, paused has more: read it.
  assert.deepEqual(columnNeeds(feed), ['paused'])
  feed = withPage(feed, 'paused', { projects: [12, 60].map((n) => project(`p${n}`, 'paused', n)) })
  assert.deepEqual(columnNeeds(feed), [])
  assert.deepEqual(columnView(feed).cards.map((p) => p.id), ['p1', 'p2', 'p3', 's4', 'p10', 'p11', 'p12', 's50', 'p60'])
  assert.equal(columnView(feed).more, false)
})

test('a page read twice does not show a card twice', () => {
  let feed = emptyColumn('done')
  const page = { projects: [project('a', 'done', 1)] }
  feed = withPage(feed, 'done', page)
  feed = withPage(feed, 'done', page)
  assert.equal(columnView(feed).cards.length, 1)
})

test('a renamed card keeps its place and an archived one leaves', () => {
  let feed = withPage(emptyColumn('done'), 'done', { projects: [project('a', 'done', 1), project('b', 'done', 2)] })
  feed = renamedIn(feed, 'b', 'Book club')
  assert.deepEqual(columnView(feed).cards.map((p) => p.name), ['Project a', 'Book club'])
  feed = withoutProject(feed, 'a')
  assert.deepEqual(columnView(feed).cards.map((p) => p.id), ['b'])
})

test('search keeps names that hold the words, ignoring case, and groups by column', () => {
  assert.equal(matchesName({ name: 'Book Club sign-up' }, ' book club '), true)
  assert.equal(matchesName({ name: '독서모임 신청' }, '모임'), true)
  assert.equal(matchesName({ name: 'Café' }, 'tea'), false)
  assert.equal(matchesName({ name: 'Anything' }, ''), true)
  const groups = groupByColumn([project('a', 'done', 5), project('b', 'starting', 1), project('c', 'done', 1), project('d', 'stuck', 2)])
  assert.deepEqual(groups.done.map((p) => p.id), ['c', 'a'])
  assert.deepEqual(groups.working.map((p) => p.id), ['b'])
  assert.deepEqual(groups.stopped.map((p) => p.id), ['d'])
  assert.deepEqual(groups.your_turn, [])
})

test('home shows a waiting project first, then the newest others, three at most', () => {
  const recent = [project('a', 'working', 1), project('b', 'done', 2), project('c', 'done', 3)]
  const waiting = project('w', 'your_turn', 30)
  assert.deepEqual(homeRecent(recent, waiting).map((p) => p.id), ['w', 'a', 'b'])
  assert.deepEqual(homeRecent(recent, undefined).map((p) => p.id), ['a', 'b', 'c'])
  // The waiting one is also among the newest: shown once.
  assert.deepEqual(homeRecent([waiting, ...recent], waiting).map((p) => p.id), ['w', 'a', 'b'])
  assert.deepEqual(homeRecent([], undefined), [])
})

test("a card's line follows the server's board facts", () => {
  const now = new Date(Date.UTC(2026, 10, 25, 12, 0, 0))
  const line = (b: Partial<EasyListed['board']> & { state: EasyBoardState }) => cardLine({ changed_at: now.toISOString(), ...b }, 'en', now)
  assert.deepEqual(line({ state: 'your_turn', waiting: 'plan' }), { line: { key: 'plan' }, tone: 'turn' })
  assert.deepEqual(line({ state: 'your_turn', waiting: 'question' }).line, { key: 'question' })
  assert.deepEqual(line({ state: 'working' }), { line: { key: 'working' }, tone: 'work' })
  assert.deepEqual(line({ state: 'starting' }).line, { key: 'starting' })
  assert.deepEqual(line({ state: 'failed' }), { line: { key: 'failed' }, tone: 'problem' })
  assert.deepEqual(line({ state: 'stuck', stuck: 'blocked' }), { line: { key: 'stuck' }, tone: 'problem' })
  assert.deepEqual(line({ state: 'stuck', stuck: 'stopped' }), { line: { key: 'stopped' }, tone: 'quiet' })
  assert.deepEqual(line({ state: 'paused' }).line, { key: 'pausedSoon' })
  assert.deepEqual(line({ state: 'paused', resets: '5pm (Asia/Seoul)' }).line, { key: 'paused', when: '5pm (Asia/Seoul)' })
  const paused = line({ state: 'paused', resets_at: new Date(Date.UTC(2026, 10, 25, 14, 40)).toISOString() }).line
  assert.equal(paused.key, 'paused')
  assert.deepEqual(line({ state: 'done', save_failed: true }), { line: { key: 'saveFailed' }, tone: 'problem' })
  const done = line({ state: 'done', changed_at: new Date(now.getTime() - 86_400_000).toISOString() })
  assert.deepEqual(done, { line: { key: 'done', ago: 'yesterday' }, tone: 'quiet' })
})

test('how long ago reads in the person’s language', () => {
  const now = new Date(Date.UTC(2026, 10, 25, 12, 0, 0))
  const ago = (ms: number, locale = 'en') => easyAgo(new Date(now.getTime() - ms).toISOString(), locale, now)
  assert.equal(ago(10_000), 'now')
  assert.equal(ago(10 * 60_000), '10 minutes ago')
  assert.equal(ago(3 * 3_600_000), '3 hours ago')
  assert.equal(ago(86_400_000), 'yesterday')
  assert.equal(ago(86_400_000, 'ko'), '어제')
  assert.equal(ago(14 * 86_400_000), '2 weeks ago')
  assert.equal(easyAgo('not a time', 'en', now), '')
})

test('refusals map to plain sentences, and busy work is not offered to move', () => {
  assert.equal(manageErrorKey('busy'), 'busy')
  assert.equal(manageErrorKey('name_required'), 'nameRequired')
  assert.equal(manageErrorKey('name_too_long'), 'nameTooLong')
  assert.equal(manageErrorKey('folder_outside'), 'folderOutside')
  assert.equal(manageErrorKey('not_found'), 'notFound')
  assert.equal(manageErrorKey('write_failed'), 'unknown')
  assert.equal(manageable('working'), false)
  assert.equal(manageable('starting'), false)
  assert.equal(manageable('stuck'), true)
  assert.equal(manageable('done'), true)
})

test('the shell, home and board strings hide the machinery in both languages', () => {
  for (const [lang, strings] of [['en', easyEn], ['ko', easyKo]] as const) {
    for (const section of ['shell', 'home', 'board'] as const) {
      const text = JSON.stringify(strings[section], (_k, v) => (typeof v === 'function' ? v('X', 'Y', 'Z') : v))
      assert.deepEqual(hiddenWordsIn(text), [], `${lang}.${section}`)
    }
  }
})
