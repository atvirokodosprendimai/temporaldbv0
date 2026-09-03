# Task ADR-001-T2: The append-only event write path and the live projection upsert

**Depends-on:** T1
**Covers:** none — no spec
**Estimated scope:** M (multi-file)
**Owner:** unassigned
**Produces:** `event.Store.Append(ctx, Event) (seq int64, err error)`, `event.Store.History(...)`, `event.Store.ReplayAsOf(...)`, `event.Store.After(seq)`
**Consumes:** `storage.Open(ctx, Config) (*sql.DB, error)` (T1)
**Data dependency:** hermetic
**Proof map:** v1

## Goal

Implement the single write path: append an event to `events` and upsert the derived `live` row in
one transaction, under one process-wide writer mutex.

## Affected Files

| File | Change | Why |
|------|--------|-----|
| `internal/event/event.go` | add | `Store`, `Append`, `History`, `ReplayAsOf`, `After` (§D2) |
| `internal/event/event_test.go` | add | the failing tests below |
| `internal/storage/storage.go` | edit | expose whatever `Store` needs to open a writer connection (T1's contract, unchanged otherwise) |

`event.NewStore` is the first caller of `storage.Open` — that call site is what selects T1's work,
and the mutation in Reachability rung 2 proves it.

## Ordered Steps

1. [S1] Write and commit the failing tests for `Append` writing both tables in one transaction and
   for the writer mutex serialising concurrent appends — red first (TDD red). [proof: acceptance]
2. [S2] Define the `Event` struct: collection, key, op, value (JSON), valid_from, valid_to, tx_time,
   meta. [proof: acceptance]
3. [S3] Implement `Append`: one `BEGIN IMMEDIATE` transaction, insert into `events`, then the
   partial-index-safe `ON CONFLICT (collection, key) DO UPDATE` into `live`; guard the whole call
   with the single-writer mutex (§D13).
4. [S4] Implement the read side: `History` (all versions of one key), `ReplayAsOf` (state at an
   instant), `After(seq)` (the streaming-backup cursor T7 consumes).
5. [S5] Run the Acceptance fence and record it with `adr-verify`. [proof: acceptance]

## Acceptance

```bash
set -o pipefail
CGO_ENABLED=0 go build ./... && CGO_ENABLED=0 go vet ./... || exit 1
CGO_ENABLED=0 go test ./internal/event/... 2>&1 | tee /tmp/acc-adr001-t2.out && ! grep -qE "no test files|no tests to run|^FAIL|^--- FAIL" /tmp/acc-adr001-t2.out || exit 1
CGO_ENABLED=0 go test ./... 2>&1 | tee /tmp/acc-adr001-t2-reg.out && ! grep -qE "^FAIL|^--- FAIL" /tmp/acc-adr001-t2-reg.out
```

## Tests

| Test name | File | Verifies | Covers | Steps |
|-----------|------|----------|--------|-------|
| `TestAppendWritesEventAndLiveAtomically` | `internal/event/event_test.go` | both rows land, or neither does when the `live` upsert fails | — | S3 |
| `TestAppendIsSerialisedUnderConcurrency` | `internal/event/event_test.go` | N goroutines appending yield N gapless sequence numbers, no `SQLITE_BUSY` | — | S3 |
| `TestAppendOpensStorageThroughStorageOpen` | `internal/event/event_test.go` | the store is built on T1's `storage.Open`, so WAL and migrations are in force (the rung-2 selection check) | — | S3 |
| `TestHistoryReturnsAllVersionsInSequenceOrder` | `internal/event/event_test.go` | `History` is ordered and complete | — | S4 |
| `TestAfterReturnsOnlyEventsPastCursor` | `internal/event/event_test.go` | `After(seq)` is exclusive — the contract T7 ships on | — | S4 |
| `TestEventsTableIsNeverUpdated` | `internal/event/event_test.go` | an append never mutates a prior row (append-only invariant) | — | S3 |

## Reachability

| Rung | How this task shows it |
|------|------------------------|
| 1 — exists | `TestAppendWritesEventAndLiveAtomically` |
| 2 — something selects it | `event.NewStore` calls `storage.Open`; delete that call and `TestAppendOpensStorageThroughStorageOpen` goes red — that is the mutant to record |
| 3 — the caller can discover it | `Append`/`History`/`After` are the exported Go API; godoc is the declared interface |
| 4 — it is used | nothing measures this yet — T5's executor is the first real consumer |

## Mutation Log

Tool-written by `adr-verify <task.md> --mutant …`. Empty at authoring.

## Invariants

- One writer. Every write goes through `Store.Append`; no other code opens a write transaction.
- `events` rows are never updated or deleted by the write path (purge is T7's, and is a separate,
  explicitly-bounded operation).
- The `live` upsert stays partial-index-safe: the `ON CONFLICT` target must name the columns, not an
  index name.

## Risks

- A naive `ON CONFLICT` target breaks the moment a partial index is added; mitigated by asserting the
  upsert against a schema that already carries one in the test fixture.

## Stop Condition

Stop and ask if serialising every write behind one mutex proves unacceptable for the intended write
volume — that reopens §D13, which this task may not decide alone.

## Out of Scope

- Time-travel comparison semantics: `Visible()` is T3's.
- Any query language: T4 and T5.

## Verification Log

Tool-written by `adr-verify <this-file>`. Empty at authoring.
