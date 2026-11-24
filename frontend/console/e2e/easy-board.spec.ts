// The vibe-mode shell, home and project board (docs/plans/vibe-mode-redesign
// unit 2). The first test runs on the real server: an idea typed on the
// home screen becomes a project that the board lists. The others play the
// list API in the browser (a small in-memory board below), because the
// board's columns, pages and menu need projects in every state, which the
// real server only reaches through whole runs of the team.

import { expect, test, type Page, type Route } from '@playwright/test'

import { hiddenWordsIn } from '../src/lib/easy.ts'

async function expectNoHiddenWords(page: Page, where: string) {
  const text = await page.getByTestId('easy-root').innerText()
  expect(hiddenWordsIn(text), `${where}:\n${text}`).toEqual([])
  const bar = await page.locator('.vibe-bar').innerText()
  expect(hiddenWordsIn(bar), `${where} (bar):\n${bar}`).toEqual([])
}

async function vibeOnly(page: Page) {
  await page.route('**/v1/healthz', async (route) => {
    const real = await route.fetch()
    const body = await real.json()
    return route.fulfill({ response: real, json: { ...body, console_expert_mode: false } })
  })
}

test('an idea from the home screen shows up on the board', async ({ page }) => {
  await page.goto('/console/easy')
  await expect(page.getByRole('heading', { name: 'What do you want to make?' })).toBeVisible()
  await expect(page.getByTestId('vibe-nav-make')).toHaveAttribute('aria-current', 'page')
  await expect(page.getByTestId('easy-team')).toContainText('Leader')
  await expectNoHiddenWords(page, 'home')

  // An example fills the box; the person can still change it.
  await page.getByTestId('easy-example').first().click()
  await expect(page.getByTestId('easy-idea')).toHaveValue('A to-do list I can tick off')

  const name = `Board check ${Date.now()}`
  await page.getByTestId('easy-idea').fill(name)
  await page.getByTestId('easy-start').click()
  await expect(page).toHaveURL(/\/console\/easy\/[0-9a-f]{16}$/)

  await page.getByTestId('vibe-nav-projects').click()
  await expect(page).toHaveURL(/\/console\/easy\/projects$/)
  await expect(page.getByRole('heading', { name: 'My projects' })).toBeVisible()
  const card = page.getByTestId('easy-project-item').filter({ hasText: name })
  await expect(card).toHaveCount(1)
  await expectNoHiddenWords(page, 'board')

  // Search keeps only names that hold the words.
  await page.getByTestId('easy-board-search').fill(name.toLowerCase())
  await expect(page.getByTestId('easy-project-item')).toHaveCount(1)
  await page.getByTestId('easy-board-search').fill('no project is called this')
  await expect(page.getByTestId('easy-board-nomatch')).toBeVisible()
  await page.getByTestId('easy-board-search').fill('')
  await expect(card).toHaveCount(1)

  // The card opens the project.
  await card.getByTestId('easy-project-open').click()
  await expect(page).toHaveURL(/\/console\/easy\/[0-9a-f]{16}$/)

  // The home screen lists it among the recent projects.
  await page.getByTestId('vibe-nav-make').click()
  await expect(page.getByTestId('easy-recent').getByTestId('easy-project-item').filter({ hasText: name })).toHaveCount(1)
})

// --- The board played in the browser ---------------------------------------

type State = 'starting' | 'failed' | 'your_turn' | 'working' | 'paused' | 'stuck' | 'done' | 'published'
type FakeProject = {
  id: string
  name: string
  state: State
  minutesAgo: number
  waiting?: string
  stuck?: string
  archived?: boolean
  busy?: boolean
}

const states: State[] = ['your_turn', 'working', 'paused', 'stuck', 'done', 'published', 'starting', 'failed']
const now = Date.now()

function hexId(n: number): string {
  return n.toString(16).padStart(16, '0')
}

