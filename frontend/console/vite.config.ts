import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'

// https://vite.dev/config/
export default defineConfig({
  plugins: [svelte()],
  base: '/console/',
  build: {
    outDir: '../../internal/tarsserver/consoleassets/dist',
    emptyOutDir: false,
    // Route splitting leaves Mermaid's lazy parser just above Vite's 500 kB default.
    chunkSizeWarningLimit: 550,
  },
  server: {
    host: '127.0.0.1',
    port: 5173,
    strictPort: true,
    hmr: {
      protocol: 'ws',
      host: '127.0.0.1',
      port: 5173,
      clientPort: 5173,
    },
  },
})
