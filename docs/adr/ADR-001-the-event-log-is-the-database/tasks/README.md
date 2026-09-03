# ADR-001 Tasks

Implementation tasks for ADR-001: The event log is the database; everything else is a projection.
See the parent ADR (`../../ADR-001-the-event-log-is-the-database.md`) for the decision.

**Source of truth:** the task files' `Depends-on` / `Produces` / `Consumes` / `Covers` headers.
This README is a derived index — when it disagrees with a task file, the task file wins and the
README must be regenerated. Regenerate rather than hand-edit.

## Execution Order

Ten tasks, so waves (parallel-safe groups) are required. Each wave is a topological level of the
`Depends-on` + `Consumes` DAG; do not start a wave until the previous one is fully committed.

| Wave | Tasks | Depends-on |
|------|-------|------------|
| 1 | T1 | none |
| 2 | T2, T4, T10 | T1 |
| 3 | T3 | T2 |
| 4 | T5 | T2, T3, T4, T10 |
| 5 | T6 | T5 |
| 6 | T7, T8, T9 | T6 |

## Task Index

| ID | Title | Status | Covers | Acceptance |
|----|-------|--------|--------|------------|
| T1 | Module + storage core | done | — | `go test ./internal/storage/... ./internal/config/...` |
| T2 | Event log + projection | done | — | `go test ./internal/event/...` |
| T3 | Temporal semantics | done | — | `go test ./internal/temporal/...` |
| T4 | TQL front end (lexer, parser, AST) | done | — | `go test ./internal/tql/...` |
| T5 | TQL executor + graph edges | done | — | `go test ./internal/tql/... ./internal/graph/...` |
| T6 | Server + client package | done | — | `go test ./internal/server/... ./client/...` |
| T7 | Backup: snapshot, shipper, purge, restore | done | — | `go test ./internal/backup/...` |
| T8 | CLI (REPL, -e, -f) | done | — | `go test ./cmd/temporaldb-cli/...` |
| T9 | MCP server over the official Go SDK | done | — | `go test ./cmd/temporaldb-mcp/...` |
| T10 | Vector clients (Qdrant + TEI) | done | — | `go test ./internal/vector/...` |

Status: `pending` | `partial` | `blocked` | `done`.

- `pending` — not started, or started and carrying no evidence yet.
- `partial` — genuinely part-done. A status with OBLIGATIONS, not a softer `pending`: its landed
  evidence is checked exactly as hard as a `done` task's, so a partial task with a passing
  Acceptance fence still owes a killed mutant.
- `blocked` — waiting on something outside this repository; say what in the task's `Blocked-on:`
  header, as an event a later reader can check HAS HAPPENED.
- `done` — finished, with tool-written acceptance and mutation evidence to match.

Every task is `pending`: this record is `Proposed` and no task has been executed, so no task file
carries a Verification Log entry yet. `done` may not be written here by hand — `adr-lint` requires a
matching exit-0 `adr-verify` entry and a `mutant killed` row behind it.

## Contract Coupling

Derived from the task files' `Produces` / `Consumes` headers.

| Producer | Contract | Consumer(s) | Ordering note |
|----------|----------|-------------|---------------|
| T1 | `config.Load()`, `storage.Open()` | T2, T4, T10 | T1 before all of them |
| T2 | `event.Store.Append()`, `event.Store.After()` | T3, T5, T7 | T2 before T3, T5, T7 |
| T3 | `temporal.Visible()` | T5, T7 | T3 before T5 |
| T4 | `tql.Parse()` | T5 | T4 before T5 |
| T10 | `vector.Index.Upsert()` / `.Search()` | T5 | T10 before T5 |
| T5 | `tql.Executor.Exec()` | T6 | T5 before T6 |
| T6 | `client.Client.Query()`, `cmd/temporaldb-server` | T7, T8, T9 | T6 before T7, T8, T9 |

## Notes

- **Deviation from the ADR's inline order, deliberate and surfaced rather than silent.** The ADR's
  Implementation section listed vector clients last (T10) while listing the TQL executor (T5) as
  the task that wires `SEARCH` to `internal/vector`. That is a contract cycle read literally: T5
  consumes what T10 produces. The DAG above puts T10 in wave 2, alongside the other T1-only
  dependants, and leaves the T-ids matching the ADR so cross-references still resolve. Task ids
  express identity; the wave table expresses order.
- **No spec stage.** The ADR's `Spec:` header is `None`, so every task's `Covers:` is
  `none — no spec` and `spec-verify` does not apply to this record.
- **The acceptance fences name Go packages that do not exist yet.** That is the intended state at
  authoring: each fence is red until its task lands, because the guard rejects `no test files` and
  `no tests to run` as well as `FAIL`. Do not soften a fence to make it pass.
- **Pre-flight for T9:** `go mod vendor` after adding `github.com/modelcontextprotocol/go-sdk`, per
  ADR-001 §D10. T9's Ordered Steps assume the vendor directory is committed.
- **Pre-flight for T10:** no Qdrant or TEI instance is reachable from this environment (ADR-001's
  Out of Scope, checked 2026-09-03). T10's fence is hermetic by construction (`httptest`), and the
  end-to-end path is a documented manual step, not a gate.
