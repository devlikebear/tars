// Focus mode (docs/decisions/focus-mode.md): pure helpers for the pipeline
// screen — deck ordering, the stepper, the one-line progress of a running
// turn, folding the hidden <focus-*> blocks out of chat text, and mapping
// cards back to the transcript turn they came from. Tested under Node.
import { focusEn, type FocusTranslations } from '../i18n/sections/focus.ts'
import type {
  ChatEvent,
  FocusCard,
  FocusCardKind,
  FocusChangePayload,
  FocusPipeline,
  FocusPlan,
  FocusStageId,
  FocusStageStatus,
  SessionMessage,
} from './types.ts'

// --- Deck ---

// Deck priority (ADR §7). Notices — "report format missing" and the like —
// sit with the informational cards, after a verification failure.
const kindRank: Record<FocusCardKind, number> = {
  gate: 0,
  decision: 1,
  finding: 2,
  failure: 3,
  notice: 4,
  report: 5,
  change: 6,
}

const stateRank = { unseen: 0, seen: 1, decided: 2 } as const

// orderCards sorts a deck: decided cards last, then by kind priority, unseen
// before seen, and oldest first.
export function orderCards(cards: FocusCard[]): FocusCard[] {
  return [...cards].sort((a, b) => {
    const decided = Number(a.state === 'decided') - Number(b.state === 'decided')
    if (decided) return decided
    const kind = (kindRank[a.kind] ?? 9) - (kindRank[b.kind] ?? 9)
    if (kind) return kind
    const state = (stateRank[a.state] ?? 0) - (stateRank[b.state] ?? 0)
    if (state) return state
    return a.created_at < b.created_at ? -1 : a.created_at > b.created_at ? 1 : 0
  })
}

// mustHandle: the cards the developer handles one by one (ADR §7, 1–3).
export function mustHandle(card: FocusCard): boolean {
  return (card.kind === 'gate' || card.kind === 'decision' || card.kind === 'finding') && card.state !== 'decided'
}

// acknowledgeable: the cards "acknowledge the remaining n" takes together.
export function acknowledgeable(cards: FocusCard[]): FocusCard[] {
  return cards.filter((c) => c.state !== 'decided' && (c.kind === 'report' || c.kind === 'change' || c.kind === 'failure' || c.kind === 'notice'))
}

// deckFor is one stage's cards in deck order.
export function deckFor(cards: FocusCard[], stage: FocusStageId): FocusCard[] {
  return orderCards(cards.filter((c) => c.stage === stage))
}

// --- Stepper ---

export type StepperItem = {
  id: FocusStageId
  label: string
  status: FocusStageStatus
  iteration: number
  current: boolean
}

export function stepperItems(p: FocusPipeline, labels: FocusTranslations['stages'] = focusEn.stages): StepperItem[] {
  return p.stages.map((s) => ({
    id: s.id,
    label: labels[s.id] ?? s.id,
    status: s.status,
    iteration: s.iteration,
    current: s.id === p.current,
  }))
}

// pipelinePhase mirrors Pipeline.Active in Go: a pipeline is active while
// its current stage is. A blocked current stage is stopped, unless a blocked
// gate waits for the developer (P2); a done one means every stage is passed.
export type PipelinePhase = 'active' | 'blocked' | 'stopped' | 'finished'

export function pipelinePhase(p: FocusPipeline): PipelinePhase {
  const stage = p.stages.find((s) => s.id === p.current)
  switch (stage?.status) {
    case 'active':
      return 'active'
    case 'blocked':
      return p.open_gate ? 'blocked' : 'stopped'
    default:
      return 'finished'
  }
}

// --- Progress line ---

const editTools = new Set(['write_file', 'edit_file', 'apply_patch', 'write', 'edit', 'multiedit', 'notebookedit'])
const readTools = new Set(['read_file', 'read', 'grep', 'glob', 'ls', 'list_dir', 'list_files', 'search', 'webfetch', 'websearch'])
const testCommand = /\b(go test|npm (run )?(test|check)|npx (vitest|jest|playwright)|vitest|jest|pytest|playwright|cargo test|make (test|check|lint|vet)[\w-]*)\b/

