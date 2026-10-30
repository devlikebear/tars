import type { LocalEmbedState } from './types'

export function formatBytes(bytes: number): string {
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let value = bytes
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit++
  }
  return unit === 0 ? `${value} ${units[0]}` : `${value.toFixed(1)} ${units[unit]}`
}

export function jobRunning(state: LocalEmbedState | null): boolean {
  return Boolean(state?.job?.running)
}

// On and installed but not answering yet: the model takes seconds to load
// after a restart.
export function awaitingServer(state: LocalEmbedState | null): boolean {
  if (!state || jobRunning(state) || state.restart_required) return false
  return state.enabled && state.status.model_installed && !state.status.running && !state.status.running_model
}

// A server answering on the port with another model is not ours to manage.
export function foreignServer(state: LocalEmbedState | null): string {
  const status = state?.status
  return status && !status.running && status.running_model ? status.running_model : ''
}

export function canInstall(state: LocalEmbedState | null): boolean {
  if (!state || jobRunning(state)) return false
  return state.status.env_ready || Boolean(state.status.uv_path)
}

export function canReindex(state: LocalEmbedState | null): boolean {
  if (!state || jobRunning(state)) return false
  return state.reindex_pending > 0 && state.status.running && !state.restart_required
}

export function jobPercent(state: LocalEmbedState | null): number | null {
  const job = state?.job
  if (!job?.total) return null
  return Math.round(((job.done ?? 0) / job.total) * 100)
}
