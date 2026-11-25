# Decision: The running app of a vibe project on the paired iPhone

Status: proposed (design note for unit 7 of `docs/plans/vibe-mode-redesign/README.md`; no code yet).
Date: 2026-11-25.

## Context

A vibe project's app is the project's own Vite development server: `npm run dev -- --host 127.0.0.1 --port <free port> --strictPort`, one per project, started and swept by `easyproject.Previews` (`internal/easyproject/preview.go`; stopped after 10 minutes nobody read its status, all stopped when the server closes). The console on the desk shows it in an iframe of `http://127.0.0.1:<port>/` (`EasyPreview.svelte`, `sandbox="allow-scripts allow-forms allow-same-origin allow-popups allow-modals"`). The preview routes refuse role `focus` twice: they are not in `focusEndpointRules`, and `previewProject` answers 403 (`internal/tarsserver/easy_preview.go`).

A paired phone reaches the desk only at its Tailscale serve address (`https://<desk>.<tailnet>.ts.net`, `remoteaccess.Status.URL`), with a focus device bearer from the iOS Keychain, from a Capacitor web view on `capacitor://localhost` (`docs/decisions/mobile-remote.md` §3–5). The owner decided the phone must cover every vibe feature, which includes trying the app that was made (M-Result).

What is being served matters for every option: **model-written code, run from the owner's signed-in desk**. It must not be able to read or use TARS credentials, must not share an origin with the console or API, and must be reachable only by someone the owner paired.

## What was checked

