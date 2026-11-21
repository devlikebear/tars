# Decision: TARS is a development tool for beginner vibe coders

Status: identity accepted 2026-11-21; classification and freeze changes proposed (see "Needs the maintainer's answer")
Date: 2026-11-21
Amends: `product-focus.md` (who the pipeline is for), `bot-mode.md` §2 and §8 (bots as the front)

## Context

The maintainer decided on 2026-11-21:

> "우리 타스는 바이브코더를 위한 개발툴로 정체성을 변경하자"
> ("Let's change TARS's identity to a development tool for vibe coders.")
>
> "나는 타스를 초보 바이브코더를 위한 개발툴로 만들고 싶어. 그래서 전반적인 ui/ux도 이번참에 봇 중심이면서 바이브코더 중심으로 개편하고 싶은데"
> ("I want TARS to be a development tool for beginner vibe coders, so I'd like to take this chance to rework the whole UI/UX around bots and vibe coders.")

Before this, `product-focus.md` (2026-10-10) made the focus pipeline the product, aimed at someone who already works with git, pull requests and verify commands. The freeze it set runs to 2026-12-31. Its first "reconsider" condition says: if nobody outside has finished a pipeline by the review date, the limit is reach and not scope. So far nobody outside has finished one (4 stars, 0 forks, 0 outside pull requests on 2026-10-10). Three reasons fit that result: the people a pipeline asks for already have Claude Code and their own habits; TARS takes a lot of setup; and the screens assume the user knows the vocabulary.

A beginner who cannot read code needs exactly what the pipeline already does well, which a chat-only tool does not: facts decide each step (a verify exit code, a CI result, a merged pull request), and nobody asks a model whether the work is done. What a beginner cannot use today is everything around that core.

Where a beginner gets stuck, from reviewing the current product:

- **Install and setup.** Logging in to a CLI provider, installing and logging in to `gh`, picking a folder, the setup wizard's providers and tiers.
- **Words.** worktree, PR, tier, verify, triage, gate, permission mode.
- **Checking the result.** Without reading code, "does it run" is the only judgement left, and the console shows diffs and logs, not the running thing.
- **Gates.** Nothing says what to press or what pressing it does.
- **Limits.** When a subscription limit stops a turn, the message is a provider error.

## Decision

### 1. Who the product is for

The user is **someone who cannot read code but can say what they want to make**. Every screen a beginner sees is judged by whether that person can use it without help. The maintainer, and anyone else who wants the workbench, still has it (§3).

### 2. What stays the same

- The focus pipeline stays the product. Plan, build, verify, review, PR and merge still run in that order. Every transition is still decided by facts the server reads, never by asking a model whether it is done. Goal mode's fixed policy, the push budget and the per-pipeline opt-ins keep their current meaning.
- Provider neutrality and bring-your-own-subscription stay. Together with working on a real repository and the verification gates, they are what TARS offers that hosted builders (Lovable, Bolt, Replit, v0) do not. Those services need no install, and that gap is the largest risk to this decision.

### 3. Easy screens by default, the workbench under Advanced

This is not a rewrite of the console. The direction, proposed and not objected to, still to be confirmed by the plan:

- **Make.** The user says what they want to make.
- **Progress.** The plan in plain words, what is happening now, and what the user needs to check.
- **Result.** A running preview, and "change it like this".

The pipeline is the main character, not a bot. Bots appear as the "team" working behind it and are assigned automatically (`bot-mode.md` §6 stays, §2 and §8 do not). Choices a beginner cannot judge — tier, permission mode, engine — get defaults and are hidden. The current workbench, the focus screens as they are, and the session board move under **Advanced** and keep working.

