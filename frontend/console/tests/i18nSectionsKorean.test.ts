// Every Korean string in src/i18n/sections/ is checked here, including tabs
// and states the Korean E2E (e2e/workbench-ko.spec.ts) never opens: no run of
// two English words may survive translation. Names the Korean UI keeps in
// English, and slash-command syntax, are allowed.
import test from 'node:test'
import assert from 'node:assert/strict'
import { readdirSync } from 'node:fs'

const sectionsDir = new URL('../src/i18n/sections/', import.meta.url)

const keptInEnglish = [
  'Svelte Flow', 'Mini Tree', 'openai-codex', 'TARS', 'Git', 'MCP', 'Pulse', 'LLM', 'cwd', 'CWD', 'JSON',
  'YAML', 'Light', 'Standard', 'Heavy', 'Ctrl', 'Cmd', 'Alt', 'Shift', 'Enter', 'Esc', 'HEAD', 'Codex',
  'HUD', 'diff', 'USD',
]
const englishRun = /[A-Za-z]{2,}[ \t]+[A-Za-z]{2,}/
const slashSyntax = /\/[a-z-]+(?:\s+[a-z-]+)?/g

function untranslated(text: string): boolean {
  let rest = text.replace(slashSyntax, ' ')
  for (const name of keptInEnglish) rest = rest.replaceAll(name, ' ')
  return englishRun.test(rest)
}

// Parameterized strings are called with placeholder arguments; a number
// suits counts and a string suits names, so try both.
function render(fn: (...args: unknown[]) => unknown): string | null {
  for (const arg of [1, 'x']) {
    try {
      const out = fn(...Array.from({ length: Math.max(fn.length, 1) }, () => arg))
      if (typeof out === 'string') return out
    } catch {
      // Try the next placeholder type.
    }
  }
  return null
}

function collect(value: unknown, path: string, out: Array<[string, string]>): void {
  if (typeof value === 'string') out.push([path, value])
  else if (typeof value === 'function') {
    const text = render(value as (...args: unknown[]) => unknown)
    if (text !== null) out.push([path, text])
  } else if (value && typeof value === 'object') {
    for (const [key, child] of Object.entries(value)) collect(child, `${path}.${key}`, out)
  }
}

test('Korean section strings leave no English phrase behind', async () => {
  const files = readdirSync(sectionsDir).filter((name) => name.endsWith('.ts'))
  assert.ok(files.length >= 13)
  const found: string[] = []
  let checked = 0
  for (const file of files) {
    const mod = await import(new URL(file, sectionsDir).href) as Record<string, unknown>
    const [name, ko] = Object.entries(mod).find(([key]) => key.endsWith('Ko')) ?? []
    assert.ok(name, `${file} exports no Korean object`)
    const strings: Array<[string, string]> = []
    collect(ko, name, strings)
    checked += strings.length
    for (const [path, text] of strings) if (untranslated(text)) found.push(`${path}: ${text}`)
  }
  assert.ok(checked > 900, `only ${checked} strings found`)
  assert.deepEqual(found, [])
})
