# Task ADR-001-T9: MCP server over the official Go SDK, vendored

**Depends-on:** T6
**Covers:** none — no spec
**Estimated scope:** M (multi-file)
**Owner:** unassigned
**Produces:** `cmd/temporaldb-mcp` exposing `tql_query`, `tql_get`, `tql_put`, `tql_history`, `tql_search`
**Consumes:** `client.Client.Query(ctx, tql string) (Result, error)` (T6)
**Data dependency:** hermetic — tools are exercised against an `httptest` server
**Proof map:** v1
**Rests-on:** `the five tool registrations`, `the tool input schemas matching the handlers`

## Goal

Expose the same TQL contract to MCP clients as a thin wrapper over the client package, using the
official `modelcontextprotocol/go-sdk` rather than a third-party binding (§D10).

## Affected Files

| File | Change | Why |
|------|--------|-----|
| `go.mod` | edit | add `github.com/modelcontextprotocol/go-sdk` |
| `vendor/` | add | `go mod vendor`, per §D10 |
| `cmd/temporaldb-mcp/main.go` | add | server construction and the tool registrations |
| `cmd/temporaldb-mcp/tools.go` | add | the five tool handlers over `client.Client` |
| `cmd/temporaldb-mcp/tools_test.go` | add | the tests below |

Tool *registration* is the selection site: a handler written and never registered is invisible to
every MCP client while every handler test passes.

## Ordered Steps

1. [S1] Write and commit failing tests for each tool handler and for the registration list — red
   first (TDD red). [proof: acceptance]
