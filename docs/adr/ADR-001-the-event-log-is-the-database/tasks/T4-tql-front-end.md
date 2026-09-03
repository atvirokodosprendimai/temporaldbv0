# Task ADR-001-T4: TQL lexer, parser, and AST for the full v1 grammar

**Depends-on:** T1
**Covers:** none — no spec
**Estimated scope:** M (multi-file)
**Owner:** unassigned
**Produces:** `tql.Parse(src string) (ast.Stmt, error)`, the `ast` node types
**Consumes:** none
**Data dependency:** hermetic
**Proof map:** v1

## Goal

Turn TQL text into a typed AST for the whole v1 grammar in ADR-001 §D4, with errors a human can act
on — nothing in this task touches the database.

## Affected Files

| File | Change | Why |
|------|--------|-----|
| `internal/tql/lexer.go` | add | tokeniser |
| `internal/tql/parser.go` | add | recursive-descent parser over the §D4 grammar |
| `internal/tql/ast/ast.go` | add | node types for `PUT`, `GET`, `FIND`, `HISTORY`, `SEARCH`, `RELATE`, `PURGE` |
| `internal/tql/parser_test.go` | add | the tests below |

## Ordered Steps

1. [S1] Write and commit failing parse tests — one per verb in §D4, plus the error cases — red
   first (TDD red). [proof: acceptance]
2. [S2] Implement the lexer: identifiers, strings, numbers, timestamps, punctuation, keywords.
3. [S3] Define the AST node types, one per verb, with the clauses §D4 names (`AS OF`, `VALID AT`,
   `WHERE`, `LIMIT`).
4. [S4] Implement the parser, including the v1 restriction that `WHERE` accepts only `AND`-conjoined
   comparisons (`OR`/subqueries/joins are deferred — `docs/adr/BACKLOG.md` §4).
5. [S5] Make parse errors carry the offending position and token, so a CLI user can see where.
6. [S6] Run the Acceptance fence and record it with `adr-verify`. [proof: acceptance]

## Acceptance

```bash
set -o pipefail
CGO_ENABLED=0 go build ./... && CGO_ENABLED=0 go vet ./... || exit 1
CGO_ENABLED=0 go test ./internal/tql/... 2>&1 | tee /tmp/acc-adr001-t4.out && ! grep -qE "no test files|no tests to run|^FAIL|^--- FAIL" /tmp/acc-adr001-t4.out || exit 1
CGO_ENABLED=0 go test ./... 2>&1 | tee /tmp/acc-adr001-t4-reg.out && ! grep -qE "^FAIL|^--- FAIL" /tmp/acc-adr001-t4-reg.out
```

## Tests

| Test name | File | Verifies | Covers | Steps |
|-----------|------|----------|--------|-------|
| `TestParseEachVerbProducesItsNode` | `internal/tql/parser_test.go` | one table case per §D4 verb; the union is the grammar | — | S3, S4 |
| `TestParseAsOfAndValidAtClauses` | `internal/tql/parser_test.go` | both temporal clauses parse, independently and together | — | S4 |
| `TestParseRejectsOrInWhere` | `internal/tql/parser_test.go` | the v1 boundary is enforced by the parser, with a named error — not silently accepted | — | S4 |
| `TestParseErrorCarriesPosition` | `internal/tql/parser_test.go` | the error names line/column and the offending token | — | S5 |
| `TestLexerHandlesQuotedStringsAndTimestamps` | `internal/tql/lexer.go` tests in `parser_test.go` | escapes and RFC3339 literals tokenise | — | S2 |

## Reachability

| Rung | How this task shows it |
|------|------------------------|
| 1 — exists | `TestParseEachVerbProducesItsNode` |
| 2 — something selects it | Nothing selects `Parse` in this task; T5's executor and T6's `POST /query` are the call sites and each records its own selection mutant |
| 3 — the caller can discover it | The grammar in ADR-001 §D4 is the declared interface; `TestParseRejectsOrInWhere` pins the documented boundary |
| 4 — it is used | nothing measures this yet |

## Mutation Log

Tool-written by `adr-verify <task.md> --mutant …`. Empty at authoring.

## Invariants

- The parser is pure: no database handle, no config, no I/O. It is testable with a string alone.
- Every grammar restriction is an explicit named error, never a silent acceptance or a panic.

## Risks

- A grammar written here and a grammar documented in §D4 can drift. Mitigated by
  `TestParseEachVerbProducesItsNode` being a table whose cases are copied from §D4's own examples.

## Stop Condition

Stop and ask if a verb in §D4 turns out to be unparseable without `OR` — that is a §D4 amendment.

## Out of Scope

- Executing anything: T5.
- `OR`, subqueries, joins (deferred: `docs/adr/BACKLOG.md` §4).

## Verification Log

Tool-written by `adr-verify <this-file>`. Empty at authoring.
