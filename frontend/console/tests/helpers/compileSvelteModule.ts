// Load a `.svelte.ts` runes module under plain Node so tests can exercise its
// behavior instead of matching regexes against its source.
//
// The module is type-stripped, compiled with svelte/compiler, and written to
// a temp directory. Import specifiers are rewritten to absolute URLs so the
// compiled file resolves `svelte/*` and sibling `.ts` modules from where they
// really live.

import { existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { stripTypeScriptTypes } from 'node:module'
import { tmpdir } from 'node:os'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { compileModule } from 'svelte/compiler'

const consoleRoot = resolve(dirname(fileURLToPath(import.meta.url)), '../..')

function resolveSpecifier(specifier: string, fromDir: string): string {
  if (specifier.startsWith('svelte')) {
    return import.meta.resolve(specifier)
  }
  if (!specifier.startsWith('.')) return specifier
  const base = resolve(fromDir, specifier)
  for (const candidate of [base, `${base}.ts`, join(base, 'index.ts')]) {
    if (existsSync(candidate) && !candidate.endsWith('.svelte.ts')) {
      return pathToFileURL(candidate).href
    }
  }
  throw new Error(`compileSvelteModule: cannot resolve ${specifier} from ${fromDir}`)
}

export async function compileSvelteModule<T = Record<string, unknown>>(relativePath: string): Promise<T> {
  const sourcePath = resolve(consoleRoot, relativePath)
  const js = stripTypeScriptTypes(readFileSync(sourcePath, 'utf8'))
  const compiled = compileModule(js, { generate: 'client', filename: sourcePath }).js.code
  const rewritten = compiled.replace(
    /(from\s+|import\s*\(\s*)(['"])([^'"]+)\2/g,
    (_match, lead: string, quote: string, specifier: string) =>
      `${lead}${quote}${resolveSpecifier(specifier, dirname(sourcePath))}${quote}`,
  )
  const outDir = mkdtempSync(join(tmpdir(), 'tars-svelte-module-'))
  const outFile = join(outDir, 'module.mjs')
  writeFileSync(outFile, rewritten)
  try {
    return (await import(pathToFileURL(outFile).href)) as T
  } finally {
    // The module is in memory once imported; the file is not needed.
    rmSync(outDir, { recursive: true, force: true })
  }
}
