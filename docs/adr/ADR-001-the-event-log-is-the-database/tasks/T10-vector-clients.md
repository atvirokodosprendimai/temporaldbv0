# Task ADR-001-T10: Qdrant and TEI clients, behind a config gate

**Depends-on:** T1
**Covers:** none — no spec
**Estimated scope:** M (multi-file)
**Owner:** unassigned
**Produces:** `vector.Index.Upsert(ctx, collection, key, text) error`, `vector.Index.Search(ctx, collection, text, k) ([]Hit, error)`, `vector.New(cfg) (Index, bool)`
**Consumes:** `config.Load() (Config, error)` (T1)
**Data dependency:** needs a live Qdrant + TEI instance for end-to-end confirmation, which this environment does not have (ADR-001 Out of Scope, checked 2026-09-03). The Acceptance fence is therefore hermetic `httptest` against the documented REST contracts, and the live path is a documented manual step — not a gate.
**Proof map:** v1
**Rests-on:** `the both-URLs-required config gate`, `the HTTP client timeout`

## Goal

Provide narrow, hand-rolled clients for TEI (embedding) and Qdrant (vector storage and search), and
a constructor that reports "not configured" cleanly so every other feature works without them.

## Affected Files

| File | Change | Why |
|------|--------|-----|
| `internal/vector/vector.go` | add | the `Index` interface, `New`, and the not-configured path (§D6) |
| `internal/vector/tei.go` | add | embedding client against TEI's documented REST contract |
| `internal/vector/qdrant.go` | add | upsert/search client against Qdrant's documented REST contract |
| `internal/vector/vector_test.go` | add | the tests below, driven by `httptest` |
| `internal/config/config.go` | edit | `QDRANT_URL`, `TEI_URL`, collection naming — the keys that gate the feature |

## Ordered Steps

1. [S1] Write and commit failing tests for `New` returning `false` when unconfigured, and for the two
   REST clients against `httptest` — red first (TDD red). [proof: acceptance]
2. [S2] Define `Index` at the consumer's shape: exactly the two methods T5's executor needs, nothing
   speculative. [proof: acceptance]
3. [S3] Implement the TEI client: text in, vector out, with a request timeout and a typed error.
4. [S4] Implement the Qdrant client: upsert a point, search top-k, with the same timeout and error
   discipline.
5. [S5] Implement `New(cfg) (Index, bool)`: `false` when either URL is unset, so the caller's
   "feature absent" path is a value and not an error string comparison.
6. [S6] Write the manual end-to-end procedure into this task's Notes-equivalent (Risks, below) —
   what to run against a real Qdrant/TEI, and what output counts as a pass — so the untested leg is
   documented rather than implied. [proof: human: a person runs it against a live pair and reads the returned neighbours]
7. [S7] Run the Acceptance fence and record it with `adr-verify`. [proof: acceptance]

## Acceptance

```bash
set -o pipefail
CGO_ENABLED=0 go build ./... && CGO_ENABLED=0 go vet ./... || exit 1
CGO_ENABLED=0 go test ./internal/vector/... 2>&1 | tee /tmp/acc-adr001-t10.out && ! grep -qE "no test files|no tests to run|^FAIL|^--- FAIL" /tmp/acc-adr001-t10.out || exit 1
CGO_ENABLED=0 go test ./... 2>&1 | tee /tmp/acc-adr001-t10-reg.out && ! grep -qE "^FAIL|^--- FAIL" /tmp/acc-adr001-t10-reg.out
```

This fence proves the clients speak the documented protocol shape. It does NOT prove they work
against real Qdrant/TEI builds — that is the manual step in S6, and no exit code in this repository
covers it.

## Tests

| Test name | File | Verifies | Covers | Steps |
|-----------|------|----------|--------|-------|
| `TestNewReturnsFalseWhenURLsUnset` | `internal/vector/vector_test.go` | unconfigured is a clean absence, not an error path | — | S5 |
| `TestNewReturnsIndexWhenBothURLsSet` | `internal/vector/vector_test.go` | the gate needs both halves; one alone is still absent | — | S5 |
| `TestTEIEmbedPostsDocumentedRequestShape` | `internal/vector/vector_test.go` | method, path and body match TEI's documented contract | — | S3 |
| `TestQdrantUpsertPostsDocumentedPointShape` | `internal/vector/vector_test.go` | the point payload carries id, vector and payload as documented | — | S4 |
| `TestQdrantSearchParsesHitsAndScores` | `internal/vector/vector_test.go` | a documented response decodes to `[]Hit` in score order | — | S4 |
| `TestClientTimeoutIsSetAndHonoured` | `internal/vector/vector_test.go` | a hung service fails fast instead of blocking a query | — | S3, S4 |

## Reachability

