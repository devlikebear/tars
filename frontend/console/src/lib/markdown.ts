/**
 * Markdown renderer for chat messages.
 * Uses marked (GFM) + highlight.js for syntax highlighting, with DOMPurify
 * stripping unsafe markup from the final HTML.
 */

import { Marked } from 'marked'
import DOMPurify, { type Config as DOMPurifyConfig } from 'dompurify'
import hljs from 'highlight.js/lib/core'

// Selective language imports to keep bundle small
import go from 'highlight.js/lib/languages/go'
import typescript from 'highlight.js/lib/languages/typescript'
import javascript from 'highlight.js/lib/languages/javascript'
import python from 'highlight.js/lib/languages/python'
import bash from 'highlight.js/lib/languages/bash'
import shell from 'highlight.js/lib/languages/shell'
import json from 'highlight.js/lib/languages/json'
import yaml from 'highlight.js/lib/languages/yaml'
import css from 'highlight.js/lib/languages/css'
import xml from 'highlight.js/lib/languages/xml'
import sql from 'highlight.js/lib/languages/sql'
import markdown from 'highlight.js/lib/languages/markdown'
import java from 'highlight.js/lib/languages/java'
import rust from 'highlight.js/lib/languages/rust'
import cpp from 'highlight.js/lib/languages/cpp'
import diff from 'highlight.js/lib/languages/diff'
import dockerfile from 'highlight.js/lib/languages/dockerfile'
import kotlin from 'highlight.js/lib/languages/kotlin'
import swift from 'highlight.js/lib/languages/swift'
import ruby from 'highlight.js/lib/languages/ruby'

hljs.registerLanguage('go', go)
hljs.registerLanguage('typescript', typescript)
hljs.registerLanguage('ts', typescript)
hljs.registerLanguage('javascript', javascript)
hljs.registerLanguage('js', javascript)
hljs.registerLanguage('python', python)
hljs.registerLanguage('py', python)
hljs.registerLanguage('bash', bash)
hljs.registerLanguage('sh', bash)
hljs.registerLanguage('shell', shell)
hljs.registerLanguage('json', json)
hljs.registerLanguage('yaml', yaml)
hljs.registerLanguage('yml', yaml)
hljs.registerLanguage('css', css)
hljs.registerLanguage('xml', xml)
hljs.registerLanguage('html', xml)
hljs.registerLanguage('sql', sql)
hljs.registerLanguage('markdown', markdown)
hljs.registerLanguage('md', markdown)
hljs.registerLanguage('java', java)
hljs.registerLanguage('rust', rust)
hljs.registerLanguage('rs', rust)
hljs.registerLanguage('cpp', cpp)
hljs.registerLanguage('c', cpp)
hljs.registerLanguage('diff', diff)
hljs.registerLanguage('dockerfile', dockerfile)
hljs.registerLanguage('docker', dockerfile)
hljs.registerLanguage('kotlin', kotlin)
hljs.registerLanguage('kt', kotlin)
hljs.registerLanguage('swift', swift)
hljs.registerLanguage('ruby', ruby)
hljs.registerLanguage('rb', ruby)

