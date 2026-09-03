# Task ADR-001-T1: Module, config, and the SQLite storage core with the events/live schema

**Depends-on:** none
**Covers:** none — no spec
**Estimated scope:** M (multi-file)
**Owner:** unassigned
**Produces:** `config.Load() (Config, error)`, `storage.Open(ctx, Config) (*sql.DB, error)`, migration `0001_init.sql` creating `events` and `live`
**Consumes:** none
**Data dependency:** hermetic
**Proof map:** v1

## Goal

Stand up the Go module and the single SQLite storage core — cgo-free driver, WAL pragmas, goose
migrations, and the `events`/`live` schema every later task reads and writes.

## Affected Files

| File | Change | Why |
|------|--------|-----|
| `go.mod` | add | module declaration; `modernc.org/sqlite`, `github.com/pressly/goose/v3` (ADR-001 §D1) |
| `go.sum` | add | pinned checksums for the above |
| `internal/config/config.go` | add | env + optional `.env` loading (§D12) |
| `internal/config/config_test.go` | add | the failing test for `Load()` precedence |
| `internal/storage/storage.go` | add | `Open`, WAL/`busy_timeout`/`foreign_keys` pragmas, goose runner, connection-pool cap (§D13) |
| `internal/storage/migrations/0001_init.sql` | add | `events` append-only log and `live` projection |
| `internal/storage/storage_test.go` | add | the failing tests for pragmas, migration, pool cap |

Nothing selects `storage.Open` in this task: T2's `event.Store` constructor is its first caller.
Recorded here rather than left implicit — see Reachability rung 2.

## Ordered Steps

1. [S1] Write and commit the failing tests for `config.Load()` precedence and `storage.Open()`
   pragmas/migration — red before any production code (TDD red). [proof: acceptance]
2. [S2] `go mod init`; add `modernc.org/sqlite` and `github.com/pressly/goose/v3`; confirm
   `CGO_ENABLED=0 go build ./...` succeeds.
3. [S3] Implement `internal/config`: env first, optional `.env` as fallback, no required file.
4. [S4] Write `0001_init.sql`: `events(seq INTEGER PRIMARY KEY, collection, key, op, value,
   valid_from, valid_to, tx_time, meta)` plus the `live` projection keyed `(collection, key)`.
5. [S5] Implement `storage.Open`: apply `journal_mode=WAL`, `busy_timeout`, `foreign_keys=ON`, cap
   `SetMaxOpenConns` for the single-writer model, then run goose migrations to latest.
6. [S6] Run the Acceptance fence and record it with `adr-verify`. [proof: acceptance]

## Acceptance

```bash
set -o pipefail
CGO_ENABLED=0 go build ./... && CGO_ENABLED=0 go vet ./... || exit 1
CGO_ENABLED=0 go test ./internal/storage/... ./internal/config/... 2>&1 | tee /tmp/acc-adr001-t1.out && ! grep -qE "no test files|no tests to run|^FAIL|^--- FAIL" /tmp/acc-adr001-t1.out
```

## Tests

| Test name | File | Verifies | Covers | Steps |
|-----------|------|----------|--------|-------|
| `TestLoadPrefersEnvOverDotEnv` | `internal/config/config_test.go` | env wins over `.env`; absent `.env` is not an error | — | S3 |
| `TestOpenAppliesWALAndBusyTimeout` | `internal/storage/storage_test.go` | `PRAGMA journal_mode` returns `wal`; `busy_timeout` is non-zero | — | S5 |
| `TestOpenRunsMigrationsToLatest` | `internal/storage/storage_test.go` | `events` and `live` exist with the declared columns after `Open` | — | S4, S5 |
| `TestOpenCapsWriterConnections` | `internal/storage/storage_test.go` | pool cap is set, so the single-writer model is enforced by the pool not by luck | — | S5 |
| `TestBuildIsCGOFree` | `internal/storage/storage_test.go` | no cgo-only driver is linked (guards §D1's nocgo requirement) | — | S2 |

## Reachability

| Rung | How this task shows it |
|------|------------------------|
| 1 — exists | `TestOpenRunsMigrationsToLatest` |
| 2 — something selects it | Nothing selects `storage.Open` yet — T2's `event.NewStore` is the first caller, and the mutation proving selection belongs to T2. Recorded as a known-unreached rung rather than claimed. |
| 3 — the caller can discover it | Config keys are the declared interface; `TestLoadPrefersEnvOverDotEnv` reads them by name |
| 4 — it is used | nothing measures this yet |

## Mutation Log

Tool-written by `adr-verify <task.md> --mutant …`. Empty at authoring.

## Invariants

- The build stays cgo-free: `CGO_ENABLED=0 go build ./...` must succeed (§D1).
- `events` is append-only — no `UPDATE` or `DELETE` statement against it anywhere in the tree.
- Exactly one migration path: goose, from `internal/storage/migrations`, never hand-run DDL.

## Risks

- `modernc.org/sqlite` pragma handling differs subtly from `mattn/go-sqlite3`; mitigated by asserting
  the pragma's *read-back value* in a test rather than trusting the statement returned no error.

## Stop Condition

Stop and ask if `modernc.org/sqlite` cannot apply `journal_mode=WAL` on the target platform — that
would invalidate §D1 and §D13 together, which is an ADR-level decision, not a task-level fix.

## Out of Scope

- Any table beyond `events` and `live` — typed per-collection projections are deferred
  (`docs/adr/BACKLOG.md` §3).
- Reading or writing events: that is T2's job.

## Verification Log

Tool-written by `adr-verify <this-file>`. Empty at authoring.
