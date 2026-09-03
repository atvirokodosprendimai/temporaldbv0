# Task ADR-001-T8: The CLI — REPL plus a scriptable one-shot mode

**Depends-on:** T6
**Covers:** none — no spec
**Estimated scope:** M (multi-file)
**Owner:** unassigned
**Produces:** `cmd/temporaldb-cli` with `-e <stmt>`, `-f <file>`, and an interactive REPL
**Consumes:** `client.Client.Query(ctx, tql string) (Result, error)` (T6)
**Data dependency:** hermetic — driven against an `httptest` server
**Proof map:** v1

## Goal

Give a human and a shell script the same access to TQL, over the client package, with output that is
readable in a terminal and parseable in a pipeline.

## Affected Files

| File | Change | Why |
|------|--------|-----|
| `cmd/temporaldb-cli/main.go` | add | flag parsing, mode dispatch, exit codes (§D11) |
| `cmd/temporaldb-cli/repl.go` | add | the interactive loop |
| `cmd/temporaldb-cli/render.go` | add | table rendering for a TTY, JSON when not a TTY or when `--json` |
| `cmd/temporaldb-cli/main_test.go` | add | the tests below |

The flag parser is what makes `-e` and `-f` reachable; a mode implemented but not registered is the
defect rung 2 checks for.

## Ordered Steps

1. [S1] Write and commit failing tests for `-e`, `-f`, exit codes, and render mode — red first
   (TDD red). [proof: acceptance]
2. [S2] Implement flag parsing and mode dispatch: `-e` one statement, `-f` a file of statements,
   neither means REPL.
3. [S3] Implement the REPL: prompt, multi-line statements terminated by `;`, history, and a `\q`
   style exit that also handles EOF.
4. [S4] Implement rendering: aligned table for a TTY, JSON otherwise or on `--json`, so piping works.
5. [S5] Make exit codes meaningful — non-zero on a parse or execution error, so a script can branch.
6. [S6] Run the Acceptance fence and record it with `adr-verify`. [proof: acceptance]

## Acceptance

```bash
set -o pipefail
CGO_ENABLED=0 go build ./... && CGO_ENABLED=0 go vet ./... || exit 1
CGO_ENABLED=0 go test ./cmd/temporaldb-cli/... 2>&1 | tee /tmp/acc-adr001-t8.out && ! grep -qE "no test files|no tests to run|^FAIL|^--- FAIL" /tmp/acc-adr001-t8.out || exit 1
CGO_ENABLED=0 go test ./... 2>&1 | tee /tmp/acc-adr001-t8-reg.out && ! grep -qE "^FAIL|^--- FAIL" /tmp/acc-adr001-t8-reg.out
```

## Tests

| Test name | File | Verifies | Covers | Steps |
|-----------|------|----------|--------|-------|
| `TestDashEExecutesOneStatement` | `cmd/temporaldb-cli/main_test.go` | `-e` runs and prints the result | — | S2 |
| `TestDashFExecutesEveryStatementInOrder` | `cmd/temporaldb-cli/main_test.go` | a file of statements runs in order and stops at the first error | — | S2 |
| `TestFlagsAreRegistered` | `cmd/temporaldb-cli/main_test.go` | `-e`, `-f` and `--json` are parsed — delete a flag registration and this goes red (rung-2 selection check) | — | S2 |
| `TestNonTTYOutputIsJSON` | `cmd/temporaldb-cli/main_test.go` | piping produces machine-readable output | — | S4 |
| `TestExitCodeIsNonZeroOnQueryError` | `cmd/temporaldb-cli/main_test.go` | a script can branch on failure | — | S5 |
| `TestREPLHandlesMultiLineAndEOF` | `cmd/temporaldb-cli/main_test.go` | statements span lines; EOF exits cleanly rather than hanging | — | S3 |

## Reachability

| Rung | How this task shows it |
|------|------------------------|
| 1 — exists | `TestDashEExecutesOneStatement` |
| 2 — something selects it | `TestFlagsAreRegistered` fails when a flag registration is deleted; that is the mutant |
| 3 — the caller can discover it | `-h` help text lists every mode; assert the help output names `-e`, `-f`, `--json` so an undocumented flag cannot be the only way in |
| 4 — it is used | nothing measures this yet |

## Mutation Log

Tool-written by `adr-verify <task.md> --mutant …`. Empty at authoring.

## Invariants

- The CLI speaks only through the `client` package. No direct database access, no second protocol.
- Output is JSON whenever stdout is not a terminal, so a pipeline never has to parse a table.

## Risks

- REPL tests can hang if EOF handling is wrong, turning a red test into a timeout; mitigated by
  driving the REPL from an `io.Reader` fixture rather than a real terminal.

## Stop Condition

Stop and ask if the "schematix" reference in the ADR's Context names a specific existing tool — its
ergonomics may need to be reconciled with §D11 before this task's UX is settled (ADR-001 Follow-ups).

## Out of Scope

- Server-side changes of any kind: the CLI is a client.
- Shell completion, config profiles, and coloured output.

## Verification Log

Tool-written by `adr-verify <this-file>`. Empty at authoring.
