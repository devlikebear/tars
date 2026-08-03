# Agent Harness Evaluation Pack

The Agent Harness Evaluation Pack gives TARS a repeatable baseline for agent correctness, verification, durability, orchestration, safety, latency, and cost. It is intentionally small enough to run in pull-request CI and explicit enough to show known gaps instead of turning them into a green but meaningless benchmark.

## Current baseline

The committed baseline evaluates TARS `v0.34.3` at commit `7ec7ba84`, before the durable-control-plane roadmap changes runtime behavior:

- [Machine-readable JSONL](baseline-v0.34.3.jsonl)
- [Human-readable report](baseline-v0.34.3.md)
- [Canonical scenario pack](../../testdata/agent-harness/scenarios.json)

The pack contains 12 scenarios covering:

1. single-agent success;
2. parallel fan-out;
3. dependency handoff;
4. active-run restart recovery;
5. checkpoint restart;
6. approval allow;
7. approval deny;
8. false-success detection;
9. reusable skill loading;
10. duplicate side effects after retry;
11. parallel partial failure; and
12. pre-dispatch budget enforcement.

## Run it

Run the deterministic fake-model/fake-tool pack:

```bash
make agent-harness-eval
```

Write versioned JSONL and Markdown reports:

```bash
make agent-harness-baseline \
  AGENT_HARNESS_VERSION=0.34.3 \
  AGENT_HARNESS_COMMIT=7ec7ba84
```

The deterministic target runs in CI. It does not call a model provider, use credentials, or depend on network access.

## Optional live-provider run

Live evaluation is opt-in and never gates CI. Only scenarios marked `live_supported` execute; deterministic-only durability and fake-tool scenarios are reported as skipped.

```bash
export TARS_AGENT_EVAL_PROVIDER=openai
export TARS_AGENT_EVAL_MODEL=gpt-5.6
export TARS_AGENT_EVAL_API_KEY=...

make agent-harness-eval \
  AGENT_HARNESS_MODE=live \
  AGENT_HARNESS_JSONL=/tmp/tars-live-eval.jsonl \
  AGENT_HARNESS_MARKDOWN=/tmp/tars-live-eval.md
```

`TARS_AGENT_EVAL_BASE_URL` supports OpenAI-compatible endpoints. `TARS_AGENT_EVAL_WORKDIR` configures local CLI-backed providers. Cost is zero unless the command receives `--input-cost-per-million` and `--output-cost-per-million`; invoke `go run ./cmd/agentharness-eval --help` for those flags.

## Metric semantics

| Metric | Meaning |
| --- | --- |
| `task_success` | The worker reached its claimed task outcome. It can still fail independent verification. |
| `verifier_pass` | Evidence outside the worker's claim proves the required outcome or proves a safety block. |
| `restart_recovered` | Work resumed or completed through an explicit restart path. |
| `duplicate_side_effects` | External effects beyond the first intended effect. Lower is better. |
| `operator_interventions` | Human decisions or manual restarts needed to reach/contain the outcome. |
| `ttft_ms` | Time to first token. Deterministic runs use a fixed fixture value and label the source `deterministic`; live runs measure the provider stream. |
| token and cost fields | Provider usage and an optional price-based estimate. Deterministic fixtures record synthetic token counts and zero cost. |

`status=passed` means the measured metrics matched the versioned baseline expectation. It does **not** mean `task_success=true`. This distinction makes regressions detectable while keeping known product gaps visible in the same report.

## Versioning and change policy

- Increment `schema_version` when fields or their meaning change incompatibly.
- Add scenarios without rewriting prior baseline files.
- A runtime improvement should first make the old expectation fail, then update the scenario expectation and commit a new version baseline with the implementation.
- Keep live-provider output outside CI and out of the repository unless a release review explicitly chooses to publish it.
- Never include prompts, credentials, local absolute paths, or raw provider responses in committed reports.

The strategic interpretation of the baseline and the delivered `v0.35.0` roadmap are in the [2026-08-02 market scan](market-scan-2026-08-02.md). The Work Ledger storage rationale is in [ADR: Durable Work Ledger storage](../decisions/durable-work-ledger-storage.md). Operator behavior for versioned checkpoints, effect receipts, and the three recovery modes is documented in [Agent Runtime checkpoint recovery](checkpoint-recovery.md). Execution provider capabilities, the opt-in Claude Code harness, disposable worktrees, artifact confinement, and lifecycle recovery are documented in [Execution-plane operations](execution-plane.md). Internal worker protocol, SSH pilot, workspace sync, recovery, and A2A external-agent configuration are documented in [Remote worker and A2A operations](remote-workers-a2a.md). Proof authority, deterministic verification, stale evidence, bounded LLM judging, and baseline-gated automatic fan-out are documented in [Independent proof verification](proof-verification.md). Candidate evaluation, approval, rollout, outcome regression review, and one-action rollback are documented in [Reviewed self-improvement operations](reviewed-self-improvement.md).
