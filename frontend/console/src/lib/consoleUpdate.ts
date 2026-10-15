// tars#1257: when the server starts serving a different console bundle
// than the one running in this tab, show a small, dismissible "TARS was
// updated — Reload" notice (App.svelte). This module holds the two pure
// pieces: reading this tab's own build value, and deciding whether to
// show the notice. Neither touches the DOM beyond the one read, and
// neither ever triggers a reload itself — that stays a user click
// (handleConsoleUpdateReload in App.svelte), so typed-but-unsent input
// is never discarded out from under the user.

export const CONSOLE_BUILD_META_NAME = 'tars-console-build'

// getOwnConsoleBuild reads the <meta name="tars-console-build"> tag the
// server injected into *this* tab's index.html when it first loaded
// (internal/tarsserver/console.go's injectConsoleBuildMeta). "" when
// absent: the Vite dev server and the unbuilt placeholder never inject
// it, so a tab running under either never has an "own" value to compare
// against and the notice never shows for it (see shouldShowUpdateNotice).
export function getOwnConsoleBuild(doc: Document = document): string {
  const meta = doc.querySelector(`meta[name="${CONSOLE_BUILD_META_NAME}"]`)
  return meta?.getAttribute('content')?.trim() ?? ''
}

// shouldShowUpdateNotice is the pure decision (tars#1257):
//   - own: this tab's running build (getOwnConsoleBuild, read once at load)
//   - served: what the server's healthz just reported as console_build
//   - dismissed: the build value the user last closed the notice for in
//     this tab, or undefined if they never have
//   - isDev: true only under the Vite dev server (import.meta.env.DEV) —
//     the issue is explicit the notice never shows there: a dev server
//     restarts for every file change, so there is no stable bundle to
//     track and every reconnect would otherwise fire the notice
//
// Empty own/served means "unknown" (no meta tag, dev proxy, unbuilt
// placeholder) and never shows the notice: with no own value there is
// nothing to compare against, and a false positive on page load would be
// the first thing this feature causes rather than the problem it is
// meant to catch.
export function shouldShowUpdateNotice(own: string, served: string, dismissed: string | undefined, isDev: boolean): boolean {
  if (isDev) return false
  const ownValue = own.trim()
  const servedValue = served.trim()
  if (!ownValue || !servedValue) return false
  if (servedValue === ownValue) return false
  if (dismissed !== undefined && servedValue === dismissed) return false
  return true
}
