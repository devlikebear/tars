# Decision: TARS is a development tool for beginner vibe coders

Status: accepted 2026-11-21
Date: 2026-11-21
Amends: `product-focus.md` (audience, classification, freeze, success criteria), `bot-mode.md` §2, §5 and §8

## Context

The maintainer decided on 2026-11-21:

> "우리 타스는 바이브코더를 위한 개발툴로 정체성을 변경하자"
> ("Let's change TARS's identity to a development tool for vibe coders.")

and set five principles the same day:

1. Onboarding must be very easy.
2. The screens must not be as complex or as full of features as the Advanced workbench, Claude Code or Orca.
3. A vibe coder is someone who does not develop software and does not know the words git, TDD or end-to-end verification. TARS uses all of them internally, out of sight, so that an idea can become a product without that knowledge.
4. A user who gains experience can switch to an expert mode and control the workflow. Handing a session between TARS and Claude Code should be easy, and so should using another IDE (Orca, for example) alongside.
5. A team of bots the user understands at a glance: leader, builder, reviewer, verifier, release, operations, automation.

Before this, `product-focus.md` (2026-10-10) made the focus pipeline the product, aimed at someone who already works with git, pull requests and verify commands. Its first "reconsider" condition says: if nobody outside has finished a pipeline by the review date, the limit is reach and not scope. Nobody outside has (4 stars, 0 forks, 0 outside pull requests on 2026-10-10). Three reasons fit that result: the people a pipeline asks for already have Claude Code and their own habits; TARS takes a lot of setup; and the screens assume the user knows the vocabulary.

A person who cannot read code needs exactly what the pipeline already does and a chat-only tool does not: facts decide each step (a verify exit code, a CI result), and nobody asks a model whether the work is done. What that person cannot use today is everything around that core:

- **Install and setup.** Logging in to a CLI provider, installing and logging in to `gh`, picking a folder, the setup wizard's providers and tiers.
- **Words.** worktree, PR, tier, verify, triage, gate, permission mode.
- **Checking the result.** Without reading code, "does it run" is the only judgement left, and the console shows diffs and logs, not the running thing.
- **Gates.** Nothing says what to press or what pressing it does.
- **Limits.** When a subscription limit stops a turn, the message is a provider error.

The maintainer also said where this goes later: the same tool for "vibe AI creators" (AI novels, music, video), with outside tools and open-weight models offered as TARS skills, from idea to publishing on YouTube or social media. Vibe coding is the first theme, and the design must leave room for the others without building them now.

## Decision

### 1. Who the product is for

The user is **someone who cannot read code but can say what they want to make**, and who can follow instructions to install Claude Code and sign in to it. Every screen that person sees is judged by whether they can use it without help.

The user brings their own AI subscription. A model that TARS provides is left open for a later business; it would be one more provider in the provider layer, not a new design. The first engine is `claude-code-cli`. A Codex CLI engine comes after it (`bot-mode.md` phase 4), and onboarding leaves a place to choose an engine.

### 2. What stays the same

- The focus pipeline stays the product. Every transition is still decided by facts the server reads, never by asking a model whether it is done. Goal mode's fixed policy, the push budget and the per-pipeline opt-ins keep their current meaning.
- Provider neutrality and bring-your-own-subscription stay. Together with working in a real project folder on the user's own computer and the verification gates, they are what TARS offers that hosted builders (Lovable, Bolt, Replit, v0) do not. Those services need no install, and that gap is the largest risk to this decision.
- Nothing is removed. Everything the console shows today stays, as the expert mode (§4).

### 3. One flow for every kind of work

Every job runs the same five steps: **idea → plan → generate → check → publish**. Vibe coding is the first kind of work on that flow.

A **work kind** bundles what one kind of work needs: the stage template, the bot team, how the result is shown, how it is checked, and where it is published. It stands on focus templates (`focuspipeline.Template`), which already run development, writing and research on one stage model; it adds no new way to run or to move between stages.

Three seams are built now so that other kinds can be added later, and nothing else for them:

1. The work kind itself. The first one is "make a web app".
2. A result view per kind. The first one is a running web preview; a text viewer or a media player comes later.
3. Publish targets as skills with a companion CLI in `tars-skills` (the extension rule in `CLAUDE.md`). The first one is a web deploy; YouTube or a social post comes later.

