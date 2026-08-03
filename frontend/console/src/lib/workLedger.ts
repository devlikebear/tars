import type { WorkLedgerEvent, WorkLedgerProjection, WorkLedgerStep } from './types.ts'

export type WorkLedgerTimelineEntry = {
  id: string
  sequence: number
  type: string
  title: string
  detail: string
  actor_id: string
  created_at: string
}

function payloadString(event: WorkLedgerEvent, key: string): string {
  const value = event.payload?.[key]
  return typeof value === 'string' ? value.trim() : ''
}

function payloadNumber(event: WorkLedgerEvent, key: string): number | undefined {
  const value = event.payload?.[key]
  return typeof value === 'number' && Number.isFinite(value) ? value : undefined
}

function payloadObjectString(event: WorkLedgerEvent, objectKey: string, key: string): string {
  const object = event.payload?.[objectKey]
  if (object == null || typeof object !== 'object' || Array.isArray(object)) return ''
  const value = (object as Record<string, unknown>)[key]
  return typeof value === 'string' ? value.trim() : ''
}

function formatBytes(value?: number): string {
  if (value == null || value < 0) return ''
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`
  if (value < 1024 * 1024 * 1024) return `${(value / (1024 * 1024)).toFixed(1)} MB`
  return `${(value / (1024 * 1024 * 1024)).toFixed(1)} GB`
}

function eventPresentation(event: WorkLedgerEvent, projection: WorkLedgerProjection): Pick<WorkLedgerTimelineEntry, 'title' | 'detail'> {
  switch (event.type) {
    case 'work.created':
      return { title: 'Work created', detail: projection.work.title }
    case 'work.transitioned':
      return {
        title: `${event.from_state ?? 'unknown'} → ${event.to_state ?? 'unknown'}`,
        detail: payloadString(event, 'reason'),
      }
    case 'step.created':
      return { title: 'Step created', detail: payloadString(event, 'title') }
    case 'step.dependency_added':
      return { title: 'Step dependency added', detail: payloadString(event, 'depends_on_step_id') }
    case 'attempt.created': {
      const number = payloadNumber(event, 'number')
      const adapter = payloadString(event, 'adapter')
      const status = payloadString(event, 'status')
      return {
        title: number == null ? 'Attempt created' : `Attempt #${number} created`,
        detail: [adapter, status].filter(Boolean).join(' · '),
      }
    }
    case 'approval.created':
      return {
        title: 'Approval requested',
        detail: [payloadString(event, 'authority'), payloadString(event, 'status')].filter(Boolean).join(' · '),
      }
    case 'proof.created':
      return {
        title: 'Proof recorded',
        detail: [payloadString(event, 'kind'), payloadString(event, 'status')].filter(Boolean).join(' · '),
      }
    case 'artifact.created':
      return { title: 'Artifact attached', detail: payloadString(event, 'kind') }
    case 'step.schedule_configured':
      return { title: 'Step schedule configured', detail: payloadString(event, 'next_action') }
    case 'step.ready':
      return { title: 'Step ready', detail: payloadString(event, 'reason') }
    case 'step.claimed':
      return {
        title: 'Step claimed',
        detail: [payloadString(event, 'worker_id'), payloadString(event, 'next_action')].filter(Boolean).join(' · '),
      }
    case 'step.heartbeat':
      return { title: 'Lease renewed', detail: payloadString(event, 'worker_id') }
    case 'attempt.completed':
      return { title: 'Attempt completed', detail: payloadString(event, 'status') }
    case 'step.completed':
      return { title: 'Step completed', detail: '' }
    case 'step.retry_scheduled':
      return { title: 'Retry scheduled', detail: payloadString(event, 'reason') }
    case 'step.replan_scheduled':
      return { title: 'Replan scheduled', detail: payloadString(event, 'reason') }
    case 'step.decompose_scheduled':
      return { title: 'Decomposition scheduled', detail: payloadString(event, 'reason') }
    case 'step.released':
      return { title: 'Claim released', detail: payloadString(event, 'reason') }
    case 'step.reclaimed':
      return { title: 'Expired claim reclaimed', detail: payloadString(event, 'reason') }
    case 'step.review_requested':
      return { title: 'Operator review requested', detail: payloadString(event, 'reason') }
    case 'step.blocked':
      return { title: 'Step blocked', detail: payloadString(event, 'reason') }
    case 'step.resumed':
      return { title: 'Step resumed', detail: payloadString(event, 'reason') }
    case 'step.cancelled':
      return { title: 'Step cancelled', detail: payloadString(event, 'reason') }
    case 'execution.environment_provisioned':
      return {
        title: 'Environment provisioned',
        detail: [payloadString(event, 'provider'), payloadString(event, 'environment_id')].filter(Boolean).join(' · '),
      }
    case 'execution.credentials_issued':
      return {
        title: 'Task credentials issued',
        detail: [payloadString(event, 'worker'), payloadString(event, 'credential_id')].filter(Boolean).join(' · '),
      }
    case 'execution.worker_started':
      return {
        title: 'Worker started',
        detail: [payloadString(event, 'worker'), payloadString(event, 'provider')].filter(Boolean).join(' · '),
      }
    case 'execution.checkpoint_recorded':
      return { title: 'Worker checkpoint recorded', detail: payloadString(event, 'checkpoint_id') }
    case 'execution.environment_synced':
      return { title: 'Environment synchronized', detail: payloadObjectString(event, 'snapshot', 'digest') }
    case 'execution.artifacts_collected': {
      const count = payloadNumber(event, 'artifact_count')
      return { title: 'Artifacts collected', detail: count == null ? '' : `${count} ${count === 1 ? 'artifact' : 'artifacts'}` }
    }
    case 'execution.credentials_revoked':
      return { title: 'Task credentials revoked', detail: payloadString(event, 'credential_id') }
    case 'execution.environment_destroyed':
      return {
        title: 'Environment destroyed',
        detail: [payloadString(event, 'provider'), payloadString(event, 'environment_id')].filter(Boolean).join(' · '),
      }
    case 'execution.recovery_started':
      return {
        title: 'Execution recovery started',
        detail: [payloadString(event, 'worker'), payloadString(event, 'environment_id')].filter(Boolean).join(' · '),
      }
    case 'execution.worker_cancelled':
      return { title: 'Worker cancelled', detail: payloadString(event, 'worker') }
    case 'worker.placement_created':
      return {
        title: 'Remote placement created',
        detail: [payloadString(event, 'worker_id'), payloadString(event, 'placement_id')].filter(Boolean).join(' · '),
      }
    case 'worker.environment_provisioned':
      return {
        title: 'Remote environment provisioned',
        detail: [payloadString(event, 'environment_id'), payloadString(event, 'worker_id')].filter(Boolean).join(' · '),
      }
    case 'worker.workspace_synced': {
      const fileCount = payloadNumber(event, 'file_count')
      const totalBytes = payloadNumber(event, 'total_bytes')
      return {
        title: 'Workspace synchronized',
        detail: [
          payloadString(event, 'mode'),
          fileCount == null ? '' : `${fileCount} ${fileCount === 1 ? 'file' : 'files'}`,
          formatBytes(totalBytes),
          payloadString(event, 'digest'),
        ].filter(Boolean).join(' · '),
      }
    }
    case 'worker.lease_granted':
      return {
        title: 'Remote lease granted',
        detail: [payloadString(event, 'worker_id'), payloadString(event, 'placement_id')].filter(Boolean).join(' · '),
      }
    case 'worker.heartbeat_observed':
      return { title: 'Remote heartbeat observed', detail: payloadString(event, 'worker_id') }
    case 'worker.execution_started':
      return {
        title: 'Remote execution started',
        detail: [payloadString(event, 'worker_id'), payloadString(event, 'placement_id')].filter(Boolean).join(' · '),
      }
    case 'worker.stream_observed': {
      const textBytes = payloadNumber(event, 'text_bytes')
      return {
        title: 'Remote stream observed',
        detail: [payloadString(event, 'kind'), formatBytes(textBytes)].filter(Boolean).join(' · '),
      }
    }
    case 'worker.checkpoint_recorded':
      return {
        title: 'Remote checkpoint recorded',
        detail: [payloadString(event, 'checkpoint_id'), payloadString(event, 'digest')].filter(Boolean).join(' · '),
      }
    case 'worker.artifacts_collected': {
      const artifactCount = payloadNumber(event, 'artifact_count')
      return {
        title: 'Remote artifacts collected',
        detail: artifactCount == null ? '' : `${artifactCount} ${artifactCount === 1 ? 'artifact' : 'artifacts'}`,
      }
    }
    case 'worker.placement_destroyed':
      return { title: 'Remote placement destroyed', detail: payloadString(event, 'placement_id') }
    case 'worker.lost':
      return {
        title: 'Remote worker lost',
        detail: [payloadString(event, 'worker_id'), payloadString(event, 'placement_id')].filter(Boolean).join(' · '),
      }
    case 'worker.reclaimed':
      return { title: 'Remote placement reclaimed', detail: payloadString(event, 'placement_id') }
    case 'worker.rehydrated':
      return {
        title: 'Remote placement rehydrated',
        detail: [payloadString(event, 'replacement_worker_id'), payloadString(event, 'placement_id')].filter(Boolean).join(' · '),
      }
    case 'a2a.task_submitted':
      return {
        title: 'A2A task submitted',
        detail: [payloadString(event, 'task_id'), payloadString(event, 'protocol_version') ? `protocol ${payloadString(event, 'protocol_version')}` : ''].filter(Boolean).join(' · '),
      }
    case 'a2a.task_state_observed':
      return {
        title: 'A2A task state observed',
        detail: [payloadString(event, 'task_id'), payloadString(event, 'state')].filter(Boolean).join(' · '),
      }
    case 'a2a.artifact_quarantined': {
      const quarantined = payloadNumber(event, 'quarantined_parts')
      return {
        title: 'A2A artifact quarantined',
        detail: [payloadString(event, 'task_id'), quarantined == null ? '' : `${quarantined} ${quarantined === 1 ? 'part' : 'parts'}`].filter(Boolean).join(' · '),
      }
    }
    case 'a2a.task_canceled':
      return { title: 'A2A task canceled', detail: payloadString(event, 'task_id') }
    case 'capability.version_created': {
      const version = payloadNumber(event, 'version')
      return {
        title: 'Capability version created',
        detail: [payloadString(event, 'capability_name'), version == null ? '' : `v${version}`, payloadString(event, 'state')].filter(Boolean).join(' · '),
      }
    }
    case 'capability.evaluation_recorded':
      return {
        title: 'Capability evaluation recorded',
        detail: [payloadString(event, 'stage'), payloadString(event, 'status')].filter(Boolean).join(' · '),
      }
    case 'capability.transitioned':
      return {
        title: `${payloadString(event, 'from_state') || 'unknown'} → ${payloadString(event, 'to_state') || 'unknown'}`,
        detail: payloadString(event, 'reason'),
      }
    case 'capability.outcome_recorded':
      return {
        title: 'Capability outcome recorded',
        detail: [payloadString(event, 'status'), payloadString(event, 'verifier_status')].filter(Boolean).join(' · '),
      }
    case 'capability.regression_detected':
      return {
        title: 'Capability regression needs review',
        detail: [payloadString(event, 'status'), payloadString(event, 'verifier_status')].filter(Boolean).join(' · '),
      }
    default:
      return { title: event.type, detail: '' }
  }
}