type RunningTool = { name: string; args: Record<string, unknown> }

function parseArgs(preview?: string): Record<string, unknown> {
  if (!preview) return {}
  try {
    const parsed = JSON.parse(preview)
    return parsed && typeof parsed === 'object' ? (parsed as Record<string, unknown>) : {}
  } catch {
    return {}
  }
}

function argString(args: Record<string, unknown>, ...keys: string[]): string {
  for (const key of keys) {
    const value = args[key]
    if (typeof value === 'string' && value.trim()) return value.trim()
  }
  return ''
}

function baseName(path: string): string {
  return path.split(/[\\/]/).filter(Boolean).pop() ?? path
}

function describeTool(tool: RunningTool, text: FocusTranslations['progress']): string {
  const name = tool.name.toLowerCase()
  const command = argString(tool.args, 'command', 'cmd')
  if (command && testCommand.test(command)) return text.runningTests
  if (editTools.has(name)) {
    const path = argString(tool.args, 'file_path', 'path', 'notebook_path')
    return path ? text.editing(baseName(path)) : text.running(tool.name)
  }
  if (readTools.has(name)) return text.reading
  return text.running(tool.name)
}

// progressLine is the one line shown while a turn runs: what the stage is
// doing, how many files changed so far, and what is happening right now —
// "Implementing · 3 files changed · running tests".
export function progressLine(
  events: ChatEvent[],
  options: { stage?: FocusStageId; text?: FocusTranslations['progress'] } = {},
): string {
  const text = options.text ?? focusEn.progress
  const files = new Set<string>()
  let checkpointFiles = 0
  const running = new Map<string, RunningTool>()
  let waiting = false
  let last: 'writing' | 'thinking' | '' = ''
  for (const event of events) {
    switch (event.type) {
      case 'turn_started':
        running.clear()
        waiting = false
        last = ''
        break
      case 'file_change':
        if (event.path) files.add(event.path)
        break
      case 'checkpoint':
        checkpointFiles = Math.max(checkpointFiles, event.files ?? 0)
        break
      case 'status':
        if (event.phase === 'before_tool_call' && event.tool_name) {
          running.set(event.tool_call_id || event.tool_name, { name: event.tool_name, args: parseArgs(event.tool_args_preview) })
        } else if (event.phase === 'after_tool_call') {
          running.delete(event.tool_call_id || event.tool_name || '')
        }
        break
      case 'provider_tool':
        if (event.tool_name) running.set(event.tool_call_id || event.tool_name, { name: event.tool_name, args: parseArgs(event.tool_args_preview) })
        break
      case 'provider_tool_result': {
        const id = event.tool_call_id || ''
        const tool = running.get(id)
        if (tool && editTools.has(tool.name.toLowerCase()) && !event.tool_is_error) {
          const path = argString(tool.args, 'file_path', 'path', 'notebook_path')
          if (path) files.add(path)
        }
        running.delete(id)
        break
      }
      case 'permission_request':
        waiting = true
        break
      case 'permission_resolved':
        waiting = false
        break
      case 'delta':
        if (event.text) last = 'writing'
        break
      case 'reasoning_delta':
        if (event.text) last = 'thinking'
        break
    }
  }
  const parts = [options.stage ? text.stages[options.stage] : text.working]
  const changed = Math.max(files.size, checkpointFiles)
  if (changed > 0) parts.push(text.filesChanged(changed))
  if (waiting) {
    parts.push(text.waiting)
  } else if (running.size > 0) {
    parts.push(describeTool([...running.values()].at(-1)!, text))
  } else if (last) {
    parts.push(text[last])
  }
  return parts.join(' · ')
}

// --- Hidden blocks ---

const stageOpen = '<focus-stage>'
const stageClose = '</focus-stage>'

