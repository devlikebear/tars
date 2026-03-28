import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'

// https://vite.dev/config/
export default defineConfig(({ command }) => ({
  plugins: [svelte()],
  base: command === 'serve' ? '/' : '/console/',
  build: {
    outDir: '../../internal/tarsserver/consoleassets/dist',
    emptyOutDir: false,
  },
  server: {
    port: 5173,
    strictPort: true,
  },
}))
