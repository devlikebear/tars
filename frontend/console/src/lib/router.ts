const consoleBase = '/console'
const projectPrefix = `${consoleBase}/projects/`

export type Route =
  | { view: 'home' }
  | { view: 'projects' }
  | { view: 'project'; projectId: string }
  | { view: 'sessions' }
  | { view: 'ops' }
  | { view: 'config' }
  | { view: 'extensions' }

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

  if (path.startsWith(`${consoleBase}/sessions`)) {
    return { view: 'sessions' }
  }

  if (path.startsWith(`${consoleBase}/ops`)) {
    return { view: 'ops' }
  }

  if (path.startsWith(`${consoleBase}/config`)) {
    return { view: 'config' }
  }

  if (path.startsWith(`${consoleBase}/extensions`)) {
    return { view: 'extensions' }
  }

  return { view: 'home' }
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
