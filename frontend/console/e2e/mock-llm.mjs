// Deterministic OpenAI-compatible LLM for console E2E runs.
//
// Answers POST /v1/chat/completions with "Echo: <last user message>", either
// as an SSE stream (chunked deltas, usage, [DONE]) or a single JSON body, so
// specs can assert exact assistant text without a real provider.
//
// A message containing [e2e:write3] makes it act like an agent that edits
// files: it calls write_file three times (base.txt changed in two places,
// two new files). It keeps no state, so once the tool results come back it
// answers "Wrote N files." Specs seed base.txt with lines "line 1" to
// "line 20" (see e2e/changes.spec.ts).
//
// A message containing [e2e:narrate] makes it talk between its tool calls:
// it streams "Writing the note first." and then calls write_file once
// (notes.md), and once the result comes back answers "All written." — a
// turn that reads text, tool, text (see e2e/turn-order.spec.ts).

//
// Focus mode (docs/decisions/focus-mode.md) puts the goal in every turn's
// <focus-stage> guidance, so markers in the goal reach every turn:
// [e2e:focus-plan] answers a plan-stage turn with a <focus-plan> block, and
// [e2e:focus-report] answers any later stage with a <focus-report> block
// that asks one decision — unless the turn is that decision's answer
// ("question → option"), which gets a report without one that says every
// task is done (tasks_done), so the server's verification ends the build.
// [e2e:focus-loop] answers every build turn with a done report and no
// decision, for the server-driven build loop (P2). Review turns get no
// findings and a plain report, and a PR turn a draft without a question, so
// those pipelines rest in the pr stage.
// [e2e:focus-review] runs the review loop (P3): its plan adds an end-to-end
// command, build reports done, the first review finds two issues in
// base.txt, the fix turn ("Fix these findings…") reports, and the next
// review round finds none. [e2e:focus-pr] answers the pr stage with a
// <focus-pr> draft and a report, and the turns a PR gate sends (open the PR,
// merge it) and the merge stage with a plain report; the server's gh probe
// then finds no PR (no GitHub remote), so the developer passes those stages
// by hand (P4). A Q&A question (console context, no stage) gets the generic
// console-context answer below.

import { createServer } from 'node:http'

const WRITE3 = '[e2e:write3]'
const NARRATE = '[e2e:narrate]'
const narrateText = 'Writing the note first.'
const narrateCalls = [{
  id: 'call_e2e_narrate',
  type: 'function',
  function: { name: 'write_file', arguments: JSON.stringify({ path: 'notes.md', content: '# Notes\n' }) },
}]

function numberedLines(edit = {}) {
  return Array.from({ length: 20 }, (_, i) => edit[i + 1] ?? `line ${i + 1}`).join('\n') + '\n'
}

const write3Calls = [
  { path: 'base.txt', content: numberedLines({ 2: 'line 2 edited', 19: 'line 19 edited' }) },
  { path: 'notes.md', content: '# Notes\n\nWritten by the E2E agent.\n' },
  { path: 'src/app.txt', content: 'app v1\n' },
].map((args, i) => ({
  id: `call_e2e_${i + 1}`,
  type: 'function',
  function: { name: 'write_file', arguments: JSON.stringify(args) },
}))

// Tool results that end the conversation so far, i.e. this call follows the
// tools the mock asked for.
function trailingToolResults(messages) {
  let count = 0
  for (let i = messages.length - 1; i >= 0 && messages[i]?.role === 'tool'; i--) count++
  return count
}

function offersTool(body, name) {
  return (body.tools ?? []).some((tool) => tool?.function?.name === name)
}

const port = Number(process.env.TARS_E2E_MOCK_LLM_PORT || 43291)

function lastUserText(messages) {
  for (let i = messages.length - 1; i >= 0; i--) {
    const m = messages[i]
    if (m?.role !== 'user') continue
    if (typeof m.content === 'string') return m.content
    if (Array.isArray(m.content)) {
      return m.content.filter((p) => p?.type === 'text').map((p) => p.text).join(' ')
    }
  }
  return ''
}

