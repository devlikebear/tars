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
- **Unattended approvals**: a cron job, Telegram message, or subagent run of
  a session in Ask, Accept edits, or Plan mode queues its tool calls in the
  server's ops approvals. The tray counts them as waiting too, lists them
  with **Open chat**, **Approve**, and **Reject**, and notifies each one
  with the same choices. Their answers go to
  `POST /v1/ops/approvals/{id}/approve|reject`; they are never sent to the
  chat permission endpoint, which does not know them.
- **Global hotkey** to show or hide the window from anywhere.
- **`tars://` links** and **folders dropped on the app icon**.
- **Chats in windows of their own**, from the tray or a link, reopened on
  the next start.
- **Self-update** from GitHub releases, and on Windows of the server too.

The shell keeps no state beyond where its windows were and which chats had
a window of their own. The server runs on its own, so closing the window
(it only hides) or quitting the shell never stops a chat, a cron job, or
pulse.

## Installing

On macOS, the Homebrew cask installs the app together with the server
formula it depends on:

```bash
brew install --cask devlikebear/tap/tars-desktop
```

The cask installs the formula only when it is missing: a server installed
earlier stays at its version. The app compares the server's version (from
`/v1/healthz`) with its own and, once per server version, says when the
server is older. It then updates the server itself once no chat is running
(see Updates below); the commands to do it by hand are:

```bash
brew upgrade devlikebear/tap/tars
tars service stop && tars service start
```

On Windows, `install.ps1 -Desktop` puts the app and `tars.exe` in one
folder (`%LOCALAPPDATA%\Programs\TARS`), where the app looks first. Elsewhere,
take the archive for your platform from the GitHub release and put `tars`
on `PATH` (Homebrew or `install.sh`).

## Running

```bash
tars serve &            # or `tars service start` on macOS
tars-desktop
```

When nothing answers, the window shows a waiting page and moves to the
console as soon as the server is up. **Start server** in the tray menu runs
`tars service start --install-if-missing` on macOS and a detached
`tars serve` elsewhere (its output goes to
`<user config dir>/tars-desktop/server.log`). On a machine where
`tars init` never ran, `--install-if-missing` writes the starter config and
workspace and installs the LaunchAgent first, so the console opens on the
setup wizard; a tars from before the flag gets a plain `tars service start`. The `tars`
executable is looked up next to the shell, beside the `.app` bundle, on
`PATH`, then in Homebrew's and `install.sh`'s bin directories
(`/opt/homebrew/bin`, `/usr/local/bin`, `~/.local/bin`): an app opened from
Finder or the Dock gets launchd's minimal `PATH`, which has none of them.

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
Approving or rejecting an unattended approval uses the same user token:
`/v1/ops/*` is not an admin route. When the server refuses the answer, the
tray shows the error and the question stays open in the chat and on the Ops
page; one already answered elsewhere is dropped quietly.
Recent chats and **New chat in folder…** use admin routes, which need the
admin token in every mode except `off`. The console in the window logs in
on its own, as in a browser.

A new chat in a folder is one `POST /v1/admin/sessions` with `cwd` (and
`isolate`), the call the console makes. When the folder is in a git
repository — a `.git` entry in it or a parent — the confirm dialog, and the
tray's folder picker, also offer **Start isolated**, which starts the chat in
a worktree of its own. A server from before that call ignores `cwd`; the
shell then sets the folder with `PUT …/workdirs` as it used to, and says the
chat could not be isolated.

## Windows

The console window reopens at the size, place, and maximised state it had
when it was last hidden or the shell quit. A window saved on a monitor that
is no longer attached opens centred at its default size instead; where the
platform reports no screens, the saved values are only clamped to sane
limits.

**Recent chats → Open in new window** in the tray menu, or a
`tars://session/<id>?window=new` link, opens a chat in a window of its own,
showing the console's `/console/chat/<id>` page from the same server with
the same rules as the main window. Opening a chat that already has a window
brings that window forward. At most 8 chats get a window; past that the
chat opens in the console window. Closing a chat window closes it for good.
Chat windows still open when the shell quits reopen, where they were, on
the next start (a `--hidden` start leaves them for the one after).

All of this lives in `<user config dir>/tars-desktop/window.json`
(0600, in a 0700 folder), written through a temporary file on every change:

