# Decision: A mobile remote for focus mode, across several desktops

Status: proposed
Date: 2026-11-13

## Context

The developer runs TARS on more than one machine — a MacBook, a Mac mini and a Windows desktop — and wants to keep working while away from all of them. Ideas come while moving; today the only way to act on one is to wait until a desk.

What the phone should do, in the developer's words: remotely drive the TARS installed on a desktop, **focus mode only**, start a new focus task or continue one, and switch between desktops.

This fits `product-focus.md`: the focus pipeline and goal mode are the product, and "the `tars-desktop` tray and notifications that surface a pipeline waiting on its developer" are core. A phone that answers a waiting pipeline is the same thing in a pocket. A phone that mirrors the chat workbench, memory or config is not, and is out of scope.

Facts this record stands on (2026-11-13):

- A focus pipeline is driven by the server: gates, verify exit codes and `gh` probes decide transitions, and goal mode walks the gates with nobody present. A client only shows state and presses buttons.
- Chat turns are detached from the request. Leaving does not stop a turn; `GET /v1/chat/stream?session_id=` replays the running turn from the start. A phone on a train that drops and reconnects loses nothing.
- `POST /v1/focus/pipelines` requires the admin role (`internal/tarsserver/focus_pipeline.go`, "starting a focus task in a folder needs the admin token"), as does turning goal mode on. Gates, cards, findings, Q&A and turning goal mode off take the user role.
- The console calls the API with relative paths and `credentials: 'same-origin'` (`frontend/console/src/lib/api/client.ts`), authenticates browsers with the `tars_session` cookie, and opens `/v1/events/stream` with `EventSource`, which cannot send an `Authorization` header.
- The server sends no CORS headers. The console has a web manifest but no service worker.
- `internal/remoteaccess` (Tailscale serve detection) is frozen (`console-workbench.md`).
- `scripts/windows_test.sh` excludes `internal/tarsserver` as a whole, so the focus driver has never run its tests on Windows.
- Each server's release train sees only that server's pipelines.

## Decision

### 1. A native shell around the console's focus screens

A Capacitor app in `mobile/` (iOS first). Its web bundle is a second Vite entry in `frontend/console` that mounts only the focus screens (`src/components/focus/*`) plus the mobile-only screens below, so focus UI is written once and the desktop console and the phone never drift.

A PWA was rejected for the multi-device case: a PWA and its push subscription belong to one origin, so reaching three servers needs CORS on all of them, a shared VAPID key, and an app that will not open when the desktop that served it is asleep. The shell keeps the device list and tokens in the Keychain and loads its own bundle, so no desktop has to be awake for the app to open.

### 2. The phone joins the desktops; the desktops do not know each other

Each desktop keeps running its own `tars serve` with its own workspace, sessions, pipelines and repositories. Nothing is synchronized between them. The phone holds a device list `{name, base URL, token}` and:

- shows each device's reachability (`/v1/healthz`) and readiness (the facts `tars doctor` already reports: `claude` CLI, `gh` login, Tailscale);
- builds one **waiting on you** inbox by asking every reachable device for its pipelines and pending approvals, labeled by device;
- opens a pipeline on the device that owns it.

"Switching devices" in this record means the phone switching which device it drives. Moving a running pipeline from one device to another is out of scope for the first version: sessions, worktrees and the CLI's upstream session are local to a device. If it is wanted later, the shape is a branch handoff — push the branch on A, open a pipeline from that branch on B — which needs a "start from branch" option that focus creation does not have today.

### 3. Transport: Tailscale only

Every device is reached at its Tailscale serve HTTPS address. No port forwarding, no public endpoint, no relay. `internal/remoteaccess` moves from frozen to maintained, because this feature stands on it.

### 4. A focus device token

Pairing: the desktop console shows a QR code holding the device's base URL and a one-time pairing code; the phone exchanges the code for a **focus device token**. The code is the link `tars-mobile://pair?url=<https base URL>&code=<code>`, so the phone's camera hands it to the app, and the base URL is the device's Tailscale serve address — the console offers no code while serve is off. Tokens are per phone, stored hashed on the desktop, listed and revocable from its console.

