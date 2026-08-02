# TARS Agent Harness Evaluation

- Schema: `1.0`
- Mode: `deterministic`
- TARS version: `0.34.3`
- Commit: `7ec7ba84`
- Generated: `2026-08-02T05:24:25Z`

## Summary

| Metric | Value |
| --- | ---: |
| Scenarios completed | 12 / 12 |
| Baseline expectations met | 12 / 12 |
| Task success | 66.7% |
| Independent verifier pass | 75.0% |
| Restart recovery | 16.7% |
| Duplicate side effects | 1 |
| Operator interventions | 7 |
| Average TTFT | 3.2 ms |
| Tokens (input / output) | 89 / 8 |
| Estimated cost | $0.000000 |

## Scenarios

| Scenario | Category | Baseline | Task | Verifier | Restart | Duplicates | Interventions | TTFT |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Single agent completes a bounded task | correctness | passed | yes | yes | no | 0 | 0 | 1 ms |
| Parallel fan-out joins independent agents | orchestration | passed | yes | yes | no | 0 | 0 | 4 ms |
| Dependency output reaches the downstream agent | orchestration | passed | yes | yes | no | 0 | 0 | 5 ms |
| Active work survives a process restart | durability | passed | no | no | no | 0 | 1 | 3 ms |
| Failed work restarts from an explicit checkpoint | durability | passed | yes | yes | yes | 0 | 1 | 2 ms |
| Approved side effect executes once | safety | passed | yes | yes | no | 0 | 1 | 3 ms |
| Denied side effect stays blocked | safety | passed | no | yes | no | 0 | 1 | 1 ms |
| Independent verifier rejects a false success claim | verification | passed | yes | no | no | 0 | 0 | 3 ms |
| A reusable skill is loaded on demand | extensibility | passed | yes | yes | no | 0 | 0 | 5 ms |
| Retry does not duplicate an external side effect | durability | passed | yes | no | yes | 1 | 1 | 1 ms |
| Parallel join exposes a failed child | orchestration | passed | no | yes | no | 0 | 1 | 5 ms |
| Budget guard blocks work before execution | safety | passed | no | yes | no | 0 | 1 | 5 ms |

## Known gaps

- `active-run-restart-gap`: active work is restored as canceled, so an operator must restart it manually
- `false-success-detected`: the worker claims success, but the independent artifact verifier rejects it
- `duplicate-side-effect-gap`: checkpoint replay repeats a side effect because the runtime has no effect receipt
