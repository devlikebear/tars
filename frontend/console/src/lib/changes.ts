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
