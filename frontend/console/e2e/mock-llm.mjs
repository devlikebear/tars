// Deterministic OpenAI-compatible LLM for console E2E runs.
//
// Answers POST /v1/chat/completions with "Echo: <last user message>", either
// as an SSE stream (chunked deltas, usage, [DONE]) or a single JSON body, so
// specs can assert exact assistant text without a real provider. No tools.

import { createServer } from 'node:http'

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
  const reply = replyFor(body)
  if (process.env.TARS_E2E_MOCK_LLM_DEBUG) console.error(JSON.stringify(body.messages?.slice(-1)))

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
