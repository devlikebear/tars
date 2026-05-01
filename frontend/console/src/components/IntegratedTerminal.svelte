<script lang="ts">
  import { onDestroy, onMount } from 'svelte'
  import { Terminal } from '@xterm/xterm'
  import { FitAddon } from '@xterm/addon-fit'
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
  let status = $state('Connecting')
  let error = $state('')
  let connected = $state(false)

  let terminal: Terminal | null = null
  let fitAddon: FitAddon | null = null
  let socket: WebSocket | null = null
  let resizeObserver: ResizeObserver | null = null

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

  onMount(() => {
    if (!container) return
    terminal = new Terminal({
      cursorBlink: true,
      fontFamily: 'var(--font-mono)',
      fontSize: 12,
      lineHeight: 1.2,
      scrollback: 2000,
      convertEol: true,
      theme: {
        background: '#0d0d0d',
        foreground: '#e8e3da',
        cursor: '#f0a04b',
        selectionBackground: '#704622',
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
    fitAddon = new FitAddon()
    terminal.loadAddon(fitAddon)
    terminal.open(container)
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
    terminal?.dispose()
    socket = null
    terminal = null
    fitAddon = null
  })
</script>

<div class="integrated-terminal">
  <div class="terminal-header">
    <div class="terminal-title">
      <span class="terminal-dot" class:connected></span>
      <span class="terminal-label">{label}</span>
      <span class="terminal-status">{error || status}</span>
    </div>
    <button type="button" class="btn btn-ghost btn-sm" onclick={onClose}>Close</button>
  </div>
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
  }

  .terminal-frame {
    flex: 1;
    min-height: 0;
    padding: var(--space-2);
    overflow: hidden;
  }

  :global(.xterm) {
    height: 100%;
  }

  :global(.xterm-viewport) {
    border-radius: var(--radius-sm);
  }
</style>
