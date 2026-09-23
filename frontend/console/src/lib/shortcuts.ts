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

// Mod+<key> bindings; `shift` must match exactly.
const modBindings: { key: string; shift: boolean; action: ShortcutAction }[] = [
  { key: 'k', shift: false, action: 'palette' },
  { key: 'o', shift: true, action: 'new-session' },
  { key: 'j', shift: false, action: 'toggle-terminal' },
  { key: 'b', shift: false, action: 'toggle-sidebar' },
  { key: '.', shift: false, action: 'toggle-zen' },
]

function matchModBinding(key: string, shift: boolean): ShortcutMatch | null {
  const binding = modBindings.find((b) => b.key === key && b.shift === shift)
  return binding ? { action: binding.action } : null
}

// Alt+1..9. Match on `code`: on macOS, Option+digit changes `key` to a symbol.
function matchSessionDigit(event: ShortcutEvent): ShortcutMatch | null {
  if (event.shiftKey) return null
  const digit = /^Digit([1-9])$/.exec(event.code)
  return digit ? { action: 'switch-session', index: Number(digit[1]) - 1 } : null
}

export function matchShortcut(event: ShortcutEvent): ShortcutMatch | null {
  if (event.defaultPrevented) return null
  const mod = event.metaKey || event.ctrlKey
  const key = event.key.length === 1 ? event.key.toLowerCase() : event.key
  if (mod && !event.altKey) return matchModBinding(key, event.shiftKey)
  if (event.altKey && !mod) return matchSessionDigit(event)
  if (!mod && !event.altKey && key === '?' && !isTypingTarget(event.target)) return { action: 'help' }
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
