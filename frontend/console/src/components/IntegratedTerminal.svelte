<script lang="ts">
  import { onDestroy, onMount, tick } from 'svelte'
  import { Terminal } from '@xterm/xterm'
  import { FitAddon } from '@xterm/addon-fit'
  import { WebLinksAddon } from '@xterm/addon-web-links'
  import { SearchAddon, type ISearchOptions } from '@xterm/addon-search'
  import { Unicode11Addon } from '@xterm/addon-unicode11'
  import { WebglAddon } from '@xterm/addon-webgl'
  import '@xterm/xterm/css/xterm.css'
  import { terminalWebSocketURL } from '../lib/api'

  interface Props {
    sessionId: string
    cwd: string
    label: string
    onClose: () => void
  }

  let { sessionId, cwd, label, onClose }: Props = $props()

  let container: HTMLDivElement | undefined = $state()
  let searchInputEl: HTMLInputElement | undefined = $state()
  let status = $state('Connecting')
  let error = $state('')
  let connected = $state(false)
  let searchOpen = $state(false)
  let searchQuery = $state('')
  let searchCaseSensitive = $state(false)
  let searchRegex = $state(false)

  let terminal: Terminal | null = null
  let fitAddon: FitAddon | null = null
  let searchAddon: SearchAddon | null = null
  let webglAddon: WebglAddon | null = null
  let socket: WebSocket | null = null
  let resizeObserver: ResizeObserver | null = null

  const isMac = typeof navigator !== 'undefined' && /Mac|iPhone|iPad/i.test(navigator.platform)

  function encodeBase64(value: string): string {
    const bytes = new TextEncoder().encode(value)
    let binary = ''
    const chunkSize = 0x8000
    for (let i = 0; i < bytes.length; i += chunkSize) {
      binary += String.fromCharCode(...bytes.slice(i, i + chunkSize))
    }
    return btoa(binary)
  }

  function decodeBase64(value: string): string {
    const binary = atob(value)
    const bytes = new Uint8Array(binary.length)
    for (let i = 0; i < binary.length; i++) {
      bytes[i] = binary.charCodeAt(i)
    }
    return new TextDecoder().decode(bytes)
  }

  function send(message: Record<string, unknown>) {
    if (!socket || socket.readyState !== WebSocket.OPEN) return
    socket.send(JSON.stringify(message))
  }

  function fitAndResize() {
    if (!terminal || !fitAddon) return
    try {
      fitAddon.fit()
      send({ type: 'resize', cols: terminal.cols, rows: terminal.rows })
    } catch {
      // xterm may not be measurable before the panel has layout.
    }
  }

  function connect() {
    if (!terminal) return
    socket = new WebSocket(terminalWebSocketURL(sessionId, cwd, terminal.cols || 80, terminal.rows || 24))

    socket.onopen = () => {
      connected = true
      error = ''
      status = 'Connected'
      fitAndResize()
    }
    socket.onmessage = (event) => {
      try {
        const message = JSON.parse(String(event.data)) as {
          type?: string
          data?: string
          cwd?: string
          message?: string
        }
        switch (message.type) {
          case 'ready':
            status = message.cwd ? message.cwd : 'Connected'
            break
          case 'output':
            if (message.data) terminal?.write(decodeBase64(message.data))
            break
          case 'error':
            error = message.message || 'Terminal error'
            if (message.message) terminal?.writeln(`\r\n${message.message}`)
            break
          case 'exit':
            connected = false
            status = 'Exited'
            break
        }
      } catch (err) {
        error = err instanceof Error ? err.message : 'Invalid terminal message'
      }
    }
    socket.onerror = () => {
      error = 'Terminal connection failed'
    }
    socket.onclose = () => {
      connected = false
      if (!error) status = 'Disconnected'
    }
  }

  function reconnect() {
    if (socket && socket.readyState !== WebSocket.CLOSED) {
      socket.close()
    }
    error = ''
    status = 'Connecting'
    connect()
  }

  async function openSearch() {
    searchOpen = true
    await tick()
    searchInputEl?.focus()
    searchInputEl?.select()
  }

  function closeSearch() {
    searchOpen = false
    searchAddon?.clearDecorations()
    terminal?.focus()
  }

  function searchOptions(): ISearchOptions {
    return {
      caseSensitive: searchCaseSensitive,
      regex: searchRegex,
      decorations: {
        matchBackground: '#a45a1f',
        matchBorder: '#e09145',
        matchOverviewRuler: '#e09145',
        activeMatchBackground: '#e09145',
        activeMatchBorder: '#ffffff',
        activeMatchColorOverviewRuler: '#ffffff',
      },
    }
  }

  function findNext() {
    if (!searchAddon || !searchQuery) return
    searchAddon.findNext(searchQuery, searchOptions())
  }

  function findPrevious() {
    if (!searchAddon || !searchQuery) return
    searchAddon.findPrevious(searchQuery, searchOptions())
  }

  function onSearchKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.preventDefault()
      closeSearch()
    } else if (e.key === 'Enter') {
      e.preventDefault()
      if (e.shiftKey) findPrevious()
      else findNext()
    }
  }

  function customKeyHandler(e: KeyboardEvent): boolean {
    if (e.type !== 'keydown') return true
    const key = e.key.toLowerCase()
    const modCopy = isMac ? e.metaKey && !e.ctrlKey : e.ctrlKey && e.shiftKey
    const modPaste = modCopy
    const modFind = modCopy

    if (modCopy && key === 'c') {
      const sel = terminal?.getSelection() ?? ''
      if (sel) {
        navigator.clipboard?.writeText(sel).catch(() => {})
        return false
      }
      // No selection: fall through (Ctrl+C → SIGINT on Linux/Win; Cmd+C is no-op)
      return true
    }
    if (modPaste && key === 'v') {
      navigator.clipboard
        ?.readText()
        .then((text) => {
          if (text) send({ type: 'input', data: encodeBase64(text) })
        })
        .catch(() => {})
      return false
    }
    if (modFind && key === 'f') {
      void openSearch()
      return false
    }
    return true
  }

  onMount(() => {
    if (!container) return
    terminal = new Terminal({
      cursorBlink: true,
      fontFamily: "var(--font-mono), Menlo, Consolas, 'Liberation Mono', monospace",
      fontSize: 12,
      lineHeight: 1.2,
      scrollback: 2000,
      convertEol: true,
      rightClickSelectsWord: true,
      allowProposedApi: true,
      theme: {
        background: '#0d0d0d',
        foreground: '#e8e3da',
        cursor: '#f0a04b',
        selectionBackground: '#a45a1f',
        selectionForeground: '#ffffff',
        black: '#121212',
        red: '#e06c75',
        green: '#98c379',
        yellow: '#e5c07b',
        blue: '#61afef',
        magenta: '#c678dd',
        cyan: '#56b6c2',
        white: '#e8e3da',
        brightBlack: '#5f5f5f',
        brightWhite: '#ffffff',
      },
    })

    terminal.open(container)

    fitAddon = new FitAddon()
    terminal.loadAddon(fitAddon)
    terminal.loadAddon(new WebLinksAddon())
    searchAddon = new SearchAddon()
    terminal.loadAddon(searchAddon)
    terminal.loadAddon(new Unicode11Addon())
    terminal.unicode.activeVersion = '11'

    try {
      const addon = new WebglAddon()
      addon.onContextLoss(() => {
        addon.dispose()
        webglAddon = null
      })
      terminal.loadAddon(addon)
      webglAddon = addon
    } catch {
      // WebGL unavailable — fall back to default DOM renderer.
    }

    terminal.attachCustomKeyEventHandler(customKeyHandler)
    terminal.onData((data) => {
      send({ type: 'input', data: encodeBase64(data) })
    })

    resizeObserver = new ResizeObserver(() => fitAndResize())
    resizeObserver.observe(container)
    requestAnimationFrame(() => {
      fitAndResize()
      connect()
      terminal?.focus()
    })
  })

  onDestroy(() => {
    resizeObserver?.disconnect()
    send({ type: 'close' })
    socket?.close()
    webglAddon?.dispose()
    searchAddon?.dispose()
    terminal?.dispose()
    socket = null
    terminal = null
    fitAddon = null
    searchAddon = null
    webglAddon = null
  })
