---
version: alpha
name: Warm Workshop
description: TARS Console — dark neutral base, warm amber accent, generous spacing for an unhurried command-line feel.
colors:
  # Brand
  primary: "#e09145"
  primary-hover: "#cc7e35"
  primary-muted: "#372c22"
  primary-text: "#f0b878"

  # Surface
  surface-base: "#141414"
  surface: "#1c1c1c"
  surface-elevated: "#242424"
  surface-hover: "#2a2a2a"
  surface-active: "#303030"
  surface-inset: "#111111"

  # Border
  border-subtle: "#282828"
  border-default: "#333333"
  border-strong: "#444444"

  # Text
  text-primary: "#e8e4df"
  text-secondary: "#9a9590"
  text-tertiary: "#6b6560"
  text-ghost: "#4a4540"

  # Semantic
  success: "#4ade80"
  success-muted: "#223328"
  warning: "#fbbf24"
  warning-muted: "#37301d"
  error: "#f87171"
  error-muted: "#322525"
  info: "#818cf8"
  info-muted: "#262732"

typography:
  h1:
    fontFamily: Outfit
    fontSize: 1.75rem
    fontWeight: 500
    lineHeight: 1.25
  h2:
    fontFamily: Outfit
    fontSize: 1.375rem
    fontWeight: 500
    lineHeight: 1.25
  h3:
    fontFamily: Outfit
    fontSize: 1.125rem
    fontWeight: 500
    lineHeight: 1.25
  h4:
    fontFamily: Outfit
    fontSize: 1rem
    fontWeight: 500
    lineHeight: 1.25
  body-md:
    fontFamily: DM Sans
    fontSize: 0.875rem
    fontWeight: 400
    lineHeight: 1.6
  body-sm:
    fontFamily: DM Sans
    fontSize: 0.8125rem
    fontWeight: 400
    lineHeight: 1.55
  label-caps:
    fontFamily: Outfit
    fontSize: 0.75rem
    fontWeight: 500
    lineHeight: 1.5
    letterSpacing: 0.04em
  code:
    fontFamily: JetBrains Mono
    fontSize: 0.875rem
    fontWeight: 400
    lineHeight: 1.5

rounded:
  sm: 4px
  md: 6px
  lg: 8px

spacing:
  xs: 4px
  sm: 8px
  md: 12px
  lg: 16px
  xl: 20px
  "2xl": 24px
  "3xl": 32px
  "4xl": 40px

components:
  card:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.text-primary}"
    rounded: "{rounded.lg}"
    padding: "{spacing.xl}"

  button-primary:
    backgroundColor: "{colors.primary}"
    textColor: "#ffffff"
    typography: "{typography.label-caps}"
    rounded: "{rounded.md}"
    padding: "{spacing.sm}"
  button-primary-hover:
    backgroundColor: "{colors.primary-hover}"

  button-secondary:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.text-primary}"
    typography: "{typography.label-caps}"
    rounded: "{rounded.md}"
    padding: "{spacing.sm}"
  button-secondary-hover:
    backgroundColor: "{colors.surface-elevated}"

  button-ghost:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.text-secondary}"
    typography: "{typography.label-caps}"
    rounded: "{rounded.md}"
    padding: "{spacing.sm}"
  button-ghost-hover:
    backgroundColor: "{colors.surface-elevated}"
    textColor: "{colors.text-primary}"

  button-danger:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.error}"
    typography: "{typography.label-caps}"
    rounded: "{rounded.md}"
    padding: "{spacing.sm}"
  button-danger-hover:
    backgroundColor: "{colors.error-muted}"

  button-warning:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.primary}"
    typography: "{typography.label-caps}"
    rounded: "{rounded.md}"
    padding: "{spacing.sm}"
  button-warning-hover:
    backgroundColor: "{colors.primary-muted}"

  button-sm:
    padding: "{spacing.xs}"

  badge-default:
    backgroundColor: "{colors.surface-elevated}"
    textColor: "{colors.text-secondary}"
    typography: "{typography.label-caps}"
    rounded: "{rounded.sm}"
    padding: "{spacing.xs}"
  badge-accent:
    backgroundColor: "{colors.primary-muted}"
    textColor: "{colors.primary-text}"
    typography: "{typography.label-caps}"
    rounded: "{rounded.sm}"
    padding: "{spacing.xs}"
  badge-success:
    backgroundColor: "{colors.success-muted}"
    textColor: "{colors.success}"
    typography: "{typography.label-caps}"
    rounded: "{rounded.sm}"
    padding: "{spacing.xs}"
  badge-warning:
    backgroundColor: "{colors.warning-muted}"
    textColor: "{colors.warning}"
    typography: "{typography.label-caps}"
    rounded: "{rounded.sm}"
    padding: "{spacing.xs}"
  badge-error:
    backgroundColor: "{colors.error-muted}"
    textColor: "{colors.error}"
    typography: "{typography.label-caps}"
    rounded: "{rounded.sm}"
    padding: "{spacing.xs}"
  badge-info:
    backgroundColor: "{colors.info-muted}"
    textColor: "{colors.info}"
    typography: "{typography.label-caps}"
    rounded: "{rounded.sm}"
    padding: "{spacing.xs}"

  input:
    backgroundColor: "{colors.surface-inset}"
    textColor: "{colors.text-primary}"
    typography: "{typography.body-md}"
    rounded: "{rounded.md}"
    padding: "{spacing.md}"
  input-focus:
    backgroundColor: "{colors.surface-inset}"
    textColor: "{colors.text-primary}"

  empty-state:
    textColor: "{colors.text-tertiary}"
    typography: "{typography.body-sm}"
    padding: "{spacing.3xl}"

  error-banner:
    backgroundColor: "{colors.error-muted}"
    textColor: "{colors.error}"
    typography: "{typography.body-sm}"
    rounded: "{rounded.md}"
    padding: "{spacing.md}"
