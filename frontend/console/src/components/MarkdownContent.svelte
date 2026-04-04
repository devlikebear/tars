<script lang="ts">
  import { renderMarkdown } from '../lib/markdown'

  interface Props {
    text: string
  }

  let { text }: Props = $props()

  let containerEl: HTMLDivElement | undefined = $state()

  // Attach copy-button click handlers and render mermaid blocks
  $effect(() => {
    if (!containerEl) return
    // Re-run when text changes (renderMarkdown is called in the template)
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

    // Mermaid blocks — lazy load
    const mermaidBlocks = containerEl.querySelectorAll<HTMLDivElement>('.mermaid-block:not([data-rendered])')
    if (mermaidBlocks.length > 0) {
      import('mermaid').then(({ default: mermaid }) => {
        mermaid.initialize({
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

        mermaidBlocks.forEach(async (block, i) => {
          const graph = block.getAttribute('data-graph')
          if (!graph) return
          try {
            const id = `mermaid-${Date.now()}-${i}`
            const { svg } = await mermaid.render(id, graph)
            block.innerHTML = svg
            block.setAttribute('data-rendered', 'true')
          } catch {
            // Leave the source code visible on render failure
            block.classList.add('mermaid-error')
          }
        })
      }).catch(() => {
        // Mermaid not available — leave source visible
      })
    }

    return () => {
      for (const [btn, handler] of handlers) {
        btn.removeEventListener('click', handler)
      }
    }
  })
</script>

<div class="chat-md" bind:this={containerEl}>
  {@html renderMarkdown(text || '\u2026')}
</div>
