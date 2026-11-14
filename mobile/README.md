# TARS mobile

A phone remote for focus mode on your desktops (`docs/decisions/mobile-remote.md`). This folder is the Capacitor shell, iOS first. The screens are a second Vite entry of the console in `frontend/console`, so focus UI is written once:

- `frontend/console/mobile.html` and `src/mobile/`: the app's screens.
- `frontend/console/src/lib/mobile/`: the desktop list, pairing, and per-desktop requests.
- `frontend/console/vite.mobile.config.ts` builds them into `mobile/www`, which `cap sync` copies into the iOS project.

## What it does now

- **Pairing.** On a desktop, open Settings → Channels & Remote → Phones and choose **Pair a phone**. Scan the QR code with the iPhone camera. The code is a `tars-mobile://pair?url=…&code=…` link, so iOS opens this app with the pairing form filled in. You can also type the address and code by hand.
- **Waiting.** One list, across every paired desktop, of the focus pipelines that wait on you: an open gate, cards to decide, or a tool call waiting for an answer. Each row names its desktop. Answer a tool call right there, or open the pipeline. A desktop that does not answer is named under the list. The list is read again when the app comes back to the front and every 30 seconds while it is open.
- **Ideas.** Write an idea down whenever it comes; it stays on this phone (in the Keychain), with no desktop needed. When it's ready, **Send as task**: pick a desktop that answers, one of the folders registered for phones at that desk (Settings → Channels & Remote → Phones), and a template. The desktop drafts a plan and waits for your approval in the Waiting list. Goal mode is never turned on from the phone.
- **Desktops.** The app shows each paired desktop and whether it answers now: online, unreachable, pair again (revoked at the desk), or needs setup. Under a desktop that answers are what it lacks to run a task end to end (the chat model's CLI, a signed-in `gh` for pull requests, Tailscale serve for alert links) and its running pipelines, so you can open one that waits on nothing.
- **A pipeline.** The same screen as the desktop console's focus mode, talking to the desktop that owns the pipeline: approve or change the plan, decide cards, add a finding, ask about a card, send an instruction, follow the running turn, stop the pipeline or turn goal mode off. Turning goal mode on, and opening the full chat workbench, stay at the desk.

Each desktop's device token is kept in the iOS Keychain by `ios/App/App/KeychainPlugin.swift`. The token stays on this phone: it is neither synced to iCloud nor restored to another phone. Like any Keychain item it outlives deleting the app, so a reinstall finds its desktops again; revoke the phone at the desk to end its access.

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

## Run it on your iPhone

The phone reaches each desktop at its Tailscale serve address, over https. Nothing here is distributed yet, so you install it from Xcode with your own Apple ID.

**At the desk (each desktop you want on the phone):**

1. Run a TARS server that has the phone routes. They are on `main` after 0.68.0: a server that answers 404 to `GET /v1/focus/readiness` is too old. Until the next release that means a build of `main` (`make build`), not the app bundle's server.
2. Settings → Channels & Remote → **Tailscale** → **Enable**. That points Tailscale serve at TARS and gives the desk its `https://<machine>.<tailnet>.ts.net` address. The Phones card stays disabled until this is on.
3. Settings → Channels & Remote → **Phones** → **Add folder** for each folder a phone may start tasks in. Without one, a phone can continue tasks but not start them.

**On the iPhone:**

4. Install Tailscale and sign in to the same tailnet as the desk.
5. Turn on Developer Mode: Settings → Privacy & Security → Developer Mode, then restart when asked. It appears once the phone has been connected to Xcode.

**On the Mac with Xcode:**

6. Build the release bundle and open the project:

   ```bash
   cd mobile
   npm install
   npm run sync
   npm run open
   ```

7. In Xcode, select the **App** target → Signing & Capabilities → **Team**: your Apple ID (a free Personal Team works). If Xcode says the bundle identifier is taken, change it to something of your own (e.g. `com.<you>.tars.mobile`); nothing on the desk depends on it.
8. Connect the iPhone by cable (or the same Wi-Fi once paired), pick it as the run destination, and **Run** (⌘R).
9. The first time, iOS refuses to open an app from an unknown developer: Settings → General → VPN & Device Management → your Apple ID → **Trust**. A Personal Team's install expires after 7 days; run from Xcode again to renew it.

Without clicking through Xcode, the same build, signing and install run from `mobile/` once the phone is connected and trusted. `TEAMID` is your team (`defaults read com.apple.dt.Xcode IDEProvisioningTeamByIdentifier`), `UDID` the phone (`xcrun xctrace list devices`); the team is passed on the command line, so the project file stays unchanged:

```bash
xcodebuild -project ios/App/App.xcodeproj -scheme App -configuration Debug -destination "id=UDID" -derivedDataPath build/device DEVELOPMENT_TEAM=TEAMID -allowProvisioningUpdates -allowProvisioningDeviceRegistration build
xcrun devicectl device install app --device UDID build/device/Build/Products/Debug-iphoneos/App.app
xcrun devicectl device process launch --device UDID com.devlikebear.tars.mobile
```

The first build fails with "Your team has no devices" while the phone is not connected: connect it, unlock it and trust the Mac, then run it again.

**Pair and try it:**

10. At the desk, Phones → **Pair a phone**. Point the iPhone's Camera at the QR code and tap **Open in TARS**; the form fills in. Tap **Pair**.
11. Waiting lists what waits on you; Ideas → **Send as task** starts a pipeline in a registered folder; open a pipeline to approve the plan and watch the turn run.

If pairing says the desktop cannot be reached: check that Tailscale is connected on the phone, that the address in the form is the `https://…ts.net` one, and that `curl https://<machine>.<tailnet>.ts.net/v1/healthz` answers from another tailnet device.

## Not yet

Phase 2's pieces are all here, and the development build has been run in the iOS Simulator end to end: pairing from a link, the Keychain across a relaunch, deciding a plan and a card, and a running turn streaming into the pipeline screen. Still open: an event stream for phones (the app reads again on a timer), and a run on a real phone over Tailscale (steps above). Nothing here is distributed; a device build is signed with your own Apple ID in Xcode.

The icon and launch image come from the desktop shell's icon code; to redraw them, from `desktop/`:

```bash
go run ./tools/appicon -kind mobile -size 1024 -out ../mobile/ios/App/App/Assets.xcassets/AppIcon.appiconset/AppIcon-512@2x.png
```

and `-kind launch -size 2732` for each `splash-2732x2732*.png` in `Splash.imageset`. iOS caches launch screens, so uninstall the app from the Simulator before checking a new one.