function seed(): FakeProject[] {
  let n = 1
  const p = (name: string, state: State, minutesAgo: number, extra: Partial<FakeProject> = {}): FakeProject => ({ id: hexId(n++), name, state, minutesAgo, ...extra })
  return [
    p('Club dues', 'your_turn', 10, { waiting: 'plan' }),
    p('Cafe page', 'working', 3),
    p('Getting going', 'starting', 1),
    p('Family to-dos', 'paused', 20),
    p('Trip checklist', 'stuck', 60 * 24, { stuck: 'blocked' }),
    p('Broken start', 'failed', 60 * 30),
    ...[1, 2, 3, 4, 5, 6, 7].map((i) => p(`Finished ${i}`, 'done', 60 * i)),
    p('Old guest book', 'done', 60 * 24 * 40, { archived: true }),
  ]
}

function listed(p: FakeProject) {
  const at = new Date(now - p.minutesAgo * 60_000).toISOString()
  return {
    id: p.id, name: p.name, idea: p.name, folder: `/projects/${p.id}`, status: p.state === 'failed' ? 'failed' : p.state === 'starting' ? 'creating' : 'ready',
    created_at: at, updated_at: at, archived_at: p.archived ? at : undefined,
    board: { state: p.state, changed_at: at, waiting: p.waiting, stuck: p.stuck },
  }
}

type Calls = { patches: Array<{ id: string; body: unknown }>; deletes: string[]; lists: string[] }

// The list, rename/archive and delete routes over an in-memory board. The
// cursor is a plain offset; the server's own is opaque to the console.
async function fakeBoard(page: Page, projects: FakeProject[]): Promise<Calls> {
  const calls: Calls = { patches: [], deletes: [], lists: [] }
  await page.route(/\/v1\/easy\/projects(\?.*)?$/, async (route: Route) => {
    if (route.request().method() !== 'GET') return route.fallback()
    const url = new URL(route.request().url())
    calls.lists.push(url.search)
    const archived = url.searchParams.get('archived') === '1'
    const state = url.searchParams.get('state')
    const limit = Number(url.searchParams.get('limit') || 0)
    const offset = Number(url.searchParams.get('cursor') || 0)
    const side = projects.filter((p) => !!p.archived === archived).sort((a, b) => a.minutesAgo - b.minutesAgo)
    const counts = Object.fromEntries(states.map((s) => [s, side.filter((p) => p.state === s).length]))
    const picked = side.filter((p) => !state || p.state === state)
    const end = limit ? Math.min(picked.length, offset + limit) : picked.length
    return route.fulfill({
      json: {
        projects: picked.slice(offset, end).map(listed),
        next_cursor: end < picked.length ? String(end) : undefined,
        counts,
        archived: projects.filter((p) => p.archived).length,
      },
    })
  })
  await page.route(/\/v1\/easy\/projects\/[0-9a-f]{16}$/, async (route: Route) => {
    const method = route.request().method()
    const id = route.request().url().split('/').pop()!
    const project = projects.find((p) => p.id === id)
    if (!project) return route.fulfill({ status: 404, json: { code: 'not_found', error: 'no such project' } })
    if (method === 'PATCH') {
      const body = route.request().postDataJSON() as { name?: string; archived?: boolean }
      calls.patches.push({ id, body })
      if (body.name !== undefined && !body.name.trim()) return route.fulfill({ status: 400, json: { code: 'name_required', error: 'name required' } })
      if (body.archived !== undefined && project.busy) return route.fulfill({ status: 409, json: { code: 'busy', error: 'busy' } })
      if (body.name !== undefined) project.name = body.name.trim()
      if (body.archived !== undefined) project.archived = body.archived
      const l = listed(project)
      return route.fulfill({ json: { project: { ...l, board: undefined }, board: l.board } })
    }
    if (method === 'DELETE') {
      calls.deletes.push(id)
      if (project.busy) return route.fulfill({ status: 409, json: { code: 'busy', error: 'busy' } })
      projects.splice(projects.indexOf(project), 1)
      return route.fulfill({ json: { deleted_to: '/projects/.deleted/x' } })
    }
    return route.fallback()
  })
  return calls
}

function column(page: Page, name: string) {
  return page.getByTestId(`easy-column-${name}`)
}