// stripFocusStage takes the server's stage guidance off a stored user
// message: only the block the server appends, the trailing
// "\n\n<focus-stage>\n…</focus-stage>" (like splitReviewNotes). The user's
// own mention of the tag stays.
export function stripFocusStage(text: string): string {
  const trimmed = text.trimEnd()
  if (!trimmed.endsWith(stageClose)) return text
  const at = trimmed.lastIndexOf(`\n\n${stageOpen}\n`)
  if (at >= 0) return trimmed.slice(0, at)
  // A turn of guidance alone (an empty message).
  return trimmed.trimStart().startsWith(`${stageOpen}\n`) && trimmed.indexOf(stageOpen) === trimmed.lastIndexOf(stageOpen) ? '' : text
}

const tagPattern = /<(\/?)focus-([a-z_]+)>/g
const partialTag = /<(?:\/?(?:f(?:o(?:c(?:u(?:s(?:-[a-z_]*)?)?)?)?)?)?)?$/

// unfencedSegments are the [start, end) spans outside ``` fences, as the
// server's parser sees them; text after an unclosed fence is not one.
function unfencedSegments(text: string): [number, number][] {
  const segs: [number, number][] = []
  let inFence = false
  let segStart = 0
  let pos = 0
  while (pos <= text.length) {
    const lineEnd = text.indexOf('\n', pos)
    const line = lineEnd >= 0 ? text.slice(pos, lineEnd) : text.slice(pos)
    const next = lineEnd >= 0 ? lineEnd + 1 : text.length + 1
    if (line.trim().startsWith('```')) {
      if (inFence) segStart = Math.min(next, text.length)
      else segs.push([segStart, pos])
      inFence = !inFence
    }
    pos = next
  }
  if (!inFence) segs.push([segStart, text.length])
  return segs
}

// stripFocusBlocks folds the <focus-*> blocks out of assistant text so a
// bubble shows only the prose. It pairs tags as the server's scanBlocks
// does — a close tag closes the nearest preceding open tag of its kind — so
// a mention of a tag before or after the real block stays, as does anything
// inside code fences. While the message is still streaming, a block opened
// but not yet closed is hidden to the end.
export function stripFocusBlocks(text: string, options: { streaming?: boolean } = {}): string {
  if (!text.includes('<focus-') && !options.streaming) return text
  const spans: [number, number][] = []
  let tailOpen = -1
  for (const [segStart, segEnd] of unfencedSegments(text)) {
    const part = text.slice(segStart, segEnd)
    let openTag = ''
    let openStart = -1
    for (const m of part.matchAll(tagPattern)) {
      const tag = m[2]
      if (m[1] === '') {
        openTag = tag
        openStart = m.index
        continue
      }
      if (openStart < 0 || tag !== openTag) continue
      spans.push([segStart + openStart, segStart + m.index + m[0].length])
      openTag = ''
      openStart = -1
    }
    if (segEnd === text.length && openStart >= 0) tailOpen = segStart + openStart
  }
  let end = text.length
  if (options.streaming) {
    if (tailOpen >= 0) {
      end = tailOpen
    } else {
      const partial = text.match(partialTag)
      if (partial && partial[0].length > 1) end = text.length - partial[0].length
    }
  }
  if (spans.length === 0 && end === text.length) return text
  let out = ''
  let last = 0
  for (const [from, to] of spans) {
    if (from >= end) break
    out += text.slice(last, from)
    last = to
  }
  out += text.slice(last, Math.max(last, end))
  return out.replace(/\n{3,}/g, '\n\n').trim()
}

// --- Transcript turns ---

// turnSlice is the transcript of one turn: the turn-th user message (1-based)
// and everything until the next one.
export function turnSlice(history: SessionMessage[], turn: number): SessionMessage[] {
  if (turn < 1) return []
  let count = 0
  let start = -1
  for (let i = 0; i < history.length; i++) {
    if (history[i].role !== 'user') continue
    count++
    if (count === turn) {
      start = i
    } else if (start >= 0) {
      return history.slice(start, i)
    }
  }
  return start >= 0 ? history.slice(start) : []
}

// turnIndex is the 1-based turn number of a user message, 0 when not found.
export function turnIndex(history: SessionMessage[], messageId: string): number {
  let count = 0
  for (const message of history) {
    if (message.role !== 'user') continue
    count++
    if (message.id === messageId) return count
  }
  return 0
}

