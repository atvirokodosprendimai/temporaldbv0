# Task ADR-001-T6: HTTP server, the client package, and the server binary

**Depends-on:** T5
**Covers:** none — no spec
**Estimated scope:** L (cross-boundary)
**Owner:** unassigned
**Produces:** `client.Client.Query(ctx, tql string) (Result, error)`, `POST /query`, `GET /healthz`, `cmd/temporaldb-server`
**Consumes:** `tql.Executor.Exec(ctx, ast.Stmt) (Result, error)` (T5), `config.Load() (Config, error)` (T1)
**Data dependency:** hermetic — `httptest` only
**Proof map:** v1
**Rests-on:** `the /query route registration`, `the /healthz route registration`, `the 400-vs-500 error mapping`

## Goal

Expose the executor over one transport whose wire protocol is TQL text, and ship the reference client
every other binary in this record talks through.

## Affected Files

| File | Change | Why |
|------|--------|-----|
| `internal/server/server.go` | add | chi router, `POST /query`, `GET /healthz` (§D8) |
| `internal/server/server_test.go` | add | the tests below |
| `client/client.go` | add | the public client — `Client`, `Query`, typed wrappers (§D9) |
| `client/client_test.go` | add | client↔server round trip against `httptest` |
| `cmd/temporaldb-server/main.go` | add | the composition root: config → storage → executor → server |

`cmd/temporaldb-server/main.go` is the line that selects everything T1–T5 built; a route registered
but never mounted is exactly the defect the rung-2 check below exists to catch.

## Ordered Steps

1. [S1] Write and commit failing tests for `POST /query`, `/healthz`, and the client round trip —
   red first (TDD red). [proof: acceptance]
2. [S2] Implement the handler: read TQL text from the body, `tql.Parse`, `Executor.Exec`, marshal
   `Result` as JSON; map a parse error to 400 and an execution error to 500, each with the message.
3. [S3] Implement `client.Client.Query` against exactly that contract, plus the typed wrappers §D9
   names — no second protocol.
4. [S4] Write `cmd/temporaldb-server/main.go`: load config, open storage, build the executor, mount
   the router, serve, and shut down cleanly on SIGTERM.
5. [S5] Run the Acceptance fence and record it with `adr-verify`. [proof: acceptance]

## Acceptance

```bash
set -o pipefail
CGO_ENABLED=0 go build ./... && CGO_ENABLED=0 go vet ./... || exit 1
CGO_ENABLED=0 go test ./internal/server/... ./client/... 2>&1 | tee /tmp/acc-adr001-t6.out && ! grep -qE "no test files|no tests to run|^FAIL|^--- FAIL" /tmp/acc-adr001-t6.out || exit 1
CGO_ENABLED=0 go test ./... 2>&1 | tee /tmp/acc-adr001-t6-reg.out && ! grep -qE "^FAIL|^--- FAIL" /tmp/acc-adr001-t6-reg.out
```

## Tests

| Test name | File | Verifies | Covers | Steps |
|-----------|------|----------|--------|-------|
| `TestPostQueryExecutesTQL` | `internal/server/server_test.go` | a real statement round-trips through the handler | — | S2 |
| `TestPostQueryParseErrorIs400` | `internal/server/server_test.go` | a client mistake is 400 with the parser's message, not 500 | — | S2 |
| `TestHealthzReports200` | `internal/server/server_test.go` | liveness works with an empty database | — | S2 |
| `TestClientQueryRoundTripsAgainstServer` | `client/client_test.go` | the client speaks the server's actual contract, over `httptest` | — | S3 |
| `TestRouterMountsQueryAndHealthz` | `internal/server/server_test.go` | both routes are registered — delete a `r.Post`/`r.Get` line and this goes red (the rung-2 selection check) | — | S2, S4 |

## Reachability

| Rung | How this task shows it |
|------|------------------------|
| 1 — exists | `TestPostQueryExecutesTQL` |
| 2 — something selects it | `TestRouterMountsQueryAndHealthz` fails when the route registration is deleted; record it as the mutant |
| 3 — the caller can discover it | `POST /query` and `GET /healthz` are the declared HTTP interface, asserted by path and method in the test rather than only exercised through the client |
| 4 — it is used | nothing measures this yet — no request logging or metrics in v1 |

