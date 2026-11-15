package aside

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// Search engines the aside web_search provider can read.
const (
	EngineGoogle     = "google"
	EngineNaver      = "naver"
	EngineDuckDuckGo = "duckduckgo"
)

// Engines lists the supported engines, default first.
var Engines = []string{EngineGoogle, EngineNaver, EngineDuckDuckGo}

// ErrBlocked reports that the engine answered with a bot check instead of
// results. TARS never tries to get past one: the developer can open the
// page in Aside and answer it themselves.
var ErrBlocked = errors.New("the search engine asked for a human check")

// SearchResult is one organic result.
type SearchResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
}

// resultMarker prefixes the script's single line of JSON so it can be told
// apart from the tab notices aside prints around it.
const resultMarker = "__TARS_ASIDE_RESULTS__"

// NormalizeEngine returns the engine for name, defaulting to Google.
func NormalizeEngine(name string) (string, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return EngineGoogle, nil
	}
	for _, e := range Engines {
		if e == name {
			return e, nil
		}
	}
	return "", fmt.Errorf("engine must be one of: %s", strings.Join(Engines, "|"))
}

// SearchURL is the results page Search opens for query on engine.
func SearchURL(engine, query string) (string, error) {
	engine, err := NormalizeEngine(engine)
	if err != nil {
		return "", err
	}
	q := url.QueryEscape(query)
	switch engine {
	case EngineNaver:
		return "https://search.naver.com/search.naver?where=web&query=" + q, nil
	case EngineDuckDuckGo:
		return "https://html.duckduckgo.com/html/?q=" + q, nil
	default:
		return "https://www.google.com/search?q=" + q, nil
	}
}

// Search opens engine's results page for query in a background tab of the
// developer's browser, reads up to count organic results from the page and
// closes the tab. No model is called.
func (c Client) Search(ctx context.Context, engine, query string, count int) ([]SearchResult, error) {
	engine, err := NormalizeEngine(engine)
	if err != nil {
		return nil, err
	}
	pageURL, err := SearchURL(engine, query)
	if err != nil {
		return nil, err
	}
	if count < 1 {
		count = 1
	}
	out, err := c.Repl(ctx, SearchScript(engine, pageURL, count))
	if err != nil {
		return nil, err
	}
	return ParseSearchOutput(out, count)
}

// SearchScript is the JavaScript Search runs. Everything lives inside an
// async function so its names never collide with an earlier call in the
// same REPL scope, and the tab is closed even when reading it fails.
func SearchScript(engine, pageURL string, count int) string {
	args, _ := json.Marshal(map[string]any{"url": pageURL, "count": count})
	return fmt.Sprintf(`await (async () => {
  const args = %s;
  let tab = null;
  try {
    tab = await openTab(args.url);
    const found = await tab.evaluate(%s, args.count);
    console.log(%q + JSON.stringify(found));
  } finally {
    if (tab) await closeTab(tab);
  }
})();`, args, extractors[engine], resultMarker)
}

// extractors read one engine's results page. Each returns
// {blocked, results:[{title,url,description}]}.
var extractors = map[string]string{
	EngineGoogle: `(count) => {
  const blocked = location.pathname.startsWith('/sorry') || !!document.querySelector('#captcha-form, iframe[src*="recaptcha"]');
  const clean = s => (s || '').replace(/\s+/g, ' ').trim();
  const results = [];
  const seen = new Set();
  for (const a of document.querySelectorAll('#search a:has(h3)')) {
    if (results.length >= count) break;
    if (!/^https?:/.test(a.href) || seen.has(a.href)) continue;
    seen.add(a.href);
    const box = a.closest('div.g, div[data-hveid]');
    const snippet = box ? box.querySelector('[data-sncf], .VwiC3b') : null;
    results.push({title: clean(a.querySelector('h3').innerText), url: a.href, description: clean(snippet ? snippet.innerText : '')});
  }
  return {blocked, results};
}`,
	// Naver's class names are generated, so a result is read by meaning:
	// the link whose text is a breadcrumb ("site › path") points at the
	// document, the shortest other text on that link is its title and the
	// longest its summary. Naver's own links (Keep, help) are skipped.
	EngineNaver: `(count) => {
  const clean = s => (s || '').replace(/새 창 열림/g, '').replace(/\s+/g, ' ').trim();
  const isMeta = t => /^\d{4}\.\d{1,2}\.\d{1,2}\.?/.test(t) || t.includes('›') || t.length < 2;
  const results = [];
  for (const el of document.querySelectorAll('.fds-web-doc-root')) {
    if (results.length >= count) break;
    const links = Array.from(el.querySelectorAll('a[href^="http"]'))
      .map(a => ({t: clean(a.innerText), href: a.href}))
      .filter(x => { try { return !/(^|\.)naver\.com$/.test(new URL(x.href).hostname); } catch (e) { return false; } });
    const crumb = links.find(x => x.t.includes('›'));
    const href = crumb ? crumb.href : (links[0] || {}).href;
    if (!href) continue;
    const same = links.filter(x => x.href === href && !isMeta(x.t));
    if (!same.length) continue;
    const title = same.reduce((a, b) => b.t.length < a.t.length ? b : a);
    const desc = same.reduce((a, b) => b.t.length > a.t.length ? b : a);
    results.push({title: title.t, url: href, description: desc === title ? '' : desc.t});
  }
  return {blocked: false, results};
}`,
	EngineDuckDuckGo: `(count) => {
  const blocked = !!document.querySelector('.anomaly-modal, #challenge-form');
  const clean = s => (s || '').replace(/\s+/g, ' ').trim();
  const results = [];
  for (const el of document.querySelectorAll('.result')) {
    if (results.length >= count) break;
    if (el.classList.contains('result--ad')) continue;
    const a = el.querySelector('a.result__a');
    if (!a) continue;
    let href = a.href;
    try { const u = new URL(href); const real = u.searchParams.get('uddg'); if (real) href = real; } catch (e) {}
    const snippet = el.querySelector('.result__snippet');
    results.push({title: clean(a.innerText), url: href, description: clean(snippet ? snippet.innerText : '')});
  }
  return {blocked, results};
}`,
}

// ParseSearchOutput reads the script's marked JSON line out of aside's
// output and keeps up to count results.
func ParseSearchOutput(out string, count int) ([]SearchResult, error) {
	var line string
	for _, l := range strings.Split(out, "\n") {
		if i := strings.Index(l, resultMarker); i >= 0 {
			line = l[i+len(resultMarker):]
		}
	}
	if line == "" {
		return nil, fmt.Errorf("aside returned no search results: %s", lastLines(out, 3))
	}
	var payload struct {
		Blocked bool           `json:"blocked"`
		Results []SearchResult `json:"results"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &payload); err != nil {
		return nil, fmt.Errorf("decode aside search results: %w", err)
	}
	if payload.Blocked {
		return nil, fmt.Errorf("%w: open the search in Aside and answer it there, or use another engine", ErrBlocked)
	}
	results := make([]SearchResult, 0, len(payload.Results))
	for _, r := range payload.Results {
		if len(results) >= count {
			break
		}
		r.Title, r.URL, r.Description = strings.TrimSpace(r.Title), strings.TrimSpace(r.URL), strings.TrimSpace(r.Description)
		if r.URL == "" || r.Title == "" {
			continue
		}
		results = append(results, r)
	}
	return results, nil
}
