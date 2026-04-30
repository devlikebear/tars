import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const navSource = readFileSync(new URL('../src/components/Nav.svelte', import.meta.url), 'utf8')

test('Console nav groups routes into Work, Operate, and Setup sections', () => {
  assert.match(navSource, /interface NavGroup/)
  assert.match(navSource, /const groups: NavGroup\[\]/)
  assert.match(navSource, /label: 'Work'/)
  assert.match(navSource, /label: 'Operate'/)
  assert.match(navSource, /label: 'Setup'/)
  assert.match(navSource, /Work[\s\S]*Chat[\s\S]*Memory[\s\S]*System Prompt[\s\S]*Extensions/)
  assert.match(navSource, /Operate[\s\S]*Agent Runtime[\s\S]*Approvals[\s\S]*Pulse[\s\S]*Reflection/)
  assert.match(navSource, /Setup[\s\S]*Settings/)
  assert.match(navSource, /nav-group-label/)
  assert.match(navSource, /aria-label=\{`\$\{group\.label\} navigation`\}/)
})
