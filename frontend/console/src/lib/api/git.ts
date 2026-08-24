import { requestJSON } from './client.ts'
import type {
  GitBranchesResponse,
  GitCommitDetail,
  GitDiff,
  GitLogResponse,
  GitMutationPlan,
  GitStatus,
  GitWorktreesResponse,
} from '../types'

// --- Git Inspector ---

type GitQuery = {
  sessionId?: string
  root?: string
}

function gitQueryParams(query: GitQuery = {}): URLSearchParams {
  const params = new URLSearchParams()
  if (query.sessionId?.trim()) params.set('session_id', query.sessionId.trim())
  if (query.root?.trim()) params.set('root', query.root.trim())
  return params
}

function gitEndpoint(path: string, query: GitQuery = {}): string {
  const params = gitQueryParams(query)
  const suffix = params.toString()
  return `/v1/git/${path}${suffix ? `?${suffix}` : ''}`
}

export function getGitStatus(query: GitQuery = {}): Promise<GitStatus> {
  return requestJSON<GitStatus>(gitEndpoint('status', query))
}

export function getGitDiff(query: GitQuery & { path?: string; staged?: boolean; hash?: string } = {}): Promise<GitDiff> {
  const params = gitQueryParams(query)
  if (query.path?.trim()) params.set('path', query.path.trim())
  if (query.staged) params.set('staged', '1')
  if (query.hash?.trim()) params.set('hash', query.hash.trim())
  const suffix = params.toString()
  return requestJSON<GitDiff>(`/v1/git/diff${suffix ? `?${suffix}` : ''}`)
}

export function getGitCommit(query: GitQuery & { hash: string }): Promise<GitCommitDetail> {
  const params = gitQueryParams(query)
  params.set('hash', query.hash.trim())
  return requestJSON<GitCommitDetail>(`/v1/git/commit?${params.toString()}`)
}

export function getGitWorktrees(query: GitQuery = {}): Promise<GitWorktreesResponse> {
  return requestJSON<GitWorktreesResponse>(gitEndpoint('worktrees', query))
}

export function getGitLog(query: GitQuery & { limit?: number } = {}): Promise<GitLogResponse> {
  const params = gitQueryParams(query)
  if (query.limit) params.set('limit', String(query.limit))
  const suffix = params.toString()
  return requestJSON<GitLogResponse>(`/v1/git/log${suffix ? `?${suffix}` : ''}`)
}

export function getGitBranches(query: GitQuery = {}): Promise<GitBranchesResponse> {
  return requestJSON<GitBranchesResponse>(gitEndpoint('branches', query))
}

export type CreateGitMutationApprovalRequest = {
  session_id: string
  root?: string
  action:
    | 'stage'
    | 'unstage'
    | 'discard'
    | 'commit'
    | 'switch_branch'
    | 'checkout_commit'
    | 'worktree_add'
    | 'worktree_remove'
    | 'fetch'
  path?: string
  branch?: string
  message?: string
  hash?: string
  worktree_path?: string
  new_branch?: string
  reason?: string
}

export function createGitMutationApproval(payload: CreateGitMutationApprovalRequest): Promise<GitMutationPlan> {
  return requestJSON<GitMutationPlan>('/v1/git/mutations', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
}
