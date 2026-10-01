// Folder picker in the Files dock panel ("+"): a typed or pasted path opens
// that folder directly, a filter narrows the list, and dot folders wait in a
// collapsed group at the end.

import { mkdirSync } from 'node:fs'
import { join } from 'node:path'
import { expect, test, type Page } from '@playwright/test'

const root = join(process.env.TARS_E2E_WORKSPACE ?? '', 'picker-fixture')
for (const dir of ['alpha', 'beta', 'Workbench', '.cache', '.claude/worktrees/deep']) {
  mkdirSync(join(root, dir), { recursive: true })
}

async function openPicker(page: Page) {
  await page.goto('/console/chat')
  await expect(page.locator('.dock-left .session-btn').first()).toBeVisible()
  await page.locator('.dock-left .new-chat-btn').click()
  await expect(page).toHaveURL(/\/console\/chat\/[^/]+$/)
  await page.locator('.chat-rail [data-panel="artifacts"]').click() // Files
  const pane = page.locator('.dock-right')
  await pane.getByRole('button', { name: '+', exact: true }).click()
  const picker = pane.locator('.pick-overlay')
  await expect(picker).toBeVisible()
  // The picker opens at the server's home folder.
  await expect(picker.locator('.pick-current')).not.toHaveValue('')
  return picker
}

const names = (picker: ReturnType<Page['locator']>) => picker.locator('.pick-list .artifact-name')

test('typing a path opens it; dot folders are folded at the end', async ({ page }) => {
  const picker = await openPicker(page)
  const path = picker.getByRole('textbox', { name: 'Folder path' })
  await path.fill(`${root}/`)
  await path.press('Enter')
  await expect(path).toHaveValue(root)
  await expect(names(picker)).toHaveText(['..', 'alpha', 'beta', 'Workbench'])

  const toggle = picker.getByRole('button', { name: 'Hidden folders (2)' })
  await expect(toggle).toHaveAttribute('aria-expanded', 'false')
  await toggle.click()
  await expect(toggle).toHaveAttribute('aria-expanded', 'true')
  await expect(names(picker)).toHaveText(['..', 'alpha', 'beta', 'Workbench', '.cache', '.claude'])

  // A deep dot-folder path in one step, then pick it.
  await path.fill(join(root, '.claude', 'worktrees'))
  await path.press('Enter')
  await expect(names(picker)).toHaveText(['..', 'deep'])
})

test('a missing path shows an error and keeps the listing', async ({ page }) => {
  const picker = await openPicker(page)
  const path = picker.getByRole('textbox', { name: 'Folder path' })
  await path.fill(root)
  await path.press('Enter')
  await expect(names(picker)).toHaveText(['..', 'alpha', 'beta', 'Workbench'])

  await path.fill(join(root, 'nope'))
  await path.press('Enter')
  await expect(picker.getByRole('alert')).toHaveText(`Folder not found: ${join(root, 'nope')}`)
  await expect(names(picker)).toHaveText(['..', 'alpha', 'beta', 'Workbench'])

  await path.fill('relative/path')
  await path.press('Enter')
  await expect(picker.getByRole('alert')).toHaveText('Use a full path, or one starting with ~')

  // Escape puts back the folder being shown.
  await path.press('Escape')
  await expect(path).toHaveValue(root)
  await expect(picker.getByRole('alert')).toHaveCount(0)
})

test('the filter narrows the list, dot folders included, and Enter opens the first match', async ({ page }) => {
  const picker = await openPicker(page)
  const path = picker.getByRole('textbox', { name: 'Folder path' })
  await path.fill(root)
  await path.press('Enter')
  await expect(names(picker)).toHaveText(['..', 'alpha', 'beta', 'Workbench'])

  const filter = picker.getByRole('searchbox', { name: 'Filter folders' })
  await filter.fill('C')
  await expect(names(picker)).toHaveText(['..', 'Workbench', '.cache', '.claude'])
  await filter.fill('zzz')
  await expect(picker).toContainText('No folders match "zzz"')

  await filter.fill('clau')
  await filter.press('Enter')
  await expect(path).toHaveValue(join(root, '.claude'))
  await expect(filter).toHaveValue('')
  await expect(names(picker)).toHaveText(['..', 'worktrees'])

  // Selecting adds the folder as the session's working directory.
  await path.fill(join(root, 'beta'))
  await path.press('Enter')
  await expect(path).toHaveValue(join(root, 'beta'))
  await picker.getByRole('button', { name: 'Select Here' }).click()
  await expect(picker).toHaveCount(0)
})
