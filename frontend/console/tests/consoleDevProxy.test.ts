import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const viteConfigSource = readFileSync(new URL('../vite.config.ts', import.meta.url), 'utf8')
const indexHtmlSource = readFileSync(new URL('../index.html', import.meta.url), 'utf8')

test('dev console uses the /console base path in serve and build modes', () => {
  assert.doesNotMatch(viteConfigSource, /command\s*===\s*['"]serve['"]/)
  assert.match(viteConfigSource, /base:\s*['"]\/console\/['"]/)
})

test('dev console HMR connects directly to the Vite dev server', () => {
  assert.match(viteConfigSource, /hmr:\s*\{/)
  assert.match(viteConfigSource, /host:\s*['"]127\.0\.0\.1['"]/)
  assert.match(viteConfigSource, /clientPort:\s*5173/)
  assert.match(viteConfigSource, /port:\s*5173/)
})

test('console favicon resolves under the mounted console path', () => {
  assert.match(indexHtmlSource, /href="\.\/tars-icon\.png"/)
  assert.doesNotMatch(indexHtmlSource, /href="\/tars-icon\.png"/)
  assert.doesNotMatch(indexHtmlSource, /href="\/console\/tars-icon\.png"/)
  assert.doesNotMatch(indexHtmlSource, /%BASE_URL%tars-icon\.png/)
})
