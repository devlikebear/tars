import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import {
  getSyspromptTemplates,
  isSyspromptTemplateEligible,
} from '../src/lib/syspromptTemplates.ts'

const syspromptSource = readFileSync(new URL('../src/components/SyspromptCenter.svelte', import.meta.url), 'utf8')

test('sysprompt starter templates match supported prompt files', () => {
  assert.deepEqual(
    getSyspromptTemplates('IDENTITY.md').map((item) => item.label),
    ['Minimal', 'Helpful generalist', 'Coding assistant', 'Custom blank'],
  )
  assert.deepEqual(
    getSyspromptTemplates('AGENTS.md').map((item) => item.label),
    ['Cautious', 'Autonomous in workspace', 'Read-only assistant'],
  )
  assert.deepEqual(
    getSyspromptTemplates('TOOLS.md').map((item) => item.label),
    ['All defaults', 'Bash whitelist', 'Dev workflow'],
  )
  assert.deepEqual(getSyspromptTemplates('USER.md'), [])
})

test('sysprompt template eligibility only accepts blank or starter stub content', () => {
  const starter = '# TOOLS.md\n\n## Environment Tools\n- Document environment-specific tools, CLI utilities, and integrations available to the agent.\n'

  assert.equal(isSyspromptTemplateEligible('', starter), true)
  assert.equal(isSyspromptTemplateEligible('   \n\t', starter), true)
  assert.equal(isSyspromptTemplateEligible(starter, starter), true)
  assert.equal(isSyspromptTemplateEligible('# TOOLS.md\n\n## Environment Tools\n- Docker is available.\n', starter), false)
})

test('SyspromptCenter exposes an insert-template control for eligible files', () => {
  assert.match(syspromptSource, /getSyspromptTemplates/)
  assert.match(syspromptSource, /isSyspromptTemplateEligible/)
  assert.match(syspromptSource, /starter-template-bar/)
  assert.match(syspromptSource, /Insert template/)
  assert.match(syspromptSource, /handleTemplateInsert/)
})
