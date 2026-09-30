// Card labels for the tools a CLI provider (claude-code-cli) runs itself:
// what command ran, which file, what was searched — not the arguments'
// JSON. Pure, tested under Node.
//
// The server sends tool arguments as a preview. Since the fix for
// unreadable labels it keeps that preview parseable JSON with the label
// arguments first; transcripts written before then hold JSON cut
// mid-string, so the fields are also read leniently from a cut preview.

type Args = { parsed: Record<string, unknown> | null; raw: string }

const MAX_TEXT = 80
const MAX_NOTE = 60

// A labeller returns what goes inside the parentheses, or the pair of that
// and a note shown after them; undefined when the label argument is missing.
type Label = string | [string, string]

// Base folders paths are shown relative to, first match wins.
type BaseDirs = string | readonly string[] | undefined

const labellers: Record<string, (args: Args, base: BaseDirs) => Label | undefined> = {
  Bash: (a) => {
    const command = field(a, 'command')
    const description = field(a, 'description')
    if (command === undefined && description === undefined) return undefined
    const main = command === undefined ? '…' : oneLine(command, MAX_TEXT)
    return description ? [main, oneLine(description, MAX_NOTE)] : main
  },
  Read: filePathLabel,
  Edit: filePathLabel,
  Write: filePathLabel,
  MultiEdit: filePathLabel,
  NotebookEdit: (a, base) => pathText(field(a, 'notebook_path') ?? field(a, 'file_path'), base),
  Grep: patternLabel,
  Glob: patternLabel,
  WebFetch: (a) => text(field(a, 'url')),
  WebSearch: (a) => text(field(a, 'query')),
  Task: agentLabel,
  Agent: agentLabel,
}

// cliToolLabel is the card label for a CLI provider's tool, like
// `Read(internal/llm/router.go)`, or null for a tool it does not know —
// those keep the generic preview. Paths inside one of baseDirs, the folders
// the session's turns ran in (see toolBaseDirs), are shown relative to it.
export function cliToolLabel(toolName: string | undefined, rawArgs: string | undefined, baseDirs?: BaseDirs): string | null {
  const name = toolName?.trim() ?? ''
  const labeller = Object.prototype.hasOwnProperty.call(labellers, name) ? labellers[name] : undefined
  if (!labeller) return null
  const args = readArgs(rawArgs)
  const label = labeller(args, baseDirs)
  if (typeof label === 'string') return `${name}(${label})`
  if (label) return `${name}(${label[0]}) · ${label[1]}`
  // No label argument: an empty call, or a preview cut before it.
  return args.parsed || !args.raw ? `${name}()` : `${name}(…)`
}

// toolBaseDirs lists the folders a session's CLI turns ran in: its worktree
// while isolated (#971), and its working folder, where turns before the
// move ran.
export function toolBaseDirs(session: { worktree?: { dir?: string; path?: string } | null } | null | undefined, cwd?: string): string[] {
  const dirs = [session?.worktree?.dir || session?.worktree?.path, cwd]
  return dirs.map((d) => d?.trim() ?? '').filter((d, i, all) => d !== '' && all.indexOf(d) === i)
}

function filePathLabel(a: Args, base: BaseDirs): string | undefined {
  return pathText(field(a, 'file_path'), base)
}

function patternLabel(a: Args, base: BaseDirs): string | undefined {
  const pattern = field(a, 'pattern')
  if (pattern === undefined) return undefined
  const path = field(a, 'path')
  const where = path ? ` in ${pathText(path, base)}` : ''
  return `${oneLine(pattern, MAX_TEXT)}${where}`
}

function agentLabel(a: Args): string | undefined {
  return text(field(a, 'description') ?? field(a, 'prompt'))
}

function text(value: string | undefined): string | undefined {
  return value === undefined ? undefined : oneLine(value, MAX_TEXT)
}

function pathText(path: string | undefined, baseDirs: BaseDirs): string | undefined {
  if (path === undefined) return undefined
  const shown = relativeTo(path.trim(), baseDirs)
  // Keep the end of a long path: the file name is what tells files apart.
  return shown.length > MAX_TEXT ? `…${shown.slice(shown.length - (MAX_TEXT - 1))}` : shown
}

function relativeTo(path: string, baseDirs: BaseDirs): string {
  const bases = typeof baseDirs === 'string' ? [baseDirs] : baseDirs ?? []
  for (const dir of bases) {
    const base = dir.trim().replace(/\/+$/, '')
    if (!base) continue
    if (path === base) return '.'
    if (path.startsWith(`${base}/`)) return path.slice(base.length + 1) || '.'
  }
  return path
}

function oneLine(value: string, max: number): string {
  const normalized = value.replace(/\s+/g, ' ').trim()
  if (normalized.length <= max) return normalized
  return `${normalized.slice(0, max - 1)}…`
}

function readArgs(rawArgs?: string): Args {
  const raw = rawArgs?.trim() ?? ''
  if (!raw) return { parsed: null, raw }
  try {
    const value = JSON.parse(raw) as unknown
    if (value && typeof value === 'object' && !Array.isArray(value)) return { parsed: value as Record<string, unknown>, raw }
  } catch {
    // A cut preview: fields are read from the text below.
  }
  return { parsed: null, raw }
}

// field returns a string argument. From a cut preview it reads the value
// up to its closing quote, or up to the cut — then marked with "…".
function field(args: Args, key: string): string | undefined {
  if (args.parsed) {
    const value = args.parsed[key]
    return typeof value === 'string' && value.trim() ? value : undefined
  }
  const match = new RegExp(`"${key}"\\s*:\\s*"`).exec(args.raw)
  if (!match) return undefined
  let body = ''
  let closed = false
  for (let i = match.index + match[0].length; i < args.raw.length; i++) {
    const ch = args.raw[i]
    if (ch === '\\') {
      body += args.raw.slice(i, i + 2)
      i++
      continue
    }
    if (ch === '"') {
      closed = true
      break
    }
    body += ch
  }
  if (!closed) body = body.replace(/(\.\.\.|…)$/, '')
  const value = decodeJSONString(body)
  if (!value.trim()) return undefined
  return closed ? value : `${value.trimEnd()}…`
}

function decodeJSONString(body: string): string {
  // A cut can leave half an escape (`\`, `\u00`) at the end; drop it.
  for (let trim = 0; trim <= 6 && trim <= body.length; trim++) {
    try {
      return JSON.parse(`"${body.slice(0, body.length - trim)}"`) as string
    } catch {
      // try a shorter tail
    }
  }
  return body.replace(/\\(.)/g, '$1')
}
