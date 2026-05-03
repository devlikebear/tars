<script lang="ts">
  import { onDestroy, onMount, tick, untrack } from 'svelte'
  import { Terminal } from '@xterm/xterm'
  import { FitAddon } from '@xterm/addon-fit'
  import { WebLinksAddon } from '@xterm/addon-web-links'
  import { SearchAddon, type ISearchOptions } from '@xterm/addon-search'
  import { Unicode11Addon } from '@xterm/addon-unicode11'
  import { WebglAddon } from '@xterm/addon-webgl'
  import { SerializeAddon } from '@xterm/addon-serialize'
  import '@xterm/xterm/css/xterm.css'
  import { terminalWebSocketURL } from '../lib/api'

  const FONT_SIZE_KEY = 'tars.terminal.fontSize'
  const DEFAULT_FONT_SIZE = 12
  const MIN_FONT_SIZE = 8
  const MAX_FONT_SIZE = 24

  function loadFontSize(): number {
    if (typeof localStorage === 'undefined') return DEFAULT_FONT_SIZE
    const raw = localStorage.getItem(FONT_SIZE_KEY)
    const parsed = raw ? parseInt(raw, 10) : NaN
    if (!Number.isFinite(parsed)) return DEFAULT_FONT_SIZE
    return Math.max(MIN_FONT_SIZE, Math.min(MAX_FONT_SIZE, parsed))
  }

  interface Props {
    sessionId: string
    cwd: string
    label: string
    onClose: () => void
    visible?: boolean
    hideLabel?: boolean
    onStatusChange?: (state: { connected: boolean; status: string; error: string }) => void
  }

  let { sessionId, cwd, label, onClose, visible = true, hideLabel = false, onStatusChange }: Props = $props()

  let container: HTMLDivElement | undefined = $state()
  let searchInputEl: HTMLInputElement | undefined = $state()
  let status = $state('Connecting')
  let error = $state('')
  let connected = $state(false)
  let searchOpen = $state(false)
  let searchQuery = $state('')
  let searchCaseSensitive = $state(false)
  let searchRegex = $state(false)
  let bellFlash = $state(false)
  let menuOpen = $state(false)
  let menuX = $state(0)
  let menuY = $state(0)
  let menuHasSelection = $state(false)

  let terminal: Terminal | null = null
  let fitAddon: FitAddon | null = null
  let searchAddon: SearchAddon | null = null
  let serializeAddon: SerializeAddon | null = null
  let webglAddon: WebglAddon | null = null
  let socket: WebSocket | null = null
  let resizeObserver: ResizeObserver | null = null
  let bellTimer: ReturnType<typeof setTimeout> | null = null

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

  function applyFontSize(size: number) {
    if (!terminal) return
    const clamped = Math.max(MIN_FONT_SIZE, Math.min(MAX_FONT_SIZE, Math.round(size)))
    if (terminal.options.fontSize === clamped) return
    terminal.options.fontSize = clamped
    try {
      localStorage.setItem(FONT_SIZE_KEY, String(clamped))
    } catch {
      // localStorage may be disabled — non-fatal.
    }
    fitAndResize()
  }

  function zoomIn() {
    if (!terminal) return
    applyFontSize((terminal.options.fontSize ?? DEFAULT_FONT_SIZE) + 1)
  }

  function zoomOut() {
    if (!terminal) return
    applyFontSize((terminal.options.fontSize ?? DEFAULT_FONT_SIZE) - 1)
  }

  function zoomReset() {
    applyFontSize(DEFAULT_FONT_SIZE)
  }

  function clearTerminal() {
    terminal?.clear()
    terminal?.scrollToBottom()
  }

  function copySelection() {
    const sel = terminal?.getSelection() ?? ''
    if (!sel) return
    navigator.clipboard?.writeText(sel).catch(() => {})
  }

  function pasteFromClipboard() {
    navigator.clipboard
      ?.readText()
      .then((text) => {
        if (text) send({ type: 'input', data: encodeBase64(text) })
      })
      .catch(() => {})
  }

  function saveBuffer() {
    if (!serializeAddon) return
    let payload: string
    try {
      payload = serializeAddon.serialize()
    } catch {
      // Fallback to plain text iteration if serialize fails.
      const buf = terminal?.buffer.active
      if (!buf) return
      const lines: string[] = []
      for (let i = 0; i < buf.length; i++) {
        lines.push(buf.getLine(i)?.translateToString(true) ?? '')
      }
      payload = lines.join('\n')
    }
    const blob = new Blob([payload], { type: 'text/plain;charset=utf-8' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    const stamp = new Date().toISOString().replace(/[:.]/g, '-')
    const safeLabel = (label || 'terminal').replace(/[^a-zA-Z0-9_-]+/g, '-').slice(0, 40) || 'terminal'
    a.href = url
    a.download = `${safeLabel}-${stamp}.log`
    document.body.appendChild(a)
    a.click()
    a.remove()
    URL.revokeObjectURL(url)
  }

  function openMenu(e: MouseEvent) {
    e.preventDefault()
    menuHasSelection = !!terminal?.getSelection()
    const rect = container?.getBoundingClientRect()
    if (rect) {
      menuX = e.clientX - rect.left
      menuY = e.clientY - rect.top
    } else {
      menuX = e.clientX
      menuY = e.clientY
    }
    menuOpen = true
  }

  function closeMenu() {
    menuOpen = false
  }

  function onMenuItem(action: 'copy' | 'paste' | 'clear' | 'save') {
    closeMenu()
    if (action === 'copy') copySelection()
    else if (action === 'paste') pasteFromClipboard()
    else if (action === 'clear') clearTerminal()
    else if (action === 'save') saveBuffer()
    if (action !== 'save') terminal?.focus()
  }

  function onWindowKeydown(e: KeyboardEvent) {
    if (menuOpen && e.key === 'Escape') {
      e.preventDefault()
      closeMenu()
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
      pasteFromClipboard()
      return false
    }
    if (modFind && key === 'f') {
      void openSearch()
      return false
    }

    // Cmd+K (mac) / Ctrl+Shift+K (others): clear + scroll to bottom.
    if (modCopy && key === 'k') {
      clearTerminal()
      return false
    }

    // Font-size shortcuts: Cmd+= / Cmd+- / Cmd+0 (mac) and Ctrl+= / Ctrl+- / Ctrl+0 (others).
    // Use plain Ctrl on Linux/Win for zoom (no Shift) since this is a common terminal emulator convention.
    const modZoom = isMac ? e.metaKey && !e.ctrlKey : e.ctrlKey && !e.shiftKey && !e.altKey
    if (modZoom && (key === '=' || key === '+')) {
      zoomIn()
      return false
    }
    if (modZoom && key === '-') {
      zoomOut()
      return false
    }
    if (modZoom && key === '0') {
      zoomReset()
      return false
    }

    return true
  }

  onMount(() => {
    if (!container) return
    terminal = new Terminal({
      cursorBlink: true,
      fontFamily: "var(--font-mono), Menlo, Consolas, 'Liberation Mono', monospace",
      fontSize: loadFontSize(),
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
    serializeAddon = new SerializeAddon()
    terminal.loadAddon(serializeAddon)
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
    terminal.onBell(() => {
      bellFlash = true
      if (bellTimer) clearTimeout(bellTimer)
      bellTimer = setTimeout(() => {
        bellFlash = false
        bellTimer = null
      }, 600)
    })

    container.addEventListener('contextmenu', openMenu)
    window.addEventListener('keydown', onWindowKeydown)

    resizeObserver = new ResizeObserver(() => fitAndResize())
    resizeObserver.observe(container)
    requestAnimationFrame(() => {
      fitAndResize()
      connect()
      terminal?.focus()
    })
  })

  $effect(() => {
    if (visible && terminal) {
      // Refit + focus after the browser applies the now-visible layout.
      requestAnimationFrame(() => {
        fitAndResize()
        terminal?.focus()
      })
    }
  })

  $effect(() => {
    // Track only the data fields. Read the callback inside untrack so a parent
    // re-rendering an inline arrow (new reference each time) doesn't reschedule
    // this effect and create a write→render→re-run loop.
    const snapshot = { connected, status, error }
    untrack(() => onStatusChange?.(snapshot))
  })

  onDestroy(() => {
    resizeObserver?.disconnect()
    if (bellTimer) clearTimeout(bellTimer)
    if (container) container.removeEventListener('contextmenu', openMenu)
    window.removeEventListener('keydown', onWindowKeydown)
    send({ type: 'close' })
    socket?.close()
    webglAddon?.dispose()
    searchAddon?.dispose()
    serializeAddon?.dispose()
    terminal?.dispose()
    socket = null
    terminal = null
    fitAddon = null
    searchAddon = null
    serializeAddon = null
    webglAddon = null
    bellTimer = null
  })
</script>

<div class="integrated-terminal">
  <div class="terminal-header" class:compact={hideLabel}>
    <div class="terminal-title">
      {#if !hideLabel}
        <span class="terminal-dot" class:connected class:bell={bellFlash}></span>
        <span class="terminal-label">{label}</span>
      {/if}
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
      {#if !hideLabel}
        <button type="button" class="btn btn-ghost btn-sm" onclick={onClose}>Close</button>
      {/if}
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
  <div class="terminal-frame-wrap">
    <div class="terminal-frame" bind:this={container}></div>
    {#if menuOpen}
      <div
        class="terminal-menu-overlay"
        role="presentation"
        onclick={closeMenu}
        oncontextmenu={(e) => {
          e.preventDefault()
          closeMenu()
        }}
      ></div>
      <div
        class="terminal-menu"
        role="menu"
        style="left: {menuX}px; top: {menuY}px"
      >
        <button type="button" role="menuitem" disabled={!menuHasSelection} onclick={() => onMenuItem('copy')}>
          <span>Copy</span>
          <kbd>{isMac ? '⌘C' : 'Ctrl+Shift+C'}</kbd>
        </button>
        <button type="button" role="menuitem" onclick={() => onMenuItem('paste')}>
          <span>Paste</span>
          <kbd>{isMac ? '⌘V' : 'Ctrl+Shift+V'}</kbd>
        </button>
        <div class="terminal-menu-sep" role="separator"></div>
        <button type="button" role="menuitem" onclick={() => onMenuItem('clear')}>
          <span>Clear</span>
          <kbd>{isMac ? '⌘K' : 'Ctrl+Shift+K'}</kbd>
        </button>
        <button type="button" role="menuitem" onclick={() => onMenuItem('save')}>
          <span>Save buffer…</span>
        </button>
      </div>
    {/if}
  </div>
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

  .terminal-dot.bell {
    background: var(--accent, #e09145);
    animation: terminal-bell-flash 0.6s ease-out;
  }

  @keyframes terminal-bell-flash {
    0% {
      box-shadow: 0 0 0 0 var(--accent, #e09145);
      transform: scale(1.4);
    }
    100% {
      box-shadow: 0 0 0 6px transparent;
      transform: scale(1);
    }
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

  .terminal-frame-wrap {
    position: relative;
    flex: 1;
    min-height: 0;
    display: flex;
  }

  .terminal-frame {
    flex: 1;
    min-height: 0;
    padding: var(--space-2);
    overflow: hidden;
    user-select: text;
  }

  .terminal-menu-overlay {
    position: fixed;
    inset: 0;
    z-index: 40;
  }

  .terminal-menu {
    position: absolute;
    z-index: 41;
    min-width: 200px;
    background: var(--surface);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.45);
    padding: 4px;
    display: flex;
    flex-direction: column;
  }

  .terminal-menu button {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    background: transparent;
    border: none;
    color: var(--text-primary);
    font-family: var(--font-display);
    font-size: var(--text-xs);
    text-align: left;
    padding: 6px 10px;
    border-radius: var(--radius-sm);
    cursor: pointer;
  }

  .terminal-menu button:hover:not(:disabled) {
    background: var(--surface-inset);
    color: var(--accent, #e09145);
  }

  .terminal-menu button:disabled {
    color: var(--text-ghost);
    cursor: not-allowed;
  }

  .terminal-menu kbd {
    font-family: var(--font-mono);
    font-size: 10px;
    color: var(--text-ghost);
    background: transparent;
    padding: 0;
  }

  .terminal-menu-sep {
    height: 1px;
    background: var(--border-subtle);
    margin: 4px 0;
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
