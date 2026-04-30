import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const syspromptSource = readFileSync(new URL('../src/components/SyspromptCenter.svelte', import.meta.url), 'utf8')
const apiSource = readFileSync(new URL('../src/lib/api.ts', import.meta.url), 'utf8')
const typesSource = readFileSync(new URL('../src/lib/types.ts', import.meta.url), 'utf8')

test('System Prompt page surfaces prompt impact metadata and reloadable preview', () => {
  assert.match(typesSource, /SyspromptPromptImpact/)
  assert.match(typesSource, /prompt_impact\?: SyspromptPromptImpact/)
  assert.match(typesSource, /SyspromptPreview/)

  assert.match(apiSource, /getSyspromptPreview/)
  assert.match(apiSource, /\/v1\/admin\/sysprompt\/preview/)

  assert.match(syspromptSource, /promptImpactLine/)
  assert.match(syspromptSource, /file-impact-line/)
  assert.match(syspromptSource, /will be truncated/)
  assert.match(syspromptSource, /Reload preview/)
  assert.match(syspromptSource, /preview-modal/)
  assert.match(syspromptSource, /preview\.prompt/)
})
