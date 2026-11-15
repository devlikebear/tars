// Paired phones (docs/decisions/mobile-remote.md §4): the admin side of
// pairing, the device list and the folders a phone may start a focus
// pipeline in. Every route is under /v1/admin/devices.
import { requestJSON } from './client.ts'

export type PairedDevice = {
  id: string
  name: string
  role: string
  active: boolean
  created_at: string
  revoked_at?: string
}

export type DevicePairingCode = { code: string; role: string; expires_at: string }

export type DeviceFolder = { path: string; name: string; exists: boolean }

export async function listPairedDevices(): Promise<PairedDevice[]> {
  const res = await requestJSON<{ devices: PairedDevice[] }>('/v1/admin/devices')
  return res.devices ?? []
}

export async function createDevicePairingCode(): Promise<DevicePairingCode> {
  return requestJSON<DevicePairingCode>('/v1/admin/devices/pairing-code', { method: 'POST' })
}

export async function revokePairedDevice(id: string): Promise<void> {
  await requestJSON<{ ok: boolean }>(`/v1/admin/devices/${encodeURIComponent(id)}`, { method: 'DELETE' })
}

export async function getDeviceFolders(): Promise<DeviceFolder[]> {
  const res = await requestJSON<{ folders: DeviceFolder[] }>('/v1/admin/devices/folders')
  return res.folders ?? []
}

export async function setDeviceFolders(folders: string[]): Promise<DeviceFolder[]> {
  const res = await requestJSON<{ folders: DeviceFolder[] }>('/v1/admin/devices/folders', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ folders }),
  })
  return res.folders ?? []
}

// Keeping this computer awake for a paired phone: the owner's choice, and
// what holds now. reason says why it is not active while enabled.
export type KeepAwakeStatus = {
  enabled: boolean
  supported: boolean
  active: boolean
  reason?: 'unsupported' | 'no_phone' | 'failed'
}

export function getKeepAwake(): Promise<KeepAwakeStatus> {
  return requestJSON<KeepAwakeStatus>('/v1/admin/devices/keep-awake')
}

export function setKeepAwake(enabled: boolean): Promise<KeepAwakeStatus> {
  return requestJSON<KeepAwakeStatus>('/v1/admin/devices/keep-awake', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ enabled }),
  })
}
