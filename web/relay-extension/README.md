# TARS Relay Extension

This Chrome extension is an experimental bridge for the local relay server.

It is not the primary browser automation path. Prefer the Playwright runtime unless you specifically need relay-based Chrome debugging.

## Scope

- Connects to the local relay service on loopback only
- Forwards relay commands to `chrome.debugger`
- Forwards debugger events back to the relay runtime

## Install

1. Open `chrome://extensions`
2. Enable developer mode
3. Click **Load unpacked**
4. Select `web/relay-extension`

## Runtime Setup

1. Start the TARS server:

```bash
make dev-serve
```

2. Open the extension options page and configure the relay port and token.

3. Keep a normal `http(s)` tab focused and connect the extension.

## Notes

- Local-only workflow
- Experimental and legacy compared with the Playwright runtime
- Protected Chrome URLs may reject debugger attachment
- Connection failures usually mean relay auth, tab focus, or debugger ownership issues