---

# TARS Console — Warm Workshop

A canonical specification of the TARS Console design system. The YAML front matter is the normative source of truth for tokens; the prose below explains intent and application.

## Overview

The TARS Console is an operator surface for an autonomous-agent runtime — a place to read system signals, edit memory, run jobs, and chat with the model behind the curtain. It should feel like the lit corner of a workshop after hours: dark, focused, warm where it counts. Not a polished SaaS dashboard, not a brutalist terminal — closer to a well-organized engineer's workbench.

The aesthetic resists three temptations:

- **Decoration for its own sake.** Borders are thin, shadows are absent, accents are rationed. Visual noise distracts from signal.
- **Clinical neutrality.** Pure greys and blues read as cold dashboards. The amber accent and warm-tinted neutrals keep the tool feeling human.
- **Density theatre.** Spacing is generous because most surfaces (memory, sessions, pulse) involve reading prose, not packing rows.

The system is dark-first. There is no light theme; light backgrounds wash out the warmth of the accent and break the workshop framing.

## Colors

The palette is rooted in deep neutrals, a single warm accent, and four semantic states.

- **Primary (`#e09145`):** Warm amber. The sole driver of interaction — primary buttons, focus rings, link emphasis, focused borders. Used sparingly so that when it appears, it commands attention. Pair `primary-hover` (`#cc7e35`) for hover/pressed, `primary-muted` (alpha 0.14) for accent backgrounds, and `primary-text` (`#f0b878`) for text on muted accent surfaces.
- **Surfaces (`surface-base` → `surface-active`):** A six-step neutral elevation ladder. `surface-base` (`#141414`) is the page; `surface` (`#1c1c1c`) is a card; `surface-elevated` (`#242424`) is hover/header; `surface-inset` (`#111111`) is for inputs and code. Steps are tonal, not shadow-driven.
- **Borders (`border-subtle` → `border-strong`):** Three weights of grey for separation. `border-subtle` divides cards from background; `border-default` outlines secondary buttons and inputs; `border-strong` is reserved for hover/focus states on outlined surfaces.
- **Text (`text-primary` → `text-ghost`):** Four levels of warm-tinted off-white. `text-primary` for body, `text-secondary` for labels and metadata, `text-tertiary` for placeholders and inactive captions, `text-ghost` for nearly-decorative micro-labels (code-block lang badges, etc.).
- **Semantic (`success`, `warning`, `error`, `info`):** Each ships with a `*-muted` tinted-background variant. Use the muted variant as the background and the solid variant as the text — never the reverse, and never solid-on-solid.

> **On muted tokens.** The DESIGN.md spec requires opaque sRGB hex values, so each `*-muted` token (including `primary-muted`) is the pre-blended result of the corresponding tint at low alpha (`primary` 14%, semantic states 10–12%) composited over `surface` (`#1c1c1c`). The runtime CSS in `src/app.css` still expresses these as `rgba()` so they remain translucent on whatever surface they sit on; the design tokens here are the canonical *flat* values for tooling that requires opaque colors (Tailwind, Figma, DTCG export). If you change the runtime alpha, recompute the hex here.

### Pairings to avoid