```json
{
  "bounds": { "x": 80, "y": 60, "width": 1280, "height": 840 },
  "maximised": true,
  "sessions": [
    { "id": "abc123", "bounds": { "x": 200, "y": 120, "width": 1100, "height": 780 } }
  ]
}
```

A maximised window keeps the size it had before it was maximised. Entries
that make no sense (no area, a malformed or repeated chat id) are dropped
on load.

## Links

| Link | Does |
|------|------|
| `tars://` or `tars://open` | show the console |
| `tars://session/<id>` | open that chat |
| `tars://session/<id>?window=new` | open that chat in a window of its own |
| `tars://new?cwd=<absolute folder>` | ask, then start a chat working in the folder |
| `tars://new?cwd=<absolute folder>&isolate=1` | the same, with **Start isolated** (a worktree of the chat's own) as the default button |

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

On Windows the shell also keeps the server current, two minutes after it
starts and then every six hours (`internal/serverupdate`). It runs
`tars update --check --json` with the tars it starts the server with, and
when a newer release exists runs `tars update --yes --json`, which replaces
`tars.exe` and restarts the server onto it. A restart cuts off a chat turn
and an unattended run waiting on an approval, so the update waits, retrying
every fifteen minutes, until `/v1/chat/activity` shows nothing running and
nothing waiting, or the activity cannot be read. The admin token goes to
`tars update` through `TARS_ADMIN_API_TOKEN`, never the command line. The same
check announces a newer shell once per version; clicking that notification,
or **Check for updates…**, installs it, and that menu item also runs the
server update at once. A server older than the shell (see Running) wakes the
check early.

On macOS the same check runs, and what it does depends on how tars was
installed. A Homebrew install (the tars the shell found resolves into a
Homebrew prefix) is updated with Homebrew, since `tars update` refuses one:
the check is `brew update` then `brew outdated --json=v2 --formula
devlikebear/tap/tars`, and the update `brew upgrade --formula
devlikebear/tap/tars`, under the same idle rule. Afterwards the shell
restarts the server with `tars service stop` and the Start server command,
but only when `tars service status` says launchd has the service loaded: a
server started by hand keeps running the old version until it is restarted,
and the notification says so. A formula pinned with `brew pin` is left
alone. brew is looked for on PATH, then in `/opt/homebrew/bin` and
`/usr/local/bin`. An `install.sh` install is updated with `tars update`, as
on Windows. Linux installs are not updated by the shell.

The release signs the macOS bundle with a Developer ID and the hardened
runtime, notarizes it and staples the ticket, so Gatekeeper opens the
downloaded app (and the cask, which installs it quarantined) without a
prompt. `scripts/desktop_package.sh` does this when
`DESKTOP_CODESIGN_IDENTITY` and the App Store Connect API key
(`DESKTOP_NOTARY_KEY_PATH`, `DESKTOP_NOTARY_KEY_ID`,
`DESKTOP_NOTARY_ISSUER`) are set; the release workflow takes them from the
`APPLE_*` repository secrets. Without them the bundle is ad-hoc signed,
which runs where it was built but is refused once downloaded (the release
job warns). For such a build, clear the quarantine flag yourself:
`xattr -dr com.apple.quarantine /Applications/TARS.app`.

## Layout

| Path | What |
|------|------|
| `main.go`, `shell.go` | Wails wiring: window, tray, notifications, hotkey, links, updater |
| `internal/server` | server URL, health probe, how to start the server |
| `internal/activity` | API client: activity, chat and unattended approvals, sessions, event stream |
| `internal/tray` | tray state, menu model, which approvals to notify |
| `internal/deeplink` | `tars://` parsing |
| `internal/links` | opening the console's outbound links in the browser |
| `internal/protocol` | `tars://` registration on Windows and Linux |
| `internal/winstate` | saving and fitting the windows' places, the chat windows to reopen |
| `internal/update` | picking the shell's archive out of a release |
| `internal/serverupdate` | updating the server while it is idle: `tars update` (Windows, `install.sh`), Homebrew (macOS) |
| `internal/icon` | app and tray icons, drawn in code |
| `frontend/` | the page shown while the server is down |

Everything under `internal/` is plain Go with tests; `main.go` and
`shell.go` only connect it to Wails.
