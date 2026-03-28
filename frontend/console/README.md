# TARS Console

Embedded Svelte console for the TARS server.

## Commands

```bash
npm install
npm run dev
npm run check
npm run build
```

- `npm run dev` starts the Vite dev server on `127.0.0.1:5173`
- `npm run build` writes the production bundle into `internal/tarsserver/consoleassets/dist`

The Go server serves the embedded bundle at `/console` and can proxy to the dev server when `TARS_CONSOLE_DEV_URL` is set.
