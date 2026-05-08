<script lang="ts">
  import { onDestroy, onMount, tick, untrack } from 'svelte'
  import { Terminal } from '@xterm/xterm'
  import { FitAddon } from '@xterm/addon-fit'
  import { WebLinksAddon } from '@xterm/addon-web-links'
  import { SearchAddon, type ISearchOptions } from '@xterm/addon-search'
  import { Unicode11Addon } from '@xterm/addon-unicode11'
  import type { WebglAddon as WebglAddonType } from '@xterm/addon-webgl'
  import { SerializeAddon } from '@xterm/addon-serialize'
  import '@xterm/xterm/css/xterm.css'
  import { terminalWebSocketURL } from '../lib/api'

  const FONT_SIZE_KEY = 'tars.terminal.fontSize'
  const FONT_FAMILY_KEY = 'tars.terminal.fontFamily'
  const DEFAULT_FONT_SIZE = 14
  const MIN_FONT_SIZE = 8
  const MAX_FONT_SIZE = 24

  // Font family presets. Every entry must resolve to a true monospace —
  // xterm assumes uniform glyph width, so non-monospace fallbacks break
  // rendering. The CSS string includes a system-monospace fallback so
  // the choice degrades gracefully when the named font isn't installed.
  type FontPreset = { id: string; label: string; css: string }
  const FONT_FAMILIES: FontPreset[] = [
    {
      id: 'jetbrains-mono',
      label: 'JetBrains Mono',
      css: "'JetBrains Mono', ui-monospace, 'SFMono-Regular', Menlo, Consolas, monospace",
    },
    {
      id: 'sf-mono',
      label: 'SF Mono / Menlo',
      css: "ui-monospace, 'SFMono-Regular', Menlo, Consolas, monospace",
    },
    {
      id: 'consolas',
      label: 'Consolas',
      css: "Consolas, 'Liberation Mono', Menlo, monospace",
    },
    {
      id: 'cascadia-code',
      label: 'Cascadia Code',
      css: "'Cascadia Code', 'Cascadia Mono', Consolas, Menlo, monospace",
    },
    {
      id: 'fira-code',
      label: 'Fira Code',
      css: "'Fira Code', 'JetBrains Mono', Menlo, Consolas, monospace",
    },
    {
      id: 'system',
      label: 'System default',
      css: 'monospace',
    },
  ]
  const DEFAULT_FONT_FAMILY_ID = 'jetbrains-mono'

  function fontFamilyByID(id: string): FontPreset {
    return FONT_FAMILIES.find((f) => f.id === id) ?? FONT_FAMILIES[0]
  }

  function loadFontSize(): number {
    if (typeof localStorage === 'undefined') return DEFAULT_FONT_SIZE
    const raw = localStorage.getItem(FONT_SIZE_KEY)
    const parsed = raw ? parseInt(raw, 10) : NaN
    if (!Number.isFinite(parsed)) return DEFAULT_FONT_SIZE
    return Math.max(MIN_FONT_SIZE, Math.min(MAX_FONT_SIZE, parsed))
  }

  function loadFontFamilyID(): string {
    if (typeof localStorage === 'undefined') return DEFAULT_FONT_FAMILY_ID
    const raw = localStorage.getItem(FONT_FAMILY_KEY)
    if (!raw) return DEFAULT_FONT_FAMILY_ID
    // Reject unknown IDs so a stale localStorage value can't break xterm
    // with a non-monospace font.
    return FONT_FAMILIES.some((f) => f.id === raw) ? raw : DEFAULT_FONT_FAMILY_ID
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
  let menuEl: HTMLDivElement | undefined = $state()
  let settingsOpen = $state(false)
  let settingsFontFamilyID = $state(loadFontFamilyID())
  let settingsFontSize = $state(loadFontSize())

  let terminal: Terminal | null = null
  let fitAddon: FitAddon | null = null
  let searchAddon: SearchAddon | null = null
  let serializeAddon: SerializeAddon | null = null
  let webglAddon: WebglAddonType | null = null
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
    if (terminal.options.fontSize === clamped) {
      settingsFontSize = clamped
      return
    }
    terminal.options.fontSize = clamped
    settingsFontSize = clamped
    try {
      localStorage.setItem(FONT_SIZE_KEY, String(clamped))
    } catch {
      // localStorage may be disabled — non-fatal.
    }
    fitAndResize()
  }

  function applyFontFamily(id: string) {
    if (!terminal) return
    const preset = fontFamilyByID(id)
    settingsFontFamilyID = preset.id
    if (terminal.options.fontFamily === preset.css) return
    terminal.options.fontFamily = preset.css
    try {
      localStorage.setItem(FONT_FAMILY_KEY, preset.id)
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
    // Clamp to the wrap rect after the menu mounts so it doesn't extend
    // past the bottom/right edge of the terminal frame (#669).
    void tick().then(() => {
      if (!menuOpen || !menuEl) return
      const wrap = menuEl.parentElement
      if (!wrap) return
      const wrapRect = wrap.getBoundingClientRect()
      const menuRect = menuEl.getBoundingClientRect()
      const margin = 4
      const maxX = Math.max(0, wrapRect.width - menuRect.width - margin)
      const maxY = Math.max(0, wrapRect.height - menuRect.height - margin)
      if (menuX > maxX) menuX = maxX
      if (menuY > maxY) menuY = maxY
    })
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

    // When the right-click menu is open, swallow Escape here so xterm
    // doesn't write an ESC byte to the shell. The window-level keydown
    // listener fires too late on focused xterm — pre-empting in the
    // custom handler is the cheapest fix.
    if (menuOpen && e.key === 'Escape') {
      closeMenu()
      return false
    }

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
      fontFamily: fontFamilyByID(loadFontFamilyID()).css,
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
      // Load WebGL renderer AFTER the first fit so the canvas is created
      // at the correct dimensions. Initializing it before fit (when the
      // pane may have just transitioned from display:none → flex) leaves
      // the canvas with stale size and produces a blank terminal even
      // though the buffer is being written to.
      void import('@xterm/addon-webgl')
        .then(({ WebglAddon }) => {
          if (!terminal) return
          const addon = new WebglAddon()
          addon.onContextLoss(() => {
            addon.dispose()
            webglAddon = null
          })
          terminal.loadAddon(addon)
          webglAddon = addon
          terminal.refresh(0, terminal.rows - 1)
        })
        .catch(() => {
          // WebGL unavailable — fall back to default DOM renderer.
        })
      // Force a redraw so any output already buffered actually paints.
      if (terminal) terminal.refresh(0, terminal.rows - 1)
      connect()
      terminal?.focus()
    })

    // Web fonts (e.g. JetBrains Mono) may not be ready when xterm
    // measures glyph metrics on first mount. If we don't refit after
    // they finish loading, the terminal renders with the fallback's
    // metrics — uneven spacing, drifting cursor — until the next
    // resize. fonts.ready resolves once all currently-loading fonts
    // settle; the refresh + fit call brings xterm back in sync (#686).
    if (typeof document !== 'undefined' && document.fonts) {
      document.fonts.ready
        .then(() => {
          if (!terminal) return
          fitAndResize()
          terminal.refresh(0, terminal.rows - 1)
        })
        .catch(() => {
          // fonts.ready can reject on edge browsers — non-fatal.
        })
    }
  })

  $effect(() => {
    if (visible && terminal) {
      // Refit + redraw + focus after the browser applies the now-visible
      // layout. The refresh() call repaints the canvas so a tab returning
      // from display:none doesn't show a stale (often blank) frame.
      requestAnimationFrame(() => {
        fitAndResize()
        if (terminal) terminal.refresh(0, terminal.rows - 1)
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
      <button
        type="button"
        class="btn btn-ghost btn-sm"
        onclick={() => (settingsOpen = !settingsOpen)}
        aria-expanded={settingsOpen}
        title="Terminal font settings"
      >
        Aa
      </button>
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
  {#if settingsOpen}
    <div class="terminal-settings" role="region" aria-label="Terminal font settings">
      <label class="terminal-settings-row">
        <span>Font</span>
        <select
          value={settingsFontFamilyID}
          onchange={(e) => applyFontFamily((e.currentTarget as HTMLSelectElement).value)}
        >
          {#each FONT_FAMILIES as preset (preset.id)}
            <option value={preset.id}>{preset.label}</option>
          {/each}
        </select>
      </label>
      <label class="terminal-settings-row">
        <span>Size</span>
        <input
          type="range"
          min={MIN_FONT_SIZE}
          max={MAX_FONT_SIZE}
          step="1"
          value={settingsFontSize}
          oninput={(e) => applyFontSize(Number((e.currentTarget as HTMLInputElement).value))}
        />
        <span class="terminal-settings-value">{settingsFontSize}px</span>
      </label>
      <button
        type="button"
        class="btn btn-ghost btn-sm"
        onclick={() => {
          applyFontSize(DEFAULT_FONT_SIZE)
          applyFontFamily(DEFAULT_FONT_FAMILY_ID)
        }}
        title="Reset to defaults"
      >
        Reset
      </button>
      <button type="button" class="btn btn-ghost btn-sm" onclick={() => (settingsOpen = false)} title="Close">✕</button>
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
        bind:this={menuEl}
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

  .terminal-settings {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--border-subtle);
    background: var(--surface-raised);
    flex-shrink: 0;
    font-family: var(--font-display);
    font-size: var(--text-xs);
  }

  .terminal-settings-row {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    color: var(--text-secondary);
  }

  .terminal-settings-row > span:first-child {
    min-width: 36px;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }

  .terminal-settings-row select,
  .terminal-settings-row input[type='range'] {
    background: var(--surface-inset);
    color: var(--text-primary);
    border: 1px solid var(--border-subtle);
    border-radius: 4px;
    padding: 2px 6px;
    font-family: var(--font-display);
    font-size: var(--text-xs);
  }

  .terminal-settings-row input[type='range'] {
    padding: 0;
    min-width: 120px;
    accent-color: var(--accent-primary, #e09145);
  }

  .terminal-settings-value {
    font-variant-numeric: tabular-nums;
    color: var(--text-primary);
    min-width: 36px;
    text-align: right;
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
