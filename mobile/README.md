# TARS mobile

A phone remote for focus mode on your desktops (`docs/decisions/mobile-remote.md`). This folder is the Capacitor shell, iOS first. The screens are a second Vite entry of the console in `frontend/console`, so focus UI is written once:

- `frontend/console/mobile.html` and `src/mobile/`: the app's screens.
- `frontend/console/src/lib/mobile/`: the desktop list, pairing, and per-desktop requests.
- `frontend/console/vite.mobile.config.ts` builds them into `mobile/www`, which `cap sync` copies into the iOS project.

## What it does now

- **Pairing.** On a desktop, open Settings → Channels & Remote → Phones and choose **Pair a phone**. Scan the QR code with the iPhone camera. The code is a `tars-mobile://pair?url=…&code=…` link, so iOS opens this app with the pairing form filled in. You can also type the address and code by hand.
- **Waiting.** One list, across every paired desktop, of the focus pipelines that wait on you: an open gate, cards to decide, or a tool call waiting for an answer. Each row names its desktop. Answer a tool call right there; a gate or a card opens in that desktop's console for now. A desktop that does not answer is named under the list. The list is read again when the app comes back to the front and every 30 seconds while it is open.
- **Desktops.** The app shows each paired desktop and whether it answers now: online, unreachable, pair again (revoked at the desk), or needs setup.

Each desktop's device token is kept in the iOS Keychain by `ios/App/App/KeychainPlugin.swift`. The token stays on this phone: it is neither synced to iCloud nor restored to another phone.

## Build

You need Xcode with an iOS Simulator, plus Node.

```bash
cd mobile
npm install
npm run sync        # builds the web bundle and copies it into ios/
npm run open        # opens Xcode; run the App scheme
```

A release build only pairs with an `https` address: the desktop's Tailscale serve URL. To try the app in the Simulator against a server on this Mac, build the development bundle instead:

```bash
npm --prefix ../frontend/console run build:mobile:dev
npx cap sync ios
```

This bundle also accepts `http://127.0.0.1:<port>`, keeps desktops in `localStorage` when it runs in a desktop browser, and reads `?pair=<link>` there in place of a scan:

```bash
npm --prefix ../frontend/console run dev:mobile
```

The server answers CORS only for the app's own origin (`capacitor://localhost`). A desktop browser therefore reaches a TARS server only with web security turned off, which is how the pairing flow was checked.

## Not yet

The focus screens for each desktop (deciding a gate or a card in the app) and the idea inbox come in later phase 2 changes. There is no app icon yet, and nothing here is signed for a device or distributed.
