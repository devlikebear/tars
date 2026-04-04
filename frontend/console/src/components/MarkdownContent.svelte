<script lang="ts">
  import { renderMarkdown } from '../lib/markdown'

  interface Props {
    text: string
  }

  let { text }: Props = $props()

  let containerEl: HTMLDivElement | undefined = $state()
  let mermaidTimer: ReturnType<typeof setTimeout> | null = null
  let mermaidModule: typeof import('mermaid')['default'] | null = null

  $effect(() => {
    if (!containerEl) return
    void text

    const handlers: Array<[Element, string, EventListener]> = []

    function on(el: Element, event: string, fn: EventListener) {
      el.addEventListener(event, fn)
      handlers.push([el, event, fn])
    }

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

    // Code/Preview toggle buttons
    for (const btn of containerEl.querySelectorAll<HTMLButtonElement>('.code-toggle')) {
      on(btn, 'click', () => {
        const block = btn.closest('.code-block')
        if (!block) return
        const mode = btn.getAttribute('data-mode')
        const pre = block.querySelector('pre')
        const preview = block.querySelector<HTMLElement>('[data-preview]')
        if (!pre || !preview) return

        // Update active state on sibling toggles
        for (const sib of block.querySelectorAll('.code-toggle')) {
          sib.classList.toggle('active', sib === btn)
        }

        if (mode === 'preview') {
          pre.style.display = 'none'
          preview.style.display = 'block'
        } else {
          pre.style.display = ''
          preview.style.display = 'none'
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
