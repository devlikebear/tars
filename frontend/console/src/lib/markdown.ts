/**
 * Markdown renderer for chat messages.
 * Uses marked (GFM) + highlight.js for syntax highlighting.
 */

import { Marked } from 'marked'
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

const marked = new Marked({
  gfm: true,
  breaks: false,
  renderer: {
    code({ text, lang }: { text: string; lang?: string }) {
      const language = lang?.trim().toLowerCase() || ''

      // Mermaid diagrams: toolbar + code/preview toggle + lazy-load
      if (language === 'mermaid') {
        return `<div class="mermaid-block" data-graph="${escapeAttr(text)}"><div class="code-toolbar"><span class="code-lang">mermaid</span><div class="code-actions"><button type="button" class="code-toggle" data-mode="code" title="View code">Code</button><button type="button" class="code-toggle active" data-mode="preview" title="Preview diagram">Preview</button><button type="button" class="code-copy" data-code="${escapeAttr(text)}" title="Copy code">Copy</button></div></div><pre class="mermaid-src" style="display:none"><code>${escapeAttr(text)}</code></pre><div class="mermaid-preview" data-mermaid-preview></div></div>`
      }

      // Syntax highlighting
      let highlighted: string
      if (language && hljs.getLanguage(language)) {
        highlighted = hljs.highlight(text, { language }).value
      } else if (language) {
        try {
          highlighted = hljs.highlightAuto(text).value
        } catch {
          highlighted = escapeAttr(text)
        }
      } else {
        highlighted = escapeAttr(text)
      }

      const langLabel = language ? `<span class="code-lang">${escapeAttr(language)}</span>` : ''
      const previewable = ['html', 'svg'].includes(language)
      const toolbar = previewable
        ? `<div class="code-toolbar">${langLabel}<div class="code-actions"><button type="button" class="code-toggle active" data-mode="code" title="View code">Code</button><button type="button" class="code-toggle" data-mode="preview" title="Preview">Preview</button><button type="button" class="code-copy" data-code="${escapeAttr(text)}" title="Copy code">Copy</button></div></div>`
        : `<div class="code-toolbar">${langLabel}<div class="code-actions"><button type="button" class="code-copy" data-code="${escapeAttr(text)}" title="Copy code">Copy</button></div></div>`
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

// Strip dangerous tags from output
function sanitize(html: string): string {
  return html.replace(/<script\b[^<]*(?:(?!<\/script>)<[^<]*)*<\/script>/gi, '')
}

export function renderMarkdown(source: string): string {
  if (!source) return ''
  const result = marked.parse(source)
  if (typeof result === 'string') {
    return sanitize(result)
  }
  return ''
}
