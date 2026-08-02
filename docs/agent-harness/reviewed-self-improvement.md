# Reviewed self-improvement operations

TARS treats a learned skill as a versioned capability proposal, not as a file
write. Session extraction may append a candidate to the existing Skill Inbox,
but it cannot activate or overwrite `workspace/skills/<name>`.

## Lifecycle and gates

| State | Required evidence | Active skill mutation |
| --- | --- | --- |
| `candidate` | Session provenance, evidence, and outcome/correction signals | None |
| `draft` | Immutable draft snapshot and content digest | None |
| `sandbox` | Companion CLI smoke result from an isolated test directory | None |
| `offline_eval` | Parsed `SKILL.md`, valid draft paths, metrics, permission delta | None |
| `shadow` | A second independent evaluator run and linked Proof | None |
| `approved` | Human Approval linked to the capability Work | None |
| `canary` | Operator-scoped canary evaluation and passed Proof | None |
| `promoted` | Every earlier gate passed | Atomically replaces the active skill directory |
| `rolled_back` | Operator rollback request | Atomically restores the previous snapshot, or removes a first version |
| `rejected` | Operator rejection | None |

The Work Ledger rejects skipped transitions. A failed or missing sandbox,
offline, shadow, or canary evaluation cannot advance through its corresponding
gate. Human approval is mandatory before canary and promotion. Generated code
is only materialized into the active Skills directory during promotion.

## Stored records

- `CapabilityVersion` contains the candidate ID, Work ID, ordered version,
  content digest, complete draft snapshot, provenance, requested permissions,
  approval, rollout policy, previous version, and rollback target.
- `EvaluationRun` contains sandbox/offline/shadow/canary status, success and
  verification rates, cost, latency, baseline deltas, a report, and a linked
  independent Proof.
- `CapabilityOutcome` links a promoted version to later Work and Attempt
  outcomes. Failed or independently failed/stale outcomes mark the rollout
  `review_required` and append a `capability.regression_detected` event.

When chat routing explicitly selects a promoted workspace skill, TARS resolves
the active `CapabilityVersion` and carries its validated ID into any durable
subagent Work created by that turn. The scheduler rejects non-promoted
references and records one idempotent outcome per capability and Step Attempt,
including execution status, independent-verifier status, cost, and latency.
Arbitrary Work metadata cannot forge capability attribution.

These records appear in Work projections, deterministic JSONL exports, backup
and restore, migration checks, and `workstore doctor`. The Console Skill Inbox
shows provenance, content changes, evaluation deltas, permission expansion,
rollout percentage, rollback target, observed Work outcomes, and regression
review status before an operator chooses the next action.

## Existing Skill Inbox migration

Opening or extracting through the Skill Inbox imports existing
`_shared/skill_extraction/inbox.jsonl` candidates into the ledger in original
creation order. The import is idempotent on candidate ID and never rewrites the
JSONL source. Existing approved entries are marked promoted only when their
recorded skill directory can be safely read inside `workspace/skills`; rejected
entries remain rejected, and all other entries return to review as candidates.

## API actions

`POST /v1/admin/skills/extractions/review` accepts the following actions:

- `evaluate`: build the draft and run sandbox, offline, and shadow evaluation.
- `approve`: ensure evaluation is complete, record human approval, and run the
  operator canary. It does not activate the skill.
- `promote`: atomically activate a canary with passed evidence.
- `rollback`: restore the known-good target (or remove a first version) in one
  operation.
- `reject`: terminate a proposal without activating it.

If `work_ledger.enabled` is false, lifecycle mutations return service
unavailable rather than falling back to the former immediate-write behavior.

## Recovery notes

Skill directory replacement stages the new snapshot on the same filesystem,
moves any active directory aside, activates the staged version, and restores
the previous directory if the ledger transition fails. A successful ledger
transition removes the temporary backup. After promotion or rollback, TARS
reloads Extensions; reload failure is logged as an operator warning without
discarding the already-recorded rollout decision.
