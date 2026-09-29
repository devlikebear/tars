// Package links sends the console's outbound links to the default browser.
//
// The console marks links out of the app with target="_blank". A webview
// has no tabs: WKWebView and WebKitGTK ignore such a click and WebView2
// opens a bare popup window. The shell injects Script into the page, which
// hands those clicks (and window.open calls) to Go as a raw message; Go
// checks the message with Parse and opens the URL in the browser.
package links

import (
	"net/url"
	"strings"
)

// Prefix starts the raw message. Wails routes messages that do not start
// with "wails:" to the application's RawMessageHandler.
const Prefix = "tars:open-external:"

// Script is injected into every page the window loads. Only clicks on links
// that leave the page's origin or ask for a new window are taken over;
// in-console navigation is left to the console's router.
const Script = `(function () {
  if (window.__tarsLinks || !window._wails || !window._wails.invoke) return;
  window.__tarsLinks = true;
  var send = function (href) { window._wails.invoke("` + Prefix + `" + href); };
  document.addEventListener("click", function (e) {
    if (e.defaultPrevented || e.button !== 0) return;
    var a = e.target && e.target.closest ? e.target.closest("a[href]") : null;
    if (!a) return;
    var u;
    try { u = new URL(a.href, location.href); } catch (_) { return; }
    if (a.target !== "_blank" && u.origin === location.origin) return;
    if (u.protocol !== "http:" && u.protocol !== "https:" && u.protocol !== "mailto:") return;
    e.preventDefault();
    send(u.href);
  }, true);
  var open = window.open;
  window.open = function (href) {
    try {
      var u = new URL(href, location.href);
      if (u.origin !== location.origin) { send(u.href); return null; }
    } catch (_) {}
    return open.apply(window, arguments);
  };
})();`

// Parse reads a raw message from origin. It returns the URL to open when
// the message is one of ours, came from the console's origin, and names an
// http, https, or mailto URL.
func Parse(message, origin, consoleOrigin string) (string, bool) {
	raw, ok := strings.CutPrefix(message, Prefix)
	if !ok || !sameOrigin(origin, consoleOrigin) {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		if u.Host == "" {
			return "", false
		}
	case "mailto":
	default:
		return "", false
	}
	return u.String(), true
}

// sameOrigin compares scheme and host. Wails reports the sender as the full
// page URL on some platforms and as a bare origin on others.
func sameOrigin(a, b string) bool {
	ua, errA := url.Parse(a)
	ub, errB := url.Parse(b)
	if errA != nil || errB != nil || ua.Host == "" {
		return false
	}
	return strings.EqualFold(ua.Scheme, ub.Scheme) && strings.EqualFold(ua.Host, ub.Host)
}