test('the board groups projects by the server state and pages each column', async ({ page }) => {
  await fakeBoard(page, seed())
  await page.goto('/console/easy/projects')

  await expect(page.getByTestId('easy-board-total')).toHaveText('13')
  await expect(page.getByTestId('vibe-waiting-count')).toHaveText('1')
  await expect(page.getByTestId('easy-board-archive')).toHaveText('Archive 1')

  await expect(column(page, 'your_turn').getByTestId('easy-project-item')).toHaveText([/Club dues.*Please check the blueprint/s])
  // A project being set up is already the team's work.
  await expect(column(page, 'working').getByTestId('easy-project-item')).toHaveText([/Getting going.*Getting ready/s, /Cafe page.*The team is working/s])
  // Paused, stuck and failed share one column, newest first.
  await expect(column(page, 'stopped').getByTestId('easy-project-item')).toHaveText([
    /Family to-dos.*Taking a break/s,
    /Trip checklist.*The team got stuck/s,
    /Broken start.*Could not start/s,
  ])
  await expect(column(page, 'stopped').getByTestId('easy-column-count')).toHaveText('3')
  await expect(column(page, 'published').getByTestId('easy-column-empty')).toBeVisible()

  // Five cards, then more.
  const done = column(page, 'done')
  await expect(done.getByTestId('easy-column-count')).toHaveText('7')
  await expect(done.getByTestId('easy-project-item')).toHaveCount(5)
  await expect(done.getByTestId('easy-project-item').first()).toContainText('Finished 1')
  await done.getByTestId('easy-column-more').click()
  await expect(done.getByTestId('easy-project-item')).toHaveCount(7)
  await expect(done.getByTestId('easy-project-item').last()).toContainText('Finished 7')
  await expect(done.getByTestId('easy-column-more')).toHaveCount(0)
  await expectNoHiddenWords(page, 'board')

  // The menu does not offer to move work the team is doing.
  await column(page, 'working').getByTestId('easy-card-menu').first().click()
  await expect(page.getByTestId('easy-menu-archive')).toBeDisabled()
  await expect(page.getByTestId('easy-menu-delete')).toBeDisabled()
})

test('a project is renamed from its card', async ({ page }) => {
  const calls = await fakeBoard(page, seed())
  await page.goto('/console/easy/projects')
  const card = column(page, 'your_turn').getByTestId('easy-project-item')
  await card.getByTestId('easy-card-menu').click()
  await page.getByTestId('easy-menu-rename').click()

  const input = card.getByTestId('easy-rename-input')
  await expect(input).toHaveValue('Club dues')
  await input.fill('   ')
  await card.getByTestId('easy-rename-save').click()
  await expect(card.getByTestId('easy-rename-error')).toHaveText('Write a name first.')

  await input.fill('Club dues ledger')
  await input.press('Enter')
  await expect(card.getByTestId('easy-project-open')).toHaveText('Club dues ledger')
  expect(calls.patches.at(-1)).toEqual({ id: hexId(1), body: { name: 'Club dues ledger' } })
})

test('a project is archived, found in the archive and brought back', async ({ page }) => {
  const calls = await fakeBoard(page, seed())
  await page.goto('/console/easy/projects')
  const done = column(page, 'done')
  await done.getByTestId('easy-project-item').filter({ hasText: 'Finished 1' }).getByTestId('easy-card-menu').click()
  await page.getByTestId('easy-menu-archive').click()
  await expect(done.getByTestId('easy-project-item').filter({ hasText: 'Finished 1' })).toHaveCount(0)
  await expect(done.getByTestId('easy-column-count')).toHaveText('6')
  await expect(page.getByTestId('easy-board-archive')).toHaveText('Archive 2')
  expect(calls.patches.at(-1)?.body).toEqual({ archived: true })

  await page.getByTestId('easy-board-archive').click()
  await expect(page).toHaveURL(/\/console\/easy\/archive$/)
  await expect(page.getByRole('heading', { name: 'Archive' })).toBeVisible()
  const list = page.getByTestId('easy-archive-list').getByTestId('easy-project-item')
  await expect(list).toHaveText([/Finished 1/, /Old guest book/])
  await expectNoHiddenWords(page, 'archive')

  await list.filter({ hasText: 'Finished 1' }).getByTestId('easy-card-menu').click()
  await page.getByTestId('easy-menu-restore').click()
  await expect(list).toHaveText([/Old guest book/])
  expect(calls.patches.at(-1)?.body).toEqual({ archived: false })

  await page.getByTestId('easy-board-back').click()
  await expect(column(page, 'done').getByTestId('easy-project-item').filter({ hasText: 'Finished 1' })).toHaveCount(1)
})

