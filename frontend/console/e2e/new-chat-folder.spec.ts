// A new chat in a folder, isolated or not, in one step: the caret beside the
// board's New chat and the sidebar's + New Chat, and `/new [path]
// [--isolate]`. The server creates the session in the folder and, when
// asked, moves it into a worktree before the first turn.

import { execFileSync } from 'node:child_process'
import { mkdtempSync, realpathSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { expect, test, type Page } from '@playwright/test'

function git(dir: string, ...args: string[]) {
  execFileSync('git', ['-c', 'user.name=E2E', '-c', 'user.email=e2e@example.com', '-c', 'commit.gpgSign=false', ...args], { cwd: dir }) // NOSONAR: the machine's own git from PATH
}

function newRepo(prefix: string): string {
  const repo = realpathSync(mkdtempSync(join(tmpdir(), prefix)))
  git(repo, 'init', '-q', '-b', 'main')
  writeFileSync(join(repo, 'base.txt'), 'base\n')
  git(repo, 'add', '-A')
  git(repo, 'commit', '-q', '-m', 'init')
  return repo
}

const sessionId = (page: Page) => page.url().split('/').pop() ?? ''
const composer = (page: Page) => page.locator('.chat-main textarea')

async function cwdOf(page: Page, id: string): Promise<string> {
  const resp = await page.request.get(`/v1/admin/sessions/${id}/cwd`)
  return (await resp.json()).current as string
}

async function sessionOf(page: Page, id: string): Promise<{ worktree?: { branch: string; source_dir: string; reason?: string } | null }> {
  return (await page.request.get(`/v1/admin/sessions/${id}`)).json()
}

test('the board starts a chat isolated in a repository', async ({ page }) => {
  const repo = newRepo('tars-e2e-newchat-')
  await page.goto('/console')
  await page.getByTestId('new-chat-folder').click()
  const panel = page.getByRole('dialog', { name: 'New chat in a folder' })
  await expect(panel).toBeVisible()

  // A folder that is not there is caught before anything is created.
  await panel.getByTestId('new-chat-folder-path').fill(join(repo, 'missing'))
  await expect(panel.getByTestId('new-chat-folder-status')).toHaveText('That folder does not exist.')
  await expect(panel.getByTestId('new-chat-folder-start')).toBeDisabled()

  await panel.getByTestId('new-chat-folder-path').fill(repo)
  await expect(panel.getByTestId('new-chat-folder-status')).toContainText('Git repository')
  await panel.getByTestId('new-chat-folder-isolate').check()
  await panel.getByTestId('new-chat-folder-start').click()

  await expect(page).toHaveURL(/\/console\/chat\/[^/]+$/)
  const id = sessionId(page)
  const chip = page.getByTestId('worktree-chip')
  await expect(chip).toHaveAttribute('title', new RegExp(`tars/session-${id}`))
  await chip.click()
  await expect(page.getByText('Isolated when the chat started')).toBeVisible()
  const sess = await sessionOf(page, id)
  expect(sess.worktree?.source_dir).toBe(repo)
  expect(sess.worktree?.reason).toBe('new_chat')
})

test('the sidebar starts a chat in a recent folder, and a plain folder has no isolate', async ({ page }) => {
  const repo = newRepo('tars-e2e-recent-')
  const plain = realpathSync(mkdtempSync(join(tmpdir(), 'tars-e2e-plain-')))
  // One session that worked in the repository makes it a recent folder.
  const seeded = await (await page.request.post('/v1/admin/sessions', { data: { title: 'seed', cwd: repo } })).json()
  expect(seeded.current_dir).toBe(repo)

  await page.goto('/console/chat')
  await expect(page.locator('.dock-left .session-btn').first()).toBeVisible()
  const sidebar = page.locator('.dock-left')
  await sidebar.getByTestId('new-chat-folder').click()
  const panel = sidebar.getByRole('dialog', { name: 'New chat in a folder' })
  await panel.getByRole('button', { name: new RegExp(repo.split('/').pop() ?? '') }).first().click()
  await expect(panel.getByTestId('new-chat-folder-path')).toHaveValue(repo)
  await expect(panel.getByTestId('new-chat-folder-isolate')).toBeVisible()

  await panel.getByTestId('new-chat-folder-path').fill(plain)
  await expect(panel.getByTestId('new-chat-folder-status')).toHaveText('Not a git repository, so the chat works in the folder itself.')
  await expect(panel.getByTestId('new-chat-folder-isolate')).toHaveCount(0)
  await panel.getByTestId('new-chat-folder-path').press('Enter')

  await expect(page).toHaveURL(/\/console\/chat\/[^/]+$/)
  expect(await cwdOf(page, sessionId(page))).toBe(plain)
  // The cwd chip is there without a reload.
  await expect(page.getByRole('group', { name: 'Session status' }).locator('.cwd-chip')).toContainText(plain.split('/').pop() ?? '')
  await expect(page.getByTestId('worktree-chip')).toHaveCount(0)
})

test('/new starts a chat in this chat’s folder, isolated on request', async ({ page }) => {
  const repo = newRepo('tars-e2e-slash-')
  const seeded = await (await page.request.post('/v1/admin/sessions', { data: { title: 'in repo', cwd: repo } })).json()
  await page.goto(`/console/chat/${seeded.id}`)
  await expect(composer(page)).toBeVisible()

  // --isolate with no path isolates from this chat's folder.
  await composer(page).fill('/new --isolate')
  await composer(page).press('Enter')
  await expect(page).not.toHaveURL(new RegExp(`${seeded.id}$`))
  await expect(page.getByTestId('worktree-chip')).toBeVisible()
  const isolated = sessionId(page)
  expect((await sessionOf(page, isolated)).worktree?.source_dir).toBe(repo)

  // From an isolated chat, /new starts in the checkout, not the worktree.
  await composer(page).fill('/new')
  await composer(page).press('Enter')
  await expect(page).not.toHaveURL(new RegExp(`${isolated}$`))
  const plainNew = sessionId(page)
  expect(await cwdOf(page, plainNew)).toBe(repo)
  expect((await sessionOf(page, plainNew)).worktree ?? null).toBeNull()

  // A path that is not there leaves you where you were.
  await composer(page).fill(`/new ${join(repo, 'missing')}`)
  await composer(page).press('Enter')
  await expect(page.locator('.action-feedback')).toContainText('Could not start the chat: folder not found')
  expect(sessionId(page)).toBe(plainNew)
})

test.describe('Korean', () => {
  test.use({ locale: 'ko-KR' })

  test('the folder menu speaks Korean', async ({ page }) => {
    const plain = realpathSync(mkdtempSync(join(tmpdir(), 'tars-e2e-ko-')))
    await page.goto('/console')
    await page.getByTestId('new-chat-folder').click()
    const panel = page.getByRole('dialog', { name: '폴더에서 새 채팅' })
    await expect(panel.getByText('최근 폴더')).toBeVisible()
    await panel.getByTestId('new-chat-folder-path').fill(join(plain, 'missing'))
    await expect(panel.getByTestId('new-chat-folder-status')).toHaveText('없는 폴더입니다.')
    await panel.getByTestId('new-chat-folder-path').fill(plain)
    await expect(panel.getByTestId('new-chat-folder-status')).toHaveText('Git 저장소가 아니라서 이 폴더에서 바로 작업합니다.')
    await expect(panel.getByRole('button', { name: '채팅 시작' })).toBeEnabled()
    await panel.getByRole('button', { name: '취소' }).click()
    await expect(panel).toHaveCount(0)
  })
})
