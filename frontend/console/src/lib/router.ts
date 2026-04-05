const consoleBase = '/console'
const chatPrefix = `${consoleBase}/chat`

export type Route =
  | { view: 'chat'; sessionId?: string }
  | { view: 'memory' }
  | { view: 'sysprompt' }
  | { view: 'ops' }
  | { view: 'config' }
  | { view: 'extensions' }
  | { view: 'pulse' }
  | { view: 'reflection' }

export function resolveRoute(pathname: string): Route {
  const path = pathname.trim()

  // /console/chat or /console/chat/:sessionId
  if (path.startsWith(chatPrefix)) {
    const rest = path.slice(chatPrefix.length)
    if (rest.startsWith('/') && rest.length > 1) {
      const sessionId = decodeURIComponent(rest.slice(1).split('/')[0]?.trim() || '')
      if (sessionId) return { view: 'chat', sessionId }
    }
    return { view: 'chat' }
  }

  // Legacy /console/sessions → redirect to chat
  if (path.startsWith(`${consoleBase}/sessions`)) {
    return { view: 'chat' }
  }

  if (path.startsWith(`${consoleBase}/ops`)) {
    return { view: 'ops' }
  }

  if (path.startsWith(`${consoleBase}/memory`) || path.startsWith(`${consoleBase}/knowledge`)) {
    return { view: 'memory' }
  }

  if (path.startsWith(`${consoleBase}/sysprompt`) || path.startsWith(`${consoleBase}/workspace`)) {
    return { view: 'sysprompt' }
  }

  if (path.startsWith(`${consoleBase}/config`)) {
    return { view: 'config' }
  }

  if (path.startsWith(`${consoleBase}/extensions`)) {
    return { view: 'extensions' }
  }

  if (path.startsWith(`${consoleBase}/pulse`) || path.startsWith(`${consoleBase}/heartbeat`)) {
    // legacy /console/heartbeat URLs redirect to the pulse view
    return { view: 'pulse' }
  }

  if (path.startsWith(`${consoleBase}/reflection`)) {
    return { view: 'reflection' }
  }

  // Default: /console → chat
  return { view: 'chat' }
}

export function chatPath(sessionId?: string): string {
  if (sessionId) return `${chatPrefix}/${encodeURIComponent(sessionId)}`
  return chatPrefix
}

export function isConsoleRoot(pathname: string): boolean {
  const path = pathname.trim()
  return path === consoleBase || path === `${consoleBase}/`
}