A switch of every screen at once would shake the whole console e2e suite (`make console-e2e`; see the #1194 note in CLAUDE.md). The easy screens are added beside the existing ones and become the default only once they cover the first-run path.

### 4. Checking the result is the core of the product

A beginner trusts the result only if it is shown to work. Scripted checks go into `verify`: a browser script with an exit code, such as `agent-browser` or Playwright. A model judging the screen does not decide. In one comparison on 2026-11-21, `agent-browser` (no model, about 1.3 s) and `aside exec` (model verdict, about 28 s) each caught the same planted bug 3 out of 3 times. The scripted one is the kind a transition may rest on. The opt-in computer-use e2e stays off by default and frozen (`focus-mode.md`). The result screen's preview is what the user looks at. The verify exit code is what the pipeline moves on.

### 5. Relation to other records

- **`product-focus.md`:** its pipeline-first rule holds. Its audience changes. This record is the answer to its first reconsider condition, taken before the review date.
- **`bot-mode.md`:** §2 ("TARS is the primary bot") and §8 ("the console opens on bots") are replaced by §3 above. §1, §3, §4, §6 and §7 stand. §5 (fork out / adopt in) is owned by `claude-code-handoff.md`. It is a tool for someone who also runs Claude Code in a terminal, so it belongs under Advanced.
- **A known bug, independent of this record:** when a session goes to another provider and then comes back to `claude-code-cli`, `handler_chat_execution.go` resumes the old `UpstreamSessionID` and loses the turns made in between (`bot-mode.md` phase 0). It is a fix, not a feature.

## Needs the maintainer's answer

These are open. The plan that follows this record asks them before any screen is designed in detail.

1. **Show or hide git and pull requests.** If git and pull requests are hidden, TARS takes over the merge and the release for the user, and the PR, PR review and merge stages need a beginner path with no GitHub knowledge. If they are shown, the beginner learns the words, and the product has to teach them. Every later screen depends on this answer.
2. **How far the freeze changes.** The proposal moves these areas to Core:
   - Onboarding and first run (setup wizard, `tars doctor`, readiness).
   - The desktop app (`tars-desktop`), because a beginner installs an app, not a CLI.
   - The result preview.
   - Scripted verification.

   Should the freeze date (2026-12-31) and rule 1 ("a `feat:` outside Core needs a pipeline sentence") stay, with only these areas reclassified? Or does the freeze end now?
3. **Whether `product-focus.md`'s success criteria change.** It counts "three outside people ran a pipeline to a merged pull request on their own repository". If git is hidden (question 1), "merged pull request" may not be the right measure of success for a beginner.

## Proposed classification change

These changes are proposed and are applied only after question 2 is answered.

| Area | Now (`product-focus.md`) | Proposed |
| --- | --- | --- |
| Onboarding, `tars doctor`, readiness | Maintained | Core |
| Desktop shell (`tars-desktop`) | Core (tray and notifications only) | Core (install, first run, the easy screens' window) |
| Result preview (running the user's app and showing it) | — (does not exist) | Core |
| Scripted verification in `verify` | Core (verification) | Core, named explicitly |
| Advanced workbench | Maintained | Maintained, moved behind Advanced |
| Bot records and stage assignment (`bot-mode.md` §6) | proposed | proposed, shown as "the team" |
| Claude Code handoff (`claude-code-handoff.md`) | proposed | proposed, under Advanced |

Nothing is frozen or unfrozen by this table until the maintainer confirms it.

## Consequences

- **The README and the marketing site lead with the beginner.** For example: "Say what you want to make; TARS builds it, checks that it works and shows you." The pipeline explanation moves under it.
- **Work aimed at experienced users slows down.** This applies to the workbench and to handoff features like the Claude Code handoff. They keep working, but they are not where new effort goes.
- **The first-run path becomes the measure.** A new user needs at most 10 minutes from install to seeing their first change running. The plan states it as a measurable target.
- **Setup is the hard part.** A beginner still needs a provider subscription and, unless question 1 hides it, a GitHub account. The plan has to state which of these TARS can do for the user and which it can only explain.

## Re-evaluate when

- The easy screens ship, and no one outside the maintainer finishes a first run within a month. Then the gap is install and setup, not the screens, and a hosted path has to be considered.
- A hosted builder adds bring-your-own-subscription and work on a local repository. Then TARS's remaining difference is the verification gates alone.
