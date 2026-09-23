// Global keyboard shortcuts for the console (#968).
//
// `Mod` is ⌘ on macOS and Ctrl elsewhere. Browsers never deliver some
// combinations to the page (Chrome reserves Ctrl/⌘+N, +T, +W, and ⌘+1..9
// for tabs), so those are not used: a new session is Mod+Shift+O and
// session switching is Alt+1..9. The desktop shell (#972) may add native
// bindings through its bridge.

export type ShortcutAction =
  | 'palette'
  | 'help'
  | 'new-session'
  | 'switch-session'
  | 'toggle-terminal'
  | 'toggle-sidebar'
  | 'toggle-zen'

export type ShortcutMatch = { action: ShortcutAction; index?: number }

// Only the fields matchShortcut reads, so tests can pass plain objects.
export type ShortcutEvent = Pick<KeyboardEvent, 'key' | 'code' | 'metaKey' | 'ctrlKey' | 'altKey' | 'shiftKey' | 'defaultPrevented'> & {
  target?: EventTarget | null
}

// Shortcuts that only make sense on the chat route.
export const chatOnlyActions: ReadonlySet<ShortcutAction> = new Set(['switch-session', 'toggle-terminal', 'toggle-sidebar', 'toggle-zen'])

export function isMacPlatform(platform = typeof navigator === 'undefined' ? '' : navigator.platform): boolean {
  return /mac|iphone|ipad|ipod/i.test(platform)
}

// True when the event comes from a text field, where single-key shortcuts
// must type instead of firing.
export function isTypingTarget(target: EventTarget | null | undefined): boolean {
  const el = target as { tagName?: string; isContentEditable?: boolean } | null | undefined
  if (!el) return false
  const tag = el.tagName?.toUpperCase()
  return tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || el.isContentEditable === true
}

export function matchShortcut(event: ShortcutEvent): ShortcutMatch | null {
  if (event.defaultPrevented) return null
  const mod = event.metaKey || event.ctrlKey
  const key = event.key.length === 1 ? event.key.toLowerCase() : event.key

  if (mod && !event.altKey) {
    if (!event.shiftKey && key === 'k') return { action: 'palette' }
    if (event.shiftKey && key === 'o') return { action: 'new-session' }
    if (!event.shiftKey && key === 'j') return { action: 'toggle-terminal' }
    if (!event.shiftKey && key === 'b') return { action: 'toggle-sidebar' }
    if (!event.shiftKey && key === '.') return { action: 'toggle-zen' }
    return null
  }

  // Alt+1..9. Match on `code`: on macOS, Option+digit changes `key` to a symbol.
  if (event.altKey && !mod && !event.shiftKey) {
    const digit = /^Digit([1-9])$/.exec(event.code)
    if (digit) return { action: 'switch-session', index: Number(digit[1]) - 1 }
    return null
  }

  if (!mod && !event.altKey && key === '?' && !isTypingTarget(event.target)) {
    return { action: 'help' }
  }
  return null
}

export type ShortcutDoc = { action: ShortcutAction; keys: string[] }

// Display order for the help overlay and palette hints.
export const shortcutDocs: ShortcutDoc[] = [
  { action: 'palette', keys: ['Mod', 'K'] },
  { action: 'new-session', keys: ['Mod', 'Shift', 'O'] },
  { action: 'switch-session', keys: ['Alt', '1…9'] },
  { action: 'toggle-terminal', keys: ['Mod', 'J'] },
  { action: 'toggle-sidebar', keys: ['Mod', 'B'] },
  { action: 'toggle-zen', keys: ['Mod', '.'] },
  { action: 'help', keys: ['?'] },
]

export function formatShortcutKeys(keys: string[], mac = isMacPlatform()): string {
  const glyph = (k: string): string => {
    if (k === 'Mod') return mac ? '⌘' : 'Ctrl'
    if (k === 'Alt') return mac ? '⌥' : 'Alt'
    if (k === 'Shift') return mac ? '⇧' : 'Shift'
    return k
  }
  return keys.map(glyph).join(mac ? '' : '+')
}

export function shortcutLabel(action: ShortcutAction, mac = isMacPlatform()): string {
  const doc = shortcutDocs.find((d) => d.action === action)
  return doc ? formatShortcutKeys(doc.keys, mac) : ''
}
