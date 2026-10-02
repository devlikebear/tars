// Turns the server's provider probe results (POST /v1/admin/providers/test)
// into the rows Settings shows under Test connection. Pure, so the phrasing
// per provider kind and problem is tested under Node.
import type { ProviderTestTranslations } from '../i18n/sections/providerTest.ts'
import type { ProviderProbeResult, ProviderProbeStatus } from './types'

export type ProviderProbeTone = 'success' | 'info' | 'warning' | 'error'

export type ProviderProbeRow = {
  alias: string
  kind: string
  isDefault: boolean
  status: ProviderProbeStatus
  tone: ProviderProbeTone
  statusLabel: string
  summary: string
  // Server-side specifics (the CLI path, the provider's own error) shown in
  // mono under the summary.
  detail: string
}

const toneByStatus: Record<ProviderProbeStatus, ProviderProbeTone> = {
  ok: 'success',
  info: 'info',
  warn: 'warning',
  error: 'error',
}

const cliKinds = new Set(['claude-code-cli', 'antigravity-cli'])

function problemSummary(result: ProviderProbeResult, s: ProviderTestTranslations): string {
  switch (result.problem) {
    case 'cli_missing':
      return s.cliMissing
    case 'not_logged_in':
      return s.notLoggedIn
    case 'auth_unknown':
      return s.authUnknown
    case 'auth_check_failed':
      return s.authCheckFailed
    case 'version_old':
      return s.versionOld(result.version || '?', result.min_version || '?')
    case 'models_failed':
      return s.modelsFailed
    case 'models_warning':
      return s.modelsWarning
    case 'models_empty':
      return s.modelsEmpty
    default:
      return ''
  }
}

function okSummary(result: ProviderProbeResult, s: ProviderTestTranslations): string {
  const parts: string[] = []
  if (result.kind === 'claude-code-cli') {
    parts.push(s.signedIn(result.auth_method || ''))
  } else if (result.model_count) {
    parts.push(s.modelsAvailable(result.model_count))
  }
  if (cliKinds.has(result.kind) && result.version) {
    parts.push(s.cliVersion(result.version))
  }
  return parts.join(' · ')
}

export function describeProviderProbe(result: ProviderProbeResult, s: ProviderTestTranslations): ProviderProbeRow {
  const status: ProviderProbeStatus = result.status in toneByStatus ? result.status : 'error'
  const summary = problemSummary(result, s) || okSummary(result, s)
  return {
    alias: result.alias,
    kind: result.kind,
    isDefault: !!result.default,
    status,
    tone: toneByStatus[status],
    statusLabel: s.status[status],
    summary,
    detail: (result.detail || result.cli_path || '').trim(),
  }
}
