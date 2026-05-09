# SonarCloud Maintainability Triage - 2026-05-09

## Snapshot

This triage records the high-volume maintainability clusters from issue #788.
Counts below came from SonarCloud queries on 2026-05-09 after the first
static-analysis burn-down slices had landed.

Already completed implementation slices:

- #786 / PR #799 reduced the `go:S3776` stalled-chat detector complexity
  cluster in `internal/pulse`.
- #790 / PR #798 reduced a duplicated-code cluster in Pulse chat signal detail
  generation.
- #787 / PR #797 reduced repeated Go string literals in `internal/git`.

## Rule Decisions

| Rule | Count | Representative paths | Decision | Next action |
| --- | ---: | --- | --- | --- |
| `godre:S8209` | 72 | `internal/tarsserver/handler_extensions.go`, `internal/tool/tool_subagents.go`, `internal/launchagent/launchagent.go` | Treat as opportunistic refactor work. Consecutive same-type parameters are noisy, but many findings are internal helpers where converting every call to option structs would create review churn. | When touching a clustered package, prefer a small request/options struct only if it names a real domain concept and reduces call-site ambiguity. Do not run a repository-wide mechanical rewrite. |
| `godre:S8193` | 22 | `*_test.go` files such as `internal/extensions/manager_test.go`, `internal/tarsserver/handler_logs_test.go`, `internal/skill/frontmatter_test.go` | Accept as low-risk cleanup. These are mostly test-local temporary variables that can be removed safely but do not justify standalone PRs. | Clean up alongside nearby test edits. No dedicated issue needed unless one package accumulates related test cleanup. |
| `typescript:S6551` | 20 | `frontend/console/src/lib/onboarding.ts`, `configStructured.ts`, `quickStartFields.ts`, `pulseIncidentCards.ts` | Implementation-worthy, but should be handled as one frontend coercion/normalization slice rather than many one-line `String(...)` edits. | For the next frontend-maintainability pass, add typed coercion helpers for schema-derived `unknown` values and update representative onboarding/config paths with targeted tests. |
| `go:S107` | 23 | `internal/tarsserver/helpers_build_tools.go`, `handler_chat_context.go`, `helpers_chat.go`, `helpers_cron.go` | Implementation-worthy only at package boundaries with stable call shapes. The repeated theme is handler/helper parameter bags, not a single global defect. | Use package-local request structs when a touched helper already has many related arguments. Keep changes package-scoped and covered by existing handler tests. |
| `typescript:S4624` | 14 | `frontend/console/src/lib/api.ts`, `configMetaBadges.ts`, `toolCalls.ts`, `agentruntime-graph.ts` | Mostly mechanical readability cleanup around nested template literals. | Fix opportunistically in the owning frontend file, ideally near existing tests or while editing the same API/helper surface. |
| `typescript:S6571` | 10 | `frontend/console/src/lib/types.ts` | Mixed. Some `literal | string` unions intentionally allow backend extensibility, but they make the literal members redundant to the type checker. | Preserve extensible API fields when the backend can add values. When a field is closed, remove `| string`; when it is open, use a named alias or comment at the type definition rather than repeating literals. |

## Follow-Up Policy

Do not create placeholder GitHub issues for every SonarCloud cluster. The useful
unit of work is a package- or workflow-scoped implementation slice with clear
tests. Create a new issue only when a cluster is selected for immediate work and
the issue can name the touched package, representative paths, acceptance
criteria, and validation commands.

For the current burn-down, #786, #787, #790, and this triage document cover the
implementation and decision work requested by #788. Remaining findings are
documented, accepted as opportunistic cleanup, or scoped for future issue
creation at implementation time.
