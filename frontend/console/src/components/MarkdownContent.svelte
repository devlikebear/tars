<script lang="ts">
  import { renderMarkdown } from '../lib/markdown'

  interface Props {
    text: string
  }

  let { text }: Props = $props()

  let containerEl: HTMLDivElement | undefined = $state()
  let mermaidTimer: ReturnType<typeof setTimeout> | null = null
  let mermaidModule: typeof import('mermaid')['default'] | null = null

  // Attach copy-button click handlers (runs on every text change)
  $effect(() => {
    if (!containerEl) return
    void text

    // Copy buttons
    const copyButtons = containerEl.querySelectorAll<HTMLButtonElement>('.code-copy')
    const handlers: Array<[HTMLButtonElement, () => void]> = []
    for (const btn of copyButtons) {
      const handler = () => {
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
      }
      btn.addEventListener('click', handler)
      handlers.push([btn, handler])
    }

    // Debounced mermaid rendering — wait for streaming to settle
    if (mermaidTimer) clearTimeout(mermaidTimer)
    mermaidTimer = setTimeout(() => renderMermaidBlocks(), 500)

    return () => {
      for (const [btn, handler] of handlers) {
        btn.removeEventListener('click', handler)
      }
    }
  })

  async function renderMermaidBlocks() {
    if (!containerEl) return
    const blocks = containerEl.querySelectorAll<HTMLDivElement>('.mermaid-block:not([data-rendered])')
    if (blocks.length === 0) return

    // Lazy load mermaid once
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

    // Re-query after async load — DOM may have changed
    const freshBlocks = containerEl.querySelectorAll<HTMLDivElement>('.mermaid-block:not([data-rendered])')
    for (let i = 0; i < freshBlocks.length; i++) {
      const block = freshBlocks[i]
      const graph = block.getAttribute('data-graph')
      if (!graph) continue
      try {
        const id = `mermaid-${Date.now()}-${i}`
        const { svg } = await mermaidModule!.render(id, graph)
        block.innerHTML = svg
        block.setAttribute('data-rendered', 'true')
      } catch {
        block.classList.add('mermaid-error')
        block.setAttribute('data-rendered', 'true')
      }
    }
  }
</script>

<div class="chat-md" bind:this={containerEl}>
  {@html renderMarkdown(text || '\u2026')}
</div>
