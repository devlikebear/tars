import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const navSource = readFileSync(new URL('../src/components/Nav.svelte', import.meta.url), 'utf8')
const chatMessageSource = readFileSync(new URL('../src/components/ChatMessageItem.svelte', import.meta.url), 'utf8')
const readmeSource = readFileSync(new URL('../../../README.md', import.meta.url), 'utf8')

interface PNGMeta {
  width: number
  height: number
  colorType: number
}

function readPNGMeta(path: string): PNGMeta {
  const bytes = readFileSync(new URL(path, import.meta.url))
  assert.equal(bytes.subarray(0, 8).toString('hex'), '89504e470d0a1a0a')
  assert.equal(bytes.subarray(12, 16).toString('ascii'), 'IHDR')
  return {
    width: bytes.readUInt32BE(16),
    height: bytes.readUInt32BE(20),
    colorType: bytes.readUInt8(25),
  }
}

test('brand PNG assets have expected dimensions and alpha channel', () => {
  assert.deepEqual(readPNGMeta('../../../docs/brand/tars-icon.png'), { width: 512, height: 512, colorType: 6 })
  assert.deepEqual(readPNGMeta('../../../docs/brand/tars-avatar.png'), { width: 512, height: 512, colorType: 6 })
  assert.deepEqual(readPNGMeta('../../../docs/brand/tars-logo.png'), { width: 620, height: 220, colorType: 6 })
  assert.deepEqual(readPNGMeta('../../../docs/brand/tars-logo-dark.png'), { width: 620, height: 220, colorType: 6 })
  assert.deepEqual(readPNGMeta('../../../docs/brand/tars-readme-header.png'), { width: 760, height: 220, colorType: 6 })
  assert.deepEqual(readPNGMeta('../../../docs/brand/tars-readme-header-dark.png'), { width: 760, height: 220, colorType: 6 })
  assert.deepEqual(readPNGMeta('../public/tars-icon.png'), { width: 256, height: 256, colorType: 6 })
  assert.deepEqual(readPNGMeta('../public/tars-avatar.png'), { width: 256, height: 256, colorType: 6 })
})

test('README and console surfaces reference the refreshed TARS brand assets', () => {
  assert.match(readmeSource, /<div align="center">/)
  assert.match(readmeSource, /<picture>/)
  assert.match(readmeSource, /docs\/brand\/tars-readme-header-dark\.png/)
  assert.match(readmeSource, /docs\/brand\/tars-readme-header\.png/)
  assert.doesNotMatch(readmeSource, /&nbsp;&nbsp;&nbsp;/)
  assert.doesNotMatch(readmeSource, /<h1>TARS<\/h1>/)
  assert.match(navSource, /src="\/console\/tars-icon\.png"/)
  assert.match(navSource, /object-fit:\s*contain/)
  assert.match(navSource, /box-shadow:\s*0 0 0 2px rgba\(255, 211, 170, 0\.14\)/)
  assert.match(chatMessageSource, /src="\/console\/tars-avatar\.png"/)
  assert.match(chatMessageSource, /chat-avatar/)
  assert.match(chatMessageSource, /box-shadow:\s*0 0 0 2px rgba\(255, 211, 170, 0\.16\)/)
})
