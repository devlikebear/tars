import { APIRequestError, requestJSON } from './client.ts'
import type { EasyPreview, EasyProject, EasyReadiness, EasyVersion } from '../easy.ts'

// Easy mode's first run (internal/apihandlers/setup_easy.go): Claude Code
// on every tier, then a restart. A server already set up (409) is fine:
// there is nothing left to do but wait for it.
export async function setUpEasy(): Promise<void> {
  try {
    await requestJSON('/v1/admin/setup/easy', { method: 'POST' })
  } catch (err) {
    if (err instanceof APIRequestError && err.status === 409) return
    throw err
  }
}

// What this computer still needs before a project can be made
// (internal/tarsserver/easy_readiness.go).
export async function getEasyReadiness(): Promise<EasyReadiness> {
  return (await requestJSON<{ readiness: EasyReadiness }>('/v1/easy/readiness')).readiness
}

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

// "Change it like this" (internal/tarsserver/easy_change.go): opens the next
// run in the project's folder and answers the project pointing at it.
export async function changeEasyProject(id: string, request: string): Promise<EasyProject> {
  const res = await requestJSON<{ project: EasyProject }>(`/v1/easy/projects/${encodeURIComponent(id)}/changes`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ request }),
  })
  return res.project
}

// A project's saved versions, newest first (internal/tarsserver/easy_versions.go).
export async function listEasyVersions(id: string): Promise<EasyVersion[]> {
  const res = await requestJSON<{ versions: EasyVersion[] }>(`/v1/easy/projects/${encodeURIComponent(id)}/versions`)
  return res.versions ?? []
}

// Going back saves the old version's files as a new version and answers the
// list after it.
export async function restoreEasyVersion(id: string, commit: string): Promise<EasyVersion[]> {
  const res = await requestJSON<{ versions: EasyVersion[] }>(
    `/v1/easy/projects/${encodeURIComponent(id)}/versions/${encodeURIComponent(commit)}/restore`,
    { method: 'POST' },
  )
  return res.versions ?? []
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
