# Task ADR-001-T3: One bitemporal visibility function, with half-open intervals

**Depends-on:** T2
**Covers:** none — no spec
**Estimated scope:** S (single file)
**Owner:** unassigned
**Produces:** `temporal.Visible(events, asOf, validAt) (Event, bool)`, `temporal.Parse(s string) (time.Time, error)`
**Consumes:** `event.Event` (T2)
**Data dependency:** hermetic
**Proof map:** v1
**Rests-on:** `the half-open valid_to boundary`, `the tx_time cutoff`

## Goal

Funnel every bitemporal boundary comparison in the system through one tested function, so the
inclusive-end bug class documented in the ADR's Existing Primitives Audit has exactly one place it
could live.

## Affected Files

| File | Change | Why |
|------|--------|-----|
| `internal/temporal/temporal.go` | add | `Visible`, `Parse`, the `Event` view type (§D3) |
| `internal/temporal/temporal_test.go` | add | the boundary table below |

## Ordered Steps

1. [S1] Write and commit the failing boundary tests first, including the exact inclusive-end failure
   shape named in the ADR's Existing Primitives Audit (TDD red). [proof: acceptance]
2. [S2] Define the interval convention explicitly in a doc comment: `[valid_from, valid_to)` and
   `[tx_time, ...)`, half-open at both ends, and say why.
3. [S3] Implement `Visible`: given a key's events, the transaction instant `asOf` and the valid
   instant `validAt`, return the one visible version, or `false`.
4. [S4] Implement `Parse` so every timestamp in the system is parsed in one place, in UTC.
5. [S5] Run the Acceptance fence and record it with `adr-verify`. [proof: acceptance]

## Acceptance

```bash
set -o pipefail
CGO_ENABLED=0 go build ./... && CGO_ENABLED=0 go vet ./... || exit 1
CGO_ENABLED=0 go test ./internal/temporal/... 2>&1 | tee /tmp/acc-adr001-t3.out && ! grep -qE "no test files|no tests to run|^FAIL|^--- FAIL" /tmp/acc-adr001-t3.out || exit 1
CGO_ENABLED=0 go test ./... 2>&1 | tee /tmp/acc-adr001-t3-reg.out && ! grep -qE "^FAIL|^--- FAIL" /tmp/acc-adr001-t3-reg.out
```

## Tests

| Test name | File | Verifies | Covers | Steps |
|-----------|------|----------|--------|-------|
| `TestVisibleExcludesValidToBoundary` | `internal/temporal/temporal_test.go` | an instant exactly equal to `valid_to` is NOT visible — the documented bug class | — | S2, S3 |
| `TestVisibleIncludesValidFromBoundary` | `internal/temporal/temporal_test.go` | an instant exactly equal to `valid_from` IS visible | — | S2, S3 |
| `TestVisibleIgnoresEventsWrittenAfterAsOf` | `internal/temporal/temporal_test.go` | transaction-time travel: a later write is invisible to an earlier `asOf` | — | S3 |
| `TestVisibleReturnsFalseForDeletedKey` | `internal/temporal/temporal_test.go` | a delete event makes the key not visible rather than returning a stale version | — | S3 |
| `TestParseIsUTCAndRejectsAmbiguousInput` | `internal/temporal/temporal_test.go` | one parse path, no local-zone drift | — | S4 |

## Reachability

| Rung | How this task shows it |
|------|------------------------|
| 1 — exists | `TestVisibleExcludesValidToBoundary` |
| 2 — something selects it | No caller yet in this task; T5's executor and T7's restore are the two call sites, and each records its own selection mutant. Stated rather than claimed. |
| 3 — the caller can discover it | The doc comment on `Visible` states the half-open convention; it is the declared contract |
| 4 — it is used | nothing measures this yet |

## Prescribed Mutant

Chosen at authoring, because `adr-verify --mutant` refuses an absent, non-unique or comment-only
mutant but **cannot judge whether the mutant was well chosen**. ⚠ `--from` must match exactly one
place, and this was written before the code existed: adapt the string, keep the mechanism.

- **Break:** the half-open upper bound in `internal/temporal/temporal.go` — e.g.
  `--from 'at.Before(e.ValidTo)' --to '!at.After(e.ValidTo)'`, turning `[from, to)` into `[from, to]`.
- **Must go red:** `TestVisibleExcludesValidToBoundary`.
- **Why this one:** this IS the inclusive-end bug class the ADR's Existing Primitives Audit
  documents. If it survives, the boundary test is asserting something else and the whole
  time-travel guarantee rests on nothing.

One entry per mechanism in `Rests-on:`; use `--covers` to name which.

## Mutation Log

Tool-written by `adr-verify <task.md> --mutant …`. Empty at authoring.

## Invariants

- Every interval comparison in the repository goes through `Visible`. A `valid_to`/`tx_time`
  comparison written anywhere else is a defect, whatever it returns.
- Intervals are half-open. Changing that convention is an ADR amendment, not a task edit.

## Risks

- The function is small enough that a future caller "just inlines the comparison"; mitigated by the
  invariant above plus a grep in the audit step of whichever task next touches a comparison.

## Stop Condition

Stop and ask if a real requirement needs closed intervals — that contradicts §D3 and every test here.

## Out of Scope

- Compiling temporal clauses to SQL: that is T5.

## Verification Log

Tool-written by `adr-verify <this-file>`. Empty at authoring.
