import { requestJSON } from './client.ts'
import type {
  ExtensionHealthResponse,
  ExtensionRepairResponse,
  HubDryRunResult,
  HubInstallResponse,
  HubInstalled,
  HubRegistry,
  HubSkillSearchResult,
  HubSource,
  MCPServerCreatorDraftRequest,
  MCPServerCreatorDraftResponse,
  MCPServerCreatorSaveResponse,
  MCPServerCreatorSubmitResponse,
  MCPServerCreatorTestResponse,
  MCPServerStatus,
  SkillCreatorDraftRequest,
  SkillCreatorDraftResponse,
  SkillCreatorSaveResponse,
  SkillCreatorTestResponse,
  SkillCreatorSubmitResponse,
  SkillDef,
  SkillExtractionCandidateAction,
  SkillExtractionCandidateStatus,
  SkillExtractionListResponse,
  SkillExtractionReviewResponse,
} from '../types'

// --- Hub / Extensions ---

export async function getHubRegistry(): Promise<HubRegistry> {
  return requestJSON<HubRegistry>('/v1/hub/registry')
}

export async function getHubInstalled(): Promise<HubInstalled> {
  return requestJSON<HubInstalled>('/v1/hub/installed')
}

export async function hubInstall(
  type: string,
  name: string,
  opts: { source?: string; yes?: boolean; dry_run?: boolean } = {},
): Promise<HubInstallResponse> {
  return requestJSON<HubInstallResponse>('/v1/hub/install', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      type,
      name,
      source: opts.source,
      yes: opts.yes,
      dry_run: opts.dry_run,
    }),
  })
}

export async function getHubSources(): Promise<HubSource[]> {
  const data = await requestJSON<{ sources: HubSource[] }>('/v1/hub/sources')
  return data.sources ?? []
}

export async function searchHubSkills(
  opts: { q?: string; source?: string } = {},
): Promise<HubSkillSearchResult[]> {
  const params = new URLSearchParams()
  if (opts.q?.trim()) params.set('q', opts.q.trim())
  if (opts.source?.trim()) params.set('source', opts.source.trim())
  const qs = params.toString()
  const data = await requestJSON<{ skills: HubSkillSearchResult[] }>(
    `/v1/hub/skills${qs ? `?${qs}` : ''}`,
  )
  return data.skills ?? []
}

export async function previewHubInstall(
  name: string,
  source: string,
): Promise<HubDryRunResult | null> {
  const resp = await hubInstall('skill', name, { source, dry_run: true })
  return resp.preview ?? null
}

export async function hubUninstall(type: string, name: string): Promise<void> {
  await requestJSON<{ ok: string }>('/v1/hub/uninstall', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ type, name }),
  })
}

export async function hubUpdate(): Promise<{ updated_skills: string[]; updated_plugins: string[] }> {
  return requestJSON<{ updated_skills: string[]; updated_plugins: string[] }>('/v1/hub/update', { method: 'POST' })
}

export async function listSkills(sessionId?: string): Promise<SkillDef[]> {
  const params = new URLSearchParams()
  if (sessionId?.trim()) params.set('session_id', sessionId.trim())
  const qs = params.toString()
  return requestJSON<SkillDef[]>(`/v1/skills${qs ? `?${qs}` : ''}`)
}

export async function listMCPServers(): Promise<MCPServerStatus[]> {
  return requestJSON<MCPServerStatus[]>('/v1/mcp/servers')
}

export async function getExtensionsHealth(): Promise<ExtensionHealthResponse> {
  return requestJSON<ExtensionHealthResponse>('/v1/runtime/extensions/health')
}

export async function repairExtension(kind: string, name: string): Promise<ExtensionRepairResponse> {
  return requestJSON<ExtensionRepairResponse>('/v1/runtime/extensions/repair', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ kind, name }),
  })
}

export async function getDisabledExtensions(): Promise<{ skills: string[]; plugins: string[]; mcp_servers: string[] }> {
  return requestJSON('/v1/runtime/extensions/disabled')
}

export async function setExtensionDisabled(kind: string, name: string, disabled: boolean): Promise<void> {
  await requestJSON<{ ok: boolean }>('/v1/runtime/extensions/disabled', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ kind, name, disabled }),
  })
}