The token carries a new role, `focus`, accepted only by:

- the focus pipeline, card, gate, finding, plan-edit and Q&A routes;
- the chat turn and turn-feed routes, for focus pipeline sessions only;
- the event stream, the chat activity and board reads, and `/v1/ops/approvals/{id}/approve|reject` for focus sessions;
- turning goal mode off.

Creating a pipeline keeps its admin rule for every other caller. For a `focus` token it is allowed only in a folder the desktop's owner has registered for the phone; the phone picks from that list and never sends a free path. This keeps "a phone can make the server run an agent in any folder" off the table.

Requests send `Authorization: Bearer`. The mobile bundle replaces `EventSource` with the `fetch` stream reader the turn feed already uses, so every stream carries the header.

### 5. CORS for the app's origin only

The shell's web view runs on its own origin (`capacitor://localhost` on iOS, `https://localhost` on Android), so its requests to a device are cross-origin. Each device answers CORS for exactly those origins, only on the routes the `focus` role reaches, and only with a bearer token — never a wildcard, never with cookies.

Capacitor's native HTTP (`CapacitorHttp`) is not used for API calls: it returns a whole response at once, so it cannot carry the turn feed or the event stream. (Confirmed 2026-11-14 in the iOS Simulator: the turn feed streams to the app's web view over plain `fetch`, event by event.)

### 6. The idea inbox

Ideas are written on the phone and stored on the phone, so capture works with every desktop asleep or offline. An idea becomes a task when the developer sends it: pick a device, a registered folder and a template. The task runs to its plan and stops at G1, as every focus pipeline already does — the desktop drafts the plan while the developer keeps moving, and approval comes later from the phone or the desk. Goal mode is not turned on from the inbox.

### 7. Notifications: existing channels first

The first version sends no push of its own. A device that has a pipeline waiting (an open gate, an undecided card, a queued approval) alerts through the Telegram channel that already exists, with a link to that pipeline.

The link is the device's own Tailscale HTTPS address and the console's focus path (`https://<device>.<tailnet>.ts.net/console/focus/<session>`), not a `tars-mobile://` URL: Telegram makes only `http(s)` and `tg://` links tappable, in message text and on buttons alike. The phone is on the tailnet, so the link opens the pipeline in the console today; when the app exists it claims these links (how — universal links on the tailnet name or a hand-off page — is decided with the app, phase 2). The host in the link also tells the phone which device sent it. Native push needs an APNs relay holding an Apple key — the first hosted part TARS would have — and is decided after phase 1 shows the app is used.

## Consequences

- One repository worked on from two desktops splits the release train. The release train runs on one home device per repository; the others merge but do not release.
- Windows is the least verified host for focus mode. Phase 1 runs the focus driver on the Windows desktop for real before the app is offered for it.
- A device that sleeps is unreachable. The phone says so plainly; keeping a desktop awake on power is the desktop shell's job and a separate decision.
- New server code that needs no chat internals (pairing, device tokens, the registered folder list, CORS) goes in a new `internal/` package, classified in `internal/architecture/layers.go`, not in `internal/tarsserver`.
- `pkg/` is untouched unless the token role has to cross it; if it does, `make api-snapshot` in the same change.

## Open decisions

1. Whether a `focus` token may turn goal mode **on**. Proposed: no in the first version.
2. Android in phase 1 or later. Proposed: later; the shell makes it a build target, not a rewrite.
3. Whether the Telegram alert stays after native push exists.

## Phases

1. **Server**: pairing, `focus` device tokens, registered folders, CORS, deep-link alerts; `remoteaccess` back to maintained. Prove the turn feed streams in an iOS web view over Tailscale.
2. **App**: device list and readiness, the waiting-on-you inbox, focus screens per device, the idea inbox.
3. **Measure** how often gates are answered from the phone; decide native push and Android from that.