const stageLine = /<focus-stage>[\s\S]*?current stage: ([a-z_]+)/

// turnStage reads which stage a user message's guidance was written for.
export function turnStage(content: string): FocusStageId | null {
  const match = content.match(stageLine)
  return match ? (match[1] as FocusStageId) : null
}

// --- Change cards ---

export type ChangeTurn = {
  turnId: string
  turn: number
  stage: FocusStageId
  at: string
  files: { path: string; status: string; additions: number; deletions: number; binary?: boolean; patch?: string }[]
}

export function changeCardId(turnId: string, path: string): string {
  return `change:${turnId}:${path}`
}

// changeCards turns checkpoint diffs into one change card per file. The
// server keeps no change cards (P1b), so acknowledgements are the console's.
export function changeCards(turns: ChangeTurn[], acknowledged: ReadonlySet<string>): FocusCard[] {
  const out: FocusCard[] = []
  for (const t of turns) {
    for (const file of t.files) {
      const id = changeCardId(t.turnId, file.path)
      const payload: FocusChangePayload = {
        turn_id: t.turnId,
        path: file.path,
        status: file.status,
        additions: file.additions,
        deletions: file.deletions,
        binary: file.binary,
        patch: file.patch,
      }
      out.push({
        id,
        kind: 'change',
        stage: t.stage,
        turn: t.turn,
        title: file.path,
        payload,
        state: acknowledged.has(id) ? 'decided' : 'unseen',
        created_at: t.at,
      })
    }
  }
  return out
}

// --- Plan gate ---

// planEdits is the plan G1 approves: stages the developer unticked are left
// out (plan always stays), and verification commands are one per line.
export function planEdits(plan: FocusPlan, edits: { skipped: ReadonlySet<string>; verify: string }): FocusPlan {
  return {
    ...plan,
    tasks: [...plan.tasks],
    stages: plan.stages.filter((s) => s === 'plan' || !edits.skipped.has(s)),
    verify: edits.verify.split('\n').map((line) => line.trim()).filter(Boolean),
  }
}

// --- Default mode ---

// defaultModeRedirect is where the console lands instead of the board when
// console_default_mode is focus. Only the landing redirects: the board stays
// reachable from the nav.
export function defaultModeRedirect(mode: unknown, route: { view: string }): string | null {
  return mode === 'focus' && route.view === 'board' ? '/console/focus' : null
}

// --- Deck cursor ---

// deckCursor is the card the deck shows after the deck changed: the pinned
// card stays on screen while cards reorder under it (being seen moves a
// card back), and the deck goes to its first card when a new card arrives,
// the pinned one left, or none is pinned.
export function deckCursor(known: ReadonlySet<string>, ids: string[], current: string | null): string | null {
  if (ids.length === 0) return null
  if (ids.some((id) => !known.has(id))) return ids[0]
  if (current && ids.includes(current)) return current
  return ids[0]
}

// onboardingModeUpdate is the config write that finishing the onboarding
// wizard makes: focus becomes the default mode only when none is set, so a
// mode the user chose (advanced included) is never overwritten. null values
// (config unreadable) mean unknown: write nothing.
export function onboardingModeUpdate(values: Record<string, unknown> | null | undefined): Record<string, string> | null {
  if (!values) return null
  const mode = values.console_default_mode
  if (typeof mode === 'string' && mode.trim() !== '') return null
  return { console_default_mode: 'focus' }
}

// deckOrder is the order the deck shows its cards in: the sorted order,
// held while the same cards stay in the deck, so a card marked seen (which
// sorts it back) does not move under the developer and ←/→ step one card
// at a time through what is on screen. Cards arriving or leaving re-sort.
export function deckOrder(shown: string[], sorted: string[]): string[] {
  if (shown.length !== sorted.length) return sorted
  const now = new Set(sorted)
  return shown.every((id) => now.has(id)) ? shown : sorted
}

// focusChromeHidden: focus mode hides the app sidebar and the companion
// (ADR §3); its own header leads back to the tasks, Advanced and the board.
export function focusChromeHidden(route: { view: string }): boolean {
  return route.view === 'focus'
}
