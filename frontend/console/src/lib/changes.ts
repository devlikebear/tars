// Where turn change cards go in the chat thread (#969).

type ThreadMessage = {
  id: string
  role: string
  sourceMessageId?: string
}

// Maps the last message of each turn to that turn's ID. A turn starts at a
// user message the server knows (its ID is the turn ID) and runs until the
// next user message, so its card sits under the turn's final answer, tool
// call, or error.
export function turnCardAnchors(messages: ThreadMessage[]): Map<string, string> {
  const anchors = new Map<string, string>()
  let turnId = ''
  let lastId = ''
  const close = () => {
    if (turnId && lastId) anchors.set(lastId, turnId)
  }
  for (const message of messages) {
    if (message.role === 'user') {
      close()
      turnId = message.sourceMessageId?.trim() ?? ''
    }
    lastId = message.id
  }
  close()
  return anchors
}

const notesOpen = '<review-notes>'
const notesClose = '</review-notes>'

// Splits the server's review-notes block off a stored user message, so the
// thread can fold it. count is the number of notes in it.
export function splitReviewNotes(text: string): { text: string; notes: string; count: number } {
  const start = text.lastIndexOf(`\n\n${notesOpen}`)
  if (start < 0 || !text.trimEnd().endsWith(notesClose)) return { text, notes: '', count: 0 }
  const notes = text.slice(start + 2).trimEnd()
  const count = (notes.match(/^\d+\. /gm) ?? []).length
  return { text: text.slice(0, start), notes, count }
}

// The Changes panel lists a diff's files as a folder tree (#969): folders
// first, then files, each by name; a folder holding only one folder is
// shown as one row ("src/lib"). A folder row sums its files' counts.
export type FileTreeRow<F> =
  | { kind: 'dir'; path: string; name: string; depth: number; files: number; additions: number; deletions: number }
  | { kind: 'file'; path: string; name: string; depth: number; file: F }

type TreeFile = { path: string; additions: number; deletions: number }

type TreeNode<F> = { name: string; path: string; dirs: Map<string, TreeNode<F>>; files: F[] }

export function fileTreeRows<F extends TreeFile>(files: F[], collapsed: ReadonlySet<string> = new Set()): FileTreeRow<F>[] {
  const root: TreeNode<F> = { name: '', path: '', dirs: new Map(), files: [] }
  for (const file of files) {
    const parts = file.path.split('/').filter(Boolean)
    let node = root
    for (const part of parts.slice(0, -1)) {
      const path = node.path ? `${node.path}/${part}` : part
      let child = node.dirs.get(part)
      if (!child) {
        child = { name: part, path, dirs: new Map(), files: [] }
        node.dirs.set(part, child)
      }
      node = child
    }
    node.files.push(file)
  }
  const rows: FileTreeRow<F>[] = []
  const walk = (node: TreeNode<F>, depth: number) => {
    for (const dir of [...node.dirs.values()].sort((a, b) => a.name.localeCompare(b.name))) {
      let shown = dir
      let name = dir.name
      while (shown.files.length === 0 && shown.dirs.size === 1) {
        shown = [...shown.dirs.values()][0]
        name = `${name}/${shown.name}`
      }
      const totals = sumTree(shown)
      rows.push({ kind: 'dir', path: shown.path, name, depth, ...totals })
      if (!collapsed.has(shown.path)) walk(shown, depth + 1)
    }
    for (const file of [...node.files].sort((a, b) => a.path.localeCompare(b.path))) {
      rows.push({ kind: 'file', path: file.path, name: file.path.split('/').pop() ?? file.path, depth, file })
    }
  }
  walk(root, 0)
  return rows
}

function sumTree<F extends TreeFile>(node: TreeNode<F>): { files: number; additions: number; deletions: number } {
  let files = node.files.length
  let additions = node.files.reduce((sum, f) => sum + f.additions, 0)
  let deletions = node.files.reduce((sum, f) => sum + f.deletions, 0)
  for (const child of node.dirs.values()) {
    const sub = sumTree(child)
    files += sub.files
    additions += sub.additions
    deletions += sub.deletions
  }
  return { files, additions, deletions }
}