2. [S2] Add the SDK and run `go mod vendor`; commit the vendor directory. [proof: acceptance]
3. [S3] Read the SDK's own godoc for the current server/tool API rather than assuming a shape — §D10
   explicitly defers the exact call signatures to implementation time. [proof: human: the implementer reads the vendored SDK's godoc and confirms each handler signature against the version actually pinned in go.mod]
4. [S4] Implement the five handlers as thin translations to `client.Client.Query`; no query logic
   lives here.
5. [S5] Register all five tools with input schemas that actually describe their arguments, so a
   caller reading the schema can call them correctly.
6. [S6] Run the Acceptance fence and record it with `adr-verify`. [proof: acceptance]

## Acceptance

```bash
set -o pipefail
CGO_ENABLED=0 go build ./... && CGO_ENABLED=0 go vet ./... || exit 1
CGO_ENABLED=0 go test ./cmd/temporaldb-mcp/... 2>&1 | tee /tmp/acc-adr001-t9.out && ! grep -qE "no test files|no tests to run|^FAIL|^--- FAIL" /tmp/acc-adr001-t9.out || exit 1
CGO_ENABLED=0 go test ./... 2>&1 | tee /tmp/acc-adr001-t9-reg.out && ! grep -qE "^FAIL|^--- FAIL" /tmp/acc-adr001-t9-reg.out
```

## Tests

| Test name | File | Verifies | Covers | Steps |
|-----------|------|----------|--------|-------|
| `TestAllFiveToolsAreRegistered` | `cmd/temporaldb-mcp/tools_test.go` | the registration list holds exactly the five §D10 names — delete one and this goes red (rung-2 selection check) | — | S5 |
| `TestToolSchemasDeclareEveryArgumentTheHandlerReads` | `cmd/temporaldb-mcp/tools_test.go` | no argument is honoured but undocumented — the rung-3 check | — | S5 |
| `TestTqlQueryDelegatesToClient` | `cmd/temporaldb-mcp/tools_test.go` | the handler is a translation, not a second implementation | — | S4 |
| `TestTqlGetPutHistorySearchRoundTrip` | `cmd/temporaldb-mcp/tools_test.go` | each typed tool produces the statement it claims to | — | S4 |
| `TestToolErrorIsReturnedNotPanicked` | `cmd/temporaldb-mcp/tools_test.go` | a server error becomes an MCP error result | — | S4 |

## Reachability

| Rung | How this task shows it |
|------|------------------------|
| 1 — exists | `TestTqlQueryDelegatesToClient` |
| 2 — something selects it | `TestAllFiveToolsAreRegistered` fails when a registration line is removed; that is the mutant |
| 3 — the caller can discover it | `TestToolSchemasDeclareEveryArgumentTheHandlerReads` — a tool argument the handler honours but the schema never advertises is invisible to every caller who reads the schema |
| 4 — it is used | nothing measures this yet |

## Prescribed Mutant

Chosen at authoring, because `adr-verify --mutant` refuses an absent, non-unique or comment-only
mutant but **cannot judge whether the mutant was well chosen**. ⚠ `--from` must match exactly one
place, and this was written before the code existed: adapt the string, keep the mechanism.

- **Break:** drop ONE of the five tool registrations in `cmd/temporaldb-mcp/main.go` — e.g. the
  `tql_history` registration.
- **Must go red:** `TestAllFiveToolsAreRegistered`.
- **Why this one:** an unregistered tool is invisible to every MCP client while its own handler test
  passes. Rung 3 has a second mechanism worth its own entry — break a schema field the handler reads
  and check `TestToolSchemasDeclareEveryArgumentTheHandlerReads` goes red.

One entry per mechanism in `Rests-on:`; use `--covers` to name which.

## Mutation Log

Tool-written by `adr-verify <task.md> --mutant …`. Empty at authoring.
- 2026-09-03 · 957e8ad* · mutant survived · exit 0 · `cmd/temporaldb-mcp/tools.go` · drop the tql_history registration: the handler exists but no MCP client can reach it by its §D10 name · acceptance-sha256:8a3a0e3b5c58df40a030860fd72e2d44236682743acd3fbe0b2ea49849540c26 · covers:the five tool registrations
  ```
  the fence passed with the mechanism broken; it may not materialize, compile, load, or assert on the changed path
  ```
- 2026-09-03 · 3ef6787* · mutant killed · exit 1 · `cmd/temporaldb-mcp/tools.go` · drop the tql_history registration under its §D10 name: the handler exists but no MCP client can reach it as tql_history · acceptance-sha256:8a3a0e3b5c58df40a030860fd72e2d44236682743acd3fbe0b2ea49849540c26

## Invariants

- The MCP binary contains no query logic; it translates arguments into TQL and calls the client.
- The vendored SDK is committed, so the build does not depend on network access at build time.

## Risks

- The SDK's Go API may differ from what §D10 assumes. Mitigated by S3: read its godoc at
  implementation time and adjust the handlers, not the decision.

## Stop Condition

Stop and ask if the official SDK cannot express one of the five tools' schemas — that is grounds to
revisit §D10's rejection of `mark3labs/mcp-go`, which this task may not decide alone.

## Out of Scope

- New TQL capability: an MCP tool that cannot be expressed as TQL is a §D4 change, not an MCP change.
- Transport beyond what the SDK's default server provides.

## Verification Log

Tool-written by `adr-verify <this-file>`. Empty at authoring.
- 2026-09-03 · 957e8ad* · exit 1 · `set -o pipefail …` · acceptance-sha256:8a3a0e3b5c58df40a030860fd72e2d44236682743acd3fbe0b2ea49849540c26 · ms:3998
  ```
  --- last 10 line(s) of stdout (of 15 after folding 15 raw)
  ok  	oneshottest/internal/backup	(cached)
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
- 2026-09-03 · 957e8ad* · exit 0 · `set -o pipefail …` · acceptance-sha256:8a3a0e3b5c58df40a030860fd72e2d44236682743acd3fbe0b2ea49849540c26 · ms:7109
- 2026-09-03 · 957e8ad* · exit 0 · `set -o pipefail …` · acceptance-sha256:8a3a0e3b5c58df40a030860fd72e2d44236682743acd3fbe0b2ea49849540c26 · ms:6769
- 2026-09-03 · 3ef6787* · exit 1 · `set -o pipefail …` · acceptance-sha256:8a3a0e3b5c58df40a030860fd72e2d44236682743acd3fbe0b2ea49849540c26 · ms:4117
  ```
  --- last 10 line(s) of stdout (of 15 after folding 15 raw)
  ok  	oneshottest/internal/backup	(cached)
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
- 2026-09-03 · 3ef6787* · exit 0 · `set -o pipefail …` · acceptance-sha256:8a3a0e3b5c58df40a030860fd72e2d44236682743acd3fbe0b2ea49849540c26 · ms:6005
- 2026-09-03 · 3ef6787* · exit 0 · `set -o pipefail …` · acceptance-sha256:8a3a0e3b5c58df40a030860fd72e2d44236682743acd3fbe0b2ea49849540c26 · ms:6209
