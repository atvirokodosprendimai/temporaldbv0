# Task ADR-001-T7: Snapshot, streaming shipper, purge, and restore

**Depends-on:** T6
**Covers:** none — no spec
**Estimated scope:** L (cross-boundary)
**Owner:** unassigned
**Produces:** `backup.Sink` interface (`WriteEvents`, `WriteSnapshot`, `Purge`), the local-file sink, `backup.Restore(ctx, sink, db)`
**Consumes:** `event.Store.After(seq)` (T2), `temporal.Visible(events, asOf, validAt) (Event, bool)` (T3), `cmd/temporaldb-server` composition root (T6)
**Data dependency:** hermetic — the sink is a temp directory, never a remote target
**Proof map:** v1
**Rests-on:** `the purge floor at the newest snapshot`, `the durable shipper cursor`, `the snapshot being a stock SQLite file`

## Goal

Ship every event to a pluggable sink as it is appended, take periodic stock-openable snapshots via
`VACUUM INTO`, and be able to restore from the pair — with purge bounded by an explicit retention.

## Affected Files

| File | Change | Why |
|------|--------|-----|
| `internal/backup/backup.go` | add | `Sink`, shipper loop, snapshot, purge, restore (§D7) |
| `internal/backup/local.go` | add | the concrete local-file sink v1 ships |
| `internal/backup/backup_test.go` | add | the tests below |
| `cmd/temporaldb-server/main.go` | edit | wire the shipper and the snapshot ticker into the server — the line that selects this package |
| `internal/config/config.go` | edit | backup directory, snapshot interval, retention window |

Without the `main.go` edit this whole package is finished, tested and unreachable — the exact defect
class rung 2 exists to catch.

## Ordered Steps

1. [S1] Write and commit failing tests for shipping, snapshotting, purge bounds and restore — red
   first (TDD red). [proof: acceptance]
2. [S2] Define `Sink` narrowly: `WriteEvents`, `WriteSnapshot`, `Purge`. Accept the interface at the
   consumer; return the concrete local sink from its constructor. [proof: acceptance]
3. [S3] Implement the shipper: poll `event.Store.After(cursor)`, write, advance the cursor durably so
   a restart neither skips nor duplicates.
4. [S4] Implement the snapshot as `VACUUM INTO`, producing a stock SQLite file usable as the restore
   anchor.
5. [S5] Implement `Purge` with an explicit retention window, refusing to purge past the newest
   snapshot's sequence — a purge that outruns its anchor is unrecoverable.
6. [S6] Implement `Restore`: open the snapshot, replay shipped events after it, resolving visibility
   through `temporal.Visible`.
7. [S7] Wire shipper + snapshot ticker into `cmd/temporaldb-server`, gated on config.
8. [S8] Run the Acceptance fence and record it with `adr-verify`. [proof: acceptance]

## Acceptance

```bash
set -o pipefail
CGO_ENABLED=0 go build ./... && CGO_ENABLED=0 go vet ./... || exit 1
CGO_ENABLED=0 go test ./internal/backup/... 2>&1 | tee /tmp/acc-adr001-t7.out && ! grep -qE "no test files|no tests to run|^FAIL|^--- FAIL" /tmp/acc-adr001-t7.out || exit 1
CGO_ENABLED=0 go test ./... 2>&1 | tee /tmp/acc-adr001-t7-reg.out && ! grep -qE "^FAIL|^--- FAIL" /tmp/acc-adr001-t7-reg.out
```

## Tests

| Test name | File | Verifies | Covers | Steps |
|-----------|------|----------|--------|-------|
| `TestShipperSendsEveryEventExactlyOnce` | `internal/backup/backup_test.go` | no gaps, no duplicates across a simulated restart | — | S3 |
| `TestSnapshotIsAStockSQLiteFile` | `internal/backup/backup_test.go` | the `VACUUM INTO` output opens with a plain driver and holds the rows | — | S4 |
| `TestPurgeRefusesToOutrunNewestSnapshot` | `internal/backup/backup_test.go` | the destructive operation has a hard floor | — | S5 |
| `TestPurgeRemovesOnlyEventsOutsideRetention` | `internal/backup/backup_test.go` | retention is a window, not "everything old" | — | S5 |
| `TestRestoreFromSnapshotPlusShippedEvents` | `internal/backup/backup_test.go` | snapshot + tail reproduces the source database exactly | — | S6 |
| `TestServerStartsShipperWhenBackupConfigured` | `internal/backup/backup_test.go` | delete the wiring line in `main.go` and this goes red (the rung-2 selection check) | — | S7 |

## Reachability

| Rung | How this task shows it |
|------|------------------------|
| 1 — exists | `TestShipperSendsEveryEventExactlyOnce` |
| 2 — something selects it | `TestServerStartsShipperWhenBackupConfigured` — the mutant is deleting the shipper start in `cmd/temporaldb-server/main.go` |
| 3 — the caller can discover it | The backup config keys are the declared interface; assert them by name so an undocumented key cannot be the only way in |
| 4 — it is used | nothing measures this yet — no backup lag metric in v1 |

## Prescribed Mutant

Chosen at authoring, because `adr-verify --mutant` refuses an absent, non-unique or comment-only
mutant but **cannot judge whether the mutant was well chosen**. ⚠ `--from` must match exactly one
place, and this was written before the code existed: adapt the string, keep the mechanism.

- **Break:** the purge floor in `internal/backup/backup.go` — remove the clamp that stops the purge
  bound at the newest successful snapshot's sequence, so `Purge` honours the retention window alone.
- **Must go red:** `TestPurgeRefusesToOutrunNewestSnapshot`.
- **Why this one:** purge is the only destructive operation in the system, and this clamp is its only
  hard floor. **A `survived` verdict here is a blocker, not a note** — it means the system can delete
  events that no snapshot has yet anchored, which is unrecoverable by construction.

One entry per mechanism in `Rests-on:`; use `--covers` to name which.

## Mutation Log

Tool-written by `adr-verify <task.md> --mutant …`. Empty at authoring.

## Invariants

- `Purge` never deletes past the newest successful snapshot's sequence.
- The shipper cursor is durable: a crash re-ships at most, never skips.
- A snapshot is always a stock, directly-openable SQLite file — that is the property that makes the
  non-standard streaming format acceptable (§D7, and the ADR's Risks table).

## Risks

- The streaming format is this project's own event JSON, not WAL frames, so a restore depends on this
  code being able to read it. Mitigated by the snapshot anchor, which needs none of this code.
- Purge is the only destructive operation in the system; both purge tests are load-bearing and a
  survived mutant on either is a blocker, not a note.

## Stop Condition

Stop and ask before shipping any purge whose bound cannot be stated as a sequence number — "old
enough" is not a bound.

## Out of Scope

- Remote sinks (S3, GCS): the interface is pluggable, v1 ships local-file only.
- Literal SQLite WAL-frame replication (permanent: rejected in the ADR's Alternatives Considered).

## Verification Log

Tool-written by `adr-verify <this-file>`. Empty at authoring.
