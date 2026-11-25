package previewrelay

import (
	"html"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

// ServeHTTP is the relay listener: EnterPath trades a ticket, everything
// else needs a live session and is forwarded to the session's project.
func (r *Relay) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.URL.Path == EnterPath {
		r.enter(w, req)
		return
	}
	s, ok := r.session(req)
	if !ok {
		page(w, http.StatusUnauthorized, "Open this app from TARS",
			"This address shows an app only after you open it from the TARS app on your paired phone.", false)
		return
	}
	if why := refusal(req, s.folders); why != "" {
		http.Error(w, "refused", http.StatusForbidden)
		return
	}
	state, base := r.opts.Preview(s.project)
	switch state {
	case StateRunning:
	case StateStarting:
		page(w, http.StatusServiceUnavailable, "The app is starting",
			"This page reloads by itself in a moment.", true)
		return
	default:
		// The preview stopped (nobody looked for a while, or it failed):
		// the session ends with it, and Try it in TARS starts both again.
		if c, err := req.Cookie(CookieName); err == nil {
			r.endSession(hashKey(c.Value))
		}
		page(w, http.StatusServiceUnavailable, "The app is asleep",
			"Open it again from TARS to wake it up.", false)
		return
	}
	target, err := url.Parse(base)
	if err != nil || target.Host == "" {
		http.Error(w, "the app is not available", http.StatusBadGateway)
		return
	}
	r.proxy(target).ServeHTTP(w, req)
}

// enter trades the ticket in ?t= for a session cookie. It answers a page
// that moves to "/" rather than a redirect: a navigation started by the
// relay's own page is same-site, so the SameSite=Strict cookie goes with
// it whatever opened the ticket URL.
func (r *Relay) enter(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if req.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cookie, ok := r.consume(req.URL.Query().Get("t"))
	if !ok {
		page(w, http.StatusUnauthorized, "This link has expired",
			"Open the app again from TARS on your phone.", false)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    cookie,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<!doctype html><html><head><meta charset="utf-8">` +
		`<meta name="viewport" content="width=device-width, initial-scale=1">` +
		`<meta http-equiv="refresh" content="0;url=/"><title>Opening…</title></head>` +
		`<body><p><a href="/">Open the app</a></p></body></html>`))
}

func (r *Relay) proxy(target *url.URL) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Transport: r.transport,
		Rewrite: func(pr *httputil.ProxyRequest) {
			// Host is the development server's own address: Vite refuses
			// any other host (server.allowedHosts).
			pr.SetURL(target)
			pr.Out.Host = target.Host
			stripCookies(pr.Out.Header, ConsoleCookieName, CookieName)
			for _, h := range []string{"Authorization", "Tailscale-User-Login", "Tailscale-User-Name", "Tailscale-User-Profile-Pic", "Tailscale-Headers-Info"} {
				pr.Out.Header.Del(h)
			}
		},
		ModifyResponse: func(resp *http.Response) error {
			dropSetCookies(resp.Header, ConsoleCookieName, CookieName)
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
			page(w, http.StatusBadGateway, "The app did not answer",
				"Try again in a moment, or open it again from TARS.", false)
		},
	}
}

// stripCookies removes the named cookies from every Cookie header and
// keeps the rest (the app's own cookies) as they were sent.
func stripCookies(h http.Header, names ...string) {
	values := h.Values("Cookie")
	if len(values) == 0 {
		return
	}
	var kept []string
	for _, line := range values {
		for _, pair := range strings.Split(line, ";") {
			pair = strings.TrimSpace(pair)
			if pair == "" {
				continue
			}
			name, _, _ := strings.Cut(pair, "=")
			if matchesName(strings.TrimSpace(name), names) {
				continue
			}
			kept = append(kept, pair)
		}
	}
	h.Del("Cookie")
	if len(kept) > 0 {
		h.Set("Cookie", strings.Join(kept, "; "))
	}
}

// dropSetCookies removes the app's attempts to set the named cookies.
func dropSetCookies(h http.Header, names ...string) {
	values := h.Values("Set-Cookie")
	if len(values) == 0 {
		return
	}
	h.Del("Set-Cookie")
	for _, line := range values {
		name, _, _ := strings.Cut(line, "=")
		if matchesName(strings.TrimSpace(name), names) {
			continue
		}
		h.Add("Set-Cookie", line)
	}
}

func matchesName(name string, names []string) bool {
	for _, n := range names {
		if strings.EqualFold(name, n) {
			return true
		}
	}
	return false
}

// page answers a short plain page; the relay never shows the app instead.
func page(w http.ResponseWriter, status int, title, body string, reload bool) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	refresh := ""
	if reload {
		refresh = `<meta http-equiv="refresh" content="2">`
	}
	_, _ = w.Write([]byte(`<!doctype html><html><head><meta charset="utf-8">` +
		`<meta name="viewport" content="width=device-width, initial-scale=1">` + refresh +
		`<title>` + html.EscapeString(title) + `</title>` +
		`<style>body{font-family:-apple-system,system-ui,sans-serif;margin:0;padding:48px 24px;` +
		`background:#f6f6f4;color:#1d1d1b;text-align:center}h1{font-size:22px}p{color:#555}</style></head>` +
		`<body><h1>` + html.EscapeString(title) + `</h1><p>` + html.EscapeString(body) + `</p></body></html>`))
}