function escapeAttr(text: string): string {
  return text
    .replace(/&/g, '&amp;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
}

// Code/mermaid blocks stash their original source in data-code/data-graph so
// the toolbar can read it back. DOMPurify's mXSS protection strips attributes
// whose value contains `-->` (it could close an HTML comment under hostile
// parsers), which collides with mermaid arrow syntax and most code samples.
// URL-encoding the payload sidesteps the heuristic without weakening it for
// other markup — consumers decode with readEncodedAttr.
function encodeAttrPayload(text: string): string {
  return encodeURIComponent(text)
}

export function readEncodedAttr(value: string | null | undefined): string {
  if (!value) return ''
  try {
    return decodeURIComponent(value)
  } catch {
    return value
  }
}

export function highlightTerms(text: string, terms: string[]): string {
  if (!text) return ''
  const normalizedTerms = terms
    .map((term) => term.trim())
    .filter(Boolean)
    .sort((a, b) => b.length - a.length)
  if (normalizedTerms.length === 0) {
    return escapeAttr(text)
  }

  const lowered = text.toLowerCase()
  const ranges: Array<[number, number]> = []
  for (const rawTerm of normalizedTerms) {
    const term = rawTerm.toLowerCase()
    if (!term) continue
    let index = lowered.indexOf(term)
    while (index !== -1) {
      const end = index + term.length
      if (!ranges.some(([start, stop]) => index < stop && end > start)) {
        ranges.push([index, end])
      }
      index = lowered.indexOf(term, end)
    }
  }
  if (ranges.length === 0) {
    return escapeAttr(text)
  }
  ranges.sort((a, b) => a[0] - b[0])

  let cursor = 0
  let out = ''
  for (const [start, end] of ranges) {
    out += escapeAttr(text.slice(cursor, start))
    out += `<mark>${escapeAttr(text.slice(start, end))}</mark>`
    cursor = end
  }
  out += escapeAttr(text.slice(cursor))
  return out
}

function normalizeLanguage(lang?: string): string {
  return lang?.trim().toLowerCase() || ''
}

function highlightCode(text: string, lang?: string): string {
  const language = normalizeLanguage(lang)
  if (language && hljs.getLanguage(language)) {
    return hljs.highlight(text, { language }).value
  }
  if (language) {
    try {
      return hljs.highlightAuto(text).value
    } catch {
      return escapeAttr(text)
    }
  }
  return escapeAttr(text)
}

export function renderHighlightedCodeBlock(text: string, lang?: string): string {
  return `<div class="code-block"><pre><code class="hljs">${highlightCode(text, lang)}</code></pre></div>`
}

const marked = new Marked({
  gfm: true,
  breaks: false,
  renderer: {
    code({ text, lang }: { text: string; lang?: string }) {
      const language = normalizeLanguage(lang)

      // Mermaid diagrams: toolbar + code/preview toggle + lazy-load
      if (language === 'mermaid') {
        const encoded = encodeAttrPayload(text)
        return `<div class="mermaid-block" data-graph="${encoded}"><div class="code-toolbar"><span class="code-lang">mermaid</span><div class="code-actions"><button type="button" class="code-toggle" data-mode="code" title="View code">Code</button><button type="button" class="code-toggle active" data-mode="preview" title="Preview diagram">Preview</button><button type="button" class="code-copy" data-code="${encoded}" title="Copy code">Copy</button></div></div><pre class="mermaid-src" style="display:none"><code>${escapeAttr(text)}</code></pre><div class="mermaid-preview" data-mermaid-preview></div></div>`
      }

      const highlighted = highlightCode(text, language)

      const langLabel = language ? `<span class="code-lang">${escapeAttr(language)}</span>` : ''
      const previewable = ['html', 'svg'].includes(language)
      const encodedSource = encodeAttrPayload(text)
      const toolbar = previewable
        ? `<div class="code-toolbar">${langLabel}<div class="code-actions"><button type="button" class="code-toggle active" data-mode="code" title="View code">Code</button><button type="button" class="code-toggle" data-mode="preview" title="Preview">Preview</button><button type="button" class="code-copy" data-code="${encodedSource}" title="Copy code">Copy</button></div></div>`
        : `<div class="code-toolbar">${langLabel}<div class="code-actions"><button type="button" class="code-copy" data-code="${encodedSource}" title="Copy code">Copy</button></div></div>`
      const previewHtml = previewable
        ? `<div class="code-preview" style="display:none" data-preview>${text}</div>`
        : ''
      return `<div class="code-block"${previewable ? ' data-previewable' : ''}>${toolbar}<pre><code class="hljs">${highlighted}</code></pre>${previewHtml}</div>`
    },

    link({ href, text }: { href: string; text: string }) {
      return `<a href="${escapeAttr(href)}" target="_blank" rel="noopener">${text}</a>`
    },

    checkbox({ checked }: { checked: boolean }) {
      return `<input type="checkbox" disabled ${checked ? 'checked' : ''} />`
    },
  },
})

// DOMPurify configuration that preserves the renderer's intentional markup
// (code/mermaid toolbars with data-* attributes, inline style="display:none"
// toggles, html/svg previews inside <div class="code-preview">) while
// stripping <script>, event handler attributes, and unsafe URL schemes.
const SANITIZER_CONFIG: DOMPurifyConfig = {
  RETURN_TRUSTED_TYPE: false,
  ADD_ATTR: [
    'target',
    'data-graph',
    'data-mode',
    'data-code',
    'data-preview',
    'data-previewable',
    'data-mermaid-preview',
  ],
  // marked produces flow content with a few semantic-only tags. Keep
  // DOMPurify's allowlist intact; explicitly forbid <style> so a remote
  // markdown source can't add @import or expression() payloads.
  FORBID_TAGS: ['style'],
  // We never want javascript:, vbscript:, data: (except images), or file:
  // URLs in attribute values. DOMPurify enforces this via ALLOWED_URI_REGEXP.
  ALLOWED_URI_REGEXP: /^(?:(?:https?|mailto|ftp|tel):|[^a-z]|[a-z+.-]+(?:[^a-z+.\-:]|$))/i,
}

// Add target=_blank links automatically get rel="noopener noreferrer" so a
// sanitized link cannot reach window.opener.
DOMPurify.addHook('afterSanitizeAttributes', (node) => {
  if (node.nodeName === 'A' && node.hasAttribute('href')) {
    if (node.getAttribute('target') === '_blank') {
      node.setAttribute('rel', 'noopener noreferrer')
    }
  }
})

export function renderMarkdown(source: string): string {
  if (!source) return ''
  const result = marked.parse(source)
  if (typeof result !== 'string') {
    return ''
  }
  return DOMPurify.sanitize(result, SANITIZER_CONFIG) as unknown as string
}
