import type {
  RemoteControlEvent,
  RemotePlacementState,
  RemoteWorkerState,
} from './types.ts'

export function workerStateBadge(state: RemoteWorkerState): string {
  switch (state) {
    case 'ready':
    case 'leased':
    case 'executing':
      return 'badge-success'
    case 'registered':
    case 'draining':
      return 'badge-warning'
    case 'disconnected':
    case 'lost':
      return 'badge-error'
    default:
      return 'badge-default'
  }
}

export function placementStateBadge(state: RemotePlacementState): string {
  switch (state) {
    case 'completed':
    case 'ready':
      return 'badge-success'
    case 'failed':
    case 'lost':
      return 'badge-error'
    case 'reclaiming':
    case 'rehydrating':
    case 'checkpointed':
      return 'badge-warning'
    case 'executing':
    case 'collecting':
      return 'badge-info'
    default:
      return 'badge-default'
  }
}

export function controlEventPresentation(event: RemoteControlEvent): { title: string; detail: string } {
  const transition = event.from_state && event.to_state
    ? `${event.from_state} → ${event.to_state}`
    : event.to_state || event.type
  const detail = [event.type, event.placement_id, event.worker_id].filter(Boolean).join(' · ')
  return { title: transition, detail }
}