const FOCUS_PLAN = '[e2e:focus-plan]'
// A question typed while the plan gate is open: a plain answer, no block.
const FOCUS_ASK = '[e2e:focus-ask]'
const FOCUS_REPORT = '[e2e:focus-report]'
const FOCUS_LOOP = '[e2e:focus-loop]'
const FOCUS_REVIEW = '[e2e:focus-review]'

function reviewReply(typed) {
  if (typed.startsWith('Fix these findings')) {
    return `Fixed the accepted finding.\n\n<focus-report>${JSON.stringify({ summary: 'Fixed the greeting line.', risks: [] })}</focus-report>`
  }
  const findings = typed.includes('Review the changes again')
    ? []
    : [
        { id: 'f1', severity: 'high', file: 'base.txt', line: 2, title: 'Greeting has no punctuation', scenario: 'greet() prints "hello" → the UI shows a bare word' },
        { id: 'f2', severity: 'low', file: 'base.txt', line: 1, title: 'File name is vague', scenario: 'a reader opens base.txt → cannot tell it holds the greeting' },
      ]
  const summary = findings.length ? `Found ${findings.length} issues.` : 'Reviewed the change again.'
  return `${summary}\n\n<focus-findings>${JSON.stringify(findings)}</focus-findings>\n<focus-report>${JSON.stringify({ summary, risks: [] })}</focus-report>`
}
const FOCUS_PR = '[e2e:focus-pr]'

function focusReply(text) {
  const stage = text.match(/<focus-stage>[\s\S]*?current stage: ([a-z_]+)/)?.[1]
  if (!stage) return null
  const typed = text.slice(0, text.indexOf('<focus-stage>'))
  if (typed.includes(FOCUS_ASK)) return 'The verification commands look right.'
  if (stage === 'plan' && text.includes(FOCUS_PLAN)) {
    const plan = {
      goal: 'Add a greeting',
      tasks: [{ title: 'Add greet()', done: 'greet() returns a greeting' }, { title: 'Test greet()', done: 'make test passes' }],
      stages: ['plan', 'build', 'review', 'pr', 'pr_review', 'merge'],
      verify: ['make test'],
      e2e: text.includes(FOCUS_REVIEW) ? ['git status --short'] : [],
      limits: { build: 3, review: 2, pr: 3 },
    }
    return `Here is the plan.\n\n<focus-plan>${JSON.stringify(plan)}</focus-plan>`
  }
  if (stage === 'review' && text.includes(FOCUS_REVIEW)) return reviewReply(typed)
  if ((stage === 'pr' || stage === 'pr_review' || stage === 'merge') && text.includes(FOCUS_PR)) {
    if (stage === 'pr' && !typed.includes('PR gate approved')) {
      const draft = { title: 'feat: add a greeting', body: 'Adds greet() and its test.' }
      return `Drafted the PR.\n\n<focus-pr>${JSON.stringify(draft)}</focus-pr>\n<focus-report>${JSON.stringify({ summary: 'Drafted the pull request.', risks: [] })}</focus-report>`
    }
    return `Done.\n\n<focus-report>${JSON.stringify({ summary: `Ran the ${stage} step.`, risks: [] })}</focus-report>`
  }
  if (stage === 'pr' && (text.includes(FOCUS_REPORT) || text.includes(FOCUS_LOOP) || text.includes(FOCUS_REVIEW))) {
    const draft = { title: 'Add a greeting', body: 'Adds greet().' }
    return `Drafted the PR.\n\n<focus-pr>${JSON.stringify(draft)}</focus-pr>\n<focus-report>${JSON.stringify({ summary: 'Drafted the PR.', risks: [] })}</focus-report>`
  }
  if (stage === 'review' && (text.includes(FOCUS_REPORT) || text.includes(FOCUS_LOOP))) {
    return `Looked over the change.\n\n<focus-findings>[]</focus-findings>\n<focus-report>${JSON.stringify({ summary: 'Reviewed the change.', risks: [] })}</focus-report>`
  }
  if (stage === 'build' && (text.includes(FOCUS_LOOP) || text.includes(FOCUS_REVIEW))) {
    const fixing = text.startsWith('Verification failed')
    const report = { summary: fixing ? 'Fixed the failing test.' : 'Implemented greet().', tasks_done: true, risks: [] }
    return `${fixing ? 'Fixed it.' : 'Implemented it.'}\n\n<focus-report>${JSON.stringify(report)}</focus-report>`
  }
  if (stage !== 'plan' && text.includes(FOCUS_REPORT)) {
    const answered = typed.includes('→')
    const report = answered
      ? { summary: 'Applied the chosen greeting.', tasks_done: true, risks: [] }
      : {
          // Two lines: the card's heading is the first, its body only the second (#1109).
          summary: 'Implemented greet() and its test.\nThe test covers both punctuation marks.',
          decisions: [{ id: 'd1', question: 'Which greeting should greet() return?', options: ['Hello', 'Hi there'] }],
          risks: ['The greeting is not localized.'],
        }
    // A summary table with long code cells, as models write them.
    const table = answered ? '' : `\n\n| File | Change |\n|---|---|\n| \`${focusLongPath}\` | \`export const greet = (name: string, punctuation: string = '!') => \\\`Hello, \${name}\${punctuation}\\\`\` |`
    return `${answered ? 'Done.' : 'Implemented the greeting.'}${table}\n\n<focus-report>${JSON.stringify(report)}</focus-report>`
  }
  return null
}