Throwaway experiment on 2026-11-25 (scratch folder, not in this PR): a project scaffolded exactly as `easyproject` does (`create-cloudflare@2.75.0` with `scaffold.go`'s answers, plus `hono`, `tailwindcss`, `@tailwindcss/vite`) — Vite 8.3.8, `@cloudflare/vite-plugin` 1.70.1, Node 26. A D1 binding and an `/api/*` handler that counts rows were added to the worker. A 40-line Go `httputil.ReverseProxy` stood in for the relay.

| # | Check | Result |
|---|---|---|
| E1 | Dev server, request with `Host: desk.tail1234.ts.net:8443` | **403** — Vite's host check (`server.allowedHosts`) refuses a foreign host |
| E2 | Proxy at the **root** of its own listener, `ProxyRequest.SetURL` (rewrites Host to `127.0.0.1:<port>`) | 200; `index.html`, `/@vite/client`, `/src/main.tsx`, `/api/x` all work unchanged |
| E3 | Same proxy but forwarding the client's Host | 403 (as E1) — the relay must rewrite Host |
| E4 | HMR socket through the proxy, browser-style `Origin: https://desk…:8443`, with the `?token=` the client script carries | **101**; without the token 400. Vite's client builds the socket URL from the page's own host and port (`socketHost = importMetaUrl.hostname:port + base`), so it follows the relay with no config |
| E5 | `vite --base /p/abc/` (path-prefix relay) with the Cloudflare plugin | `/p/abc/` **404**, `/p/abc/api/x` 404; only `/p/abc/src/main.tsx` served. The scaffold's own `App.tsx` also calls `fetch('/api/')` — an absolute path no `base` rewrites |
| E6 | `/@fs/etc/hosts` through the proxy | 403 (outside `server.fs.allow`) |
| E7 | `/cdn-cgi/local/explorer/api/d1/database` through the proxy, no `Origin` (as a same-origin GET from the app's page would be) | **200**, lists D1 databases. The plugin printed that it "detected this dev session is running in an AI agent" and advertised this Local Explorer API (D1, KV, R2, Durable Objects, trace SQL). With a foreign `Origin` it answers 403. `/__debug` also answers 200 |
| E8 | `vite build` | 0.76 s for the scaffold (`dist/client` + `dist/app` worker) |
| E9 | `vite preview` after the build | Serves the build in workerd; `/api/a` continued the **same local D1** count as the dev server (`.wrangler/state` is shared); same host check (403) and the same Local Explorer at `/cdn-cgi/…` |
| E10 | Capacitor iOS 8.6.0 `WebViewDelegationHandler.swift` (from the npm package) | A top-level navigation or `window.open` to a host outside `server.allowNavigation` is cancelled and handed to `UIApplication.shared.open` (Safari); a **non-top-level frame (iframe) to an outside host falls through to allow** |
| E11 | Tailscale docs (kb/1312 serve, kb/1242 serve CLI, Funnel CLI, Tailscale Services), read 2026-11-25 | ACLs "apply to Serve just like any other service". Only Funnel restricts ports (443/8443/10000). Serve adds `Tailscale-User-Login`, strips spoofed copies, and omits it for tagged devices. A separate hostname needs Tailscale Services: tagged host, admin-defined `svc:` name, v1.86+ |

**Unverified** (marked where used): that `tailscale serve` keeps 443 and 8443 active side by side on one node (the CLI takes `--https=<port>` and the serve config is a map keyed by `host:port`, which `remoteaccess.parseServeConfig` already iterates, but no page says so in words); whether `--set-path` strips its prefix (moot, see option 1b); that WKWebView in this app drops cookies in a cross-origin iframe (WebKit blocks third-party cookies by default — expected, not measured here); and SFSafariViewController cookie behavior on current iOS.

## Options

### 1. Reverse proxy through the TARS server

**1a. Under an authenticated path of the TARS origin** (`https://desk/v1/easy/projects/{id}/app/…`). Rejected on both counts:
- *Breaks.* E5: the Cloudflare plugin does not serve the app under `--base`, and model-written code calls absolute paths (`fetch('/api/…')` is in the scaffold itself). HTML rewriting cannot reach URLs built in JS.
- *Unsafe.* The app would be **same-origin with the console and API**. Its script could call `/v1/*` with whatever the browser holds for that origin (the console's `tars_session` cookie in a desk or phone browser — `Sec-Fetch-Site: same-origin` passes the CSRF check in `secFetchSiteAllowsMutation`), read the console's `localStorage`, and register a service worker over console paths. No header the proxy adds fixes same-origin.

**1b. A dedicated relay origin: a second listener, served at its root on a second Tailscale HTTPS port** (`https://desk.<tailnet>.ts.net:8443/` → `127.0.0.1:<relay port>` → the project's dev server). The relay is TARS code; only the transport port is new.
- *What the person sees:* on M-Result, **Try it** opens the app full screen, live: when the team changes the app, the page reloads (HMR, E4).
- *Breaks:* nothing measured. Root path keeps absolute URLs, the worker's routes and Vite's module paths as they are (E2); Host must be rewritten (E1/E3); the HMR socket passes (`httputil.ReverseProxy` handles `Upgrade`).
- *Authorizing a page that cannot set a header:* the phone asks the API (bearer, focus table) for a **preview ticket**: random, single use, 60 s, bound to project id + device id. It opens `https://desk:8443/__tars/enter?t=<ticket>`; the relay trades it for an **HttpOnly, Secure, SameSite=Strict, host-only cookie** `tars_preview=<random id>` and redirects to `/`. Every later request, socket included, is first-party and carries that cookie. The relay session ends when the preview stops, the device is revoked, or after an hour unused. Revoking the phone ends it at the next request.
- *Where the page opens:* top level, not in an iframe — the relay's cookie would be a third-party cookie inside `capacitor://localhost` and WebKit blocks those by default (unverified here; an iframe also cannot be sized or navigated as an app). E10: the shell's `window.open` already hands it to Safari, which needs **no app change**. Better, once wanted: `@capacitor/browser` (SFSafariViewController) keeps it inside the app with a Done button, its own cookie jar, and no script access from the shell.
- *Security:* the app is on its own origin (`…:8443`), so it cannot read console storage or call the API with readable responses (`NewAppCORS` answers only `capacitor://localhost`). It never sees the bearer: the bearer only buys the ticket, in the app's own request. Cookies ignore ports, so the relay also: **strips `tars_session` (and every cookie but the app's own) from forwarded requests, and drops `Set-Cookie` for `tars_session`/`tars_preview` from the app's responses**; and TARS ignores `tars_preview` on 443. Same-site requests from the app's page to `:443` still carry a Lax `tars_session` if that browser has one — harmless for mutations (`Sec-Fetch-Site: same-site` fails the CSRF check) and unreadable for reads (no CORS); the phone's Safari/SFSafariViewController normally has no console session at all. The relay **refuses `/cdn-cgi/` and `/__debug`** (E7: an unauthenticated D1/KV/R2 explorer the page could reach same-origin) and **refuses `/@fs/` outside the project folder** (Vite's own allow list holds today, E6, but has had bypass CVEs), and refuses service-worker script requests (`Service-Worker: script`), because one origin hosts every project in turn and a worker registered by one would intercept the next. What a malicious app can still do is what it can already do on the desk: its worker runs in workerd on the desk with network access (including the desk's loopback TARS API — true today, not widened here), and its npm dependencies ran with the owner's rights at install. The relay widens *who can load the page* only to the paired phone.
- *Tailnet reach:* anyone the tailnet ACL lets reach the desk can reach port 8443 (E11), same as 443; without a ticket they get 401 and nothing is forwarded.
- *Changes, rough size:* server ~400–600 lines + tests — a `previewrelay` package (proxy, deny list, cookie rules, tickets, sessions; standard library), one listener in `tars serve`, `POST /v1/easy/projects/{id}/preview/ticket` (focus table; starts the preview if stopped; 409 `not_ready`), and the focus role allowed `GET|POST …/preview` (status keeps the idle sweep fed; the relay also counts as reading). `remoteaccess`: a second desired port (`8443` → relay) in `Enable`/`Disable`/`Detect` and the Tailscale card, plus readiness (`/v1/focus/readiness` gains `preview: ok|not_serving`). App: ~50 lines (ask, open). Desk console unchanged.
- *Lifecycle:* the relay reads the preview's address from `Previews`; a stopped preview answers a plain "the app is asleep — open it again from TARS" page, and the phone's next **Try it** restarts it. While the relay serves requests it refreshes `lastSeen`, so the 10-minute sweep stops a preview only when nobody looks on either screen.

### 2. A second Tailscale serve port straight at the preview

`tailscale serve --https=8443 --bg http://127.0.0.1:<preview port>`.
- *Sees:* the same live app.
- *Security:* **no authentication at all** — every tailnet member the ACL allows gets the raw dev server, its Local Explorer (E7) and `/__debug`. Identity headers (`Tailscale-User-Login`) arrive, but nothing reads them: Vite does not. Not tied to pairing; revoking the phone does nothing.
- *Breaks:* E1 — Vite answers 403 to the `ts.net` host unless TARS edits `allowedHosts` in the project's config (model-owned file) or passes it some other way. The port changes on every start, so serve must be rewritten per start and per sweep, by a server that may run under launchd, touching the owner's tailnet config each time.
- Rejected. (A per-project hostname via Tailscale Services would isolate better, but needs a tagged desk, an admin-defined service and approval — not something a beginner's desk has.)

### 3. Serve a production build instead of the dev server

`vite build` then `vite preview` (or `wrangler dev` on `dist/app/wrangler.json`), still behind 1b's relay.
- *Sees:* the app as of the last saved version, not live; a short build before the first load.
- *Works:* the Hono worker and **local D1 work and share data with the dev server** (E9), build is under a second for the scaffold (E8).
- *Security:* smaller surface than the dev server — no module graph, no `/@fs/`, no HMR socket — but the same Local Explorer at `/cdn-cgi/` (E9), so the relay rules are the same.
- *Cost:* a second process per project (or stopping the dev server, which the desk console shows), a build step and its failures to word for a beginner, and source maps in `dist`. ~150 more server lines on top of 1b.
- Not the first choice; it is the fallback if the dev server misbehaves behind the relay on the phone (reload loops, HMR errors on flaky mobile links).

### 4. Screenshots or a recorded walk-through only

The desk captures the app (the scripted browser check already drives it — `agent-browser`) and the phone shows images.
- *Sees:* pictures, not an app; nothing to tap. Fails the owner's "use every vibe feature" decision.
- *Security:* nothing served. *Size:* small, but capture needs a browser CLI on the desk.
- Kept as the stated fallback only if 1b is judged unsafe in review: then the phone shows the result's checks, versions and images, and opens the app only after publishing (plan, unit 7).

## Decision (recommended)

**Option 1b**: a TARS relay on its own origin — a second loopback listener behind a second Tailscale serve HTTPS port, serving the project's existing dev server at the root path — authorized by a short-lived, project- and device-bound ticket traded for a first-party cookie on that origin, and opened top level (Safari first, SFSafariViewController next). It keeps the app off the console's origin, keeps the bearer out of the page, ties access to pairing, and needs no change to the scaffold, Vite config or model-written code (E2, E4).

## Testing without a tailnet

- Go: `httptest` upstream standing in for Vite (asserts the forwarded Host, stripped cookies, refused `/cdn-cgi/`, `/__debug`, `/@fs/` outside the folder and `Service-Worker: script`, a websocket echo through the relay), tickets (single use, expiry, wrong project, revoked device), the stopped-preview page. `remoteaccess` with its fake `Runner` for the second port.
- Live, local: the relay listener on loopback answers like the served port; `TestLivePreview`'s scaffold plus a relay request proves E2/E4 again on each Vite/plugin bump (`-tags integration`).
- Console e2e: `e2e/fake-preview.sh` already stands in for the dev server; a spec gets a ticket with a focus token and loads the relay origin in Playwright (it is a plain https/http origin there).
- Simulator: dev bundle against a loopback desk — the relay on `http://127.0.0.1:<relay port>`, opened via `window.open`.
- Demo desk: it has no second origin, so its ticket route answers a URL of a page bundled with the app (a static sample "app"), shown in an in-app frame. That is what **App Store review** sees: the result screen and a sample app, no tailnet, no network. Real projects are reachable only on the owner's tailnet.

## Smallest first slice

Server only, behind the existing Tailscale card: the relay listener with the deny list and cookie rules, the ticket route, the focus role on preview status/start, `remoteaccess` serving 8443 → relay, readiness `preview`. The app's Try it is `POST ticket` + `window.open` (Safari) — a few lines in unit 8's result screen, no native change. Done when the simulator opens a running project over the tailnet, as the plan asks. SFSafariViewController, the demo desk's sample and the build fallback come after.

## Open questions for the owner

1. Is leaving the app for Safari acceptable for the first slice, or should **Try it** stay in-app from day one (`@capacitor/browser`, a new pinned dependency and an App Store build)?
2. May TARS take a second Tailscale HTTPS port (8443 proposed) when the owner turns Tailscale on, and should the Tailscale card show it as one switch or two?
3. Live (dev server, follows the team's changes) or the last saved version (build)? Recommended live.
4. Should the phone be able to *stop* a preview, or only start it and let the sweep stop it?
5. Should the relay also require the request's `Tailscale-User-Login` to equal the desk owner's login, as a second factor beside the ticket? It would refuse a shared-device guest holding a leaked link, at the cost of tagged-device setups.

## What would change this recommendation

- `tailscale serve` cannot keep 443 and 8443 at once on one node (unverified) → serve the relay on a Funnel-free alternative port the owner picks, or fall back to option 4.
- WKWebView keeps the relay cookie in a cross-origin iframe after all → an in-app iframe becomes possible, but top level would still be preferred for sizing and isolation.
- A Vite or Cloudflare plugin release breaks root-path proxying (host check, HMR token or explorer changes) → option 3 behind the same relay.
- Review finds the remaining surface (the app's worker on the desk reaching loopback services) too wide to expose even to the owner's phone → option 4, with the app opened only after publishing.
