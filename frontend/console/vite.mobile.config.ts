import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import { renameSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

// The mobile app's web bundle (docs/decisions/mobile-remote.md §1): a second
// entry of this console, built into mobile/www for the Capacitor shell.
// Relative base: Capacitor serves the bundle from the app's own root.
const here = dirname(fileURLToPath(import.meta.url))
const outDir = resolve(here, '../../mobile/www')

export default defineConfig({
  plugins: [
    svelte(),
    {
      // Capacitor loads index.html; the source entry is mobile.html so it
      // sits next to the console's own index.html.
      name: 'tars-mobile-index',
      closeBundle() {
        renameSync(resolve(outDir, 'mobile.html'), resolve(outDir, 'index.html'))
      },
    },
  ],
  base: './',
  // public/ holds the console's web-app icons and manifest; the app has
  // native ones.
  publicDir: false,
  build: {
    outDir,
    emptyOutDir: true,
    rollupOptions: { input: resolve(here, 'mobile.html') },
  },
  server: {
    host: '127.0.0.1',
    port: 5174,
    strictPort: true,
    open: '/mobile.html',
  },
})
