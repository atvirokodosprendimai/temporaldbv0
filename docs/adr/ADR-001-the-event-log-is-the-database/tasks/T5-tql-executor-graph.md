# Task ADR-001-T5: Compile the AST to SQL, add graph edges, and wire vector SEARCH

**Depends-on:** T2, T3, T4, T10
**Covers:** none — no spec
**Estimated scope:** L (cross-boundary)
**Owner:** unassigned
**Produces:** `tql.Executor.Exec(ctx, ast.Stmt) (Result, error)`, `graph` edge helpers over `internal/event`
**Consumes:** `tql.Parse(src string) (ast.Stmt, error)` (T4), `temporal.Visible(events, asOf, validAt) (Event, bool)` (T3), `event.Store.Append(ctx, Event) (seq int64, err error)` (T2), `vector.Index.Search(ctx, collection, text, k)` (T10)
**Data dependency:** hermetic — the vector leg is exercised against a stubbed `vector.Index`, never a live service
**Proof map:** v1
**Rests-on:** `parameter binding in FIND`, `the SEARCH dispatch to vector.Index`, `the temporal.Visible delegation`

## Goal

Execute a parsed statement: compile `WHERE` to `json_extract`/`json_each` predicates, resolve
temporal clauses through `temporal.Visible`, store relations as ordinary events, and dispatch
`SEARCH` to the vector index only when one is configured.

## Affected Files

| File | Change | Why |
|------|--------|-----|
| `internal/tql/executor.go` | add | AST → SQL / vector calls → `Result` (§D4) |
| `internal/tql/executor_test.go` | add | the tests below |
| `internal/graph/graph.go` | add | edge collection helpers, stored as events (§D5) |
| `internal/graph/graph_test.go` | add | edges are events, not a second subsystem |
| `internal/tql/result.go` | add | the `Result` type both the server and the client marshal |

`SEARCH` dispatch is the selection site for T10's work: the executor is what makes
`vector.Index.Search` reachable, and nothing else calls it.

## Ordered Steps

1. [S1] Write and commit the failing executor tests, one per verb, plus the "vector unconfigured"
   case — red first (TDD red). [proof: acceptance]
2. [S2] Implement `PUT`/`GET`/`HISTORY` over `event.Store`, routing every visibility decision through
   `temporal.Visible`.
3. [S3] Implement `FIND`: compile `WHERE` comparisons to `json_extract`/`json_each` predicates with
   bound parameters — never string concatenation of user input.
4. [S4] Implement `RELATE` and edge traversal in `internal/graph` as ordinary events in the same log
   (§D5), so time travel over relations is the same code path.
5. [S5] Implement `SEARCH`: dispatch to `vector.Index` when configured; return a named
   `feature not configured` error when it is not — absent, never broken (§D6).
6. [S6] Implement `PURGE` as the executor's side of T7's retention contract.
7. [S7] Run the Acceptance fence and record it with `adr-verify`. [proof: acceptance]

## Acceptance

```bash
set -o pipefail
CGO_ENABLED=0 go build ./... && CGO_ENABLED=0 go vet ./... || exit 1
CGO_ENABLED=0 go test ./internal/tql/... ./internal/graph/... 2>&1 | tee /tmp/acc-adr001-t5.out && ! grep -qE "no test files|no tests to run|^FAIL|^--- FAIL" /tmp/acc-adr001-t5.out || exit 1
CGO_ENABLED=0 go test ./... 2>&1 | tee /tmp/acc-adr001-t5-reg.out && ! grep -qE "^FAIL|^--- FAIL" /tmp/acc-adr001-t5-reg.out
```

## Tests

| Test name | File | Verifies | Covers | Steps |
|-----------|------|----------|--------|-------|
| `TestExecPutThenGetRoundTrips` | `internal/tql/executor_test.go` | the write path and the read path agree | — | S2 |
| `TestExecGetAsOfUsesTemporalVisible` | `internal/tql/executor_test.go` | time travel is delegated, not reimplemented — delete the `temporal.Visible` call and this goes red | — | S2 |
| `TestExecFindBindsParametersNotStrings` | `internal/tql/executor_test.go` | a value containing SQL syntax is matched literally, proving binding | — | S3 |
| `TestExecFindJSONPushdown` | `internal/tql/executor_test.go` | nested field and array-membership predicates both filter correctly | — | S3 |
| `TestRelateIsStoredAsAnEvent` | `internal/graph/graph_test.go` | an edge appears in `events`, so §D5's "one storage mechanism" holds | — | S4 |
| `TestTraverseAsOfSeesHistoricalEdges` | `internal/graph/graph_test.go` | relations time-travel because they are events | — | S4 |
| `TestExecSearchDispatchesToVectorIndex` | `internal/tql/executor_test.go` | the stub index is called — the rung-2 selection check for T10 | — | S5 |
| `TestExecSearchWithoutVectorConfigIsNamedError` | `internal/tql/executor_test.go` | unconfigured is "feature absent", not a panic or a 500 | — | S5 |
| `TestExecPurgeRemovesOnlyEventsPastRetention` | `internal/tql/executor_test.go` | purge is bounded and never touches live state | — | S6 |

## Reachability

