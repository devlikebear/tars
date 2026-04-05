<script lang="ts">
  import { renderMarkdown } from '../lib/markdown'
  import type { Artifact } from '../lib/artifacts'

  interface Props {
    text: string
    artifacts?: Artifact[]
    onArtifactOpen?: (path: string) => void
  }

  type ArtifactLinkTarget = {
    label: string
    path: string
  }

  let { text, artifacts = [], onArtifactOpen }: Props = $props()

  let containerEl: HTMLDivElement | undefined = $state()
  let mermaidTimer: ReturnType<typeof setTimeout> | null = null
  let mermaidModule: typeof import('mermaid')['default'] | null = null

  const FILE_TOKEN_CHARS = /[A-Za-z0-9._-]/

  function basename(path: string): string {
    return path.split('/').pop() || path
  }

  function buildArtifactLinkTargets(values: Artifact[]): ArtifactLinkTarget[] {
    if (!onArtifactOpen || values.length === 0) return []

    const pathByLabel = new Map<string, string | null>()

    for (const artifact of values) {
      const labels = [artifact.path, basename(artifact.path)]
      for (const label of labels) {
        if (!label.trim()) continue
        const existing = pathByLabel.get(label)
        if (existing === undefined) {
          pathByLabel.set(label, artifact.path)
        } else if (existing !== artifact.path) {
          pathByLabel.set(label, null)
        }
      }
    }

    return Array.from(pathByLabel.entries())
      .filter((entry): entry is [string, string] => !!entry[1])
      .map(([label, path]) => ({ label, path }))
      .sort((a, b) => b.label.length - a.label.length)
  }

  function isBoundary(textValue: string, start: number, end: number): boolean {
    const before = start === 0 ? '' : textValue[start - 1]
    const after = end >= textValue.length ? '' : textValue[end]
    const beforeOkay = !before || !FILE_TOKEN_CHARS.test(before)
    const afterOkay = !after || !FILE_TOKEN_CHARS.test(after)
    return beforeOkay && afterOkay
  }

  function findNextArtifactMatch(
    textValue: string,
    offset: number,
    targets: ArtifactLinkTarget[],
  ): { index: number; target: ArtifactLinkTarget } | null {
    let best: { index: number; target: ArtifactLinkTarget } | null = null

    for (const target of targets) {
      let index = textValue.indexOf(target.label, offset)
      while (index !== -1) {
        const end = index + target.label.length
        if (isBoundary(textValue, index, end)) {
          if (!best || index < best.index || (index === best.index && target.label.length > best.target.label.length)) {
            best = { index, target }
          }
          break
        }
        index = textValue.indexOf(target.label, index + 1)
      }
    }

    return best
  }

  function linkifyArtifactReferences(targets: ArtifactLinkTarget[]) {
    if (!containerEl || targets.length === 0) return

    const walker = document.createTreeWalker(containerEl, NodeFilter.SHOW_TEXT)
    const textNodes: Text[] = []

    for (let node = walker.nextNode(); node; node = walker.nextNode()) {
      const textNode = node as Text
      const textValue = textNode.nodeValue || ''
      const parent = textNode.parentElement
      if (!textValue.trim() || !parent) continue
      if (parent.closest('a, button, pre, textarea, input, script, style')) continue
      textNodes.push(textNode)
    }

    for (const textNode of textNodes) {
      const textValue = textNode.nodeValue || ''
      let cursor = 0
      let foundAny = false
      const fragment = document.createDocumentFragment()

      while (cursor < textValue.length) {
        const match = findNextArtifactMatch(textValue, cursor, targets)
        if (!match) break

        foundAny = true
        if (match.index > cursor) {
          fragment.append(document.createTextNode(textValue.slice(cursor, match.index)))
        }

        const link = document.createElement('a')
        link.href = '#'
        link.className = 'artifact-inline-link'
        link.dataset.artifactPath = match.target.path
        link.textContent = match.target.label
        fragment.append(link)

        cursor = match.index + match.target.label.length
      }

      if (!foundAny) continue
      if (cursor < textValue.length) {
        fragment.append(document.createTextNode(textValue.slice(cursor)))
      }
      textNode.replaceWith(fragment)
    }
  }

  $effect(() => {
    if (!containerEl) return
    void text
    void artifacts
    void onArtifactOpen

    const handlers: Array<[Element, string, EventListener]> = []

    function on(el: Element, event: string, fn: EventListener) {
      el.addEventListener(event, fn)
      handlers.push([el, event, fn])
    }

    linkifyArtifactReferences(buildArtifactLinkTargets(artifacts))

    on(containerEl, 'click', (event) => {
      const target = event.target as HTMLElement | null
      const link = target?.closest<HTMLAnchorElement>('a[data-artifact-path]')
      if (!link) return
      event.preventDefault()
      const path = link.dataset.artifactPath
      if (path) onArtifactOpen?.(path)
    })

    // Copy buttons
    for (const btn of containerEl.querySelectorAll<HTMLButtonElement>('.code-copy')) {
      on(btn, 'click', () => {
        const code = btn.getAttribute('data-code') || ''
        navigator.clipboard.writeText(code).then(() => {
          const original = btn.textContent
          btn.textContent = 'Copied!'
          btn.classList.add('copied')
          setTimeout(() => {
            btn.textContent = original
            btn.classList.remove('copied')
          }, 1500)
        }).catch(() => {})
      })
    }

    // Code/Preview toggle buttons (for code-block and mermaid-block)
    for (const btn of containerEl.querySelectorAll<HTMLButtonElement>('.code-toggle')) {
      on(btn, 'click', () => {
        const block = btn.closest('.code-block, .mermaid-block')
        if (!block) return
        const mode = btn.getAttribute('data-mode')

        // Update active state on sibling toggles
        for (const sib of block.querySelectorAll('.code-toggle')) {
          sib.classList.toggle('active', sib === btn)
        }

        // For code-block (html/svg preview)
        const codeBlockPre = block.querySelector<HTMLElement>(':scope > pre')
        const codeBlockPreview = block.querySelector<HTMLElement>('[data-preview]')
        if (codeBlockPre && codeBlockPreview) {
          if (mode === 'preview') {
            codeBlockPre.style.display = 'none'
            codeBlockPreview.style.display = 'block'
          } else {
            codeBlockPre.style.display = ''
            codeBlockPreview.style.display = 'none'
          }
        }

        // For mermaid-block (code/diagram toggle)
        const mermaidSrc = block.querySelector<HTMLElement>('.mermaid-src')
        const mermaidPreview = block.querySelector<HTMLElement>('[data-mermaid-preview]')
        if (mermaidSrc && mermaidPreview) {
          if (mode === 'code') {
            mermaidSrc.style.display = ''
            mermaidPreview.style.display = 'none'
          } else {
            mermaidSrc.style.display = 'none'
            mermaidPreview.style.display = ''
          }
        }
      })
    }

    // Debounced mermaid rendering
    if (mermaidTimer) clearTimeout(mermaidTimer)
    mermaidTimer = setTimeout(() => renderMermaidBlocks(), 500)

    return () => {
      for (const [el, event, fn] of handlers) {
        el.removeEventListener(event, fn)
      }
    }
  })

  async function renderMermaidBlocks() {
    if (!containerEl) return
    const blocks = containerEl.querySelectorAll<HTMLDivElement>('.mermaid-block:not([data-rendered])')
    if (blocks.length === 0) return

    if (!mermaidModule) {
      try {
        const mod = await import('mermaid')
        mermaidModule = mod.default
        mermaidModule.initialize({
          startOnLoad: false,
          theme: 'dark',
          themeVariables: {
            darkMode: true,
            background: '#1c1c1c',
            primaryColor: '#e09145',
            primaryTextColor: '#e8e4df',
            primaryBorderColor: '#3a3a3a',
            lineColor: '#6b6560',
            secondaryColor: '#242424',
            tertiaryColor: '#2a2a2a',
          },
        })
      } catch {
        return
      }
    }

    const freshBlocks = containerEl.querySelectorAll<HTMLDivElement>('.mermaid-block:not([data-rendered])')
    for (let i = 0; i < freshBlocks.length; i++) {
      const block = freshBlocks[i]
      const graph = block.getAttribute('data-graph')
      if (!graph) continue
      const previewEl = block.querySelector<HTMLElement>('[data-mermaid-preview]')
      if (!previewEl) continue
      try {
        const id = `mermaid-${Date.now()}-${i}`
        const { svg } = await mermaidModule!.render(id, graph)
        previewEl.innerHTML = svg
        block.setAttribute('data-rendered', 'true')
      } catch {
        previewEl.innerHTML = '<span style="color:var(--error);font-size:var(--text-xs)">Diagram render failed</span>'
        block.classList.add('mermaid-error')
        block.setAttribute('data-rendered', 'true')
      }
    }
  }
</script>

<div class="chat-md" bind:this={containerEl}>
  {@html renderMarkdown(text || '\u2026')}
</div>

<style>
  .chat-md :global(a.artifact-inline-link) {
    display: inline-block;
    padding: 0 3px;
    border-radius: 3px;
    border-bottom: 1px dashed rgba(224, 145, 69, 0.45);
    background: rgba(224, 145, 69, 0.08);
    color: var(--accent-text);
  }

  .chat-md :global(a.artifact-inline-link:hover) {
    text-decoration: none;
    background: rgba(224, 145, 69, 0.14);
  }
</style>
