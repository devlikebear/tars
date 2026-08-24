import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const apiSource = readFileSync(new URL('../src/lib/api/extensions.ts', import.meta.url), 'utf8')
const apiClientSource = readFileSync(new URL('../src/lib/api/client.ts', import.meta.url), 'utf8')
const typesSource = readFileSync(new URL('../src/lib/types.ts', import.meta.url), 'utf8')
const extensionsSource = readFileSync(new URL('../src/components/Extensions.svelte', import.meta.url), 'utf8')

test('Hub install API returns extension sandbox reports', () => {
  assert.match(typesSource, /export type SkillSandboxReport/)
  assert.match(typesSource, /package_type\?: 'skill' \| 'plugin' \| 'mcp'/)
  assert.match(typesSource, /sandbox_report\?: SkillSandboxReport/)
  assert.match(apiSource, /HubInstallResponse/)
  assert.match(apiSource, /export async function hubInstall\(\s*type: string,\s*name: string,/)
})

test('API errors preserve sandbox report payloads for failed installs', () => {
  assert.match(apiClientSource, /export class APIRequestError extends Error/)
  assert.match(apiClientSource, /sandbox_report/)
  assert.match(apiClientSource, /throw new APIRequestError/)
})

test('Extensions renders the latest extension sandbox report', () => {
  assert.match(extensionsSource, /let skillSandboxReport: SkillSandboxReport \| null = \$state\(null\)/)
  assert.match(extensionsSource, /function sandboxTitle\(report: SkillSandboxReport\)/)
  assert.match(extensionsSource, /sandbox-report/)
  assert.match(extensionsSource, /skillSandboxReport\.checks/)
  assert.match(extensionsSource, /APIRequestError/)
})
