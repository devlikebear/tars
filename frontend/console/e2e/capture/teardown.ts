// Best-effort removal of the throwaway workspace created in
// playwright.capture.config.ts. Mirrors e2e/teardown.ts.

import { rmSync } from 'node:fs'
import { tmpdir } from 'node:os'

export default function teardown() {
  const workspace = process.env.TARS_CAPTURE_WORKSPACE
  if (!workspace?.startsWith(tmpdir())) return
  try {
    rmSync(workspace, { recursive: true, force: true, maxRetries: 3 })
  } catch { /* leave it for the OS temp cleaner */ }
}