export async function getSkillDetail(name: string): Promise<SkillDef & { content?: string }> {
  return requestJSON<SkillDef & { content?: string }>(`/v1/skills/${encodeURIComponent(name)}`)
}

export async function getHubSkillContent(name: string): Promise<{ name: string; version: string; content: string }> {
  return requestJSON<{ name: string; version: string; content: string }>(`/v1/hub/skill-content?name=${encodeURIComponent(name)}`)
}

export async function reloadExtensions(): Promise<{ reloaded: boolean; skills: number; plugins: number; mcp_count: number }> {
  return requestJSON<{ reloaded: boolean; skills: number; plugins: number; mcp_count: number }>('/v1/runtime/extensions/reload', { method: 'POST' })
}

// --- Skill creator ---

export async function draftSkill(payload: SkillCreatorDraftRequest): Promise<SkillCreatorDraftResponse> {
  return requestJSON<SkillCreatorDraftResponse>('/v1/admin/skills/draft', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
}

export async function saveLocalSkill(draft: SkillCreatorDraftResponse): Promise<SkillCreatorSaveResponse> {
  return requestJSON<SkillCreatorSaveResponse>('/v1/admin/skills/save-local', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(draft),
  })
}

export async function getWorkspaceSkillContent(name: string): Promise<{ name: string; content: string; path: string }> {
  return requestJSON<{ name: string; content: string; path: string }>(`/v1/admin/skills/${encodeURIComponent(name)}`)
}

export async function updateWorkspaceSkill(name: string, content: string): Promise<{ name: string; path: string }> {
  return requestJSON<{ name: string; path: string }>(`/v1/admin/skills/${encodeURIComponent(name)}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ content }),
  })
}

export async function deleteWorkspaceSkill(name: string): Promise<{ ok: boolean }> {
  return requestJSON<{ ok: boolean }>(`/v1/admin/skills/${encodeURIComponent(name)}`, {
    method: 'DELETE',
  })
}

export async function testSkillDraft(draft: SkillCreatorDraftResponse): Promise<SkillCreatorTestResponse> {
  return requestJSON<SkillCreatorTestResponse>('/v1/admin/skills/test', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(draft),
  })
}

export async function submitSkillDraftPR(name: string): Promise<SkillCreatorSubmitResponse> {
  return requestJSON<SkillCreatorSubmitResponse>('/v1/admin/skills/submit-pr', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  })
}

// --- Skill extraction inbox ---

export async function listSkillExtractions(status: SkillExtractionCandidateStatus | 'all' = 'pending'): Promise<SkillExtractionListResponse> {
  const qs = new URLSearchParams()
  if (status) qs.set('status', status)
  return requestJSON<SkillExtractionListResponse>(`/v1/admin/skills/extractions?${qs}`)
}

export async function extractSkillsFromSession(sessionId: string, maxCandidates = 5): Promise<SkillExtractionListResponse> {
  return requestJSON<SkillExtractionListResponse>('/v1/admin/skills/extractions/extract', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ session_id: sessionId, max_candidates: maxCandidates }),
  })
}

export async function reviewSkillExtractionCandidate(id: string, action: SkillExtractionCandidateAction): Promise<SkillExtractionReviewResponse> {
  return requestJSON<SkillExtractionReviewResponse>('/v1/admin/skills/extractions/review', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ id, action }),
  })
}

// --- MCP server creator ---

export async function draftMCPServer(payload: MCPServerCreatorDraftRequest): Promise<MCPServerCreatorDraftResponse> {
  return requestJSON<MCPServerCreatorDraftResponse>('/v1/admin/mcp-servers/draft', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
}

export async function saveLocalMCPServer(draft: MCPServerCreatorDraftResponse): Promise<MCPServerCreatorSaveResponse> {
  return requestJSON<MCPServerCreatorSaveResponse>('/v1/admin/mcp-servers/save-local', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(draft),
  })
}

export async function testMCPServerDraft(draft: MCPServerCreatorDraftResponse): Promise<MCPServerCreatorTestResponse> {
  return requestJSON<MCPServerCreatorTestResponse>('/v1/admin/mcp-servers/test', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(draft),
  })
}

export async function submitMCPServerDraftPR(name: string): Promise<MCPServerCreatorSubmitResponse> {
  return requestJSON<MCPServerCreatorSubmitResponse>('/v1/admin/mcp-servers/submit-pr', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  })
}
