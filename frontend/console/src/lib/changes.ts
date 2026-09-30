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
