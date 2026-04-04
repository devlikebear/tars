const consoleBase = '/console'
const projectPrefix = `${consoleBase}/projects/`
const chatPrefix = `${consoleBase}/chat`

export type Route =
  | { view: 'chat'; sessionId?: string }
  | { view: 'projects' }
  | { view: 'project'; projectId: string }
  | { view: 'knowledge' }
  | { view: 'ops' }
  | { view: 'config' }
  | { view: 'extensions' }
  | { view: 'heartbeat' }

export function resolveRoute(pathname: string): Route {
  const path = pathname.trim()

  if (path.startsWith(projectPrefix)) {
    const projectId = path.slice(projectPrefix.length).split('/')[0]?.trim()
    if (projectId) {
      return { view: 'project', projectId: decodeURIComponent(projectId) }
    }
    return { view: 'projects' }
  }

  if (path === `${consoleBase}/projects` || path === `${consoleBase}/projects/`) {
    return { view: 'projects' }
  }

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

  if (path.startsWith(`${consoleBase}/knowledge`)) {
    return { view: 'knowledge' }
  }

  if (path.startsWith(`${consoleBase}/config`)) {
    return { view: 'config' }
  }

  if (path.startsWith(`${consoleBase}/extensions`)) {
    return { view: 'extensions' }
  }

  if (path.startsWith(`${consoleBase}/heartbeat`)) {
    return { view: 'heartbeat' }
  }

  // Default: /console → chat
  return { view: 'chat' }
}

export function chatPath(sessionId?: string): string {
  if (sessionId) return `${chatPrefix}/${encodeURIComponent(sessionId)}`
  return chatPrefix
}

export function currentProjectIdFromPath(pathname: string): string | null {
  const route = resolveRoute(pathname)
  return route.view === 'project' ? route.projectId : null
}

export function projectPath(projectId: string): string {
  return `${projectPrefix}${encodeURIComponent(projectId.trim())}`
}

export function isConsoleRoot(pathname: string): boolean {
  const path = pathname.trim()
  return path === consoleBase || path === `${consoleBase}/`
}
