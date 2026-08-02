import type { WorkLedgerEvent, WorkLedgerProjection } from './types.ts'

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
