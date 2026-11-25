// The demo desk: a desktop that lives inside the app, for someone who has no
// TARS desktop paired yet (and for App Store review, which cannot join the
// owner's tailnet). It answers the same routes a paired desktop does — the
// ones in the server's focus table — from memory, so every screen is the
// real one: the Waiting list, a pipeline with its gates and cards, sending
// an idea as a task, and the vibe (easy) projects a paired desk now answers
// a phone (vibe redesign unit 6). Nothing leaves the phone, no model runs, and its state
// starts over each time the app starts.
//
// It follows the pipeline's own order (plan gate → build → triage → fix →
// PR gate → merge gate → finished) with fixed text. A "turn" is a few
// seconds during which the pipeline's session is listed as running and its
// turn feed streams a few lines; what the turn ends in was decided when it
// started. Time comes in through `now`, so tests run it without waiting.
import type { EasyProject, EasyReadiness, EasyVersion } from '../easy.ts'
import type { EasyBoard, EasyBoardState, EasyListed } from '../easyBoard.ts'
import type { QueuedApproval } from '../sessionBoard.ts'
import type {
  FocusCard,
  FocusFinding,
  FocusGate,
  FocusListItem,
  FocusPipeline,
  FocusPlan,
  FocusPRDraft,
  FocusStage,
  FocusStageId,
  FocusTemplate,
  Session,
  SessionMessage,
} from '../types.ts'
import type { PairedDesktop } from './desktops.ts'

// .invalid never resolves (RFC 2606): a request that misses the handler
// fails instead of reaching someone's server.
export const demoBaseURL = 'https://demo-desk.invalid'
export const demoDesktopName = 'Demo desk'

export function isDemoDesktop(desktop: Pick<PairedDesktop, 'baseURL'>): boolean {
  return desktop.baseURL === demoBaseURL
}

export function demoDesktop(now: Date): PairedDesktop {
  return { id: demoBaseURL, name: demoDesktopName, baseURL: demoBaseURL, token: 'demo', deviceID: 'demo', pairedAt: now.toISOString() }
}

export type DemoDesk = {
  // handle answers one request the way fetch would.
  handle(path: string, init?: RequestInit): Promise<Response>
}

type Turn = 'planning' | 'building' | 'fixing' | 'opening' | 'merging' | 'typed'

type Content = {
  plan: FocusPlan
  built: string
  findings: FocusFinding[]
  fixed: string
  draft: FocusPRDraft
  prNumber: number
}

type Running = { turn: Turn; startedAt: number; until: number; lines: string[] }

type Demo = {
  pipeline: FocusPipeline
  title: string
  folder: string
  content: Content
  history: SessionMessage[]
  qaHistory: SessionMessage[]
  running: Running | null
  // stopped is the turn a stop cut off, for resume to run again.
  stopped?: { turn: Turn; prompt: string; lines: string[] }
  seq: number
}

const devStages: FocusStageId[] = ['plan', 'build', 'review', 'pr', 'pr_review', 'merge']
const folders = [
  { path: '/Volumes/code/storefront', name: 'storefront', exists: true },
  { path: '/Volumes/code/notes-app', name: 'notes-app', exists: true },
]
const templates: FocusTemplate[] = [
  { id: 'dev', name: 'Development', description: 'Plan, build, review, pull request, CI review, merge.', builtin: true, stages: devStages.map((id) => ({ id })) },
]

