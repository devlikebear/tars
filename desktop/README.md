# tars-desktop

A thin native shell around the TARS console. It opens the console that a
local `tars` server serves in a native window and adds what a browser tab
cannot:

- **Tray icon** showing the server's state: offline, idle, running, or
  waiting for a tool approval. Its menu lists pending approvals (allow once,
  allow for the session, deny), running chats, and recent chats, and can
  start the server.
- **Native notifications** for each tool approval, with the same three
  answers as buttons. A question answered elsewhere has its notification
  withdrawn.
- **Global hotkey** to show or hide the window from anywhere.
- **`tars://` links** and **folders dropped on the app icon**.
- **Self-update** from GitHub releases.

The shell keeps no state beyond the window's position. The server runs on
its own, so closing the window (it only hides) or quitting the shell never
stops a chat, a cron job, or pulse.

## Running

```bash
tars serve &            # or `tars service start` on macOS
tars-desktop
```

When nothing answers, the window shows a waiting page and moves to the
console as soon as the server is up. **Start server** in the tray menu runs
`tars service start` on macOS and a detached `tars serve` elsewhere (its
output goes to `<user config dir>/tars-desktop/server.log`). The `tars`
executable is looked up next to the shell, beside the `.app` bundle, then on
`PATH`.

| Flag | Environment | Config file key | Default |
|------|-------------|-----------------|---------|
| `--server-url` | `TARS_SERVER_URL` | `server_url` | `http://127.0.0.1:43180` |
| | `TARS_API_TOKEN` | `api_token` | none |
| | `TARS_ADMIN_API_TOKEN` | `admin_api_token` | none |
| `--hotkey` | `TARS_DESKTOP_HOTKEY` | | `CmdOrCtrl+Alt+K` (`off` disables it) |
| `--hidden` | | | start with the window hidden |

A flag wins over the environment, which wins over the config file,
`<user config dir>/tars-desktop/config.json` (`~/Library/Application
Support` on macOS, `%AppData%` on Windows, `~/.config` on Linux). An app
opened from Finder, the Start menu, or a desktop launcher gets no shell
environment, so the file is where its tokens go:

```json
{ "api_token": "…", "admin_api_token": "…" }
```

On macOS and Linux the file must be readable only by you (`chmod 600`);
otherwise the shell ignores it.

The server URL must be on this machine (`127.0.0.1`, `::1`, or
`localhost`): the shell sends it tokens and shows its pages with a bridge
to native code. Reach a remote server with a browser.

Which tokens the tray needs depends on the server's `api_auth_mode`. With
the default `required`, the user token is needed for the running and
waiting state, approvals, and notifications; without it the tray says so.
Recent chats and **New chat in folder…** use admin routes, which need the
admin token in every mode except `off`. The console in the window logs in
on its own, as in a browser.

## Links

| Link | Does |
|------|------|
| `tars://` or `tars://open` | show the console |
| `tars://session/<id>` | open that chat |
| `tars://new?cwd=<absolute folder>` | ask, then start a chat working in the folder |

Any web page or document can open a link, so a link only navigates or
proposes; it never answers an approval or sends a message. macOS reads the
scheme from the bundle's `Info.plist`. On Windows (under
`HKCU\Software\Classes\tars`) and Linux (a `tars-desktop.desktop` entry plus
`xdg-mime`) the shell registers itself for the current user on every start,
so moving the app keeps links working.

## Building

The shell is its own Go module (`desktop/go.mod`), so the webview toolkit
stays out of the server's dependency graph and `go test ./...` at the
repository root does not reach it.

```bash
make desktop-test                      # go vet + go test in desktop/
make desktop-build                     # bin/tars-desktop for this machine
make desktop-package DESKTOP_GOOS=linux DESKTOP_GOARCH=amd64   # dist/ archive
```

Linux needs the GTK 4 and WebKitGTK 6.0 headers:

```bash
sudo apt-get install libgtk-4-dev libwebkitgtk-6.0-dev
```

macOS builds need Xcode's command line tools and must run on a Mac. Windows
cross-compiles from anywhere without cgo; `tools/winres` writes the icon,
version info, and the manifest that enables per-monitor DPI and Common
Controls v6 (native dialogs need it).

## Releases

A version bump publishes, next to the CLI archives in the same GitHub
release:

| Archive | Contains |
|---------|----------|
| `tars-desktop_<v>_darwin_arm64.tar.gz`, `_darwin_amd64.tar.gz` | `TARS.app` |
| `tars-desktop_<v>_linux_amd64.tar.gz` | `tars-desktop` |
| `tars-desktop_<v>_windows_amd64.zip` | `tars-desktop.exe` |

Every archive holds one top-level entry, which is what the updater swaps in
place of the running app, and is listed in the release's `checksums.txt`,
which the updater verifies before installing. **Check for updates…** in the
tray menu runs it; development builds (`dev`) do not update.

The macOS bundle is ad-hoc signed, so the first open needs a right-click →
Open (or System Settings → Privacy & Security → Open Anyway). Set
`DESKTOP_CODESIGN_IDENTITY` when packaging to sign with a Developer ID;
notarization is not wired up yet.

## Layout

| Path | What |
|------|------|
| `main.go`, `shell.go` | Wails wiring: window, tray, notifications, hotkey, links, updater |
| `internal/server` | server URL, health probe, how to start the server |
| `internal/activity` | API client: activity, approvals, sessions, event stream |
| `internal/tray` | tray state, menu model, which approvals to notify |
| `internal/deeplink` | `tars://` parsing |
| `internal/links` | opening the console's outbound links in the browser |
| `internal/protocol` | `tars://` registration on Windows and Linux |
| `internal/winstate` | saving and fitting the window's position |
| `internal/update` | picking the shell's archive out of a release |
| `internal/icon` | app and tray icons, drawn in code |
| `frontend/` | the page shown while the server is down |

Everything under `internal/` is plain Go with tests; `main.go` and
`shell.go` only connect it to Wails.
