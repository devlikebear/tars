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