</script>

<div class="integrated-terminal">
  <div class="terminal-header">
    <div class="terminal-title">
      <span class="terminal-dot" class:connected></span>
      <span class="terminal-label">{label}</span>
      <button
        type="button"
        class="terminal-status"
        class:reconnect={!connected}
        onclick={reconnect}
        title={connected ? 'Reconnect' : 'Click to reconnect'}
      >
        {error || status}
      </button>
    </div>
    <div class="terminal-actions">
      <button type="button" class="btn btn-ghost btn-sm" onclick={openSearch} title="Find ({isMac ? '⌘F' : 'Ctrl+Shift+F'})">Find</button>
      <button type="button" class="btn btn-ghost btn-sm" onclick={onClose}>Close</button>
    </div>
  </div>
  {#if searchOpen}
    <div class="terminal-search">
      <input
        bind:this={searchInputEl}
        bind:value={searchQuery}
        onkeydown={onSearchKeydown}
        placeholder="Find in terminal…"
        spellcheck="false"
        autocomplete="off"
      />
      <label class:active={searchCaseSensitive} title="Case sensitive">
        <input type="checkbox" bind:checked={searchCaseSensitive} />
        <span>Aa</span>
      </label>
      <label class:active={searchRegex} title="Regex">
        <input type="checkbox" bind:checked={searchRegex} />
        <span>.*</span>
      </label>
      <button type="button" class="btn btn-ghost btn-sm" onclick={findPrevious} title="Previous (Shift+Enter)">↑</button>
      <button type="button" class="btn btn-ghost btn-sm" onclick={findNext} title="Next (Enter)">↓</button>
      <button type="button" class="btn btn-ghost btn-sm" onclick={closeSearch} title="Close (Esc)">✕</button>
    </div>
  {/if}
  <div class="terminal-frame" bind:this={container}></div>
</div>

<style>
  .integrated-terminal {
    display: flex;
    flex-direction: column;
    min-height: 0;
    flex: 1;
    background: var(--surface-inset);
    border-top: 1px solid var(--border-subtle);
  }

  .terminal-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--border-subtle);
    flex-shrink: 0;
  }

  .terminal-title {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex: 1;
    min-width: 0;
  }

  .terminal-actions {
    display: flex;
    align-items: center;
    gap: var(--space-1);
    flex-shrink: 0;
  }

  .terminal-dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: var(--text-ghost);
    flex-shrink: 0;
  }

  .terminal-dot.connected {
    background: var(--success);
  }

  .terminal-label {
    color: var(--text-primary);
    font-family: var(--font-display);
    font-size: var(--text-xs);
    font-weight: 600;
    max-width: 42%;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .terminal-status {
    color: var(--text-ghost);
    font-family: var(--font-mono);
    font-size: 10px;
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    text-align: left;
    background: transparent;
    border: none;
    padding: 0;
    cursor: pointer;
  }

  .terminal-status.reconnect {
    color: var(--accent, #e09145);
    text-decoration: underline dotted;
  }

  .terminal-search {
    display: flex;
    align-items: center;
    gap: var(--space-1);
    padding: var(--space-1) var(--space-3);
    border-bottom: 1px solid var(--border-subtle);
    background: var(--surface);
    flex-shrink: 0;
  }

  .terminal-search input:not([type='checkbox']) {
    flex: 1;
    min-width: 0;
    background: var(--surface-inset);
    border: 1px solid var(--border-subtle);
    color: var(--text-primary);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    padding: 4px 8px;
    border-radius: var(--radius-sm);
  }

  .terminal-search input:not([type='checkbox']):focus {
    outline: none;
    border-color: var(--accent, #e09145);
  }

  .terminal-search label {
    display: inline-flex;
    align-items: center;
    cursor: pointer;
    color: var(--text-ghost);
    font-family: var(--font-mono);
    font-size: 11px;
    padding: 2px 6px;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    user-select: none;
  }

  .terminal-search label.active {
    color: var(--accent, #e09145);
    border-color: var(--accent, #e09145);
  }

  .terminal-search label input {
    display: none;
  }

  .terminal-frame {
    flex: 1;
    min-height: 0;
    padding: var(--space-2);
    overflow: hidden;
    user-select: text;
  }

  :global(.xterm) {
    height: 100%;
  }

  :global(.xterm-viewport) {
    border-radius: var(--radius-sm);
  }

  :global(.xterm .xterm-screen),
  :global(.xterm .xterm-viewport) {
    user-select: text;
  }
</style>