What differs for generated work and is allowed for in the design:

- A novel has no exit code. The check step has two parts, an automatic check and a person's check, and the work kind sets the weight of each. For code the automatic check carries it.
- Large media files do not belong in git. The easy screens say "version" and "go back"; the storage behind them can differ per kind. Only git is built now.
- More accounts get connected over time. Connecting an account is one shared screen, shown at the moment an account is first needed, not in onboarding.

### 4. Easy mode by default, expert mode one click away

**Easy mode** is what a new install opens on. It has three everyday screens and two that appear only when needed:

- **Start** (first run, or when the engine is not ready): the current state and the one next thing to do. No provider, tier or folder choice.
- **Make:** one box to say what to make.
- **Progress:** the bot team and what each is doing, and the one thing the user needs to confirm.
- **Result:** the running preview, "change it like this", go back to an earlier version, and publish.
- **Connect** (the first time publish is pressed): one account, through a browser sign-in.

**Git and pull requests are hidden in easy mode.** Version control runs on the user's computer without being named, a GitHub account is not required, and a job ends by applying verified work to the project, not by a pull request. These words do not appear on an easy-mode screen: git, commit, branch, worktree, PR, merge, tier, permission mode, verify, triage, gate, pipeline.

**Expert mode** is the console as it is today: the session board, the focus pipeline screens, the chat workbench and settings. A switch in easy mode opens the same project there. Both modes read the same data, so there is nothing to sync. An existing workspace opens in expert mode, so current use does not change. Importing an existing project, GitHub and pull requests, template editing, changing a bot's engine or instructions, goal mode and permission modes are expert-mode features.

**After the plan is approved, easy mode does not ask** (decided 2026-11-21). A beginner cannot judge a reviewer's finding, so an easy-mode project runs on goal mode's fixed policy once its plan is approved: low findings are set aside, every other one is fixed, retries have a budget, and the session's permission mode is `auto` meanwhile. Goal mode stays admin-only everywhere else; the exception is the project folder TARS created for easy mode. Publishing is never part of it: it is always a person's click (§7).

