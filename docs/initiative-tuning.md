# Initiative tuning loop

How to check, and change, when TARS speaks first (tars#1003, epic #997). Everything here works offline from the initiative ledger (`<workspace>/_shared/initiative/ledger.jsonl`, rotated to `ledger.jsonl.1`). No command changes the config and none talks to a running server.

The ledger never holds user or assistant text — only timestamps, booleans, jev probabilities, intents and fixed reason codes — so neither does anything built from it. Labels follow the same rule: there is no free-text field.

## 1. Export a dataset

```bash
tars initiative export --out initiative.jsonl
tars initiative export --since 336h --labels initiative.jsonl --out initiative.jsonl.new
```

One JSON line per tick: `signals` (Go signals), `text` (the three text signals, `source`, and jev `probabilities` when the jev backend read them), `decision`, the live `delivery`/`channel` result, and the reply-tracking `outcome` (`replied`/`ignored`/`stopped`) joined from its own ledger line. `--ledger PATH` reads a ledger without a config. `--labels` carries the labels of an earlier export into a fresh one, matched by record `id`.

Each record has a `label` slot, all `null` until you fill it in:

| field | meaning |
|---|---|
| `intent`, `speak` | what the policy should have decided on this tick |
| `quiet_requested`, `user_strained`, `special_day` | what the user's own words actually meant at the time (ground truth for the text signals) |

Edit the exported file by hand. Leave a slot `null` when you do not know.

## 2. The fixed evaluation set (regression gate)

`internal/initiative/testdata/eval/` holds the scenarios every policy change must keep passing (`go test ./internal/initiative -run TestEval`):

- `decide.jsonl` — `Decide(signals, text)` → `want.intent`/`want.speak`/`want.reason` (each optional; at least one).
- `text_signals_matter.jsonl` — whether a tick may call the text-signal backend at all (`textSignalsMatter`).
- `replies.jsonl` — `ResolveReplies` (replied / ignored / stopped / still pending).

`TestEvalDecideCoversEveryOutcome` walks every combination of the boolean signals (read by reflection, so a new `GoSignals` field joins automatically) and fails when `Decide` can reach an `(intent, reason)` pair no line in `decide.jsonl` pins. A change that adds an intent or a reason therefore adds a scenario for it in the same pull request. Unknown fields in a scenario line fail the test, so a misspelled signal cannot silently read as false.

### Adding scenarios from a labeled export

```bash
tars initiative scenarios --in initiative.jsonl > new-scenarios.jsonl
```

A record whose label sets `intent` or `speak` becomes a scenario expecting that correction, fed the recorded text signals with any corrected text booleans laid over them. `--all` also pins the recorded decision of every other record (skipping ones whose text label disagrees with the reading). The command lists on stderr every scenario the current policy fails — those are the disagreements to look at.

Then:

1. Pick the lines that show something the set does not cover yet; replace the generated `name` (`ledger <id>`) with a sentence saying what the case is.
2. Trim `signals` to the fields that are true (false is the default) to keep the line readable.
3. Append to `decide.jsonl` and run `go test ./internal/initiative -run TestEval`. A failing correction means the policy has to change — change `Decide` in the same pull request, never the expectation, unless the label was wrong.

## 3. Per-user pacing (`tars initiative adapt`)

The runtime already adapts within a day: each ignored speak in a row doubles the cooldown (up to 8×, `EffectiveCooldown`), one reply resets it, and a "stopped" reply silences the rest of the local day. `adapt` looks at the rate over a window instead (default 14 days, at least 10 resolved outcomes):

- **back_off** — two or more `stopped`, or at least 60% `ignored`: double `initiative.cooldown` (at most 8h) and lower `initiative.daily_cap` by one (at least 1).
- **lean_in** — at least 70% `replied` and never `stopped`: raise the daily cap by one only if it was actually reached on some day, and shorten the cooldown by a quarter (at least 15m) only if it actually held a decision back. With neither limit binding the verdict is **keep**.

It prints the suggestion; applying it is your edit of the config. Outcomes exist only in live mode, so a shadow-only ledger always reports `insufficient`.

## 4. jev thresholds (`tars initiative calibrate`)

Only for `initiative.backend: jev`. The default `llm` backend answers strict booleans (tars#1219) and has no probability to calibrate.

```bash
tars initiative calibrate --in initiative.jsonl
```

For each question it pairs the jev probability of every fresh reading (`source: systemone`; `cache` records repeat an earlier reading and are skipped) with its label, reports AUC and the confusion counts at the configured `initiative.thresholds.<question>`, and proposes the threshold with the best balanced accuracy (ties keep the one nearest the current value). Fewer than `--min-per-class` (default 5) labeled readings of either class gives no candidate. A better candidate prints the line to put in the config.

## Out of scope

The issue's fine-tuning step (a Laya notebook on Kaggle/Modal and a checkpoint swap procedure) and per-question temperature scaling of a local model were dropped with #1219: the text-signal backend is now the configured LLM's `initiative` role answering booleans, and a local System One is optional. Threshold calibration above is what remains of that work for users who keep the jev backend.
