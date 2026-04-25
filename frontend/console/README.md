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

## Design system

The console's visual identity — colors, typography, spacing, components — is captured in [`DESIGN.md`](./DESIGN.md), which follows the [Google DESIGN.md spec](https://github.com/google-labs-code/design.md). It is the canonical source of truth; runtime CSS in `src/app.css` implements those tokens.

```bash
# Validate (0 errors required, warnings are documented)
npx -y @google/design.md@latest lint frontend/console/DESIGN.md

# Regenerate exports after editing tokens
npx -y @google/design.md@latest export --format tailwind frontend/console/DESIGN.md > frontend/console/tailwind.theme.json
npx -y @google/design.md@latest export --format dtcg     frontend/console/DESIGN.md > frontend/console/tokens.json
```

`tailwind.theme.json` and `tokens.json` are generated artifacts — edit `DESIGN.md` and re-run the export, never edit them by hand.
