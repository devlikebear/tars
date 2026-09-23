// Console E2E (#968): the real `tars serve` with the embedded console build,
// a throwaway workspace, and a deterministic mock LLM. Run via
// `make console-e2e`, which builds the console assets first.

import { mkdtempSync, mkdirSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { defineConfig, devices } from '@playwright/test'

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
    {
      command: 'node e2e/mock-llm.mjs',
      url: `http://127.0.0.1:${mockPort}/health`,
      env: { TARS_E2E_MOCK_LLM_PORT: String(mockPort) },
      reuseExistingServer: false,
      timeout: 15_000,
    },
    {
      command: `go run ./cmd/tars serve --workspace-dir "${workspace}" --config "${join(workspace, 'config', 'tars.config.yaml')}" --api-addr 127.0.0.1:${tarsPort}`,
      cwd: repoRoot,
      url: `http://127.0.0.1:${tarsPort}/console`,
      env: {
        TARS_API_AUTH_MODE: 'off',
        TARS_DASHBOARD_AUTH_MODE: 'off',
        TARS_API_ALLOW_INSECURE_LOCAL_AUTH: 'true',
        // Serve the embedded build, never a Vite dev proxy.
        TARS_CONSOLE_DEV_URL: '',
      },
      reuseExistingServer: false,
      // The first `go run` compiles the binary.
      timeout: 240_000,
      stdout: 'ignore',
      stderr: 'pipe',
    },
  ],
})
