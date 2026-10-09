// Console E2E (#968): the real `tars serve` with the embedded console build,
// a throwaway workspace, and a deterministic mock LLM. Run via
// `make console-e2e`, which builds the console assets first.

import { mkdtempSync, mkdirSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { defineConfig, devices } from '@playwright/test'
import { mockLLMWebServer, tarsServeWebServer } from './e2e/webServers.ts'

const tarsPort = Number(process.env.TARS_E2E_PORT || 43290)
const mockPort = Number(process.env.TARS_E2E_MOCK_LLM_PORT || 43291)
const repoRoot = fileURLToPath(new URL('../..', import.meta.url))

// The config is evaluated in the runner and again in each worker. Create the
// workspace once; workers inherit the path through the environment.
if (!process.env.TARS_E2E_WORKSPACE) {
  const workspace = mkdtempSync(join(tmpdir(), 'tars-e2e-'))
  mkdirSync(join(workspace, 'config'), { recursive: true })
  writeFileSync(join(workspace, 'config', 'tars.config.yaml'), [
    '# Generated for console E2E. The provider is e2e/mock-llm.mjs.',
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
  process.env.TARS_E2E_WORKSPACE = workspace
}
const workspace = process.env.TARS_E2E_WORKSPACE

export default defineConfig({
  testDir: './e2e',
  // e2e/capture/ holds the README/tars-site screenshot + demo-video capture
  // spec (`make console-screenshots`, playwright.capture.config.ts). It is
  // not a correctness check, so it never runs as part of this suite or CI.
  testIgnore: '**/capture/**',
  globalTeardown: './e2e/teardown.ts',
  // One server and one workspace are shared, and the sidebar counts sessions.
  fullyParallel: false,
  workers: 1,
  forbidOnly: !!process.env.CI,
  retries: 0,
  timeout: 45_000,
  expect: { timeout: 10_000 },
  reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : 'list',
  use: {
    baseURL: `http://127.0.0.1:${tarsPort}`,
    trace: 'retain-on-failure',
    // Specs assert on English UI strings.
    locale: 'en-US',
    viewport: { width: 1400, height: 900 },
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'], viewport: { width: 1400, height: 900 }, locale: 'en-US' } }],
  webServer: [
    mockLLMWebServer(mockPort),
    tarsServeWebServer({ repoRoot, workspace, tarsPort }),
  ],
})
