export type LazyRouteView =
  | 'chat'
  | 'session-lineage'
  | 'tasks'
  | 'agentruntime'
  | 'memory'
  | 'sysprompt'
  | 'ops'
  | 'cron'
  | 'logs'
  | 'analytics'
  | 'config'
  | 'extensions'
  | 'pulse'
  | 'reflection'
  | 'channels'

type RouteComponentModule = { default: any }
type RouteComponentLoader = () => Promise<RouteComponentModule>

function memoizeRouteLoader(loader: RouteComponentLoader): RouteComponentLoader {
  let promise: Promise<RouteComponentModule> | null = null
  return () => {
    promise ??= loader()
    return promise
  }
}

export const routeComponentLoaders = {
  chat: memoizeRouteLoader(() => import('../components/Chat.svelte')),
  'session-lineage': memoizeRouteLoader(() => import('../components/SessionLineageGraph.svelte')),
  tasks: memoizeRouteLoader(() => import('../components/Plans.svelte')),
  agentruntime: memoizeRouteLoader(() => import('../components/AgentRuntimeRunView.svelte')),
  memory: memoizeRouteLoader(() => import('../components/MemoryCenter.svelte')),
  sysprompt: memoizeRouteLoader(() => import('../components/SyspromptCenter.svelte')),
  ops: memoizeRouteLoader(() => import('../components/Ops.svelte')),
  cron: memoizeRouteLoader(() => import('../components/Cron.svelte')),
  logs: memoizeRouteLoader(() => import('../components/Logs.svelte')),
  analytics: memoizeRouteLoader(() => import('../components/Analytics.svelte')),
  config: memoizeRouteLoader(() => import('../components/Config.svelte')),
  extensions: memoizeRouteLoader(() => import('../components/Extensions.svelte')),
  pulse: memoizeRouteLoader(() => import('../components/Pulse.svelte')),
  reflection: memoizeRouteLoader(() => import('../components/Reflection.svelte')),
  channels: memoizeRouteLoader(() => import('../components/Channels.svelte')),
} satisfies Record<LazyRouteView, RouteComponentLoader>

export function loadRouteComponent(view: LazyRouteView): Promise<RouteComponentModule> {
  return routeComponentLoaders[view]()
}