export function buildWorkLedgerTimeline(projection: WorkLedgerProjection): WorkLedgerTimelineEntry[] {
  return [...projection.events]
    .sort((left, right) => left.sequence - right.sequence || left.created_at.localeCompare(right.created_at))
    .map((event) => ({
      id: event.id,
      sequence: event.sequence,
      type: event.type,
      actor_id: event.actor_id,
      created_at: event.created_at,
      ...eventPresentation(event, projection),
    }))
}

export function latestWorkLedgerSequence(projection: WorkLedgerProjection): number {
  return projection.events.reduce((latest, event) => Math.max(latest, event.sequence), 0)
}

export function workLedgerCanCancel(projection: WorkLedgerProjection): boolean {
  return projection.work.state !== 'done' && projection.work.state !== 'cancelled'
}

export function resumableWorkLedgerSteps(projection: WorkLedgerProjection): WorkLedgerStep[] {
  const resumableStepIDs = new Set(
    projection.schedules
      .filter((schedule) => schedule.human_resume_required)
      .map((schedule) => schedule.step_id),
  )
  return projection.steps
    .filter((step) => resumableStepIDs.has(step.id) && (step.state === 'review' || step.state === 'blocked'))
    .sort((left, right) => left.position - right.position || left.id.localeCompare(right.id))
}