- `text-ghost` on `surface-base` — readable only as decoration; do not use for any meaningful copy.
- Solid `success`/`warning`/`error`/`info` as a backgroundColor for body text — saturated; always reach for the muted variant.

## Typography

Three families, each with one job:

- **Outfit** — display headings (`h1`–`h4`) and label-caps. Geometric, slightly humanist; gives titles personality without flourishing.
- **DM Sans** — running prose (`body-md`, `body-sm`). Highly readable at small sizes; carries chat transcripts and long-form memory entries.
- **JetBrains Mono** — code (`code`). Inline code, code blocks, mermaid source, terminal-style log output.

Headings step down narrowly (1.75 → 1.375 → 1.125 → 1 rem) because most of the UI is mid-density list/card surfaces — large hero type would feel out of register. `label-caps` (Outfit 0.75rem with 0.04em tracking) is reserved for slot labels above editable fields, card section titles, and badge/button text.

### Rules

- Do not introduce a fourth font family.
- Do not use weights below 400 or above 600 — the scale only exercises 400/500/600.
- Do not set `text-transform: uppercase` outside of `label-caps`-derived classes.

## Layout

Spacing follows an 8-rooted scale with a 4-unit half-step (`xs`) for micro-adjustments. The default rhythm of a card is `lg` (16px) inside `lg` (16px) outside; the default gap between siblings is `md` (12px). Larger pages use `xl`/`2xl` for outer page padding.

The console uses a fixed left navigation (`220px`) and a fixed top header (`52px`); the remainder is a single content column with a max-readable width on long-form views (memory, sysprompt). On viewports below 768px the nav collapses to zero width and a header-driven menu is expected to take over.

Don't crowd. The system has so much spacing tokenized because the worst failure mode of a dark dense UI is a panel of 12 cards with 4-pixel gaps that read as a single grey wall.

## Elevation & Depth

There are no shadows. Depth is conveyed entirely through:

1. **Tonal layering.** Stack `surface-base` → `surface` → `surface-elevated` to suggest hierarchy.
2. **Border subtlety.** A single `border-subtle` (`#282828`) line is enough to separate a card from its background — anything heavier reads as a heavy outline.
3. **Accent borders for focus.** Focused inputs and active tabs get a 1px `primary` border. This is the only place pure accent appears as an outline.

Reasoning: shadows on dark surfaces tend to either disappear (low contrast) or look like halos (high contrast). Tonal stepping reads cleanly at every brightness setting.

## Shapes

Three corner radii: `sm` (4px), `md` (6px), `lg` (8px). Buttons, inputs, and badges use `md` or smaller; cards and elevated surfaces use `lg`. Nothing is fully pill-shaped — the system has no `full` radius.

Rationale: rounded corners convey approachability, but pill-shaped buttons in a dense operator UI start to look like consumer-app marketing buttons. The 4–8px range stays utilitarian.

## Components

### Buttons

Five variants — `primary`, `secondary`, `ghost`, `danger`, `warning` — plus a `sm` size modifier.

- `button-primary` is the only filled button. Use it once per screen for the main commit/run/save action. Background is the amber accent; text is white. **Note:** the amber/white pair has a known WCAG contrast shortfall (~2.5:1) — the linter flags this. The system tolerates it because primary buttons carry `label-caps` (semibold, all-caps short copy) which is treated as large text under WCAG; do not use this combination for body-length text.
- `button-secondary` is the workhorse outlined button. Transparent background, `border-default`, `text-primary`. Most actions land here.
- `button-ghost` removes the border entirely. Use for tertiary or in-table actions where outlines would multiply visual weight.
- `button-danger` is a ghost variant tinted red. Always confirm the destructive action with a second affordance (modal, inline confirmation) before letting the click do work.
- `button-warning` mirrors `button-danger` but tinted amber. Reserved for "this is reversible but unusual" actions (e.g., "Run pulse now" out of cadence).

`button-sm` reduces padding to `xs`. Use for inline actions inside dense rows; never for a primary CTA.

### Badges

Six tonal tints (`default`, `accent`, `success`, `warning`, `error`, `info`). Each pairs a muted background with a solid foreground from the same hue.

- Badges are read-only. Do not attach click handlers — if a label needs to be interactive, it's a button.
- Keep badge copy under 18 characters. Longer text breaks the rhythm of the row.
- Do not use more than two distinct badge tints in a single dense list — it overwhelms the row.

### Card

The default container. `surface` background, `border-subtle` outline, `lg` rounded, `xl` internal padding. A card may have a header row (`card-header` / `card-title`) using `label-caps` for the title.