// The build turn of a [e2e:focus-report] task first writes one file at a
// long nested path, so its tool card (and its change card) carry a long
// unbroken line, as real tool calls do; the report follows the result.
const focusLongPath = `src/${'deeply-nested-folder/'.repeat(12)}greet.ts`

function focusBuildTool(text) {
  const stage = text.match(/<focus-stage>[\s\S]*?current stage: ([a-z_]+)/)?.[1]
  const typed = stage ? text.slice(0, text.indexOf('<focus-stage>')) : ''
  return stage === 'build' && text.includes(FOCUS_REPORT) && !typed.includes('→') && !typed.includes(FOCUS_ASK)
}

// The chat handler wraps the user's text in context blocks; keep only the
// part after the last blank line, which is what the user typed.
function replyFor(body) {
  const text = lastUserText(body.messages ?? []).trim()
  const focus = focusReply(text)
  if (focus) return focus
  // Review notes (#969) ride after the message; answer the first comment so
  // a spec can see the note arrived.
  if (text.includes('<review-notes>')) {
    return `Rework: ${text.match(/^Comment: (.*)$/m)?.[1] ?? 'reverted changes noted'}`
  }
  // Console context (the companion handoff) rides after the message; name
  // its console area so a spec can see it arrived, and echo the words.
  const context = text.match(/\n\n<console-context>\n([\s\S]*)\n<\/console-context>$/)
  if (context) {
    const typed = text.slice(0, context.index).split(/\n\s*\n/).pop()?.trim() ?? ''
    const area = context[1].match(/^Current console area: (.*?)\.?$/m)?.[1] ?? 'unknown'
    return `Companion at ${area}: ${typed || 'ok'}`
  }
  const tail = text.split(/\n\s*\n/).pop()?.trim() ?? ''
  return `Echo: ${tail || 'ok'}`
}

const usage = { prompt_tokens: 12, completion_tokens: 5, total_tokens: 17 }

function chunk(model, delta, finishReason = null, extra = {}) {
  return `data: ${JSON.stringify({
    id: 'chatcmpl-e2e',
    object: 'chat.completion.chunk',
    created: 0,
    model,
    choices: [{ index: 0, delta, finish_reason: finishReason }],
    ...extra,
  })}\n\n`
}

