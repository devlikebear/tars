import type { TasksPanelTranslations } from '../i18n/sections/tasksPanel.ts'
import type { WorkLedgerEvent, WorkLedgerProjection, WorkLedgerStep } from './types.ts'

// Labels come from the Tasks panel section so the timeline follows the console locale.
export type WorkLedgerStrings = TasksPanelTranslations['ledger']

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

// Status words map through the locale; a value the map does not know renders raw.
function stateLabel(labels: Record<string, string>, value: string): string {
  return Object.hasOwn(labels, value) ? labels[value] : value
}

function formatBytes(value?: number): string {
  if (value == null || value < 0) return ''
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`
  if (value < 1024 * 1024 * 1024) return `${(value / (1024 * 1024)).toFixed(1)} MB`
  return `${(value / (1024 * 1024 * 1024)).toFixed(1)} GB`
}

function eventPresentation(
  event: WorkLedgerEvent,
  projection: WorkLedgerProjection,
  strings: WorkLedgerStrings,
): Pick<WorkLedgerTimelineEntry, 'title' | 'detail'> {
  const { events } = strings
  const workState = (state?: string) => state == null ? strings.unknownState : stateLabel(strings.workStates, state)
  const capabilityState = (state: string) => state ? stateLabel(strings.capabilityStates, state) : strings.unknownState
  switch (event.type) {
    case 'work.created':
      return { title: events.workCreated, detail: projection.work.title }
    case 'work.transitioned':
      return {
        title: `${workState(event.from_state)} → ${workState(event.to_state)}`,
        detail: payloadString(event, 'reason'),
      }
    case 'step.created':
      return { title: events.stepCreated, detail: payloadString(event, 'title') }
    case 'step.dependency_added':
      return { title: events.stepDependencyAdded, detail: payloadString(event, 'depends_on_step_id') }
    case 'attempt.created': {
      const number = payloadNumber(event, 'number')
      const adapter = payloadString(event, 'adapter')
      const status = payloadString(event, 'status')
      return {
        title: number == null ? events.attemptCreated : events.attemptNumberCreated(number),
        detail: [adapter, status].filter(Boolean).join(' · '),
      }
    }
    case 'approval.created':
      return {
        title: events.approvalRequested,
        detail: [payloadString(event, 'authority'), payloadString(event, 'status')].filter(Boolean).join(' · '),
      }
    case 'proof.created':
      return {
        title: events.proofRecorded,
        detail: [payloadString(event, 'kind'), payloadString(event, 'status')].filter(Boolean).join(' · '),
      }
    case 'artifact.created':
      return { title: events.artifactAttached, detail: payloadString(event, 'kind') }
    case 'step.schedule_configured':
      return { title: events.stepScheduleConfigured, detail: payloadString(event, 'next_action') }
    case 'step.ready':
      return { title: events.stepReady, detail: payloadString(event, 'reason') }
    case 'step.claimed':
      return {
        title: events.stepClaimed,
        detail: [payloadString(event, 'worker_id'), payloadString(event, 'next_action')].filter(Boolean).join(' · '),
      }
    case 'step.heartbeat':
      return { title: events.leaseRenewed, detail: payloadString(event, 'worker_id') }
    case 'attempt.completed':
      return { title: events.attemptCompleted, detail: payloadString(event, 'status') }
    case 'step.completed':
      return { title: events.stepCompleted, detail: '' }
    case 'step.retry_scheduled':
      return { title: events.retryScheduled, detail: payloadString(event, 'reason') }
    case 'step.replan_scheduled':
      return { title: events.replanScheduled, detail: payloadString(event, 'reason') }
    case 'step.decompose_scheduled':
      return { title: events.decompositionScheduled, detail: payloadString(event, 'reason') }
    case 'step.released':
      return { title: events.claimReleased, detail: payloadString(event, 'reason') }
    case 'step.reclaimed':
      return { title: events.expiredClaimReclaimed, detail: payloadString(event, 'reason') }
    case 'step.review_requested':
      return { title: events.operatorReviewRequested, detail: payloadString(event, 'reason') }
    case 'step.blocked':
      return { title: events.stepBlocked, detail: payloadString(event, 'reason') }
    case 'step.resumed':
      return { title: events.stepResumed, detail: payloadString(event, 'reason') }
    case 'step.cancelled':
      return { title: events.stepCancelled, detail: payloadString(event, 'reason') }
    case 'execution.environment_provisioned':
      return {
        title: events.environmentProvisioned,
        detail: [payloadString(event, 'provider'), payloadString(event, 'environment_id')].filter(Boolean).join(' · '),
      }
    case 'execution.credentials_issued':
      return {
        title: events.credentialsIssued,
        detail: [payloadString(event, 'worker'), payloadString(event, 'credential_id')].filter(Boolean).join(' · '),
      }
    case 'execution.worker_started':
      return {
        title: events.workerStarted,
        detail: [payloadString(event, 'worker'), payloadString(event, 'provider')].filter(Boolean).join(' · '),
      }
    case 'execution.checkpoint_recorded':
      return { title: events.workerCheckpointRecorded, detail: payloadString(event, 'checkpoint_id') }
    case 'execution.environment_synced':
      return { title: events.environmentSynchronized, detail: payloadObjectString(event, 'snapshot', 'digest') }
    case 'execution.artifacts_collected': {
      const count = payloadNumber(event, 'artifact_count')
      return { title: events.artifactsCollected, detail: count == null ? '' : strings.artifactCount(count) }
    }
    case 'execution.credentials_revoked':
      return { title: events.credentialsRevoked, detail: payloadString(event, 'credential_id') }
    case 'execution.environment_destroyed':
      return {
        title: events.environmentDestroyed,
        detail: [payloadString(event, 'provider'), payloadString(event, 'environment_id')].filter(Boolean).join(' · '),
      }
    case 'execution.recovery_started':
      return {
        title: events.recoveryStarted,
        detail: [payloadString(event, 'worker'), payloadString(event, 'environment_id')].filter(Boolean).join(' · '),
      }
    case 'execution.worker_cancelled':
      return { title: events.workerCancelled, detail: payloadString(event, 'worker') }
    case 'worker.placement_created':
      return {
        title: events.remotePlacementCreated,
        detail: [payloadString(event, 'worker_id'), payloadString(event, 'placement_id')].filter(Boolean).join(' · '),
      }
    case 'worker.environment_provisioned':
      return {
        title: events.remoteEnvironmentProvisioned,
        detail: [payloadString(event, 'environment_id'), payloadString(event, 'worker_id')].filter(Boolean).join(' · '),
      }
    case 'worker.workspace_synced': {
      const fileCount = payloadNumber(event, 'file_count')
      const totalBytes = payloadNumber(event, 'total_bytes')
      return {
        title: events.workspaceSynchronized,
        detail: [
          payloadString(event, 'mode'),
          fileCount == null ? '' : strings.fileCount(fileCount),
          formatBytes(totalBytes),
          payloadString(event, 'digest'),
        ].filter(Boolean).join(' · '),
      }
    }
    case 'worker.lease_granted':
      return {
        title: events.remoteLeaseGranted,
        detail: [payloadString(event, 'worker_id'), payloadString(event, 'placement_id')].filter(Boolean).join(' · '),
      }
    case 'worker.heartbeat_observed':
      return { title: events.remoteHeartbeatObserved, detail: payloadString(event, 'worker_id') }
    case 'worker.execution_started':
      return {
        title: events.remoteExecutionStarted,
        detail: [payloadString(event, 'worker_id'), payloadString(event, 'placement_id')].filter(Boolean).join(' · '),
      }
    case 'worker.stream_observed': {
      const textBytes = payloadNumber(event, 'text_bytes')
      return {
        title: events.remoteStreamObserved,
        detail: [payloadString(event, 'kind'), formatBytes(textBytes)].filter(Boolean).join(' · '),
      }
    }
    case 'worker.checkpoint_recorded':
      return {
        title: events.remoteCheckpointRecorded,
        detail: [payloadString(event, 'checkpoint_id'), payloadString(event, 'digest')].filter(Boolean).join(' · '),
      }
    case 'worker.artifacts_collected': {
      const artifactCount = payloadNumber(event, 'artifact_count')
      return {
        title: events.remoteArtifactsCollected,
        detail: artifactCount == null ? '' : strings.artifactCount(artifactCount),
      }
    }
    case 'worker.placement_destroyed':
      return { title: events.remotePlacementDestroyed, detail: payloadString(event, 'placement_id') }
    case 'worker.lost':
      return {
        title: events.remoteWorkerLost,
        detail: [payloadString(event, 'worker_id'), payloadString(event, 'placement_id')].filter(Boolean).join(' · '),
      }
    case 'worker.reclaimed':
      return { title: events.remotePlacementReclaimed, detail: payloadString(event, 'placement_id') }
    case 'worker.rehydrated':
      return {
        title: events.remotePlacementRehydrated,
        detail: [payloadString(event, 'replacement_worker_id'), payloadString(event, 'placement_id')].filter(Boolean).join(' · '),
      }
    case 'a2a.task_submitted':
      return {
        title: events.a2aTaskSubmitted,
        detail: [payloadString(event, 'task_id'), payloadString(event, 'protocol_version') ? strings.protocol(payloadString(event, 'protocol_version')) : ''].filter(Boolean).join(' · '),
      }
    case 'a2a.task_state_observed':
      return {
        title: events.a2aTaskStateObserved,
        detail: [payloadString(event, 'task_id'), payloadString(event, 'state')].filter(Boolean).join(' · '),
      }
    case 'a2a.artifact_quarantined': {
      const quarantined = payloadNumber(event, 'quarantined_parts')
      return {
        title: events.a2aArtifactQuarantined,
        detail: [payloadString(event, 'task_id'), quarantined == null ? '' : strings.partCount(quarantined)].filter(Boolean).join(' · '),
      }
    }
    case 'a2a.task_canceled':
      return { title: events.a2aTaskCanceled, detail: payloadString(event, 'task_id') }
    case 'capability.version_created': {
      const version = payloadNumber(event, 'version')
      return {
        title: events.capabilityVersionCreated,
        detail: [payloadString(event, 'capability_name'), version == null ? '' : `v${version}`, payloadString(event, 'state')].filter(Boolean).join(' · '),
      }
    }
    case 'capability.evaluation_recorded':
      return {
        title: events.capabilityEvaluationRecorded,
        detail: [payloadString(event, 'stage'), payloadString(event, 'status')].filter(Boolean).join(' · '),
      }
    case 'capability.transitioned':
      return {
        title: `${capabilityState(payloadString(event, 'from_state'))} → ${capabilityState(payloadString(event, 'to_state'))}`,
        detail: payloadString(event, 'reason'),
      }
    case 'capability.outcome_recorded':
      return {
        title: events.capabilityOutcomeRecorded,
        detail: [payloadString(event, 'status'), payloadString(event, 'verifier_status')].filter(Boolean).join(' · '),
      }
    case 'capability.regression_detected':
      return {
        title: events.capabilityRegressionDetected,
        detail: [payloadString(event, 'status'), payloadString(event, 'verifier_status')].filter(Boolean).join(' · '),
      }
    default:
      return { title: event.type, detail: '' }
  }
}

export function buildWorkLedgerTimeline(projection: WorkLedgerProjection, strings: WorkLedgerStrings): WorkLedgerTimelineEntry[] {
  return [...projection.events]
    .sort((left, right) => left.sequence - right.sequence || left.created_at.localeCompare(right.created_at))
    .map((event) => ({
      id: event.id,
      sequence: event.sequence,
      type: event.type,
      actor_id: event.actor_id,
      created_at: event.created_at,
      ...eventPresentation(event, projection, strings),
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
