import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { chatWorkbenchSource } from './helpers/chatWorkbenchSource.ts'

const chatSource = chatWorkbenchSource
const chatComponentsSource = readFileSync(new URL('../src/lib/chatComponents.ts', import.meta.url), 'utf8')
const terminalSource = readFileSync(new URL('../src/components/IntegratedTerminal.svelte', import.meta.url), 'utf8')

const lazyChatComponents = [
  'ChatPanel',
  'ArtifactPanel',
  'TerminalTabs',
]

test('heavy chat subpanels are loaded through dynamic chat component imports', () => {
  for (const component of lazyChatComponents) {
    assert.doesNotMatch(
      chatSource,
      new RegExp(`import ${component} from '\\./${component}\\.svelte'`),
      `${component} should not be statically imported by the chat workbench`,
    )
    assert.match(
      chatComponentsSource,
      new RegExp(`import\\('\\.\\./components/${component}\\.svelte'\\)`),
      `${component} should be registered as a dynamic chat import`,
    )
  }
})

test('the chat workbench mounts each lazy subpanel through loadChatComponent', () => {
  for (const key of ['chat-panel', 'artifact-panel', 'terminal-tabs']) {
    assert.match(chatSource, new RegExp(`loadChatComponent\\('${key}'\\)`), `${key} should be mounted lazily`)
  }
})

test('terminal webgl renderer is optional after the terminal mounts', () => {
  assert.doesNotMatch(terminalSource, /import \{ WebglAddon \} from '@xterm\/addon-webgl'/)
  assert.match(terminalSource, /import\('@xterm\/addon-webgl'\)/)
})
