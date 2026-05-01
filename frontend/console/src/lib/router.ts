const consoleBase = '/console'
const chatPrefix = `${consoleBase}/chat`

export type Route =
  | { view: 'home' }
  | { view: 'chat'; sessionId?: string }
  | { view: 'agentruntime'; runId?: string; tab?: 'runs' | 'subagents' }
  | { view: 'memory' }
  | { view: 'sysprompt' }
  | { view: 'ops' }
  | { view: 'cron' }
  | { view: 'config' }
  | { view: 'extensions' }
  | { view: 'pulse' }
  | { view: 'reflection' }

export function resolveRoute(pathname: string): Route {
  const path = pathname.trim()

  if (path === consoleBase || path === `${consoleBase}/`) {
    return { view: 'home' }
  }

  if (path.startsWith(chatPrefix)) {
    const rest = path.slice(chatPrefix.length)
    if (rest.startsWith('/') && rest.length > 1) {
      const sessionId = decodeURIComponent(rest.slice(1).split('/')[0]?.trim() || '')
      if (sessionId) return { view: 'chat', sessionId }
    }
    return { view: 'chat' }
  }

  if (path.startsWith(`${consoleBase}/agentruntime/runs/`)) {
    const runId = decodeURIComponent(path.slice(`${consoleBase}/agentruntime/runs/`.length).split('/')[0]?.trim() || '')
    if (runId) return { view: 'agentruntime', runId }
  }

  if (path.startsWith(`${consoleBase}/agentruntime/subagents`)) {
    return { view: 'agentruntime', tab: 'subagents' }
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

  return { view: 'home' }
}
