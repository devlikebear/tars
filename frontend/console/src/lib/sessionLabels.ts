// Compact labels for the chat session header chips and slash feedback.

export function shortGoalLabel(description: string): string {
  const trimmed = description.trim()
  if (trimmed.length <= 40) return trimmed
  return trimmed.slice(0, 37).trimEnd() + '…'
}

export function shortCwdLabel(path: string): string {
  if (!path) return ''
  const home = '/Users/'
  if (path.startsWith(home)) {
    const trimmed = path.slice(home.length)
    const slash = trimmed.indexOf('/')
    const tail = slash >= 0 ? trimmed.slice(slash) : ''
    return `~${tail}`
  }
  return path
}

// The title the server (and this console) stores for a session nobody has
// named yet. It stays English on the wire; show it in the console's language.
export const untitledSessionTitle = 'New Chat'

export function displaySessionTitle(title: string | undefined, newChatLabel: string): string {
  const trimmed = title?.trim() ?? ''
  return trimmed === untitledSessionTitle ? newChatLabel : trimmed
}
