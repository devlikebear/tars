// Folder picker in the Files dock panel (ArtifactPanel "+"): turning a typed
// or pasted path into one GET /v1/filesystem/browse accepts, and arranging a
// long folder list. Pure so it runs under node --test.

export type PickerPathResult =
  | { ok: true; path: string }
  | { ok: false; reason: 'empty' | 'relative' }

export type BrowseErrorReason = 'notFound' | 'notDirectory' | 'unreadable' | 'failed'

const windowsRoot = /^[A-Za-z]:[\\/]*$/
const windowsAbs = /^[A-Za-z]:[\\/]/

function unquote(value: string): string {
  if (value.length >= 2) {
    const first = value[0]
    if ((first === '"' || first === "'") && value[value.length - 1] === first) return value.slice(1, -1).trim()
  }
  return value
}

function trimTrailingSeparators(path: string): string {
  if (path === '/' || /^\/+$/.test(path)) return '/'
  if (windowsRoot.test(path)) return `${path.slice(0, 2)}\\`
  return path.replace(/[\\/]+$/, '')
}

// The server takes only absolute paths, so `~` is expanded here against the
// home folder the picker opened in. `~user` and relative paths are refused
// rather than guessed at.
export function resolvePickerPath(input: string, home: string): PickerPathResult {
  const value = unquote(input.trim())
  if (!value) return { ok: false, reason: 'empty' }

  if (value === '~' || value.startsWith('~/') || value.startsWith('~\\')) {
    if (!home) return { ok: false, reason: 'relative' }
    const base = trimTrailingSeparators(home)
    const rest = value.slice(1).replace(/^[\\/]+/, '')
    if (!rest) return { ok: true, path: base }
    const sep = base.includes('\\') && !base.includes('/') ? '\\' : '/'
    return { ok: true, path: trimTrailingSeparators(`${base === '/' ? '' : base}${sep}${rest}`) }
  }

  if (value.startsWith('/') || windowsAbs.test(value) || value.startsWith('\\\\')) {
    return { ok: true, path: trimTrailingSeparators(value) }
  }
  return { ok: false, reason: 'relative' }
}

// Status codes of GET /v1/filesystem/browse (handler_filesystem.go). The
// server's English message is not shown; the console words it itself.
export function browseErrorReason(status: number | undefined): BrowseErrorReason {
  switch (status) {
    case 404: return 'notFound'
    case 400: return 'notDirectory'
    case 403: return 'unreadable'
    default: return 'failed'
  }
}

export function isHiddenFolder(name: string): boolean {
  return name.startsWith('.') && name !== '.' && name !== '..'
}

// Splits a listing into ordinary and dot folders, keeping the server's order,
// after a case-insensitive part-of-name filter. The panel shows dot folders in
// a collapsed group at the end so they don't push the rest out of view.
export function arrangePickerEntries<T extends { name: string }>(entries: T[], filter: string): { shown: T[]; hidden: T[] } {
  const needle = filter.trim().toLowerCase()
  const shown: T[] = []
  const hidden: T[] = []
  for (const entry of entries) {
    if (needle && !entry.name.toLowerCase().includes(needle)) continue
    ;(isHiddenFolder(entry.name) ? hidden : shown).push(entry)
  }
  return { shown, hidden }
}
