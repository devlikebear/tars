import { requestJSON } from './client.ts'
import type {
  ConfigFile,
  ConfigSchema,
  ProviderModelsInfo,
  ProvidersAPIInfo,
  RemoteAccessResponse,
} from '../types'

export async function getConfig(): Promise<ConfigFile> {
  return requestJSON<ConfigFile>('/v1/admin/config')
}

export async function getConfigSchema(): Promise<ConfigSchema> {
  return requestJSON<ConfigSchema>('/v1/admin/config/schema')
}

export async function getProviderModels(providerAlias = ''): Promise<ProviderModelsInfo> {
  const params = new URLSearchParams()
  if (providerAlias.trim()) {
    params.set('provider_alias', providerAlias.trim())
  }
  const suffix = params.toString()
  return requestJSON<ProviderModelsInfo>(`/v1/models${suffix ? `?${suffix}` : ''}`)
}

export async function getProviders(): Promise<ProvidersAPIInfo> {
  return requestJSON<ProvidersAPIInfo>('/v1/providers')
}

export async function saveConfig(content: string): Promise<void> {
  await requestJSON<{ ok: string }>('/v1/admin/config', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ content }),
  })
}

export async function restartServer(): Promise<{ ok: string; mode: string; info: string }> {
  return requestJSON<{ ok: string; mode: string; info: string }>('/v1/admin/restart', { method: 'POST' })
}

export type WorkspaceResetResponse = {
  removed: number
  removed_items: string[]
  failed_items?: { name?: string; path?: string; stage?: string; error: string }[]
  reinitialized: boolean
  error?: string
}

export async function resetWorkspace(): Promise<WorkspaceResetResponse> {
  return requestJSON<WorkspaceResetResponse>('/v1/admin/reset/workspace', { method: 'POST' })
}

export async function patchConfigValues(updates: Record<string, unknown>): Promise<void> {
  await requestJSON<{ ok: string }>('/v1/admin/config/values', {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ updates }),
  })
}

export async function getRemoteAccessStatus(): Promise<RemoteAccessResponse> {
  return requestJSON<RemoteAccessResponse>('/v1/admin/remote-access/status')
}

export async function enableRemoteAccess(httpsPort?: number): Promise<RemoteAccessResponse> {
  return requestJSON<RemoteAccessResponse>('/v1/admin/remote-access/enable', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(httpsPort ? { https_port: httpsPort } : {}),
  })
}

export async function disableRemoteAccess(httpsPort?: number): Promise<RemoteAccessResponse> {
  return requestJSON<RemoteAccessResponse>('/v1/admin/remote-access/disable', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(httpsPort ? { https_port: httpsPort } : {}),
  })
}
