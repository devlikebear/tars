import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { buildPulseIncidentCards } from '../src/lib/pulseIncidentCards.ts'
import type { PulseTickOutcome } from '../src/lib/types.ts'

const pulseSource = readFileSync(new URL('../src/components/Pulse.svelte', import.meta.url), 'utf8')

test('Pulse incident cards include cause, evidence, action, and safe links', () => {
  const ticks: PulseTickOutcome[] = [
    {
      at: '2026-05-01T10:00:00Z',
      signals: [
        {
          kind: 'cron_failures',
          severity: 'error',
          summary: '2 cron job(s) are failing',
          at: '2026-05-01T10:00:00Z',
          details: {
            worst_job_name: 'daily backup',
            worst_failures: 4,
            worst_job_error: 'exit status 1',
          },
        },
      ],
      decision: {
        action: 'notify',
        severity: 'error',
        title: 'Cron is failing',
        summary: 'Review the failed cron job before the next window.',
      },
      notify_delivered: true,
    },
  ]

  const [card] = buildPulseIncidentCards(ticks)

  assert.equal(card.kind, 'cron_failures')
  assert.equal(card.severity, 'error')
  assert.equal(card.title, 'Cron is failing')
  assert.match(card.cause, /daily backup/)
  assert.match(card.recommendedAction, /cron/i)
  assert.deepEqual(card.primaryAction, { label: 'Open Cron', path: '/console/cron' })
  assert.ok(card.evidence.some((item) => item.includes('exit status 1')))
  assert.ok(card.evidence.some((item) => item.includes('Decision: notify')))
})

test('Pulse incident cards link stalled chats to their session', () => {
  const ticks: PulseTickOutcome[] = [
    {
      at: '2026-05-01T10:00:00Z',
      signals: [
        {
          kind: 'stalled_chat',
          severity: 'warn',
          summary: '1 chat session appears stalled',
          at: '2026-05-01T10:00:00Z',
          details: {
            session_id: 'sess-123',
            session_title: 'release work',
            age_minutes: 45,
            can_auto_resume: true,
            resume_mode: 'record_assumption_and_proceed',
          },
        },
      ],
    },
  ]

  const [card] = buildPulseIncidentCards(ticks)

  assert.equal(card.kind, 'stalled_chat')
  assert.match(card.cause, /release work/)
  assert.match(card.recommendedAction, /resume/i)
  assert.deepEqual(card.primaryAction, { label: 'Open Chat', path: '/console/chat/sess-123' })
  assert.ok(card.evidence.some((item) => item.includes('45')))
})

test('Pulse page renders incident cards with safe actions', () => {
  const i18nEnSource = readFileSync(new URL('../src/i18n/en.ts', import.meta.url), 'utf8')
  assert.match(pulseSource, /buildPulseIncidentCards/)
  assert.match(pulseSource, /pulse-incident-card/)
  assert.match(i18nEnSource, /incidentCardsTitle: 'Incident cards'/)
  assert.match(i18nEnSource, /likelyCause: 'Likely cause'/)
  assert.match(i18nEnSource, /evidence: 'Evidence'/)
  assert.match(i18nEnSource, /recommendedAction: 'Recommended action'/)
  assert.match(i18nEnSource, /openAffectedPage: 'Open affected page'/)
  assert.match(i18nEnSource, /recheck: 'Re-check'/)
})
