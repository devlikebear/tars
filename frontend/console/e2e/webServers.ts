// Shared `webServer` entries for playwright.config.ts (console E2E) and
// playwright.capture.config.ts (README/tars-site media capture). Both drive
// the same mock LLM + `tars serve` pair against a throwaway workspace,
// differing only in ports and workspace path — kept in one place so the
// two configs don't carry a second, drifting copy of the same server
// definitions (flagged by SonarCloud as duplicated code).

import { join } from 'node:path'

export function mockLLMWebServer(mockPort: number) {
  return {
    command: 'node e2e/mock-llm.mjs',
    url: `http://127.0.0.1:${mockPort}/health`,
    env: { TARS_E2E_MOCK_LLM_PORT: String(mockPort) },
    reuseExistingServer: false,
    timeout: 15_000,
  }
}

export function tarsServeWebServer(opts: { repoRoot: string; workspace: string; tarsPort: number }) {
  const { repoRoot, workspace, tarsPort } = opts
  return {
    command: `go run ./cmd/tars serve --workspace-dir "${workspace}" --config "${join(workspace, 'config', 'tars.config.yaml')}" --api-addr 127.0.0.1:${tarsPort}`,
    cwd: repoRoot,
    url: `http://127.0.0.1:${tarsPort}/console`,
    env: {
      TARS_API_AUTH_MODE: 'off',
      TARS_DASHBOARD_AUTH_MODE: 'off',
      TARS_API_ALLOW_INSECURE_LOCAL_AUTH: 'true',
      // Serve the embedded build, never a Vite dev proxy.
      TARS_CONSOLE_DEV_URL: '',
      // Focus PR stages probe this stub, never the host's gh or the
      // network (GitHub runners have an authenticated gh; a capture run
      // has no PR to look at either way).
      TARS_FOCUS_GH_PATH: join(repoRoot, 'frontend', 'console', 'e2e', 'fake-gh.sh'),
      // A plan's end-to-end goals drive this stub, never a cua-driver
      // installed on the host — that one reads the real screen.
      CUA_DRIVER_PATH: join(repoRoot, 'frontend', 'console', 'e2e', 'fake-cua-driver.sh'),
      TARS_E2E_CUA_LOG: join(workspace, 'e2e-cua-driver.log'),
    },
    reuseExistingServer: false,
    // The first `go run` compiles the binary.
    timeout: 240_000,
    stdout: 'ignore' as const,
    stderr: 'pipe' as const,
  }
}
