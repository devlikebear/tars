// A new chat in a folder, optionally isolated in a worktree (the board's and
// sidebar's folder menu, and `/new [path] [--isolate]`). The server does both
// in one call: POST /v1/admin/sessions with `cwd` and `isolate`.

export type NewChatOptions = { cwd?: string; isolate?: boolean }

export type NewChatArgs = { path: string; isolate: boolean; unknownFlag?: string }

const isolateFlags = new Set(['--isolate', '-i'])

// `/new [path] [--isolate]`: the flag may come first or last, and the rest is
// the path, spaces and all. Quotes around the path are dropped.
export function parseNewChatArgs(args: string): NewChatArgs {
  const words = args.trim().split(/\s+/).filter(Boolean)
  let isolate = false
  const rest: string[] = []
  for (const word of words) {
    if (isolateFlags.has(word)) {
      isolate = true
      continue
    }
    if (word.startsWith('-')) return { path: '', isolate: false, unknownFlag: word }
    rest.push(word)
  }
  let path = rest.join(' ')
  const quoted = path.match(/^(["'])(.*)\1$/)
  if (quoted) path = quoted[2]
  return { path: path.trim(), isolate }
}

type SessionLike = { worktree?: { source_dir: string } | null }
type CwdLike = { current: string; eligible: string[] }

// The project folder a session works in: for an isolated session the
// checkout its worktree came from, otherwise its active cwd. A session still
// in its own artifact folder (always the first eligible cwd) works in none.
export function currentProjectFolder(session: SessionLike | null, cwd: CwdLike | null): string {
  const source = session?.worktree?.source_dir?.trim()
  if (source) return source
  const current = cwd?.current?.trim() ?? ''
  if (!current || current === cwd?.eligible[0]) return ''
  return current
}

// The create request's folder fields; nothing without a folder, since
// isolating needs one.
export function newChatRequest(folder: string, isolate: boolean): NewChatOptions {
  const cwd = folder.trim()
  if (!cwd) return {}
  return isolate ? { cwd, isolate: true } : { cwd }
}
