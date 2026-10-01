const consoleBase = '/console'
const chatPrefix = `${consoleBase}/chat`
const focusPrefix = `${consoleBase}/focus`

export type Route =
  | { view: 'board' }
  | { view: 'home' }
  | { view: 'chat'; sessionId?: string }
  // Focus mode: the focus home, one session's pipeline, or the release train.
  | { view: 'focus'; sessionId?: string; release?: boolean }
  | { view: 'session-lineage' }
  | { view: 'tasks' }
  | { view: 'agentruntime'; runId?: string; tab?: 'runs' | 'subagents' }
  | { view: 'memory' }
  | { view: 'sysprompt' }
  | { view: 'ops' }
  | { view: 'cron' }
  | { view: 'logs' }
  | { view: 'analytics' }
  | { view: 'config' }
  | { view: 'extensions' }
  | { view: 'pulse' }
  | { view: 'reflection' }
  | { view: 'channels' }
  | { view: 'onboarding'; reentry?: boolean }

export function resolveRoute(pathname: string): Route {
  let path = pathname.trim()
  let sessionQuery = ''
  let reentryQuery = false
  try {
    const url = new URL(path, 'https://tars.local')
    path = url.pathname
    sessionQuery = url.searchParams.get('session')?.trim() ?? ''
    const reentryRaw = url.searchParams.get('reentry')?.trim().toLowerCase() ?? ''
    reentryQuery = reentryRaw === '1' || reentryRaw === 'true' || reentryRaw === 'yes'
  } catch {
    path = pathname.trim()
  }

  // The session board is the console's home (#971); the system overview
  // that used to be home lives under /console/system.
  if (path === consoleBase || path === `${consoleBase}/`) {
    return { view: 'board' }
  }

  if (path.startsWith(`${consoleBase}/system`)) {
    return { view: 'home' }
  }

  if (path.startsWith(chatPrefix)) {
    const rest = path.slice(chatPrefix.length)
    if (rest.startsWith('/') && rest.length > 1) {
      const sessionId = decodeURIComponent(rest.slice(1).split('/')[0]?.trim() || '')
      if (sessionId) return { view: 'chat', sessionId }
    }
    if (sessionQuery) return { view: 'chat', sessionId: sessionQuery }
    return { view: 'chat' }
  }

  if (path === focusPrefix || path.startsWith(`${focusPrefix}/`)) {
    const sessionId = decodeURIComponent(path.slice(focusPrefix.length + 1).split('/')[0]?.trim() || '')
    // Session ids never read "release", so it names the release train.
    if (sessionId === 'release') return { view: 'focus', release: true }
    return sessionId ? { view: 'focus', sessionId } : { view: 'focus' }
  }

  if (path.startsWith(`${consoleBase}/tasks`)) {
    return { view: 'tasks' }
  }

  if (path.startsWith(`${consoleBase}/agentruntime/runs/`)) {
    const runId = decodeURIComponent(path.slice(`${consoleBase}/agentruntime/runs/`.length).split('/')[0]?.trim() || '')
    if (runId) return { view: 'agentruntime', runId }
  }

  if (path.startsWith(`${consoleBase}/agentruntime/subagents`)) {
    return { view: 'agentruntime', tab: 'subagents' }
  }

  if (path.startsWith(`${consoleBase}/sessions/graph`)) {
    return { view: 'session-lineage' }
  }

  if (path.startsWith(`${consoleBase}/sessions`)) {
    return { view: 'chat' }
  }

  if (path.startsWith(`${consoleBase}/approvals`) || path.startsWith(`${consoleBase}/ops`)) {
    return { view: 'ops' }
  }

  if (path.startsWith(`${consoleBase}/cron`)) {
    return { view: 'cron' }
  }

  if (path.startsWith(`${consoleBase}/logs`)) {
    return { view: 'logs' }
  }

  if (path.startsWith(`${consoleBase}/analytics`)) {
    return { view: 'analytics' }
  }

  if (path.startsWith(`${consoleBase}/agentruntime`)) {
    return { view: 'agentruntime', tab: 'runs' }
  }

  if (path.startsWith(`${consoleBase}/memory`)) {
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
    return { view: 'pulse' }
  }

  if (path.startsWith(`${consoleBase}/reflection`)) {
    return { view: 'reflection' }
  }

  if (path.startsWith(`${consoleBase}/channels`)) {
    return { view: 'channels' }
  }

  if (path.startsWith(`${consoleBase}/onboarding`)) {
    return reentryQuery ? { view: 'onboarding', reentry: true } : { view: 'onboarding' }
  }

  return { view: 'board' }
}