Cards are the primary chunking device. When you want users to perceive "these things go together", put them in a card. When you don't, leave them on `surface-base`.

### CWD chip (chat session header)

Sits next to the session-health badge in `.session-title-row`. Mirrors the badge dimensions (3px / `space-2` padding, `radius-sm`) so the row stays balanced. The active path renders in **`primary` (amber)** with `font-mono`, prefixed by the dimmed label `cwd`. Click toggles a `position: absolute` dropdown anchored to the chip's right edge, listing every eligible cwd; the active row also reads in amber with a small bullet marker. Disabled state uses `opacity 0.7` while a transition is in flight. The chip is hidden until the session has loaded (no eligible-cwd payload → no chip), so empty states never show a phantom widget.

### Input fields

Text inputs and textareas share a single visual: `surface-inset` background, `border-default` outline, `body-md` text. On focus the border switches to `primary`. There is no fill change on focus — the accent border carries the affordance.

### Empty state

A centered `text-tertiary` block with `body-sm` typography, padded `3xl`. Always include both a one-line "what's missing" headline and a follow-up sentence with the next action ("Run a pulse to start collecting signals"). Empty states without action guidance feel like dead ends.

### Error banner

`error-muted` background, `error` text, full-width inline. Use for synchronous form validation errors and API failure surfacing. Do not use a banner for confirmation messages — that's what `success`-tinted toasts/badges are for.

## Onboarding wizard

The wizard (`src/components/Onboarding.svelte`) is a section-router shell that delegates each section to a child component under `src/components/onboarding/`. The shell owns the form state and the cross-section orchestration; each child owns its own UI and emits navigation callbacks (`onNext`, `onBack`, `onSkip`).

### Modes

- **Quick** — first-run default. Sections: Provider → Tiers → Review → Complete. Fastest path to a working console.
- **Full** — reentry default and the path users opt into via the "Configure more" CTA on the completion screen. Adds **Tools & Permissions**, **Integrations**, and **Channels** between Tiers and Review. Each new section can be skipped without writing.

The mode badge appears in the header. Switching from Quick to Full only adds sections after Tiers; previously-saved provider+tier values are kept.

### Deep links

`/console/onboarding?section=<id>` deep-links into a section on reentry. Supported ids: `provider`, `tiers`, `tools`, `integrations`, `channels`, `review`. The shell rejects an optional-section deep-link when no provider is configured (would write a useless partial config) and falls back to the Provider step.

### Section save semantics

Provider/Tiers/Review save the LLM block via the existing alias-replace `buildConfigPayload`, which preserves untouched providers. Tools/Integrations/Channels each save their own keys via `buildSectionPayload(section, form)` — a partial PATCH that does not touch other sections. This makes per-section reentry safe (editing Channels never disturbs Tools).

Secrets follow the existing `keepExistingApiKey` pattern: when the schema endpoint returns a masked value, the wizard sets a `keep*Key` flag and drops the field from the patch payload, preserving the on-disk credential.

### Completion matrix

`OnboardingComplete.svelte` renders a row per capability — LLM provider, tier bindings, web_search, web_fetch, memory embeddings, telegram, webhook — with a ✓ / ✗ / — glyph and a contextual "Edit <section>" jump-back link. Status derives from in-memory form state first (so it reflects what the user just edited) with `setupStatus.capabilities` as a fallback for refresh-after-save scenarios.

The matrix surfaces a "restart required to activate Telegram/Webhook" notice when those workers were saved during the current run — they only start after a server restart per the setup-only-mode constraints in `internal/tarsserver/main_serve_api_setup.go`.

## Do's and Don'ts

- **Do** use `primary` (amber) only once per screen, for the single most consequential action.
- **Do** layer surfaces tonally (`surface-base` → `surface` → `surface-elevated`) instead of reaching for shadows or thicker borders.
- **Do** pair muted-background semantic tokens (`success-muted`) with solid foreground (`success`) — never the reverse.
- **Do** keep the existing three font families (Outfit / DM Sans / JetBrains Mono); each has a defined role.
- **Don't** introduce a light theme. The tonal palette and amber accent are calibrated for dark backgrounds.
- **Don't** mix sharp corners and pill shapes in the same view; use the `sm`/`md`/`lg` radii consistently.
- **Don't** use `text-ghost` for any copy that the user is meant to read — it's for purely decorative micro-labels.
- **Don't** add another button variant before exhausting the existing five; new variants dilute meaning.
- **Don't** stack two muted semantic backgrounds (e.g. `success-muted` over `warning-muted`) — they fight for attention.
