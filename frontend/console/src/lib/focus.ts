// Focus mode (docs/decisions/focus-mode.md): pure helpers for the pipeline
// screen — deck ordering, the stepper, the one-line progress of a running
// turn, folding the hidden <focus-*> blocks out of chat text, and mapping
// cards back to the transcript turn they came from. Tested under Node.
import { focusEn, type FocusTranslations } from '../i18n/sections/focus.ts'
import { userVisibleText } from './consoleContext.ts'
import type {
  ChatEvent,
  FocusCard,
  FocusCardKind,
  FocusChangePayload,
  FocusChangeTurnPayload,
  FocusPipeline,
  FocusPlan,
  FocusStage,
  FocusStageId,
  FocusStageKind,
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
// before seen; then cards handled one by one (gate, decision, finding) oldest
// first — in the order they were asked — and informational cards newest
// first, so the latest report leads even after both were seen (a card on
// screen is marked seen at once).
// openGateCardId is the open gate's card: the latest undecided gate card,
// as the server picks it (machine.go openGateCard). undefined when no gate
// is open.
export function openGateCardId(p: Pick<FocusPipeline, 'open_gate' | 'cards'>): string | undefined {
  if (!p.open_gate) return undefined
  for (let i = p.cards.length - 1; i >= 0; i--) {
    const c = p.cards[i]
    if (c.kind === 'gate' && c.state !== 'decided') return c.id
  }
  return undefined
}

export function orderCards(cards: FocusCard[]): FocusCard[] {
  return [...cards].sort((a, b) => {
    const decided = Number(a.state === 'decided') - Number(b.state === 'decided')
    if (decided) return decided
    const kind = (kindRank[a.kind] ?? 9) - (kindRank[b.kind] ?? 9)
    if (kind) return kind
    const state = (stateRank[a.state] ?? 0) - (stateRank[b.state] ?? 0)
    if (state) return state
    const older = a.created_at < b.created_at ? -1 : a.created_at > b.created_at ? 1 : 0
    return handledInOrder.has(a.kind) ? older : -older
  })
}

const handledInOrder = new Set<FocusCardKind>(['gate', 'decision', 'finding'])

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

const stageKinds: readonly string[] = ['plan', 'build', 'review', 'pr', 'pr_review', 'merge']

// stageKindOf is the behaviour a stage runs with: its kind, or its id when
// that is a development stage; null for an id the pipeline does not hold.
export function stageKindOf(stages: Pick<FocusStage, 'id' | 'kind'>[] | undefined, id: FocusStageId | null | undefined): FocusStageKind | null {
  const stage = stages?.find((s) => s.id === id)
  if (stage?.kind) return stage.kind
  return id && stageKinds.includes(id) ? (id as FocusStageKind) : null
}

type TemplateText = { name: string; description: string; stages: Record<string, string> }
export type StageLabelText = { stages: FocusTranslations['stages']; templates?: FocusTranslations['templates'] }

// templateText is the console's own wording of a built-in template, when
// it has one; a workspace template is shown as written.
export function templateText(id: string | undefined, templates: FocusTranslations['templates'] | undefined): TemplateText | null {
  return (templates as Record<string, TemplateText> | undefined)?.[id || 'dev'] ?? null
}

// stageLabel names a stage: the console's wording for a built-in template's
// stage, else the template's own label, else the development stage's name,
// else the id.
export function stageLabel(
  p: { stages?: Pick<FocusStage, 'id' | 'label'>[]; template?: string } | null | undefined,
  id: FocusStageId,
  text: StageLabelText,
): string {
  const own = p?.template ? templateText(p.template, text.templates)?.stages[id] : undefined
  if (own) return own
  const label = p?.stages?.find((s) => s.id === id)?.label
  if (label) return label
  return (text.stages as Record<string, string>)[id] ?? id
}

export type StepperItem = {
  id: FocusStageId
  label: string
  status: FocusStageStatus
  iteration: number
  current: boolean
}

export function stepperItems(
  p: FocusPipeline,
  labels: FocusTranslations['stages'] = focusEn.stages,
  templates: FocusTranslations['templates'] = focusEn.templates,
): StepperItem[] {
  return p.stages.map((s) => ({
    id: s.id,
    label: stageLabel(p, s.id, { stages: labels, templates }),
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
  // lead replaces the stage's phrase: a template's stage has no "Implementing".
  options: { stage?: FocusStageKind; text?: FocusTranslations['progress']; lead?: string } = {},
): string {
  const text = options.text ?? focusEn.progress
  const files = new Set<string>()
  let checkpointFiles = 0
  const running = new Map<string, RunningTool>()
  let waiting = false
  let last: 'writing' | 'thinking' | '' = ''
  // The focus driver's verification step (P2), when the feed is one.
  let verify: ChatEvent | null = null
  for (const event of events) {
    switch (event.type) {
      case 'focus_progress':
        verify = event
        break
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
  const parts = [options.lead || (options.stage ? text.stages[options.stage] : text.working)]
  const changed = Math.max(files.size, checkpointFiles)
  if (changed > 0) parts.push(text.filesChanged(changed))
  if (verify) {
    const at = verify.index ?? 0
    const of = verify.total ?? 0
    if (verify.phase === 'verifying') {
      parts.push(text.verifying(verify.command ?? '', at, of))
    } else if (verify.e2e_skipped) {
      parts.push(text.verifiedSkipped(at, of))
    } else {
      parts.push(text.verified(at, of))
    }
  } else if (waiting) {
    parts.push(text.waiting)
  } else if (running.size > 0) {
    parts.push(describeTool([...running.values()].at(-1)!, text))
  } else if (last) {
    parts.push(text[last])
  }
  return parts.join(' · ')
}

// --- Q&A (ADR §8) ---

// One question about a card and its answer ('' while it is being answered).
export type QAEntry = { turn: number; question: string; answer: string }

// qaThreads threads the Q&A session's transcript by card: qaTurns maps a
// card id to the 1-based user turns asked about it. The question shows as
// the developer typed it, without the card context the server added; the
// answer is the turn's last assistant reply. Turns the history does not
// have yet are left out.
export function qaThreads(history: SessionMessage[], qaTurns: Record<string, number[]> | undefined): Record<string, QAEntry[]> {
  // The server names the card in each question's context (focus-card: id);
  // older questions without it fall back to qa_turns' turn numbers, which
  // shift once the transcript compacts.
  const byTurn = new Map<number, string>()
  for (const [cardId, list] of Object.entries(qaTurns ?? {})) for (const n of list) byTurn.set(n, cardId)
  const out: Record<string, QAEntry[]> = {}
  for (const cardId of Object.keys(qaTurns ?? {})) out[cardId] = []
  let turn = 0
  let entry: QAEntry | null = null
  for (const m of history) {
    if (m.role === 'user') {
      turn++
      const cardId = qaCardMarker.exec(m.content)?.[1] ?? byTurn.get(turn)
      entry = cardId ? { turn, question: userVisibleText(m.content).trim(), answer: '' } : null
      if (cardId && entry) (out[cardId] ??= []).push(entry)
    } else if (m.role === 'assistant' && entry && m.content.trim()) {
      entry.answer = m.content.trim()
    }
  }
  return out
}

const qaCardMarker = /<console-context>\s*\nfocus-card: (\S+)/

const promoteAnswerChars = 1200

// promoteDraft is the stage instruction an answer becomes when promoted:
// the developer edits it before sending (ADR §8).
export function promoteDraft(cardTitle: string, entry: QAEntry, text: FocusTranslations['qa'] = focusEn.qa): string {
  let answer = entry.answer.trim()
  if (answer.length > promoteAnswerChars) answer = answer.slice(0, promoteAnswerChars - 1).trimEnd() + '…'
  return text.promoteDraft(cardTitle, answer)
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
// The start of a focus tag cut off by streaming: "</", or "<" or "</"
// followed by a prefix of "focus-<name>". Every alternative is non-empty.
const partialTag = /<(?:\/|\/?(?:f|fo|foc|focu|focus|focus-[a-z_]+|focus-))$/

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

function validJSON(text: string): boolean {
  try {
    JSON.parse(text)
    return true
  } catch {
    return false
  }
}

// stripFocusBlocks folds the <focus-*> blocks out of assistant text so a
// bubble shows only the prose. It pairs tags as the server's scanBlocks
// does — a close tag closes an open tag of its kind before it (after the
// previous block): the nearest whose body is valid JSON, else the nearest —
// so a mention of a tag before or after the real block stays, a tag quoted
// inside a block's JSON never splits it, and anything inside code fences
// stays. While the message is still streaming, a block opened but not yet
// closed is hidden to the end.
export function stripFocusBlocks(text: string, options: { streaming?: boolean } = {}): string {
  if (!text.includes('<focus-') && !options.streaming) return text
  const spans: [number, number][] = []
  let tailOpen = -1
  for (const [segStart, segEnd] of unfencedSegments(text)) {
    const part = text.slice(segStart, segEnd)
    let opens = new Map<string, [number, number][]>()
    for (const m of part.matchAll(tagPattern)) {
      const tag = m[2]
      if (m[1] === '') {
        opens.set(tag, [...(opens.get(tag) ?? []), [m.index, m.index + m[0].length]])
        continue
      }
      const candidates = opens.get(tag) ?? []
      if (candidates.length === 0) continue
      let open = candidates[candidates.length - 1]
      for (let i = candidates.length - 1; i >= 0; i--) {
        if (validJSON(part.slice(candidates[i][1], m.index).trim())) {
          open = candidates[i]
          break
        }
      }
      spans.push([segStart + open[0], segStart + m.index + m[0].length])
      opens = new Map()
    }
    if (segEnd === text.length) {
      const pending = [...opens.values()].flat().map(([start]) => start)
      if (pending.length) tailOpen = segStart + Math.max(...pending)
    }
  }
  let end = text.length
  if (options.streaming) {
    if (tailOpen >= 0) {
      end = tailOpen
    } else {
      const partial = text.match(partialTag)
      if (partial) end = text.length - partial[0].length
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

const stageLine = /<focus-stage>[\s\S]*?current stage: ([a-z][a-z0-9_]*)/

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

export function changeCardId(turnId: string, path?: string): string {
  return path === undefined ? `change:${turnId}` : `change:${turnId}:${path}`
}

// changeCards turns checkpoint diffs into one change card per turn (U1: one
// card per file buried the deck in dozens of cards): the card lists the
// turn's files and shows their diffs one at a time, and is acknowledged
// as a whole. The server keeps no change cards (P1b), so acknowledgements
// are the console's; ones kept per file before still count when every file
// of the turn was acknowledged.
export function changeCards(turns: ChangeTurn[], acknowledged: ReadonlySet<string>): FocusCard[] {
  const out: FocusCard[] = []
  for (const t of turns) {
    if (t.files.length === 0) continue
    const files: FocusChangePayload[] = t.files.map((file) => ({
      turn_id: t.turnId,
      path: file.path,
      status: file.status,
      additions: file.additions,
      deletions: file.deletions,
      binary: file.binary,
      patch: file.patch,
    }))
    const payload: FocusChangeTurnPayload = {
      turn_id: t.turnId,
      additions: files.reduce((n, f) => n + f.additions, 0),
      deletions: files.reduce((n, f) => n + f.deletions, 0),
      files,
    }
    const id = changeCardId(t.turnId)
    const acked = acknowledged.has(id) || files.every((f) => acknowledged.has(changeCardId(t.turnId, f.path)))
    out.push({
      id,
      kind: 'change',
      stage: t.stage,
      turn: t.turn,
      title: files.length === 1 ? files[0].path : `${files.length} files`,
      payload,
      state: acked ? 'decided' : 'unseen',
      created_at: t.at,
    })
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
export type DeckCursorCard = Pick<FocusCard, 'id' | 'kind' | 'state'>

export function deckCursor(known: ReadonlySet<string>, cards: DeckCursorCard[], current: string | null): string | null {
  if (cards.length === 0) return null
  // cards are in deck order, so the first newcomer is the most important.
  const newcomer = cards.find((c) => !known.has(c.id))
  const reading = current ? cards.find((c) => c.id === current) : undefined
  if (!reading) return (newcomer ?? cards[0]).id
  if (!newcomer) return reading.id
  // U2: a newcomer takes the screen only when it outranks the card being
  // read — a gate over a report, a new report over one already seen, any
  // card over one already handled — never because derived cards (changes)
  // finished loading under it.
  const rank = (c: DeckCursorCard) => kindRank[c.kind] ?? 9
  if (reading.state === 'decided' || rank(newcomer) < rank(reading)) return newcomer.id
  if (rank(newcomer) === rank(reading) && reading.state !== 'unseen') return newcomer.id
  return reading.id
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
// focusOwnsShortcut: on the pipeline screen `?` asks about the card on
// screen (ADR §7), so the global shortcut help yields it there.
export function focusOwnsShortcut(action: string, route: { view: string; sessionId?: string }): boolean {
  return action === 'help' && route.view === 'focus' && !!route.sessionId
}

export function focusChromeHidden(route: { view: string }): boolean {
  return route.view === 'focus'
}

// Triage progress (P3): how many finding cards of the open triage gate are
// decided, or null when no triage is open.
export function triageProgress(p: Pick<FocusPipeline, 'open_gate' | 'cards' | 'review'> | null | undefined): { decided: number; total: number } | null {
  const ids = p?.review?.triage
  if (!p || p.open_gate !== 'triage' || !ids?.length) return null
  const decided = new Set(p.cards.filter((c) => c.state === 'decided').map((c) => c.id))
  return { decided: ids.filter((id) => decided.has(id)).length, total: ids.length }
}

export type ExcerptLine = {
  kind: 'hunk' | 'add' | 'del' | 'context' | 'meta'
  text: string
  // The new-file line number (added and context lines only).
  line?: number
  // The finding's own line.
  target: boolean
}

const excerptHunk = /^@@ .*?\+(\d+)/

// excerptLines reads a finding's diff excerpt (unified hunks from the
// server) into lines with their new-file numbers, marking the finding line.
export function excerptLines(excerpt: string, target: number): ExcerptLine[] {
  if (!excerpt) return []
  let next = 0
  return excerpt.split('\n').map((text): ExcerptLine => {
    const hunk = excerptHunk.exec(text)
    if (hunk) {
      next = Number(hunk[1])
      return { kind: 'hunk', text, target: false }
    }
    switch (text[0]) {
      case '+':
      case ' ': {
        const line = next++
        return { kind: text[0] === '+' ? 'add' : 'context', text, line, target: target > 0 && line === target }
      }
      case '-':
        return { kind: 'del', text, target: false }
      default:
        return { kind: 'meta', text, target: false }
    }
  })
}

/** What a report card shows: a heading (null = the generic label) and the body under it. */
export interface ReportCardText {
  heading: string | null
  body: string
}

/**
 * Splits a report card's text so the summary never shows twice (#1109). The
 * server titles a report with the summary's first line, clipped to 120
 * characters with "…" (`reportTitle` in internal/focuspipeline/machine.go).
 * When the title is the whole first line, the body is the lines after it.
 * When it is a clipped start of the first line, the heading becomes the
 * generic label (null) and the body holds the whole summary. Works from the
 * stored title, so cards saved by earlier versions render the same way.
 */
export function reportCardText(title: string, summary: string | undefined | null): ReportCardText {
  const text = (summary ?? '').trim()
  if (!text) return { heading: title, body: '' }
  const nl = text.indexOf('\n')
  const first = (nl < 0 ? text : text.slice(0, nl)).trim()
  // The server does not trim the first line, so a stored title can end with
  // a Markdown hard break's spaces or a CRLF's \r.
  const stored = title.trim()
  if (first === stored) {
    return { heading: stored, body: nl < 0 ? '' : text.slice(nl + 1).trim() }
  }
  if (stored.endsWith('…') && first.startsWith(stored.slice(0, -1))) return { heading: null, body: text }
  return { heading: title, body: text }
}
