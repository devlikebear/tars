import { requestJSON } from './client.ts'
import type {
  Approval,
  AutomationAuditListResponse,
  CleanupApplyResult,
  CleanupPlan,
  CreateCronJobRequest,
  CronJob,
  CronRunRecord,
  CronRunResult,
  OpsStatus,
  PulseConfigView,
  PulseSnapshot,
  PulseTickOutcome,
  ReflectionConfigView,
  ReflectionRunSummary,
  ReflectionSnapshot,
  UpdateCronJobRequest,
  WorkerControlPlaneResponse,
} from '../types'

// --- Pulse (system watchdog, replaces heartbeat) ---

export async function getPulseStatus(): Promise<PulseSnapshot> {
  return requestJSON<PulseSnapshot>('/v1/pulse/status')
}

export async function runPulseOnce(): Promise<PulseTickOutcome> {
  return requestJSON<PulseTickOutcome>('/v1/pulse/run-once', { method: 'POST' })
}

export async function getPulseConfig(): Promise<PulseConfigView> {
  return requestJSON<PulseConfigView>('/v1/pulse/config')
}

// --- Reflection (nightly batch runner) ---

export async function getReflectionStatus(): Promise<ReflectionSnapshot> {
  return requestJSON<ReflectionSnapshot>('/v1/reflection/status')
}

export async function runReflectionOnce(): Promise<ReflectionRunSummary> {
  return requestJSON<ReflectionRunSummary>('/v1/reflection/run-once', { method: 'POST' })
}

export async function getReflectionConfig(): Promise<ReflectionConfigView> {
  return requestJSON<ReflectionConfigView>('/v1/reflection/config')
}

// --- Cron ---

export async function listCronJobs(): Promise<CronJob[]> {
  return requestJSON<CronJob[]>('/v1/cron/jobs')
}

export async function listCronRuns(jobId: string, limit = 5): Promise<CronRunRecord[]> {
  return requestJSON<CronRunRecord[]>(`/v1/cron/jobs/${encodeURIComponent(jobId)}/runs?limit=${limit}`)
}

export async function createCronJob(data: CreateCronJobRequest): Promise<CronJob> {
  return requestJSON<CronJob>('/v1/cron/jobs', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
}

export async function updateCronJob(jobId: string, data: UpdateCronJobRequest): Promise<CronJob> {
  return requestJSON<CronJob>(`/v1/cron/jobs/${encodeURIComponent(jobId)}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
}

export async function deleteCronJob(jobId: string): Promise<void> {
  await requestJSON<Record<string, never>>(`/v1/cron/jobs/${encodeURIComponent(jobId)}`, {
    method: 'DELETE',
  })
}

export async function runCronJob(jobId: string): Promise<CronRunResult> {
  return requestJSON<CronRunResult>(`/v1/cron/jobs/${encodeURIComponent(jobId)}/run`, {
    method: 'POST',
  })
}

// --- Ops / approvals ---

export async function getOpsStatus(): Promise<OpsStatus> {
  return requestJSON<OpsStatus>('/v1/ops/status')
}

export async function createCleanupPlan(): Promise<CleanupPlan> {
  return requestJSON<CleanupPlan>('/v1/ops/cleanup/plan', { method: 'POST' })
}

export async function applyCleanup(approvalId: string): Promise<CleanupApplyResult> {
  return requestJSON<CleanupApplyResult>('/v1/ops/cleanup/apply', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ approval_id: approvalId }),
  })
}

export async function listApprovals(): Promise<Approval[]> {
  return requestJSON<Approval[]>('/v1/ops/approvals')
}

export async function reviewApproval(approvalId: string, action: 'approve' | 'reject'): Promise<void> {
  await requestJSON<{ ok: boolean }>(`/v1/ops/approvals/${encodeURIComponent(approvalId)}/${action}`, {
    method: 'POST',
  })
}

export async function listAutomationAudit(limit = 50, sessionId = ''): Promise<AutomationAuditListResponse> {
  const params = new URLSearchParams()
  if (limit > 0) params.set('limit', String(limit))
  if (sessionId.trim()) params.set('session_id', sessionId.trim())
  const suffix = params.toString()
  return requestJSON<AutomationAuditListResponse>(`/v1/ops/automation-audit${suffix ? `?${suffix}` : ''}`)
}

export async function getWorkerControlPlane(): Promise<WorkerControlPlaneResponse> {
  return requestJSON<WorkerControlPlaneResponse>('/v1/admin/workers')
}
