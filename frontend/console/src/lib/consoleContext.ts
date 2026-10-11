// Console context: guidance the console adds to a turn without the user
// typing it, such as the companion handoff's tone and console area. It goes
// to the server as `console_context`, which appends it to the message as one
// tagged block so every provider reads it (like review notes). The stored
// message therefore carries the block; these helpers take it back off so the
// bubble, the side panel and the session title show only the user's words.
import { splitReviewNotes } from './changes.ts'
import { stripFocusStage } from './focus.ts'

const contextOpen = '<console-context>'
const contextClose = '</console-context>'

// <initiative-context> is the same kind of hidden block (#1220): a note the
// server adds right after console context so a resuming CLI provider
// (claude-code-cli/antigravity-cli, which never replays the transcript)
// learns "you spoke first a moment ago" on the one turn that follows it.
const initiativeContextOpen = '<initiative-context>'
const initiativeContextClose = '</initiative-context>'

export function splitConsoleContext(text: string): { text: string; context: string } {
  const start = text.lastIndexOf(`\n\n${contextOpen}\n`)
  if (start < 0 || !text.trimEnd().endsWith(contextClose)) return { text, context: '' }
  const inner = text.trimEnd().slice(start + 2 + contextOpen.length, -contextClose.length)
  return { text: text.slice(0, start), context: inner.trim() }
}

export function splitInitiativeContext(text: string): { text: string; context: string } {
  const start = text.lastIndexOf(`\n\n${initiativeContextOpen}\n`)
  if (start < 0 || !text.trimEnd().endsWith(initiativeContextClose)) return { text, context: '' }
  const inner = text.trimEnd().slice(start + 2 + initiativeContextOpen.length, -initiativeContextClose.length)
  return { text: text.slice(0, start), context: inner.trim() }
}

// userVisibleText is a stored user message as the user wrote it: without the
// console context, the initiative hidden note, the focus stage guidance, or
// the review notes block. The server appends them in that order: message,
// <console-context>, <initiative-context>, <focus-stage>, <review-notes> —
// stripped here innermost-last, so outermost (review notes) first.
export function userVisibleText(text: string): string {
  return splitConsoleContext(splitInitiativeContext(stripFocusStage(splitReviewNotes(text).text)).text).text
}
