/**
 * Lightweight markdown renderer for chat messages.
 * Handles: code blocks, inline code, bold, italic, links, lists, line breaks.
 * No external dependencies.
 */

function escapeHtml(text: string): string {
  return text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}

function renderInline(text: string): string {
  return text
    // inline code
    .replace(/`([^`]+)`/g, '<code>$1</code>')
    // bold
    .replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
    // italic
    .replace(/(?<!\*)\*([^*]+)\*(?!\*)/g, '<em>$1</em>')
    // links
    .replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank" rel="noopener">$1</a>')
}

export function renderMarkdown(source: string): string {
  const lines = source.split('\n')
  const out: string[] = []
  let i = 0
  let inList = false

  while (i < lines.length) {
    const line = lines[i]

    // fenced code block
    if (line.startsWith('```')) {
      const lang = line.slice(3).trim()
      const codeLines: string[] = []
      i++
      while (i < lines.length && !lines[i].startsWith('```')) {
        codeLines.push(escapeHtml(lines[i]))
        i++
      }
      i++ // skip closing ```
      if (inList) { out.push('</ul>'); inList = false }
      const langAttr = lang ? ` data-lang="${escapeHtml(lang)}"` : ''
      out.push(`<pre${langAttr}><code>${codeLines.join('\n')}</code></pre>`)
      continue
    }

    // unordered list item
    if (/^[\-\*]\s/.test(line)) {
      if (!inList) { out.push('<ul>'); inList = true }
      out.push(`<li>${renderInline(escapeHtml(line.replace(/^[\-\*]\s+/, '')))}</li>`)
      i++
      continue
    }

    // ordered list item
    if (/^\d+\.\s/.test(line)) {
      if (!inList) { out.push('<ol>'); inList = true }
      out.push(`<li>${renderInline(escapeHtml(line.replace(/^\d+\.\s+/, '')))}</li>`)
      i++
      continue
    }

    // close list if needed
    if (inList && line.trim() !== '') {
      out.push('</ul>')
      inList = false
    }

    // heading
    const headingMatch = line.match(/^(#{1,4})\s+(.+)/)
    if (headingMatch) {
      const level = headingMatch[1].length
      out.push(`<h${level + 2}>${renderInline(escapeHtml(headingMatch[2]))}</h${level + 2}>`)
      i++
      continue
    }

    // blank line
    if (line.trim() === '') {
      if (inList) { out.push('</ul>'); inList = false }
      i++
      continue
    }

    // paragraph
    out.push(`<p>${renderInline(escapeHtml(line))}</p>`)
    i++
  }

  if (inList) { out.push('</ul>') }
  return out.join('\n')
}