| Rung | How this task shows it |
|------|------------------------|
| 1 — exists | `TestQdrantSearchParsesHitsAndScores` |
| 2 — something selects it | Nothing in this task selects `Index` — T5's `SEARCH` dispatch is the only call site, and `TestExecSearchDispatchesToVectorIndex` (T5) is the mutation that proves it. Stated here rather than claimed, because T10 landing alone leaves this package unreached. |
| 3 — the caller can discover it | `QDRANT_URL` and `TEI_URL` are the declared interface; `TestNewReturnsFalseWhenURLsUnset` pins their names |
| 4 — it is used | nothing measures this yet, and nothing calls `Upsert` automatically either — automatic embedding on `PUT` is deferred (`docs/adr/BACKLOG.md` §5) |

## Prescribed Mutant

Chosen at authoring, because `adr-verify --mutant` refuses an absent, non-unique or comment-only
mutant but **cannot judge whether the mutant was well chosen**. ⚠ `--from` must match exactly one
place, and this was written before the code existed: adapt the string, keep the mechanism.

- **Break:** the config gate in `internal/vector/vector.go` — make `New` report configured when only
  one of `QDRANT_URL` / `TEI_URL` is set.
- **Must go red:** `TestNewReturnsFalseWhenURLsUnset`.
- **Why this one:** the gate is what makes vector search *additive*. A gate that reports configured
  when it is not turns an optional dependency into a runtime failure on every `SEARCH`, which is the
  precise opposite of §D6's "feature absent, not server broken".

One entry per mechanism in `Rests-on:`; use `--covers` to name which.

## Mutation Log

Tool-written by `adr-verify <task.md> --mutant …`. Empty at authoring.
- 2026-09-03 · 3e2cc29* · mutant killed · exit 1 · `internal/vector/vector.go` · a gate that reports configured with one URL set turns the optional dependency into a runtime failure on every SEARCH · acceptance-sha256:343422fa60d3406547185d238b4278459d4bce9c28042f29d68a587daad053d5 · covers:the both-URLs-required config gate

## Invariants

- With either URL unset, every non-`SEARCH` feature behaves identically. Vector search is additive
  and never the source of truth (§D6).
- The clients are hand-rolled against documented REST contracts; no vendored vector SDK.

## Risks

- Unverified against live services. **Manual end-to-end procedure (S6):** run a Qdrant and a TEI
  container, set `QDRANT_URL`/`TEI_URL`, `PUT` three documents with distinct topics, call
  `vector.Index.Upsert` for each, then `SEARCH` for a phrase matching one — a pass is that document
  returned first with a score above the others. Record who ran it and what they saw; a
  `--human` sign-off is the only evidence form available for this leg.
- A documented REST contract can drift from the running build; the hermetic tests would not notice.

## Stop Condition

Stop and ask before making vector search load-bearing for any non-`SEARCH` verb: that contradicts
§D6 and turns an optional dependency into a required one.

## Out of Scope

- Automatic embedding on `PUT` (deferred: `docs/adr/BACKLOG.md` §5).
- `SEARCH ... AS OF` / temporally-versioned embeddings (deferred: `docs/adr/BACKLOG.md` §6).
- End-to-end testing against live instances (permanent: boundary — no Qdrant or TEI instance is
  reachable from this environment, per ADR-001's Out of Scope).

## Verification Log

Tool-written by `adr-verify <this-file>`. Empty at authoring.
- 2026-09-03 · 3e2cc29* · exit 1 · `set -o pipefail …` · acceptance-sha256:343422fa60d3406547185d238b4278459d4bce9c28042f29d68a587daad053d5 · ms:2554
  ```
  --- last 7 line(s) of stdout
  ok  	oneshottest/internal/vector	(cached)
  ok  	oneshottest/internal/config	(cached)
  ok  	oneshottest/internal/event	(cached)
  ok  	oneshottest/internal/storage	(cached)
  ok  	oneshottest/internal/tql	(cached)
  ?   	oneshottest/internal/tql/ast	[no test files]
  ok  	oneshottest/internal/vector	(cached)
  --- last 1 line(s) of stderr
  [adr-verify] the acceptance command exited 0 but scored NO tests. A filter that matches nothing is not a passing gate — write the tests the Tests table names, or fix the filter so it selects them.
  ```
- 2026-09-03 · 3e2cc29* · exit 0 · `set -o pipefail …` · acceptance-sha256:343422fa60d3406547185d238b4278459d4bce9c28042f29d68a587daad053d5 · ms:3330
- 2026-09-03 · 3e2cc29* · exit 0 · `set -o pipefail …` · acceptance-sha256:343422fa60d3406547185d238b4278459d4bce9c28042f29d68a587daad053d5 · ms:3235
