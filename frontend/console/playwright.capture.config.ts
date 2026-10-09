// README/tars-site media capture (`make console-screenshots`): the same
// real `tars serve` + mock LLM pattern as playwright.config.ts, on its own
// ports and workspace so it can run alongside the normal E2E suite, driving
// a single spec (e2e/capture/capture.spec.ts) that walks the console and
// leaves screenshots plus a recorded demo video under e2e/capture/output/.
// Not part of `make console-e2e` or CI — run explicitly, by a developer,
// when the console UI has changed enough to need new marketing media.

import { mkdtempSync, mkdirSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { defineConfig, devices } from '@playwright/test'
import { mockLLMWebServer, tarsServeWebServer } from './e2e/webServers.ts'

const tarsPort = Number(process.env.TARS_CAPTURE_PORT || 43390)
const mockPort = Number(process.env.TARS_CAPTURE_MOCK_LLM_PORT || 43391)
const repoRoot = fileURLToPath(new URL('../..', import.meta.url))
const outputDir = fileURLToPath(new URL('./e2e/capture/output', import.meta.url))

if (!process.env.TARS_CAPTURE_WORKSPACE) {
  const workspace = mkdtempSync(join(tmpdir(), 'tars-capture-'))
  mkdirSync(join(workspace, 'config'), { recursive: true })
  writeFileSync(join(workspace, 'config', 'tars.config.yaml'), [
    '# Generated for console media capture. The provider is e2e/mock-llm.mjs.',
    'llm:',
    '  providers:',
    '    mock:',
    '      kind: openai',
    '      auth_mode: api-key',
    `      base_url: http://127.0.0.1:${mockPort}/v1`,
    '      api_key: sk-e2e-mock',
    '  tiers:',
    '    heavy: { provider: mock, model: e2e-model }',
    '    standard: { provider: mock, model: e2e-model }',
    '    light: { provider: mock, model: e2e-model }',
    '  default_tier: standard',
    'pulse:',
    '  enabled: false',
    'reflection:',
    '  enabled: false',
    '',
  ].join('\n'))
  process.env.TARS_CAPTURE_WORKSPACE = workspace
}
const workspace = process.env.TARS_CAPTURE_WORKSPACE

export default defineConfig({
  testDir: './e2e/capture',
  globalTeardown: './e2e/capture/teardown.ts',
  outputDir: join(outputDir, 'test-results'),
  fullyParallel: false,
  workers: 1,
  forbidOnly: !!process.env.CI,
  retries: 0,
  timeout: 90_000,
  expect: { timeout: 10_000 },
  reporter: 'list',
  use: {
    baseURL: `http://127.0.0.1:${tarsPort}`,
    trace: 'off',
    video: { mode: 'on', size: { width: 1400, height: 900 } },
    locale: 'en-US',
    viewport: { width: 1400, height: 900 },
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'], viewport: { width: 1400, height: 900 }, locale: 'en-US' } }],
  webServer: [
    mockLLMWebServer(mockPort),
    tarsServeWebServer({ repoRoot, workspace, tarsPort }),
  ],
})
