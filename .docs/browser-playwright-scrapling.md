# Browser Runtime Decision

## Decision

- remove the existing CDP relay path
- use Playwright as the core browser execution engine
- treat Scrapling as an optional future crawl/extraction adapter, not the primary runtime

## Why relay was removed

- the relay/extension path was not stable in practice
- the main target use cases are deterministic website testing and health navigation
- those use cases fit Playwright better than a custom CDP bridge

## Why Playwright is the core engine

Playwright matches the current product goals:

- authenticated browser navigation with persistent state
- deterministic click/type/wait flows
- screenshots, traces, and console/network collection
- later extension into deploy/test/monitor/fix loops

TARS now uses a small Node runner at:

- `scripts/playwright_browser_runner.mjs`

and installs the dependency via:

- `package.json`
- `make browser-install`

## Scrapling evaluation

Repository reviewed:

- <https://github.com/D4Vinci/Scrapling>

Assessment:

- good fit for markdown-friendly extraction and AI-ready page content
- useful when the goal is "read and summarize this site/page"
- weaker fit for deterministic E2E and stateful navigation workflows

Conclusion:

- do not use Scrapling as the primary browser runtime
- keep it as a future option for:
  - semantic crawl jobs
  - markdown extraction for monitoring
  - low-friction content snapshots

## Near-term follow-up

- add console/network/trace artifacts to browser runs
- add storage-state based auth helpers
- add monitoring/watchdog rules on top of Playwright results