export function createDemoDesk(options: { now?: () => number; turnMs?: number } = {}): DemoDesk {
  const now = options.now ?? (() => Date.now())
  const turnMs = options.turnMs ?? 6000
  const demos = new Map<string, Demo>()
  let approvals: QueuedApproval[] = []
  let created = 0

  const iso = () => new Date(now()).toISOString()

  function add(id: string, title: string, folder: string, content: Content): Demo {
    const pipeline: FocusPipeline = {
      version: 1,
      session_id: id,
      goal: content.plan.goal,
      stages: devStages.map((s, i): FocusStage => ({ id: s, status: i === 0 ? 'active' : 'pending', iteration: i === 0 ? 1 : 0 })),
      current: 'plan',
      cards: [],
      updated_at: iso(),
      template: 'dev',
    }
    const demo: Demo = { pipeline, title, folder, content, history: [], qaHistory: [], running: null, seq: 0 }
    demos.set(id, demo)
    return demo
  }

  // --- The pipeline's steps -------------------------------------------------

  function touch(d: Demo) {
    // Strictly later than the last read, so a screen holding the older
    // pipeline takes this one.
    const next = Math.max(now(), Date.parse(d.pipeline.updated_at) + 1)
    d.pipeline.updated_at = new Date(next).toISOString()
  }

  function stage(d: Demo, id: FocusStageId): FocusStage {
    return d.pipeline.stages.find((s) => s.id === id)!
  }

  function enter(d: Demo, id: FocusStageId) {
    const done = stage(d, d.pipeline.current)
    if (done.id !== id) done.status = 'done'
    const next = stage(d, id)
    next.status = 'active'
    next.iteration = Math.max(1, next.iteration)
    d.pipeline.current = id
  }

  function turnCount(d: Demo): number {
    return d.history.filter((m) => m.role === 'user').length
  }

  function card(d: Demo, kind: FocusCard['kind'], at: FocusStageId, title: string, payload: unknown): FocusCard {
    d.seq++
    const c: FocusCard = { id: `c${d.seq}`, kind, stage: at, turn: turnCount(d), title, payload, state: 'unseen', created_at: iso() }
    d.pipeline.cards.push(c)
    return c
  }

  function say(d: Demo, role: 'user' | 'assistant', content: string) {
    d.history.push({ id: `${d.pipeline.session_id}-m${d.history.length + 1}`, role, content, timestamp: iso() })
  }

  function openGate(d: Demo, gate: FocusGate) {
    d.pipeline.open_gate = gate
  }

  function start(d: Demo, turn: Turn, prompt: string, lines: string[]) {
    say(d, 'user', prompt)
    d.pipeline.pending_turn = prompt
    d.running = { turn, startedAt: now(), until: now() + turnMs, lines }
    touch(d)
  }

  // finish applies what the running turn ends in.
  function finish(d: Demo) {
    const running = d.running
    if (!running) return
    d.running = null
    delete d.pipeline.pending_turn
    const p = d.pipeline
    const c = d.content
    switch (running.turn) {
      case 'planning':
        say(d, 'assistant', `Here is the plan for "${c.plan.goal}".`)
        p.plan = structuredClone(c.plan)
        card(d, 'gate', 'plan', 'Approve plan', structuredClone(c.plan))
        openGate(d, 'plan')
        break
      case 'building': {
        say(d, 'assistant', c.built)
        stage(d, 'build').turns = 1
        card(d, 'report', 'build', c.built, { summary: c.built, tasks_done: true })
        enter(d, 'review')
        const ids = c.findings.map((f) => card(d, 'finding', 'review', f.title, f).id)
        if (ids.length > 0) {
          p.review = { triage: ids }
          openGate(d, 'triage')
        } else {
          draftPR(d)
        }
        break
      }
      case 'fixing':
        say(d, 'assistant', c.fixed)
        card(d, 'report', 'review', c.fixed, { summary: c.fixed })
        p.review = { fixed: true }
        draftPR(d)
        break
      case 'opening': {
        say(d, 'assistant', `Opened pull request #${c.prNumber}.`)
        const head = '4f2a9c1d8e7b6a5f4e3d2c1b0a9f8e7d6c5b4a39'
        p.pr = {
          number: c.prNumber,
          url: `https://github.com/demo/${d.folder}/pull/${c.prNumber}`,
          state: 'OPEN',
          merge_state: 'CLEAN',
          head_oid: head,
          head_ref: `tars/session-${p.session_id}`,
          checks: [
            { name: 'test', state: 'pass' },
            { name: 'lint', state: 'pass' },
            { name: 'build', state: 'pass' },
          ],
        }
        delete p.pr_wait
        enter(d, 'pr_review')
        enter(d, 'merge')
        const decided = p.cards.filter((x) => x.kind === 'finding' && x.state === 'decided')
        card(d, 'gate', 'merge', 'Merge pull request', {
          pr: p.pr,
          title: p.pr_draft?.title ?? c.draft.title,
          checks: { passed: 3, failed: 0, pending: 0 },
          fixed: decided.filter((x) => x.decision === 'fix').length,
          dismissed: decided.filter((x) => x.decision === 'dismiss').length,
          undecided: 0,
          merge_state: 'CLEAN',
        })
        openGate(d, 'merge')
        break
      }
      case 'merging':
        say(d, 'assistant', `Merged pull request #${c.prNumber}.`)
        if (p.pr) p.pr.state = 'MERGED'
        delete p.pr_wait
        stage(d, 'merge').status = 'done'
        p.worktree_end = { action: 'discard' }
        p.finished_at = iso()
        break
      case 'typed':
        say(d, 'assistant', 'Noted. This is the demo desk, so nothing was changed; on your own desktop the agent would act on this.')
        break
    }
    touch(d)
  }

  function draftPR(d: Demo) {
    enter(d, 'pr')
    d.pipeline.pr_draft = structuredClone(d.content.draft)
    card(d, 'gate', 'pr', 'Open pull request', structuredClone(d.content.draft))
    openGate(d, 'pr')
  }

  // settle ends every turn whose time has passed, the device chat session's
  // included.
  function settle() {
    for (const d of demos.values()) if (d.running && now() >= d.running.until) finish(d)
    chatSettle()
  }

  // --- Actions --------------------------------------------------------------

  class Refused extends Error {
    status: number
    pipeline?: FocusPipeline
    constructor(status: number, message: string, pipeline?: FocusPipeline) {
      super(message)
      this.status = status
      this.pipeline = pipeline
    }
  }

  function gateCard(d: Demo): FocusCard | undefined {
    return [...d.pipeline.cards].reverse().find((c) => c.kind === 'gate' && c.state !== 'decided')
  }

  function decide(c: FocusCard, decision: string) {
    c.state = 'decided'
    c.decision = decision
    c.decided_by = 'phone'
    c.decided_at = iso()
  }

  function stop(d: Demo) {
    const open = gateCard(d)
    if (open) decide(open, 'stop')
    const cut = d.running
    d.stopped = cut ? { turn: cut.turn, prompt: d.pipeline.pending_turn ?? '', lines: cut.lines } : undefined
    d.running = null
    delete d.pipeline.pending_turn
    delete d.pipeline.open_gate
    stage(d, d.pipeline.current).status = 'blocked'
    touch(d)
  }

  // resume follows the desk's rule (focuspipeline/resume.go): the gate the
  // stop closed opens again as a new card, or the stage goes on — the
  // turn the stop cut off runs again.
  function resume(d: Demo) {
    const p = d.pipeline
    const s = stage(d, p.current)
    if (s.status !== 'blocked' || p.open_gate || d.running) throw new Refused(409, 'pipeline is not stopped', p)
    s.status = 'active'
    const last = [...p.cards].reverse().find((c) => c.kind === 'gate')
    const gates: Partial<Record<FocusStageId, FocusGate>> = { plan: 'plan', pr: 'pr', merge: 'merge' }
    const reopen = last?.decision === 'stop' && last.stage === p.current ? gates[p.current] : undefined
    if (last && reopen) {
      card(d, 'gate', last.stage, last.title, structuredClone(last.payload))
      openGate(d, reopen)
    } else if (d.stopped) {
      start(d, d.stopped.turn, `The developer resumed this pipeline. Continue the ${p.current} stage. ${d.stopped.prompt}`.trim(), d.stopped.lines)
    }
    d.stopped = undefined
    touch(d)
  }

  // cleanup mimics the real cleanup route: 409 while the pipeline is still
  // active or a gate is open, else it records a worktree_end for the
  // action asked (default "keep") like the real server's cleanupWorktree.
  function cleanup(d: Demo, action: string) {
    const ph = phase(d.pipeline)
    if (ph !== 'stopped' && ph !== 'finished') throw new Refused(409, 'the pipeline is still active', d.pipeline)
    d.pipeline.worktree_end = { action: action === 'discard' ? 'discard' : 'keep' }
  }

  function gate(d: Demo, gate: string, body: { action?: string; note?: string; edits?: FocusPlan; pr?: FocusPRDraft }) {
    const p = d.pipeline
    const open = gateCard(d)
    if (p.open_gate !== gate || !open || d.running) throw new Refused(409, 'this gate is no longer open', p)
    if (body.action === 'stop') return stop(d)
    const note = body.note?.trim()
    if (body.action === 'request_changes') {
      decide(open, 'request_changes')
      delete p.open_gate
      const prompt = note || 'Please revise.'
      if (gate === 'plan') return start(d, 'planning', prompt, ['Reading the note', 'Revising the plan'])
      if (gate === 'pr') return start(d, 'fixing', prompt, ['Reading the note', 'Revising the draft'])
      return start(d, 'opening', prompt, ['Reading the note', 'Pushing a fix', 'Waiting for checks'])
    }
    if (body.action !== 'approve') throw new Refused(400, 'unknown action')
    decide(open, 'approve')
    delete p.open_gate
    switch (gate) {
      case 'plan':
        if (body.edits) p.plan = body.edits
        enter(d, 'build')
        return start(d, 'building', 'Plan approved. Build it.', ['Reading the code', 'Editing files', 'Running the checks', 'Reviewing the change'])
      case 'pr':
        if (body.pr) p.pr_draft = body.pr
        p.pr_wait = 'open'
        return start(d, 'opening', 'Draft approved. Open the pull request.', ['Pushing the branch', 'Opening the pull request', 'Waiting for checks'])
      case 'merge':
        p.pr_wait = 'merge'
        return start(d, 'merging', 'Merge it.', ['Merging the pull request'])
      default:
        throw new Refused(400, 'unknown gate')
    }
  }

  function decideCard(d: Demo, cardID: string, body: { state?: string; decision?: string }) {
    const p = d.pipeline
    const c = p.cards.find((x) => x.id === cardID)
    if (!c) throw new Refused(404, 'no such card')
    if (body.state !== 'decided') {
      if (c.state === 'unseen') c.state = 'seen'
      return touch(d)
    }
    if (c.state === 'decided') throw new Refused(409, 'already decided', p)
    decide(c, body.decision ?? '')
    const triage = (p.review?.triage ?? []).map((id) => p.cards.find((x) => x.id === id)!)
    if (p.open_gate === 'triage' && triage.every((x) => x.state === 'decided')) {
      delete p.open_gate
      if (triage.some((x) => x.decision === 'fix')) {
        p.review = { fixing: true }
        return start(d, 'fixing', 'Fix the findings marked fix.', ['Reading the findings', 'Editing files', 'Running the checks'])
      }
      p.review = {}
      draftPR(d)
    }
    touch(d)
  }

  function addFinding(d: Demo, body: { title?: string; scenario?: string; file?: string; line?: number; severity?: string }) {
    const p = d.pipeline
    if (p.current !== 'review' || p.open_gate !== 'triage') throw new Refused(409, 'findings are added during review', p)
    const title = body.title?.trim()
    if (!title) throw new Refused(400, 'a finding needs a title')
    const finding: FocusFinding = {
      id: `dev${d.seq + 1}`, severity: body.severity || 'medium', file: body.file ?? '', line: body.line ?? 0,
      title, scenario: body.scenario ?? '', source: 'developer',
    }
    const c = card(d, 'finding', 'review', title, finding)
    p.review = { ...p.review, triage: [...(p.review?.triage ?? []), c.id] }
    touch(d)
  }

  function ask(d: Demo, body: { card_id?: string; question?: string }): { qa_session_id: string; turn: number } {
    const p = d.pipeline
    const cardID = body.card_id ?? ''
    const question = body.question?.trim()
    if (!question || !p.cards.some((c) => c.id === cardID)) throw new Refused(400, 'ask about a card')
    p.qa_session_id = `${p.session_id}-qa`
    const turn = d.qaHistory.filter((m) => m.role === 'user').length + 1
    d.qaHistory.push(
      { id: `${p.qa_session_id}-m${d.qaHistory.length + 1}`, role: 'user', content: question, timestamp: iso() },
      {
        id: `${p.qa_session_id}-m${d.qaHistory.length + 2}`, role: 'assistant', timestamp: iso(),
        content: 'On your own desktop the agent answers this from the code, read-only, in a side conversation that never changes the task. This is the demo desk, so the answer is fixed.',
      },
    )
    p.qa_turns = { ...p.qa_turns, [cardID]: [...(p.qa_turns?.[cardID] ?? []), turn] }
    touch(d)
    return { qa_session_id: p.qa_session_id, turn }
  }

  function create(body: { goal?: string; cwd?: string }): { session_id: string; pipeline: FocusPipeline; started: boolean } {
    const goal = body.goal?.trim()
    if (!goal) throw new Refused(400, 'a task needs a goal')
    const folder = folders.find((f) => f.path === body.cwd)
    if (!folder) throw new Refused(403, 'this folder is not registered for phones')
    created++
    const d = add(`demo-task-${created}`, goal.split('\n', 1)[0].slice(0, 80), folder.name, contentFor(goal, 140 + created))
    start(d, 'planning', goal, ['Reading the code', 'Drafting the plan'])
    return { session_id: d.pipeline.session_id, pipeline: d.pipeline, started: true }
  }

  // --- Reads ----------------------------------------------------------------

  function phase(p: FocusPipeline): NonNullable<FocusListItem['phase']> {
    const s = p.stages.find((x) => x.id === p.current)
    if (s?.status === 'active') return 'active'
    if (s?.status === 'blocked') return p.open_gate ? 'blocked' : 'stopped'
    return 'finished'
  }

  function listItem(d: Demo): FocusListItem {
    const p = d.pipeline
    const cards = p.cards.filter((c) => (c.kind === 'finding' || c.kind === 'decision') && c.state !== 'decided').length
    const live = phase(p) === 'active'
    return {
      session_id: p.session_id,
      title: d.title,
      goal: p.goal,
      current: p.current,
      open_gate: p.open_gate ?? '',
      needs_input: cards + (p.open_gate ? 1 : 0),
      updated_at: p.updated_at,
      template: 'dev',
      waiting: live ? { gate: p.open_gate ?? '', cards } : { cards: 0 },
      qa_session_id: p.qa_session_id,
      phase: phase(p),
    }
  }

  function session(d: Demo): Session {
    const dir = `/Volumes/code/${d.folder}`
    return {
      id: d.pipeline.session_id, title: d.title, kind: 'main', work_dirs: [dir], current_dir: dir,
      created_at: d.history[0]?.timestamp ?? d.pipeline.updated_at, updated_at: d.pipeline.updated_at,
    }
  }

  function activity() {
    const running = [...demos.values()]
      .filter((d) => d.running)
      .map((d) => ({ session_id: d.pipeline.session_id, session_title: d.title, started_at: new Date(d.running!.startedAt).toISOString() }))
    return { running, pending_approvals: [], queued_approvals: approvals }
  }

  // feed is the running turn as server-sent events: what it has said so far
  // at once, the rest as its time passes, closed when the turn ends.
  function feed(d: Demo): Response {
    const running = d.running!
    const encoder = new TextEncoder()
    const event = (data: unknown) => encoder.encode(`data: ${JSON.stringify(data)}\n\n`)
    const sessionID = d.pipeline.session_id
    const step = (running.until - running.startedAt) / (running.lines.length + 1)
    let timer: ReturnType<typeof setTimeout> | undefined
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(event({ type: 'turn_started', session_id: sessionID }))
        let sent = 0
        const pump = () => {
          while (sent < running.lines.length && now() >= running.startedAt + step * (sent + 1)) {
            controller.enqueue(event({ type: 'delta', session_id: sessionID, text: `${running.lines[sent]}.\n` }))
            sent++
          }
          if (now() >= running.until || d.running !== running) {
            settle()
            controller.enqueue(event({ type: 'done', session_id: sessionID }))
            controller.close()
            return
          }
          timer = setTimeout(pump, Math.max(50, Math.min(step, running.until - now())))
        }
        pump()
      },
      cancel() {
        if (timer) clearTimeout(timer)
      },
    })
    return new Response(body, { status: 200, headers: { 'Content-Type': 'text/event-stream' } })
  }

  // --- The device's own chat session (§8) -----------------------------------
  //
  // Not a pipeline: it never appears in /v1/focus/pipelines and its turn
  // never touches a stage, a card or a gate. There is exactly one — the
  // phone never names a session id for it, so neither does the demo.

  const chatSessionID = 'demo-chat'
  let chatHistory: SessionMessage[] = []
  let chatRunning: Running | null = null
  let chatSeq = 0

  function chatSay(role: 'user' | 'assistant', content: string) {
    chatSeq++
    chatHistory.push({ id: `${chatSessionID}-m${chatSeq}`, role, content, timestamp: iso() })
  }

  function startChatTurn(message: string) {
    chatSay('user', message)
    chatRunning = { turn: 'typed', startedAt: now(), until: now() + turnMs, lines: ['Reading the message'] }
  }

  function finishChatTurn() {
    if (!chatRunning) return
    chatRunning = null
    chatSay(
      'assistant',
      'This is the demo desk, so no real model answered — nothing typed here leaves this phone. Pair a real desktop to chat with TARS for real.',
    )
  }

  function chatSettle() {
    if (chatRunning && now() >= chatRunning.until) finishChatTurn()
  }

  function chatFeed(): Response {
    const running = chatRunning!
    const encoder = new TextEncoder()
    const event = (data: unknown) => encoder.encode(`data: ${JSON.stringify(data)}\n\n`)
    const step = (running.until - running.startedAt) / (running.lines.length + 1)
    let timer: ReturnType<typeof setTimeout> | undefined
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(event({ type: 'turn_started', session_id: chatSessionID }))
        let sent = 0
        const pump = () => {
          while (sent < running.lines.length && now() >= running.startedAt + step * (sent + 1)) {
            controller.enqueue(event({ type: 'delta', session_id: chatSessionID, text: `${running.lines[sent]}.\n` }))
            sent++
          }
          if (now() >= running.until || chatRunning !== running) {
            chatSettle()
            controller.enqueue(event({ type: 'done', session_id: chatSessionID }))
            controller.close()
            return
          }
          timer = setTimeout(pump, Math.max(50, Math.min(step, running.until - now())))
        }
        pump()
      },
      cancel() {
        if (timer) clearTimeout(timer)
      },
    })
    return new Response(body, { status: 200, headers: { 'Content-Type': 'text/event-stream' } })
  }

  // --- Vibe projects ----------------------------------------------------------
  //
  // The easy routes a phone reaches (vibe redesign unit 6). A project is a
  // record whose session is one of the demo pipelines above, so its plan
  // gate, questions and turn feed are the focus routes the app already
  // calls. The state is read from that pipeline the way the server's
  // easyproject.BoardOf reads it; a project whose pipeline is gone is done.

  // savedFor is the session whose finished work is the newest version.
  type EasyDemo = { project: EasyProject; versions: EasyVersion[]; archivedAt?: string; savedFor?: string }
  const easyProjects = new Map<string, EasyDemo>()
  let easyCreated = 0
  let easyCommits = 0

  function easyBoard(e: EasyDemo): EasyBoard {
    const d = e.project.session_id ? demos.get(e.project.session_id) : undefined
    const changedAt = d && d.pipeline.updated_at > e.project.updated_at ? d.pipeline.updated_at : e.project.updated_at
    const board = (state: EasyBoardState, extra: Partial<EasyBoard> = {}): EasyBoard => ({ state, changed_at: changedAt, ...extra })
    if (!d) return board('done')
    const p = d.pipeline
    if (p.open_gate === 'plan') return board('your_turn', { waiting: 'plan' })
    if (p.cards.some((c) => c.kind === 'decision' && c.state !== 'decided')) return board('your_turn', { waiting: 'question' })
    switch (phase(p)) {
      case 'active':
        return board('working')
      case 'finished':
        return board('done')
      default:
        return board('stuck', { stuck: 'stopped' })
    }
  }

  function easyListed(e: EasyDemo): EasyListed {
    return { ...e.project, board: easyBoard(e), ...(e.archivedAt ? { archived_at: e.archivedAt } : {}) }
  }

  function easyGet(id: string): EasyDemo {
    const e = easyProjects.get(id)
    if (!e) throw new Refused(404, 'no such project')
    return e
  }

  function easyIdle(e: EasyDemo): boolean {
    const state = easyBoard(e).state
    return state !== 'working' && state !== 'starting'
  }

  function easyFolder(name: string): string {
    return `~/TARS Projects/${name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '') || 'project'}`
  }

  function easyVersion(title: string, kind: EasyVersion['kind']): EasyVersion {
    easyCommits++
    return { commit: `demo${String(easyCommits).padStart(36, '0')}`, title, kind, at: iso() }
  }

  // easySave records the finished run's work as a version, once — the
  // server saves it when the pipeline finishes (focusDriver.saveVersionAfterFinish).
  function easySave(e: EasyDemo) {
    const id = e.project.session_id
    if (!id || e.savedFor === id || easyBoard(e).state !== 'done') return
    e.savedFor = id
    e.versions.unshift(easyVersion(e.project.changes?.at(-1)?.request ?? e.project.name, 'saved'))
  }

  function easyPipeline(goal: string, title: string, folder: string): Demo {
    created++
    const d = add(`demo-task-${created}`, title, folder, contentFor(goal, 140 + created))
    start(d, 'planning', goal, ['Reading the idea', 'Drafting the plan'])
    return d
  }

  function easyCreate(body: Record<string, unknown>): EasyProject {
    const idea = String(body.idea ?? '').trim()
    if (!idea) throw new Refused(400, 'idea_required')
    if ([...idea].length > 2000) throw new Refused(400, 'idea_too_long')
    easyCreated++
    const name = idea.split('\n', 1)[0].slice(0, 60)
    const folder = easyFolder(name)
    const d = easyPipeline(idea, name, folder)
    const project: EasyProject = {
      id: `demo-project-${easyCreated}`, name, idea, folder, status: 'ready', session_id: d.pipeline.session_id,
      created_at: iso(), updated_at: iso(),
    }
    easyProjects.set(project.id, { project, versions: [easyVersion('Start the project', 'start')] })
    return project
  }

  function easyList(url: URL) {
    const archived = url.searchParams.get('archived') === '1'
    const state = url.searchParams.get('state') ?? ''
    const all = [...easyProjects.values()]
    const shown = all.filter((e) => !!e.archivedAt === archived)
    const counts: Partial<Record<EasyBoardState, number>> = {}
    for (const e of shown) {
      const s = easyBoard(e).state
      counts[s] = (counts[s] ?? 0) + 1
    }
    const projects = shown
      .map(easyListed)
      .filter((p) => !state || p.board.state === state)
      .sort((a, b) => b.board.changed_at.localeCompare(a.board.changed_at))
    return { projects, counts, archived: all.filter((e) => e.archivedAt).length }
  }

  function easyUpdate(e: EasyDemo, body: Record<string, unknown>) {
    if (body.name === undefined && body.archived === undefined) throw new Refused(400, 'nothing_to_change')
    if (body.name !== undefined) {
      const name = String(body.name).trim().replace(/\s+/g, ' ')
      if (!name) throw new Refused(400, 'name_required')
      if ([...name].length > 60) throw new Refused(400, 'name_too_long')
      e.project.name = name
    }
    if (body.archived === true && !e.archivedAt) {
      if (!easyIdle(e)) throw new Refused(409, 'busy')
      e.archivedAt = iso()
    } else if (body.archived === false) {
      delete e.archivedAt
    }
    e.project.updated_at = iso()
    return { project: e.project, board: easyBoard(e) }
  }

  function easyChange(e: EasyDemo, body: Record<string, unknown>): EasyProject {
    const request = String(body.request ?? '').trim()
    if (!request) throw new Refused(400, 'change_required')
    if (easyBoard(e).state !== 'done') throw new Refused(409, 'busy')
    easySave(e)
    const d = easyPipeline(request, e.project.name, e.project.folder)
    e.project.session_id = d.pipeline.session_id
    e.project.changes = [...(e.project.changes ?? []), { request, session_id: d.pipeline.session_id, at: iso() }]
    e.project.updated_at = iso()
    return e.project
  }

  function easyVersions(e: EasyDemo): EasyVersion[] {
    easySave(e)
    return e.versions.map(({ current: _, ...v }, i) => (i === 0 ? { ...v, current: true } : v))
  }

  function easyRestore(e: EasyDemo, commit: string) {
    if (!easyIdle(e)) throw new Refused(409, 'busy')
    easySave(e)
    const target = e.versions.find((v) => v.commit === commit)
    if (!target) throw new Refused(404, 'not_found')
    const back = easyVersion(target.title, 'restored')
    e.versions.unshift(back)
    e.project.updated_at = iso()
    return { version: back, versions: easyVersions(e) }
  }

  const easyReadiness: EasyReadiness = { ready: true, engine: 'ok', engine_kind: 'claude-code-cli', tools: 'ok' }

  function easyRoute(url: URL, method: string, parts: string[], body: Record<string, unknown>): Response | null {
    if (parts[0] !== 'v1' || parts[1] !== 'easy') return null
    if (method === 'GET' && parts.length === 3 && parts[2] === 'readiness') return json({ readiness: easyReadiness })
    if (parts[2] !== 'projects') return null
    if (parts.length === 3) {
      if (method === 'GET') return json(easyList(url))
      if (method === 'POST') return json({ project: easyCreate(body) }, 202)
      return null
    }
    const e = easyGet(parts[3])
    const rest = parts.slice(4)
    if (rest.length === 0) {
      if (method === 'GET') return json({ project: e.project, board: easyBoard(e) })
      if (method === 'PATCH') return json(easyUpdate(e, body))
      // Deleting moves a folder on the desk: the desk does it, not a phone.
      if (method === 'DELETE') throw new Refused(403, 'a paired phone cannot delete a project')
      return null
    }
    // The app runs on the desk's own address (vibe redesign unit 7).
    if (rest[0] === 'preview') throw new Refused(403, 'a paired phone cannot show a preview')
    if (method === 'POST' && rest.length === 1 && rest[0] === 'changes') return json({ project: easyChange(e, body) }, 202)
    if (method === 'GET' && rest.length === 1 && rest[0] === 'versions') return json({ versions: easyVersions(e) })
    if (method === 'POST' && rest.length === 3 && rest[0] === 'versions' && rest[2] === 'restore') return json(easyRestore(e, rest[1]))
    return null
  }

  // --- Routes ---------------------------------------------------------------

  const json = (value: unknown, status = 200) =>
    new Response(JSON.stringify(value), { status, headers: { 'Content-Type': 'application/json' } })

  function bodyOf(init?: RequestInit): Record<string, unknown> {
    if (typeof init?.body !== 'string' || !init.body) return {}
    try {
      const value = JSON.parse(init.body) as unknown
      return value && typeof value === 'object' ? (value as Record<string, unknown>) : {}
    } catch {
      return {}
    }
  }

  function route(path: string, init?: RequestInit): Response {
    const url = new URL(path, demoBaseURL)
    const method = (init?.method ?? 'GET').toUpperCase()
    const parts = url.pathname.split('/').filter(Boolean).map(decodeURIComponent)
    const is = (m: string, ...want: string[]) => method === m && parts.length === want.length && want.every((w, i) => w === '*' || w === parts[i])
    const body = bodyOf(init)

    if (is('GET', 'v1', 'healthz')) return json({ ok: true, version: 'demo' })
    if (is('GET', 'v1', 'auth', 'whoami')) return json({ role: 'focus' })
    if (is('GET', 'v1', 'focus', 'readiness')) {
      return json({ checks: [{ id: 'model', state: 'ok' }, { id: 'gh', state: 'ok' }, { id: 'tailscale', state: 'ok' }] })
    }
    if (is('GET', 'v1', 'focus', 'folders')) return json({ folders })
    if (is('GET', 'v1', 'focus', 'templates')) return json({ templates, diagnostics: [] })
    if (is('GET', 'v1', 'chat', 'activity')) return json(activity())
    if (is('GET', 'v1', 'focus', 'pipelines')) return json([...demos.values()].map(listItem))
    if (is('POST', 'v1', 'focus', 'pipelines')) return json(create(body))

    const easy = easyRoute(url, method, parts, body)
    if (easy) return easy

    if (is('POST', 'v1', 'remote', 'chat', 'session')) return json({ session_id: chatSessionID })
    if (is('GET', 'v1', 'remote', 'chat', 'history')) return json({ session_id: chatSessionID, messages: chatHistory })

    if (is('POST', 'v1', 'ops', 'approvals', '*', '*')) {
      if (!approvals.some((a) => a.approval_id === parts[3])) throw new Refused(409, 'already answered')
      approvals = approvals.filter((a) => a.approval_id !== parts[3])
      return json({ ok: true })
    }

    if (is('GET', 'v1', 'chat', 'stream')) {
      const sessionID = url.searchParams.get('session_id') ?? ''
      if (sessionID === chatSessionID) return chatRunning ? chatFeed() : new Response(null, { status: 204 })
      const d = demos.get(sessionID)
      return d?.running ? feed(d) : new Response(null, { status: 204 })
    }
    if (is('POST', 'v1', 'chat')) {
      const sessionID = String(body.session_id ?? '')
      if (sessionID === chatSessionID) {
        if (chatRunning) throw new Refused(409, 'a turn is already running')
        startChatTurn(String(body.message ?? ''))
        return chatFeed()
      }
      const d = demos.get(sessionID)
      if (!d) throw new Refused(404, 'no such session')
      if (d.running) throw new Refused(409, 'a turn is already running')
      start(d, 'typed', String(body.message ?? ''), ['Reading the instruction'])
      return feed(d)
    }
    if (is('POST', 'v1', 'chat', 'cancel')) {
      const sessionID = url.searchParams.get('session_id') ?? ''
      if (sessionID === chatSessionID) {
        const had = !!chatRunning
        chatRunning = null
        return json({ cancelled: had, ended: true })
      }
      const d = demos.get(sessionID)
      if (d?.running) stop(d)
      return json({ cancelled: !!d, ended: true })
    }

    if (parts[0] === 'v1' && parts[1] === 'focus' && parts[2] === 'pipelines' && parts.length >= 4) {
      const d = demos.get(parts[3])
      if (!d) throw new Refused(404, 'no such pipeline')
      const rest = parts.slice(4)
      const act = (run: () => void) => {
        run()
        return json({ pipeline: d.pipeline, next_prompt: '' })
      }
      if (method === 'GET') {
        if (rest.length === 0) return json(d.pipeline)
        if (rest[0] === 'session') return json(session(d))
        if (rest[0] === 'history') return json(url.searchParams.get('session') === 'qa' ? d.qaHistory : d.history)
        if (rest[0] === 'checkpoints' && rest.length === 1) return json({ session_id: d.pipeline.session_id, turns: [] })
      }
      if (method === 'POST') {
        if (rest[0] === 'gates' && rest.length === 2) return act(() => gate(d, rest[1], body))
        if (rest[0] === 'cards' && rest.length === 2) return act(() => decideCard(d, rest[1], body))
        if (rest[0] === 'findings') return act(() => addFinding(d, body))
        if (rest[0] === 'stop') return act(() => stop(d))
        if (rest[0] === 'resume') return act(() => resume(d))
        if (rest[0] === 'cleanup') return act(() => cleanup(d, String(body.action ?? 'keep')))
        if (rest[0] === 'goal') {
          if (body.enabled) throw new Refused(403, 'goal mode is turned on at the desk')
          return act(() => {})
        }
        if (rest[0] === 'advance') return act(() => {})
        if (rest[0] === 'qa') return json(ask(d, body))
      }
    }
    // The focus event stream among them: the app then reads on its timer.
    throw new Refused(404, 'not found')
  }

  seed()

  return {
    async handle(path, init) {
      settle()
      try {
        return route(path, init)
      } catch (err) {
        if (err instanceof Refused) return json(err.pipeline ? { error: err.message, pipeline: err.pipeline } : { error: err.message }, err.status)
        throw err
      }
    },
  }

  // --- What the demo desk starts with ---------------------------------------

  // seed puts one pipeline at each place a phone is asked something: a plan
  // to approve, findings to triage with a tool call waiting, a pull request
  // to merge.
  function seed() {
    // A finished vibe project, for the Projects list and its versions.
    const recipe: EasyDemo = {
      project: {
        id: 'demo-recipe-box', name: 'Recipe box', idea: 'A recipe box where I save recipes and tick off what I need to buy',
        folder: easyFolder('Recipe box'), status: 'ready', session_id: 'demo-recipe-box-run', created_at: iso(), updated_at: iso(),
      },
      versions: [],
    }
    recipe.versions = [easyVersion('Start the project', 'start')]
    easySave(recipe)
    easyProjects.set(recipe.project.id, recipe)

    const plan = add('demo-dark-mode', 'Add a dark mode toggle to Settings', 'storefront', {
      plan: {
        goal: 'Add a dark mode toggle to Settings that follows the system setting by default',
        tasks: [
          { title: 'Add a theme preference (system, light, dark) to the settings store', done: 'the preference is saved and read back after a restart' },
          { title: 'Add the toggle to the Settings screen', done: 'choosing a value switches the theme at once' },
          { title: 'Cover the store and the toggle with tests', done: 'npm test passes with the new cases' },
        ],
        stages: devStages,
        verify: ['npm test', 'npm run lint'],
        limits: { build: 3, review: 2 },
      },
      built: 'Added the theme preference, the Settings toggle and their tests. npm test and npm run lint pass.',
      findings: [
        {
          id: 'f1', severity: 'medium', file: 'src/settings/theme.ts', line: 42,
          title: 'The theme flashes light on a cold start',
          scenario: 'The preference is read after the first paint, so a dark-mode user sees a light screen for a moment when the app opens.',
        },
      ],
      fixed: 'The preference is now read before the first paint. Checks pass.',
      draft: { title: 'feat(settings): dark mode toggle', body: 'Adds a theme preference (system, light, dark) and a toggle in Settings.\n\n- follows the system setting by default\n- saved across restarts\n- read before the first paint' },
      prNumber: 131,
    })
    start(plan, 'planning', plan.content.plan.goal, [])
    finish(plan)

    const review = add('demo-upload-retry', 'Fix the flaky upload retry', 'storefront', {
      plan: {
        goal: 'Uploads that fail on a dropped connection should retry with backoff and stop after three tries',
        tasks: [
          { title: 'Retry a failed upload with exponential backoff', done: 'a dropped connection retries after 1s, 2s and 4s' },
          { title: 'Give up after three tries and tell the user', done: 'the fourth failure shows the error and stops' },
        ],
        stages: devStages,
        verify: ['npm test'],
        limits: { build: 3, review: 2 },
      },
      built: 'Uploads now retry with backoff and stop after three tries. npm test passes.',
      findings: [
        {
          id: 'f1', severity: 'high', file: 'src/upload/retry.ts', line: 58,
          title: 'A cancelled upload is retried',
          scenario: 'The user cancels during the backoff wait; the timer still fires and the upload starts again.',
          excerpt: '@@ -54,6 +54,9 @@\n   const wait = backoff(attempt)\n+  await sleep(wait)\n+  return upload(file, attempt + 1)\n }',
        },
        {
          id: 'f2', severity: 'low', file: 'src/upload/messages.ts', line: 12,
          title: 'The error message says "retrys"',
          scenario: 'The message shown after the last failure has a typo.',
        },
      ],
      fixed: 'A cancelled upload no longer retries: the backoff wait is cancelled with it. Checks pass.',
      draft: { title: 'fix(upload): retry with backoff, stop after three tries', body: 'A failed upload retries after 1s, 2s and 4s, then stops with an error.\n\nA cancelled upload is never retried.' },
      prNumber: 128,
    })
    start(review, 'planning', review.content.plan.goal, [])
    finish(review)
    gate(review, 'plan', { action: 'approve' })
    finish(review)
    review.pipeline.cards.filter((c) => c.kind === 'gate').forEach((c) => (c.decided_by = 'desk'))
    approvals = [{
      approval_id: 'demo-approval-1', session_id: review.pipeline.session_id, session_title: review.title,
      source: 'focus', tool_name: 'Bash', preview: 'npm test -- upload', requested_at: iso(),
    }]

    const merge = add('demo-search-index', 'Speed up search indexing', 'notes-app', {
      plan: {
        goal: 'Index notes in batches so a large import no longer freezes the list',
        tasks: [
          { title: 'Index notes in batches of 200 off the main thread', done: 'importing 10,000 notes keeps the list scrolling' },
          { title: 'Show indexing progress in the list footer', done: 'the footer counts up and disappears when done' },
        ],
        stages: devStages,
        verify: ['npm test', 'npm run bench:index'],
        limits: { build: 3, review: 2 },
      },
      built: 'Indexing runs in batches of 200 in a worker, with progress in the footer. Tests and the benchmark pass.',
      findings: [],
      fixed: '',
      draft: { title: 'perf(search): index notes in batches', body: 'Indexes notes in batches of 200 in a worker and shows progress in the list footer.\n\nImporting 10,000 notes: the list stays responsive.' },
      prNumber: 127,
    })
    start(merge, 'planning', merge.content.plan.goal, [])
    finish(merge)
    gate(merge, 'plan', { action: 'approve' })
    finish(merge)
    gate(merge, 'pr', { action: 'approve' })
    finish(merge)
    merge.pipeline.cards.filter((c) => c.kind === 'gate' && c.state === 'decided').forEach((c) => (c.decided_by = 'desk'))
  }

  // contentFor is what a task sent from the phone runs through.
  function contentFor(goal: string, prNumber: number): Content {
    const title = goal.split('\n', 1)[0].slice(0, 72)
    return {
      plan: {
        goal,
        tasks: [
          { title: 'Read the code the goal touches and find where the change belongs', done: 'the files to change are listed' },
          { title: 'Make the change', done: 'the goal works as described' },
          { title: 'Add tests for the new behaviour', done: 'the checks pass with the new cases' },
        ],
        stages: devStages,
        verify: ['npm test'],
        limits: { build: 3, review: 2 },
      },
      built: 'Made the change and added tests. npm test passes.',
      findings: [
        {
          id: 'f1', severity: 'medium', file: 'src/index.ts', line: 1,
          title: 'An empty input is not handled',
          scenario: 'Calling the new code with an empty value throws instead of doing nothing.',
        },
      ],
      fixed: 'An empty input is now ignored. Checks pass.',
      draft: { title, body: `${goal}\n\nIncludes tests.` },
      prNumber,
    }
  }
}
