import test from 'node:test'
import assert from 'node:assert/strict'
import { JSDOM } from 'jsdom'

const dom = new JSDOM('<!doctype html><html><body></body></html>')
// DOMPurify's default factory looks for window on globalThis when its module
// loads, so seed the globals before importing markdown.ts.
;(globalThis as { window?: unknown }).window = dom.window as unknown
;(globalThis as { document?: unknown }).document = dom.window.document
;(globalThis as { HTMLElement?: unknown }).HTMLElement = dom.window.HTMLElement
;(globalThis as { Node?: unknown }).Node = dom.window.Node
;(globalThis as { NodeFilter?: unknown }).NodeFilter = dom.window.NodeFilter

const { renderMarkdown } = await import('../src/lib/markdown.ts')

test('renderMarkdown strips literal <script> tags', () => {
  const html = renderMarkdown('Hello\n\n<script>alert(1)</script>\n\nWorld')
  assert.equal(html.includes('<script'), false)
  assert.equal(html.toLowerCase().includes('alert(1)'), false)
})

test('renderMarkdown strips whitespace-padded </script > end tags', () => {
  // Variant flagged by CodeQL js/bad-tag-filter — the old regex did not match
  // </script > with trailing whitespace, which left an opening <script tag in
  // the output.
  const html = renderMarkdown('<script>alert(1)</script   \n>')
  assert.equal(html.toLowerCase().includes('<script'), false)
})

test('renderMarkdown strips nested <script> obfuscation', () => {
  // Variant flagged by CodeQL js/incomplete-multi-character-sanitization — a
  // single regex pass on `<scr<script>ipt>` would leave `<script>` behind.
  const html = renderMarkdown('<scr<script>ipt>alert(1)</script>')
  assert.equal(html.toLowerCase().includes('<script'), false)
})

test('renderMarkdown removes inline event handlers', () => {
  const html = renderMarkdown('<a href="https://example.com" onclick="alert(1)">click</a>')
  assert.equal(html.includes('onclick'), false)
  assert.equal(html.includes('alert(1)'), false)
  assert.match(html, /href="https:\/\/example\.com"/)
})

test('renderMarkdown rejects javascript: links from markdown', () => {
  const html = renderMarkdown('[click](javascript:alert(1))')
  assert.equal(html.toLowerCase().includes('javascript:'), false)
})

test('renderMarkdown preserves data-* attributes used by code/mermaid toolbars', () => {
  const html = renderMarkdown('```mermaid\ngraph TD; A-->B;\n```')
  assert.ok(html.includes('data-graph'), `expected data-graph in output, got: ${html}`)
  assert.ok(html.includes('class="mermaid-block"'))
})

test('renderMarkdown adds rel="noopener noreferrer" to target=_blank links', () => {
  const html = renderMarkdown('[example](https://example.com)')
  assert.ok(html.includes('target="_blank"'))
  assert.ok(html.includes('rel="noopener noreferrer"'))
})

test('renderMarkdown returns empty string for empty source', () => {
  assert.equal(renderMarkdown(''), '')
})
