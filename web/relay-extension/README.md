# TARS Relay Extension (Experimental)

This extension is an experimental bridge between Chrome and the local TARS relay server.

## Scope

- Connects to `ws://127.0.0.1:43182/extension?token=...`.
- Forwards relay websocket commands to `chrome.debugger.sendCommand`.
- Forwards `chrome.debugger.onEvent` events back to relay.
- Uses strict relay token auth (`/extension`, `/cdp`, `/json*`).

## Install

1. Open `chrome://extensions`.
2. Enable **Developer mode**.
3. Click **Load unpacked**.
4. Select the `web/relay-extension` directory.

## Runtime setup

1. Run TARS server:

```bash
make dev-serve
```

2. In TARS client, check relay info:

```text
/browser relay
```

3. Open extension options and configure token:

- `chrome://extensions` -> TARS Relay -> **Extension options**
- Set:
  - `Relay Port` (default `43182`)
  - `Relay Token` (use `/browser relay` output in admin context)
- Click `Check Relay` then `Save`.

If you want a stable token across server restarts, set `browser_relay_token` in `workspace/config/tars.config.yaml`.

4. Keep one normal web tab focused (not `chrome://`).
5. Click the extension icon once to connect relay if auto-connect did not happen.

## Notes

- This extension is local-only and intended for loopback relay use.
- Chrome debugger attach may fail for protected URLs or if another debugger is attached.
- If relay disconnects, the extension retries automatically.
- If start fails with `no debuggable tab found`, open/focus a normal `http(s)` tab and retry.
- Badge meaning:
  - `ON`: relay connected
  - `...`: reconnecting/connecting
  - `!`: configuration/auth/connection error
