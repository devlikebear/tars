import { requestJSON } from './client.ts'
import type {
  AgentRuntimeRecoveryMode,
  AgentRuntimeProviderOverride,
  AgentRuntimeRun,
  AgentRuntimeRunEvent,
  AgentRuntimeSubagent,
  AgentRuntimeSubagentArchiveResponse,
  AgentRuntimeSubagentDraft,
  AgentRuntimeSubagentDraftResponse,
  AgentRuntimeSubagentRecommendationsResponse,
  AgentRuntimeSubagentsResponse,
} from '../types'

export type AgentRuntimeRunsOptions = {
  limit?: number
  status?: string
  since?: string
  search?: string
}

export async function listAgentRuntimeRuns(options: number | AgentRuntimeRunsOptions = 30): Promise<AgentRuntimeRun[]> {
  const params = new URLSearchParams()
  const opts = typeof options === 'number' ? { limit: options } : options
  params.set('limit', String(opts.limit ?? 30))
  if (opts.status?.trim() && opts.status !== 'all') params.set('status', opts.status.trim())
  if (opts.since?.trim() && opts.since !== 'all') params.set('since', opts.since.trim())
  if (opts.search?.trim()) params.set('search', opts.search.trim())
  const payload = await requestJSON<{ runs: AgentRuntimeRun[] }>(`/v1/agentruntime/runs?${params.toString()}`)
  return payload.runs ?? []
}

export async function getAgentRuntimeRun(runId: string): Promise<AgentRuntimeRun> {
	return requestJSON<AgentRuntimeRun>(`/v1/agentruntime/runs/${encodeURIComponent(runId)}`)
}

export type AgentRuntimeRestartRequest = {
	checkpoint_id?: string
	agent?: string
	tier?: string
	provider_override?: AgentRuntimeProviderOverride
	prompt_adjustment?: string
	title?: string
	mode?: AgentRuntimeRecoveryMode
	confirm_unsafe_recovery?: boolean
}

export async function restartAgentRuntimeRun(runId: string, payload: AgentRuntimeRestartRequest): Promise<AgentRuntimeRun> {
	return requestJSON<AgentRuntimeRun>(`/v1/agentruntime/runs/${encodeURIComponent(runId)}/restart`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(payload),
	})
}

export async function listAgentRuntimeSubagents(): Promise<AgentRuntimeSubagentsResponse> {
	return requestJSON<AgentRuntimeSubagentsResponse>('/v1/agentruntime/subagents')
}

export async function getAgentRuntimeSubagent(name: string): Promise<AgentRuntimeSubagent> {
	return requestJSON<AgentRuntimeSubagent>(`/v1/agentruntime/subagents/${encodeURIComponent(name)}`)
}

export async function updateAgentRuntimeSubagentTier(name: string, defaultTier: string): Promise<AgentRuntimeSubagent> {
	return requestJSON<AgentRuntimeSubagent>(`/v1/agentruntime/subagents/${encodeURIComponent(name)}`, {
		method: 'PATCH',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ default_tier: defaultTier }),
	})
}

export async function draftAgentRuntimeSubagent(payload: {
	mode: 'create' | 'edit'
	request: string
	base_name?: string
	default_tier?: string
	use_llm?: boolean
}): Promise<AgentRuntimeSubagentDraftResponse> {
	return requestJSON<AgentRuntimeSubagentDraftResponse>('/v1/agentruntime/subagents/builder/draft', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(payload),
	})
}

export async function applyAgentRuntimeSubagentDraft(draft: AgentRuntimeSubagentDraft): Promise<AgentRuntimeSubagent> {
	return requestJSON<AgentRuntimeSubagent>('/v1/agentruntime/subagents/builder/apply', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ draft }),
	})
}

export async function archiveAgentRuntimeSubagent(name: string, confirm: boolean): Promise<AgentRuntimeSubagentArchiveResponse> {
	return requestJSON<AgentRuntimeSubagentArchiveResponse>(`/v1/agentruntime/subagents/${encodeURIComponent(name)}/archive`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ confirm }),
	})
}

export async function recommendAgentRuntimeSubagents(payload: {
	limit?: number
	min_runs?: number
	include_failed?: boolean
} = {}): Promise<AgentRuntimeSubagentRecommendationsResponse> {
	return requestJSON<AgentRuntimeSubagentRecommendationsResponse>('/v1/agentruntime/subagents/recommendations', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(payload),
	})
}

export function streamAgentRuntimeRunEvents(
	runId: string,
	onEvent: (event: AgentRuntimeRunEvent) => void,
	onError?: (message: string) => void,
	onOpen?: () => void,
): () => void {
	const stream = new EventSource(`/v1/agentruntime/runs/${encodeURIComponent(runId)}/events`, { withCredentials: true })
	stream.onopen = () => {
		onOpen?.()
	}
	stream.onmessage = (message) => {
		if (!message.data) return
		try {
			onEvent(JSON.parse(message.data) as AgentRuntimeRunEvent)
		} catch {
			onError?.('Failed to parse agent runtime run event')
		}
	}
	stream.onerror = () => onError?.('Agent Runtime run event stream disconnected')
	return () => stream.close()
}
