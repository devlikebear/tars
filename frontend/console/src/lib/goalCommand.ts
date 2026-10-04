// `/goal` arguments: an optional permission flag in front of the description.
// The flag asks for the tool permission mode in the same step as the goal, so
// the run is not stopped by permission prompts; the server gives the previous
// mode back when the goal ends.

import type { PermissionMode } from './api'

export type GoalArgs = {
  description: string
  permissionMode?: PermissionMode
  // A leading `--word` that is not a known flag; nothing is set.
  unknownFlag?: string
}

const flagModes: Record<string, PermissionMode> = {
  '--auto': 'auto',
  '--accept-edits': 'accept_edits',
}

export function parseGoalArgs(args: string): GoalArgs {
  const words = args.trim().split(/\s+/).filter(Boolean)
  let permissionMode: PermissionMode | undefined
  while (words.length > 0 && words[0].startsWith('--')) {
    const mode = flagModes[words[0].toLowerCase()]
    if (!mode) return { description: '', unknownFlag: words[0] }
    permissionMode = mode
    words.shift()
  }
  const description = words.join(' ')
  return permissionMode ? { description, permissionMode } : { description }
}
