const consoleBase = '/console'
const projectPrefix = `${consoleBase}/projects/`

export function currentProjectIdFromPath(pathname: string): string | null {
  const path = pathname.trim()
  if (!path.startsWith(projectPrefix)) {
    return null
  }
  const value = path.slice(projectPrefix.length).split('/')[0]?.trim()
  return value ? decodeURIComponent(value) : null
}

export function projectPath(projectId: string): string {
  return `${projectPrefix}${encodeURIComponent(projectId.trim())}`
}

export function isConsoleRoot(pathname: string): boolean {
  const path = pathname.trim()
  return path === consoleBase || path === `${consoleBase}/`
}
