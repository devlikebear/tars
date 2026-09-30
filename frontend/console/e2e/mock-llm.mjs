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

import { createServer } from 'node:http'

const WRITE3 = '[e2e:write3]'

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

// The chat handler wraps the user's text in context blocks; keep only the
// part after the last blank line, which is what the user typed.
function replyFor(body) {
  const text = lastUserText(body.messages ?? []).trim()
  // Review notes (#969) ride after the message; answer the first comment so
  // a spec can see the note arrived.
  if (text.includes('<review-notes>')) {
    return `Rework: ${text.match(/^Comment: (.*)$/m)?.[1] ?? 'reverted changes noted'}`
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
  const reply = toolResults > 0 ? `Wrote ${toolResults} files.` : replyFor(body)

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

function sendToolCalls(res, model, stream, calls) {
  if (!stream) {
    res.writeHead(200, { 'content-type': 'application/json' })
    res.end(JSON.stringify({
      id: 'chatcmpl-e2e',
      object: 'chat.completion',
      created: 0,
      model,
      choices: [{ index: 0, message: { role: 'assistant', content: null, tool_calls: calls }, finish_reason: 'tool_calls' }],
      usage,
    }))
    return
  }
  res.writeHead(200, { 'content-type': 'text/event-stream', 'cache-control': 'no-cache', connection: 'keep-alive' })
  res.write(chunk(model, { role: 'assistant', content: null }))
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
