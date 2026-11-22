import { requestJSON } from './client.ts'
import type { EasyPreview, EasyProject } from '../easy.ts'

// Easy-mode projects (internal/tarsserver/easy_projects.go).

export function createEasyProject(idea: string): Promise<{ project: EasyProject }> {
  return requestJSON('/v1/easy/projects', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ idea }),
  })
}

export async function listEasyProjects(): Promise<EasyProject[]> {
  const res = await requestJSON<{ projects: EasyProject[] }>('/v1/easy/projects')
  return res.projects ?? []
}

export async function getEasyProject(id: string): Promise<EasyProject> {
  const res = await requestJSON<{ project: EasyProject }>(`/v1/easy/projects/${encodeURIComponent(id)}`)
  return res.project
}

// A project's preview (internal/tarsserver/easy_preview.go): reading it keeps
// it running, starting it is idempotent.
function previewPath(id: string): string {
  return `/v1/easy/projects/${encodeURIComponent(id)}/preview`
}

export async function getEasyPreview(id: string): Promise<EasyPreview> {
  return (await requestJSON<{ preview: EasyPreview }>(previewPath(id))).preview
}

export async function startEasyPreview(id: string): Promise<EasyPreview> {
  return (await requestJSON<{ preview: EasyPreview }>(previewPath(id), { method: 'POST' })).preview
}
