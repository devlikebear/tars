// The focus event stream (docs/decisions/mobile-remote.md §4): the phone
// app opens GET /v1/focus/events through fetch and reads its lists again
// when a pipeline changes. Against the real server: creating and stopping a
// pipeline each say which pipeline changed, and nothing more.
import { execFileSync } from 'node:child_process'
import { mkdtempSync, realpathSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { expect, test } from '@playwright/test'

function newRepo(): string {
  const repo = realpathSync(mkdtempSync(join(tmpdir(), 'tars-e2e-focus-events-')))
  const git = (...args: string[]) => execFileSync('git', args, { cwd: repo, stdio: 'ignore' })
  git('init', '-q', '-b', 'main')
  writeFileSync(join(repo, 'base.txt'), 'base\n')
  git('add', '-A')
  git('-c', 'user.name=e2e', '-c', 'user.email=e2e@example.invalid', 'commit', '-q', '-m', 'init')
  return repo
}

type StreamEvent = { type: string; session_id?: string }

test('a pipeline that changes is named on the focus event stream', async ({ page }) => {
  await page.goto('/console/focus')
  // Read the stream the way the app does: fetch, line by line.
  await page.evaluate(() => {
    const events: StreamEvent[] = []
    ;(window as unknown as { focusEvents: StreamEvent[] }).focusEvents = events
    void fetch('/v1/focus/events', { headers: { Accept: 'text/event-stream' } }).then(async (response) => {
      const reader = response.body!.getReader()
      const decoder = new TextDecoder()
      let buffer = ''
      for (;;) {
        const { value, done } = await reader.read()
        if (done) return
        buffer += decoder.decode(value, { stream: true })
        const lines = buffer.split('\n')
        buffer = lines.pop() ?? ''
        for (const line of lines) if (line.startsWith('data: ')) events.push(JSON.parse(line.slice(6)))
      }
    })
  })
  const events = () => page.evaluate(() => (window as unknown as { focusEvents: StreamEvent[] }).focusEvents)
  // The server has its baseline before it says "connected".
  await expect.poll(async () => (await events())[0]?.type).toBe('connected')

  const created = await (await page.request.post('/v1/focus/pipelines', { data: { goal: 'stream me', cwd: newRepo() } })).json()
  const id = created.session_id as string
  const about = async () => (await events()).filter((e) => e.session_id === id)
  await expect.poll(async () => (await about()).length).toBeGreaterThan(0)
  for (const event of await about()) expect(event).toEqual({ type: 'pipeline', session_id: id })

  const seen = (await about()).length
  expect((await page.request.post(`/v1/focus/pipelines/${id}/stop`)).ok()).toBe(true)
  await expect.poll(async () => (await about()).length).toBeGreaterThan(seen)
})