A switch of every screen at once would shake the whole console e2e suite (`make console-e2e`; see the #1194 note in `CLAUDE.md`). The easy screens are added beside the existing ones and become the default for new installs only once they cover the first-run path.

### 5. Bots are a visible team, each with its own setup

Bots are shown by name and role, doing the stages in turn. The user sees who is working. A work kind comes with its team already assigned, so nothing has to be chosen before the first job.

Each bot is its own record (`bot-mode.md` §1) with its own **model** (engine), **instructions**, **memory**, **skills** and **tools**. The maintainer asked for this on 2026-11-21: it is what makes a bot a bot in Grok Bot, Orca and the others. What exists for each today:

| Per bot | Stands on | New work |
| --- | --- | --- |
| Model | tiers and `tier_pin`, already per session and per turn | the binding from a bot record |
| Instructions | the session prompt override layer | the binding |
| Skills | the per-session skill allowlist | the binding |
| Tools | the per-session tool allowlist for native providers; for `claude-code-cli`, deny rules and the MCP server set | the binding. A CLI engine's own tools are limited by its permission rules, not chosen one by one |
| Memory | nothing: memory is one store per workspace | a memory store per bot, beside the shared facts about the user. This replaces `bot-mode.md` §7 |

**A stage of a template names its bot.** This is `bot-mode.md` §6, and it is how a team is put together: the work kind's template maps each stage to a bot, and a user's own template can map its stages to any bot, built in or added. The mapping changes who does a stage; it never changes what decides the transition.

**The user can add a bot.** A bot is added by describing it, through the same draft → preview → approve flow as focus templates (`bot-mode.md` §8): the model proposes a record, the server validates it, a person saves it. Built-in bots cannot be overwritten, and bot records are never read from a repository's `.tars/` (`bot-mode.md` §1).

| Bot | Does | Stands on | When |
| --- | --- | --- | --- |
| Leader | Hears the idea, writes the plan | plan stage | first version |
| Maker | Builds to the plan | build stage | first version |
| Reviewer | Looks for problems in what was made | review stage | first version |
| Checker | Runs it and reports pass or fail | verify commands, scripted browser checks | first version |
| Publisher | Puts it online | publish stage, a deploy skill | first version |
| Operations | Watches what was published | nothing yet (pulse watches TARS itself) | later |
| Automation | Repeats a job on a schedule | cron, with no beginner screen | later |

The names are a proposal; they were chosen to fit generated work too.

- The checker's verdict is a script's exit code, not a model's opinion. It is drawn as a bot, and the transition still rests on the fact.
- The server hands work from one bot to the next. Bots do not message each other or decide whose turn it is (`bot-mode.md` §6).
- A bot's permission ceiling can only narrow what a stage may do, and a bot is not a security boundary (`bot-mode.md` §6, §9).
- The first five bots can ship as names on existing stages before bot records exist (phase 2). Records, per-bot setup, adding a bot and mapping bots to template stages come in phase 6.

### 6. Checking the result is the core of the product

A beginner trusts the result only if it is shown to work. Scripted checks go into `verify`: a browser script with an exit code, such as `agent-browser` or Playwright. A model judging the screen does not decide. In one comparison on 2026-11-21, `agent-browser` (no model, about 1.3 s) and `aside exec` (model verdict, about 28 s) each caught the same planted bug 3 out of 3 times. The scripted one is the kind a transition may rest on. The opt-in computer-use e2e stays off by default and frozen (`focus-mode.md`).

A failed check is told in plain words: which screen, what was expected, what was seen. The result screen's preview is what the user looks at. The verify exit code is what the pipeline moves on.

### 7. The first version

- **What can be made:** web apps only, on one technical stack that the work kind fixes, because a beginner cannot choose one.
- **The stack (decided 2026-11-21):** React and TypeScript on Vite with the Cloudflare Vite plugin; server code is a Worker in the same project, written with Hono; saved data is D1; styling is Tailwind. One command starts the preview, the page and the API answer on one address, and the preview runs in the same runtime the published app does, with D1 simulated on the user's computer and no Cloudflare account until publish. Measured that day on the maintainer's Mac (Node 26, a fast connection) with Cloudflare's own scaffolder (`create-cloudflare` 2.75.0, `--framework=react`): scaffold and install 25 s with an empty package cache and 10 s with a warm one, 2 s more to add Hono and Tailwind, first page from the preview server after about 1 s, a build in 1 s; a row written through a Hono route to local D1 was still there after a restart. Not measured: a slow connection, an older Mac, installing Node itself, a real deploy, and how well a model builds on this stack.
- **Saved data is in.** A sign-up form is among the first things a beginner asks for, and local D1 needs no account, so an app that stores data can be made, previewed and published. Secrets (payments, outside API keys) stay out.
- **Where it ends:** at publish, with one real target. Publishing is the last step of the shared flow, and stopping before it would leave the most important seam untested. The candidate is Cloudflare (a free account, upload without GitHub, no deploy quota that repeated publishing runs into); its limits are checked against the official documentation before it is built.
- **Publishing is always a person's click.** It cannot be undone, so goal mode never does it. The server reads the published address itself before saying it is online.
- **Platform:** the macOS desktop app first. It is the only install that already carries the server.
- **The measure:** a new user gets from installing the app to seeing their own idea running within 10 minutes.

Not in the first version: apps that need secrets when published, importing an existing project in easy mode, Windows, a Codex engine, the operations and automation bots, other kinds of coding (mobile apps, automation scripts), and any generated-work kind.

## Relation to other records

- **`product-focus.md`:** its pipeline-first rule holds. Its audience, classification, freeze and success criteria change; that record carries the amendment. This is the answer to its first reconsider condition, taken before the review date.
- **`bot-mode.md`:** §1, §3, §4 and §6 stand, and so does §8's way of creating a bot by describing it. §2 ("TARS is the primary bot") and the rest of §8 ("the console opens on bots") are replaced by §4 and §5 above: the console opens on easy mode, where bots are a visible team. §7 (one shared memory) is replaced by a memory per bot beside shared facts about the user (§5). §5 (fork out / adopt in) is owned by `claude-code-handoff.md`.
- **`claude-code-handoff.md`:** unchanged in content. It is an expert-mode feature: it is for someone who also runs Claude Code.
- **A known bug, independent of this record:** when a session goes to another provider and then comes back to `claude-code-cli`, `handler_chat_execution.go` resumes the old `UpstreamSessionID` and loses the turns made in between (`bot-mode.md` phase 0). It is a fix, not a feature.

## Phases

The easy screens are added beside the existing ones throughout.

0. **Records.** This record and the amendments to the three above. Done (#1391).
1. **Checking.** Done (#1392), for templates that end in a pull request. Scripted browser checks in `verify`, with failures told in plain words. Done when a planted screen bug stops a pipeline.
2. **The easy flow.** The "make a web app" work kind (the built-in `webapp` template: plan, make, review, no pull request, marked `web_ui` so phase 1's checks apply), a template that ends without a pull request and applies verified work to the project, creating the project folder and its version control without asking (`internal/easyproject`, `POST /v1/easy/projects`: a folder under `~/TARS Projects`, the pinned scaffolder, git, then the pipeline, its first turn sent by the server; a real scaffold took 10 to 16 s on 2026-11-21), the Make and Progress screens, and the bots shown on stages. Done when one sentence leads to "finished" with none of the hidden words on screen (checked automatically).
3. **The result.** Running the user's app and showing it, "change it like this", and going back to an earlier version.
4. **Onboarding.** The Start screen, checking the engine's install and sign-in, and plain messages for subscription limits. Done when three people other than the maintainer get from install to a first result within 10 minutes, without help.
5. **Publish.** The publish stage, the Connect screen and the deploy skill. Done when the published address opens and the server has read its answer.
6. **Bots.** Bot records with their own model, instructions, skills, tools and memory; adding a bot by describing it; a template stage naming its bot (`bot-mode.md` phase 2 and the editor of its phase 3). Done when a bot the user added runs a stage of the user's own template with its own model and memory.
7. **Expert switch and handoff.** The mode switch, "open this folder in another tool", and `claude-code-handoff.md` phase 1.

Easy mode becomes the default for new installs after phase 4.

## Consequences

- **The README and the marketing site lead with the beginner.** For example: "Say what you want to make; TARS builds it, checks that it works and shows you." The pipeline explanation moves under it.
- **Work aimed at experienced users slows down.** The workbench and the handoff features keep working, but they are not where new effort goes first.
- **Setup is the hard part.** A beginner still needs a paid AI subscription and has to install and sign in to Claude Code. TARS can check and explain that step; it cannot do it for the user.
- **The stack brings its own setup and weight.** It needs Node on the user's computer, which a beginner's Mac usually lacks, so the app has to bring or install it. One project's dependencies took about 560 MB; ten projects would pass 5 GB unless installs share packages or old projects' dependencies are cleared. The preview server answered on `localhost` and not on `127.0.0.1`, so the preview and the scripted checks must name the address the same way.
- **Scope can widen again.** The freeze in `product-focus.md` existed because the surface was wider than one person could keep correct. The list under "Not in the first version" and the three seams of §3 are the limit; a generated-work feature outside those seams waits.

## Open decisions

1. How the stack's runtime reaches a beginner's computer and how its disk use is kept down (see Consequences). Decided in phase 4 at the latest.
2. Whether importing an existing project belongs in easy mode. This record keeps it in expert mode; that was not discussed with the maintainer.
3. The bot names (§5).
4. How the CASE companion relates to the bot team: whether CASE is the leader bot or stays separate.
5. Where adding a bot and mapping bots to stages live. Adding a bot by describing it suits easy mode; editing a template's stages is an expert-mode screen today. Not yet decided: whether easy mode gets a simple "team" screen for both.
6. What a bot's own memory holds and what stays shared. Proposed: facts about the user and the project are shared; what a bot learned doing its own role is its own.
7. Whether easy mode asks the user to approve the plan. This record keeps the approval. If a beginner cannot judge a plan, starting at once and correcting on the result screen may serve better.

## Re-evaluate when

- Phase 4 ships and no one outside the maintainer finishes a first run within a month. Then the gap is install and setup, not the screens, and a hosted path or a TARS-provided model has to be considered.
- A hosted builder adds bring-your-own-subscription and work on a local project. Then TARS's remaining difference is the verification gates alone.
- A coding tool the pipeline runs on ships a beginner mode with the same staged, verified flow. This is `product-focus.md`'s second reconsider condition, seen from this side.
