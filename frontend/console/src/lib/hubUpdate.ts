import type { HubUpdateResponse } from './types'

// The server's reason for a package skipped because nothing changed.
const UP_TO_DATE = 'up to date'

// heldUpdates lists the packages an update left alone for a reason the
// user has to act on — an external skill whose upstream changed and waits
// for review, a skill edited locally, a lookup that failed — as
// "name: reason" lines. Packages that are simply up to date are left out.
export function heldUpdates(result: HubUpdateResponse): string[] {
  const kinds = [result.skill_update_result, result.plugin_update_result, result.mcp_update_result, result.focus_template_update_result]
  const out: string[] = []
  for (const kind of kinds) {
    for (const item of kind?.skipped ?? []) {
      const detail = item.error || item.reason || ''
      if (!detail || detail === UP_TO_DATE) continue
      out.push(`${item.name}: ${detail}`)
    }
  }
  return out
}
