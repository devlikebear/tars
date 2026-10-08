#!/bin/sh
# Stand-in for cua-driver in the console E2E (the server's CUA_DRIVER_PATH,
# set by playwright.config.ts): a focus plan's end-to-end goals run through
# the computer_use engine, which would otherwise find a driver installed on
# the host, snapshot the developer's real frontmost window and send it to
# the LLM. No spec depends on whether the host has cua-driver, and none
# reads a real screen. The server runs it as `cua-driver <tool> '<json>'`;
# it answers the two read-only tools with one fixed window and refuses the
# rest, so nothing here can act on anything. Every call's tool name is
# appended to $TARS_E2E_CUA_LOG.
tool=$1
[ -n "$TARS_E2E_CUA_LOG" ] && echo "$tool" >> "$TARS_E2E_CUA_LOG" 2>/dev/null
case "$tool" in
  list_windows)
    printf '{"windows":[{"app_name":"E2E App","title":"Greeting","pid":4242,"window_id":7,"z_index":1,"is_on_screen":true,"bounds":{"x":0,"y":0,"width":800,"height":600}}]}\n'
    ;;
  get_window_state)
    printf '{"element_count":2,"tree_markdown":"- AXWindow \\"Greeting\\"\\n  - AXStaticText = \\"Hello, e2e\\"\\n  - [0] AXButton \\"OK\\"","elements":[{"element_index":0,"element_token":"e2e-ok","role":"AXButton","label":"OK","enabled":true,"depth":1},{"element_index":1,"element_token":"e2e-text","role":"AXStaticText","label":"Hello, e2e","enabled":true,"depth":1}]}\n'
    ;;
  *)
    echo "e2e cua-driver stub: $tool is not supported" >&2
    exit 1
    ;;
esac