## Prescribed Mutant

Chosen at authoring, because `adr-verify --mutant` refuses an absent, non-unique or comment-only
mutant but **cannot judge whether the mutant was well chosen**. ⚠ `--from` must match exactly one
place, and this was written before the code existed: adapt the string, keep the mechanism.

- **Break:** the route registration in `internal/server/server.go` — e.g.
  `--from 'r.Post("/query"' --to 'r.Post("/query-disabled"'`.
- **Must go red:** `TestRouterMountsQueryAndHealthz`.
- **Why this one:** a handler that exists, is unit-tested, and is never mounted is this pipeline's
  most common shipped defect — and a handler test cannot catch it by construction, because it calls
  the handler directly, which is precisely what the router is failing to do.

One entry per mechanism in `Rests-on:`; use `--covers` to name which.

## Mutation Log

Tool-written by `adr-verify <task.md> --mutant …`. Empty at authoring.
- 2026-09-03 · 5e946ee* · mutant killed · exit 1 · `internal/server/server.go` · a handler that exists and is unit-tested but is never mounted on the router is the most common shipped defect; TestRouterMountsQueryAndHealthz must go red when the route registration is broken · acceptance-sha256:4e4ca78e940c90692376cfe00ab078affa5ab36c8f4a201d3f13f92da69c4de3 · covers:the /query route registration

## Invariants

- One transport, one body format: TQL text in, JSON `Result` out. No second RPC surface.
- The client contains no query logic — it is transport plus marshalling only.
- No authentication is added here; auth is explicitly deferred (`docs/adr/BACKLOG.md` §1) and the
  server is documented as single-trusted-operator.

## Risks

- Adding a convenience endpoint here quietly creates a second protocol to document and version;
  the invariant above is the guard, and `GET /backup/status` was already removed from §D8 for this
  reason.

## Stop Condition

Stop and ask if the server needs to be reachable from an untrusted network — that pulls
`docs/adr/BACKLOG.md` §1 forward and is an ADR-level decision.

## Out of Scope

- Auth/authz (deferred: `docs/adr/BACKLOG.md` §1).
- Backup wiring: T7 mounts into this same binary.
- The REPL: T8.

## Verification Log

Tool-written by `adr-verify <this-file>`. Empty at authoring.
- 2026-09-03 · 5e946ee* · exit 1 · `set -o pipefail …` · acceptance-sha256:4e4ca78e940c90692376cfe00ab078affa5ab36c8f4a201d3f13f92da69c4de3 · ms:3683
  ```
  --- last 10 line(s) of stdout (of 12 after folding 13 raw)
  ?   	oneshottest/cmd/temporaldb-server	[no test files]
  ok  	oneshottest/internal/config	(cached)
  ok  	oneshottest/internal/event	(cached)
  ok  	oneshottest/internal/graph	(cached)
  ok  	oneshottest/internal/server	(cached)
  ok  	oneshottest/internal/storage	(cached)
  ok  	oneshottest/internal/temporal	(cached)
  ok  	oneshottest/internal/tql	(cached)
  ok  	oneshottest/internal/tql/ast	(cached)
  ok  	oneshottest/internal/vector	(cached)
  --- last 1 line(s) of stderr
  [adr-verify] the acceptance command exited 0 but scored NO tests. A filter that matches nothing is not a passing gate — write the tests the Tests table names, or fix the filter so it selects them.
  ```
- 2026-09-03 · 5e946ee* · exit 0 · `set -o pipefail …` · acceptance-sha256:4e4ca78e940c90692376cfe00ab078affa5ab36c8f4a201d3f13f92da69c4de3 · ms:4329
- 2026-09-03 · 5e946ee* · exit 0 · `set -o pipefail …` · acceptance-sha256:4e4ca78e940c90692376cfe00ab078affa5ab36c8f4a201d3f13f92da69c4de3 · ms:4446