async function handleCompletion(req, res) {
  let raw = ''
  for await (const part of req) raw += part
  let body = {}
  try { body = JSON.parse(raw || '{}') } catch { /* treat as empty */ }
  const model = body.model || 'e2e-model'
  const messages = body.messages ?? []
  if (process.env.TARS_E2E_MOCK_LLM_DEBUG) console.error(JSON.stringify(messages.slice(-1)))
  const toolResults = trailingToolResults(messages)
  if (toolResults === 0 && lastUserText(messages).includes(WRITE3) && offersTool(body, 'write_file')) {
    sendToolCalls(res, model, body.stream, write3Calls)
    return
  }
  if (toolResults === 0 && focusBuildTool(lastUserText(messages)) && offersTool(body, 'write_file')) {
    sendToolCalls(res, model, body.stream, [{
      id: 'call_e2e_focus',
      type: 'function',
      function: { name: 'write_file', arguments: JSON.stringify({ path: focusLongPath, content: 'export const greet = () => "Hello"\n' }) },
    }])
    return
  }
  const narrate = lastUserText(messages).includes(NARRATE)
  if (toolResults === 0 && narrate && offersTool(body, 'write_file')) {
    sendToolCalls(res, model, body.stream, narrateCalls, narrateText)
    return
  }
  const focusAfterTool = toolResults > 0 ? focusReply(lastUserText(messages).trim()) : null
  const reply = toolResults > 0 ? (focusAfterTool ?? (narrate ? 'All written.' : `Wrote ${toolResults} files.`)) : replyFor(body)

  if (!body.stream) {
    res.writeHead(200, { 'content-type': 'application/json' })
    res.end(JSON.stringify({
      id: 'chatcmpl-e2e',
      object: 'chat.completion',
      created: 0,
      model,
      choices: [{ index: 0, message: { role: 'assistant', content: reply }, finish_reason: 'stop' }],
      usage,
    }))
    return
  }

  res.writeHead(200, { 'content-type': 'text/event-stream', 'cache-control': 'no-cache', connection: 'keep-alive' })
  res.write(chunk(model, { role: 'assistant', content: '' }))
  // Several small deltas so the UI actually renders a stream.
  for (const piece of reply.match(/.{1,6}/gs) ?? [reply]) {
    res.write(chunk(model, { content: piece }))
    await new Promise((r) => setTimeout(r, 15))
  }
  res.write(chunk(model, {}, 'stop'))
  res.write(`data: ${JSON.stringify({ id: 'chatcmpl-e2e', object: 'chat.completion.chunk', created: 0, model, choices: [], usage })}\n\n`)
  res.write('data: [DONE]\n\n')
  res.end()
}

// text, when given, is said before the calls, as a model explains what it
// is about to do.
function sendToolCalls(res, model, stream, calls, text = null) {
  if (!stream) {
    res.writeHead(200, { 'content-type': 'application/json' })
    res.end(JSON.stringify({
      id: 'chatcmpl-e2e',
      object: 'chat.completion',
      created: 0,
      model,
      choices: [{ index: 0, message: { role: 'assistant', content: text, tool_calls: calls }, finish_reason: 'tool_calls' }],
      usage,
    }))
    return
  }
  res.writeHead(200, { 'content-type': 'text/event-stream', 'cache-control': 'no-cache', connection: 'keep-alive' })
  res.write(chunk(model, { role: 'assistant', content: null }))
  if (text) res.write(chunk(model, { content: text }))
  calls.forEach((call, index) => {
    res.write(chunk(model, { tool_calls: [{ index, ...call }] }))
  })
  res.write(chunk(model, {}, 'tool_calls'))
  res.write(`data: ${JSON.stringify({ id: 'chatcmpl-e2e', object: 'chat.completion.chunk', created: 0, model, choices: [], usage })}\n\n`)
  res.write('data: [DONE]\n\n')
  res.end()
}

const server = createServer((req, res) => {
  const url = new URL(req.url ?? '/', 'http://localhost')
  if (req.method === 'GET' && url.pathname === '/health') {
    res.writeHead(200, { 'content-type': 'text/plain' })
    res.end('ok')
    return
  }
  if (req.method === 'GET' && url.pathname === '/v1/models') {
    res.writeHead(200, { 'content-type': 'application/json' })
    res.end(JSON.stringify({ object: 'list', data: [{ id: 'e2e-model', object: 'model' }] }))
    return
  }
  if (req.method === 'POST' && url.pathname === '/v1/chat/completions') {
    handleCompletion(req, res).catch((err) => {
      res.writeHead(500, { 'content-type': 'application/json' })
      res.end(JSON.stringify({ error: { message: String(err) } }))
    })
    return
  }
  res.writeHead(404, { 'content-type': 'application/json' })
  res.end(JSON.stringify({ error: { message: `mock-llm: no route for ${req.method} ${url.pathname}` } }))
})

server.listen(port, '127.0.0.1', () => {
  console.log(`mock-llm listening on http://127.0.0.1:${port}`)
})
