// Best-effort removal of the throwaway workspace created in
// playwright.config.ts. The server may still hold files open (Windows), so
// failures are ignored; the directory lives under the OS temp dir anyway.

import { rmSync } from 'node:fs'
import { tmpdir } from 'node:os'

export default function teardown() {
  const workspace = process.env.TARS_E2E_WORKSPACE
  if (!workspace?.startsWith(tmpdir())) return
  try {
    rmSync(workspace, { recursive: true, force: true, maxRetries: 3 })
  } catch { /* leave it for the OS temp cleaner */ }
}
