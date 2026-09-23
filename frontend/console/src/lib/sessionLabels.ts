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