| Rung | How this task shows it |
|------|------------------------|
| 1 — exists | `TestExecPutThenGetRoundTrips` |
| 2 — something selects it | The executor selects `tql.Parse` (T4), `temporal.Visible` (T3), `event.Store.Append` (T2) and `vector.Index.Search` (T10). `TestExecGetAsOfUsesTemporalVisible` and `TestExecSearchDispatchesToVectorIndex` are the two that go red when the call is deleted — record both as mutants. |
| 3 — the caller can discover it | `Result`'s exported shape is the contract T6's HTTP body and T9's MCP schemas both marshal |
| 4 — it is used | nothing measures this yet — T6 is the first process that runs it |

## Prescribed Mutant

Chosen at authoring, because `adr-verify --mutant` refuses an absent, non-unique or comment-only
mutant but **cannot judge whether the mutant was well chosen**. ⚠ `--from` must match exactly one
place, and this was written before the code existed: adapt the string, keep the mechanism.

This task declares THREE mechanisms, so it owes three entries — a fence chaining several assertions
with one bound mutant has been shown able to fail for one reason, and nothing is known about the
other two.

- **Break (1):** parameter binding in `internal/tql/executor.go` — interpolate the value into the SQL
  string instead of passing a placeholder. **Must go red:** `TestExecFindBindsParametersNotStrings`.
  `--covers 'parameter binding in FIND'`. This is the injection guard; a survived mutant here is a
  security finding, not a test-quality note.
- **Break (2):** delete the `vector.Index.Search` call in the `SEARCH` arm. **Must go red:**
  `TestExecSearchDispatchesToVectorIndex`. `--covers 'the SEARCH dispatch to vector.Index'`. This is
  the rung-2 check that T10 is reachable at all.
- **Break (3):** replace the `temporal.Visible` call with an inline "take the newest event".
  **Must go red:** `TestExecGetAsOfUsesTemporalVisible`. `--covers 'the temporal.Visible delegation'`.
  A reimplemented comparison here is exactly what T3's invariant forbids.

## Mutation Log

Tool-written by `adr-verify <task.md> --mutant …`. Empty at authoring.
- 2026-09-03 · cafdd7b* · mutant survived · exit 0 · `internal/tql/executor.go` · interpolate the WHERE value into the SQL string instead of binding it · acceptance-sha256:f4190bffe9b3e17f8ddb534d8329fa958aa1ca19e166a14c8bac7fbd2931b9bd · covers:parameter binding in FIND
  ```
  the fence passed with the mechanism broken; it may not materialize, compile, load, or assert on the changed path
  ```
- 2026-09-03 · cafdd7b* · mutant killed · exit 1 · `internal/tql/executor.go` · delete the vector.Index.Search call in the SEARCH arm · acceptance-sha256:f4190bffe9b3e17f8ddb534d8329fa958aa1ca19e166a14c8bac7fbd2931b9bd · covers:the SEARCH dispatch to vector.Index
- 2026-09-03 · cafdd7b* · mutant killed · exit 1 · `internal/tql/executor.go` · replace the temporal.Visible call with an inline take-the-newest-event · acceptance-sha256:f4190bffe9b3e17f8ddb534d8329fa958aa1ca19e166a14c8bac7fbd2931b9bd · covers:the temporal.Visible delegation

## Invariants

- No user-supplied value is ever concatenated into SQL. Every predicate binds.
- Relations are events. There is no edges table.
- With `QDRANT_URL`/`TEI_URL` unset, every verb except `SEARCH` works unchanged.

## Risks

- `json_extract`/`json_each` performance on large collections is a known accepted cost with a
  numbered revisit trigger (`docs/adr/BACKLOG.md` §2, §3); this task does not optimise it.
- The executor is the widest surface in the record and the easiest place to grow scope. The Out of
  Scope list below is the boundary.

## Stop Condition

Stop and ask if a §D4 verb cannot be expressed as JSON pushdown without a typed projection table —
that pulls `docs/adr/BACKLOG.md` §3 forward into a decision this task may not make.

## Out of Scope

- HTTP, the client package, and the CLI: T6 and T8.
- Real Qdrant/TEI calls: T10 owns the client, and end-to-end verification is out of the ADR's scope.
- Automatic embedding on `PUT` (deferred: `docs/adr/BACKLOG.md` §5).
- `SEARCH ... AS OF` (deferred: `docs/adr/BACKLOG.md` §6).

## Verification Log

Tool-written by `adr-verify <this-file>`. Empty at authoring.
- 2026-09-03 · cafdd7b* · exit 0 · `set -o pipefail …` · acceptance-sha256:f4190bffe9b3e17f8ddb534d8329fa958aa1ca19e166a14c8bac7fbd2931b9bd · ms:2971
- 2026-09-03 · cafdd7b* · exit 0 · `set -o pipefail …` · acceptance-sha256:f4190bffe9b3e17f8ddb534d8329fa958aa1ca19e166a14c8bac7fbd2931b9bd · ms:2681
- 2026-09-03 · cafdd7b* · exit 0 · `set -o pipefail …` · acceptance-sha256:f4190bffe9b3e17f8ddb534d8329fa958aa1ca19e166a14c8bac7fbd2931b9bd · ms:2486
- 2026-09-03 · cafdd7b* · exit 0 · `set -o pipefail …` · acceptance-sha256:f4190bffe9b3e17f8ddb534d8329fa958aa1ca19e166a14c8bac7fbd2931b9bd · ms:2626