test('deleting asks first, and a project the team is on is refused in plain words', async ({ page }) => {
  const projects = seed()
  projects.find((p) => p.name === 'Trip checklist')!.busy = true
  const calls = await fakeBoard(page, projects)
  await page.goto('/console/easy/projects')

  const done = column(page, 'done')
  const target = done.getByTestId('easy-project-item').filter({ hasText: 'Finished 2' })
  await target.getByTestId('easy-card-menu').click()
  await page.getByTestId('easy-menu-delete').click()
  const dialog = page.getByTestId('easy-delete-dialog')
  await expect(dialog).toContainText('Delete “Finished 2”?')
  await expect(dialog).toContainText('.deleted')
  await page.getByTestId('easy-delete-cancel').click()
  await expect(dialog).toHaveCount(0)
  expect(calls.deletes).toEqual([])
  await expect(target).toHaveCount(1)

  await target.getByTestId('easy-card-menu').click()
  await page.getByTestId('easy-menu-delete').click()
  await page.getByTestId('easy-delete-confirm').click()
  await expect(target).toHaveCount(0)
  expect(calls.deletes).toEqual([hexId(8)])

  // The server answers 409 busy: nothing changes, and the screen says why.
  const stuck = column(page, 'stopped').getByTestId('easy-project-item').filter({ hasText: 'Trip checklist' })
  await stuck.getByTestId('easy-card-menu').click()
  await page.getByTestId('easy-menu-delete').click()
  await page.getByTestId('easy-delete-confirm').click()
  await expect(page.getByTestId('easy-board-error')).toHaveText('The team is still working on this project. Try again when it is done or stopped.')
  await expect(stuck).toHaveCount(1)
})

test('home shows a project waiting on the person first', async ({ page }) => {
  await fakeBoard(page, seed())
  await page.goto('/console/easy')
  const recent = page.getByTestId('easy-recent').getByTestId('easy-project-item')
  await expect(recent).toHaveText([/Club dues.*Your turn/s, /Getting going/, /Cafe page/])
  await expect(page.getByTestId('easy-see-all')).toHaveText('See all (13) →')
  await page.getByTestId('easy-see-all').click()
  await expect(page).toHaveURL(/\/console\/easy\/projects$/)
})

test('the shell has no way out in vibe mode only, and one with expert mode on', async ({ page }) => {
  await fakeBoard(page, seed())
  await page.goto('/console/easy/projects')
  await expect(page.getByTestId('vibe-expert')).toBeVisible()
  await expect(page.getByTestId('vibe-ai')).toHaveAttribute('data-state', 'ready')

  await vibeOnly(page)
  await page.goto('/console/easy/projects')
  await expect(page.getByRole('heading', { name: 'My projects' })).toBeVisible()
  await expect(page.getByTestId('vibe-expert')).toHaveCount(0)
  await expect(page.getByRole('navigation', { name: 'Main navigation' })).toHaveCount(0)
  await page.getByTestId('vibe-nav-make').click()
  await expect(page).toHaveURL(/\/console\/easy$/)
  await expect(page.getByRole('heading', { name: 'What do you want to make?' })).toBeVisible()
  await page.getByTestId('vibe-nav-projects').click()
  await expect(page).toHaveURL(/\/console\/easy\/projects$/)
})

test.describe('Korean', () => {
  test.use({ locale: 'ko-KR' })

  test('the home and board read in Korean and hide the machinery', async ({ page }) => {
    await fakeBoard(page, seed())
    await page.goto('/console/easy')
    await expect(page.getByRole('heading', { name: '무엇을 만들까요?' })).toBeVisible()
    await expect(page.getByTestId('easy-start')).toHaveText('팀에게 맡기기')
    await expect(page.getByTestId('vibe-nav-projects')).toContainText('내 프로젝트')
    await expectNoHiddenWords(page, 'home (ko)')

    await page.goto('/console/easy/projects')
    await expect(column(page, 'your_turn')).toContainText('내 차례')
    await expect(column(page, 'your_turn')).toContainText('설계도를 확인해 주세요')
    await expect(column(page, 'stopped')).toContainText('멈춤')
    await expect(column(page, 'done')).toContainText('완성 · 공개 전')
    await expectNoHiddenWords(page, 'board (ko)')
  })
})
