// Unified diff parsing shared by the Git Inspector, the Agent Runtime diff
// timeline, and the checkpoint Changes panel.
//
// Hunk bodies are read by their @@ line counts, not by prefix: a deleted
// line whose text starts with "-- " (an SQL comment, say) is "--- …" in the
// patch, and a prefix-based reader would mistake it for a file header.

export type DiffLineKind = 'context' | 'add' | 'del' | 'hunk' | 'meta'

export type DiffLine = {
  kind: DiffLineKind
  oldLine?: number
  newLine?: number
  text: string
  // Index of the hunk this line belongs to, within its file.
  hunk?: number
}

export type DiffPair = { left?: DiffLine; right?: DiffLine }

export type DiffHunk = {
  index: number
  oldStart: number
  oldLines: number
  newStart: number
  newLines: number
  header: string
}

export type DiffFile = {
  path: string
  oldPath?: string
  newPath?: string
  binary: boolean
  hunks: DiffHunk[]
  lines: DiffLine[]
}

const hunkHeader = /^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@/

function stripPrefix(path: string, prefix: string): string | undefined {
  const value = path.trim()
  if (value === '/dev/null') return undefined
  return value.startsWith(prefix) ? value.slice(prefix.length) : value
}

// "diff --git a/x b/y" names both sides; with unquoted paths the split is
// ambiguous when a path contains " b/", so the ---/+++ lines win when present.
function pathsFromGitHeader(line: string): { oldPath?: string; newPath?: string } {
  const rest = line.slice('diff --git '.length)
  const at = rest.lastIndexOf(' b/')
  if (!rest.startsWith('a/') || at < 0) return {}
  return { oldPath: rest.slice(2, at), newPath: rest.slice(at + 3) }
}

function finishFile(file: DiffFile) {
  file.path = file.newPath ?? file.oldPath ?? file.path
}

/** Splits a (possibly multi-file) unified diff into files, hunks, and lines. */
export function parsePatch(patch?: string): DiffFile[] {
  const files: DiffFile[] = []
  if (!patch) return files
  let file: DiffFile | undefined
  let oldLine = 0
  let newLine = 0
  let oldLeft = 0
  let newLeft = 0

  const ensureFile = (): DiffFile => {
    if (!file) {
      file = { path: '', binary: false, hunks: [], lines: [] }
      files.push(file)
    }
    return file
  }

  const rows = patch.split('\n')
  if (rows[rows.length - 1] === '') rows.pop()
  for (const raw of rows) {
    const inHunk = oldLeft > 0 || newLeft > 0
    if (inHunk) {
      const current = ensureFile()
      const hunk = current.hunks.length - 1
      if (raw.startsWith('\\')) {
        current.lines.push({ kind: 'meta', text: raw, hunk })
      } else if (raw.startsWith('+')) {
        current.lines.push({ kind: 'add', newLine, text: raw.slice(1), hunk })
        newLine++
        newLeft--
      } else if (raw.startsWith('-')) {
        current.lines.push({ kind: 'del', oldLine, text: raw.slice(1), hunk })
        oldLine++
        oldLeft--
      } else {
        // A context line; tools that strip trailing whitespace leave "".
        current.lines.push({ kind: 'context', oldLine, newLine, text: raw.slice(1), hunk })
        oldLine++
        newLine++
        oldLeft--
        newLeft--
      }
      continue
    }
    if (raw.startsWith('\\')) {
      // "\ No newline at end of file" after a hunk's last line.
      const current = ensureFile()
      current.lines.push({ kind: 'meta', text: raw, hunk: current.hunks.length - 1 })
      continue
    }
    if (raw.startsWith('diff --git ')) {
      if (file) finishFile(file)
      file = { path: '', binary: false, hunks: [], lines: [], ...pathsFromGitHeader(raw) }
      files.push(file)
      continue
    }
    const match = hunkHeader.exec(raw)
    if (match) {
      const current = ensureFile()
      const hunk: DiffHunk = {
        index: current.hunks.length,
        oldStart: Number(match[1]),
        oldLines: match[2] === undefined ? 1 : Number(match[2]),
        newStart: Number(match[3]),
        newLines: match[4] === undefined ? 1 : Number(match[4]),
        header: raw,
      }
      current.hunks.push(hunk)
      current.lines.push({ kind: 'hunk', text: raw, hunk: hunk.index })
      oldLine = hunk.oldStart
      newLine = hunk.newStart
      oldLeft = hunk.oldLines
      newLeft = hunk.newLines
      continue
    }
    const current = ensureFile()
    if (raw.startsWith('--- ')) {
      current.oldPath = stripPrefix(raw.slice(4), 'a/')
    } else if (raw.startsWith('+++ ')) {
      current.newPath = stripPrefix(raw.slice(4), 'b/')
    } else if (raw.startsWith('rename from ')) {
      current.oldPath = raw.slice('rename from '.length)
    } else if (raw.startsWith('rename to ')) {
      current.newPath = raw.slice('rename to '.length)
    } else if (raw.startsWith('Binary files ') || raw === 'GIT binary patch') {
      current.binary = true
    }
    // Other header lines (index, modes, similarity) are not shown.
  }
  if (file) finishFile(file)
  return files
}

/** Every file's lines in order, for views that show one flat diff. */
export function parseUnifiedDiff(patch?: string): DiffLine[] {
  return parsePatch(patch).flatMap((f) => f.lines)
}

/** Lines of a unified diff paired for a side-by-side view. */
export function pairDiffLines(lines: DiffLine[]): DiffPair[] {
  const pairs: DiffPair[] = []
  let i = 0
  while (i < lines.length) {
    const line = lines[i]
    if (line.kind === 'hunk' || line.kind === 'meta' || line.kind === 'context') {
      pairs.push({ left: line, right: line })
      i++
      continue
    }
    const dels: DiffLine[] = []
    const adds: DiffLine[] = []
    while (i < lines.length && lines[i].kind === 'del') {
      dels.push(lines[i])
      i++
    }
    while (i < lines.length && lines[i].kind === 'add') {
      adds.push(lines[i])
      i++
    }
    const max = Math.max(dels.length, adds.length)
    for (let k = 0; k < max; k++) {
      pairs.push({ left: dels[k], right: adds[k] })
    }
  }
  return pairs
}
