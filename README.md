# TemporalDB

A temporal database where the event log is the database; everything else is a projection.

**Status:** ADR-001 Accepted. Implementation complete (T1–T9 verified). T3 design refinement in progress.

## What is TemporalDB?

TemporalDB stores all data as an append-only event log (SQLite, no ORM). Current state lives in a derived `live` projection updated in the same transaction as each event write. Historical queries and point-in-time snapshots read from the log; live queries read from the projection.

- **Single writer per aggregate** (concurrency model: SQLite WAL, one writer at a time)
- **Bitemporal**: every event carries `valid_from` and implicit `valid_to` (= next version's `valid_from`)
- **Queryable**: TQL (Temporal Query Language) — GET/PUT/DELETE/FIND/SEARCH/RELATE/UNRELATE with temporal filters
- **Rewindable**: `AsOf(timestamp)` reads state at any past moment
- **Searchable**: optional vector clients (Qdrant + TEI embeddings)
- **Backed up**: streaming event log shipper + snapshot restore

## Quick Start

### Prerequisites

```bash
go 1.26
CGO_ENABLED=0  # no C dependencies
```

### Build

```bash
CGO_ENABLED=0 go build ./...
CGO_ENABLED=0 go test ./...
```

### Run the Server

```bash
export TEMPORALDB_DATA_DIR=/tmp/temporaldb
export TEMPORALDB_PORT=8080
./cmd/temporaldb-server/temporaldb-server
# Server listens on http://localhost:8080
```

### Run the CLI

```bash
./cmd/temporaldb-cli/temporaldb-cli
# Drops into interactive REPL

# Or one-shot:
./cmd/temporaldb-cli/temporaldb-cli -e "GET myCollection/myKey"
./cmd/temporaldb-cli/temporaldb-cli -f queries.tql
```

### Run the MCP Server

```bash
export TEMPORALDB_MCP_PORT=3000
./cmd/temporaldb-mcp/temporaldb-mcp
# Exposes 5 tools over the Model Context Protocol
```

## Architecture

### Packages

| Package | Purpose | Key Types |
|---------|---------|-----------|
| `internal/config` | Load config from env + optional `.env` | `Config` |
| `internal/storage` | SQLite wrapper, WAL mode, migrations | `Open(ctx, Config) (*sql.DB, error)` |
| `internal/event` | Append-only log + live projection | `Store.Append()`, `Store.After()`, `Store.History()` |
| `internal/temporal` | Bitemporal visibility logic | `Visible(events, asOf, validAt) (Event, bool)` |
| `internal/tql` | TQL parser, AST, lexer | `Parse(string) (ast.Stmt, error)` |
| `internal/graph` | Edge-based relations (knowledge graph) | Graph traversal helpers |
| `internal/server` | HTTP transport | `POST /query`, `GET /healthz` |
| `client` | Go client library | `Client.Query(ctx, tql string) (Result, error)` |
| `internal/backup` | Streaming shipper + restore | `Sink.WriteEvents()`, `Restore(ctx, sink, db)` |
| `internal/vector` | Qdrant + TEI clients (optional) | `Index.Upsert()`, `Index.Search()` |

### Data Model

Every event has:

```go
type Event struct {
    Seq        int64     // append-only log sequence
    Collection string    // user-defined group (e.g., "users", "orders")
    Key        string    // unique identifier within collection
    Op         string    // "put", "delete", "relate", "unrelate"
    Value      []byte    // serialized state (JSON recommended)
    ValidFrom  time.Time // when this version became valid
    // ValidTo is implicit: next version's ValidFrom
    // (half-open interval: [ValidFrom, NextValidFrom))
    TxTime     time.Time // transaction time (approximately insertion order)
    Meta       string    // optional metadata
}
```

Live projection by collection:

```sql
CREATE TABLE live (
    collection TEXT,
    key TEXT,
    seq INTEGER,  -- which event row is current
    value BLOB,
    valid_from TIMESTAMP,
    PRIMARY KEY (collection, key)
)
```

## API Reference

### Config

```go
import "oneshottest/internal/config"

cfg, err := config.Load()  // from env, falls back to .env
// Returns Config with fields: DataDir, Port, QdrantURL, TEIURL, etc.
```

**Environment variables:**

```bash
TEMPORALDB_DATA_DIR      # SQLite file directory (default: current dir)
TEMPORALDB_PORT          # HTTP server port (default: 8080)
TEMPORALDB_QDRANT_URL    # Qdrant endpoint (optional, e.g., http://localhost:6334)
TEMPORALDB_TEI_URL       # TEI service endpoint (optional, e.g., http://localhost:8080)
```

### Storage

```go
import "oneshottest/internal/storage"

db, err := storage.Open(ctx, cfg)
defer db.Close()
// Opens SQLite with:
// - WAL mode enabled
// - PRAGMA busy_timeout = 5000
// - PRAGMA foreign_keys = ON
// - SetMaxOpenConns(1) for single-writer model
// - Runs goose migrations from internal/storage/migrations/
```

### Event Store

```go
import "oneshottest/internal/event"

store := event.NewStore(db)

// Append an event
seq, err := store.Append(ctx, event.Event{
    Collection: "users",
    Key:        "alice",
    Op:         "put",
    Value:      []byte(`{"name":"Alice","age":30}`),
    ValidFrom:  time.Now(),
})
// Returns sequence number in append-only log

// Get all events after a sequence number (for backup shipping)
events, err := store.After(ctx, seq)

// Get full history of one collection/key
history, err := store.History(ctx, "users", "alice")

// Replay events up to a timestamp (for AsOf queries)
pastState, err := store.ReplayAsOf(ctx, "users", "alice", asOfTime)
```

### Temporal Queries

```go
import "oneshottest/internal/temporal"

// Which version is visible at a given query time?
visible, found := temporal.Visible(events, asOfTime, validAtTime)
if found {
    // visible is the Event whose [ValidFrom, NextValidFrom) contains validAtTime
}

// Parse timestamps (RFC3339 + relative like "now", "1h ago", etc.)
t, err := temporal.Parse("2026-09-04T12:00:00Z")
t, err := temporal.Parse("1h ago")
```

### TQL Query Language

```go
import "oneshottest/internal/tql"

stmt, err := tql.Parse("GET users/alice")
stmt, err := tql.Parse("FIND users WHERE age > 25 LIMIT 10 ASOF 2026-09-01T00:00:00Z")
stmt, err := tql.Parse("SEARCH users WITH 'machine learning' K 5")
stmt, err := tql.Parse("RELATE documents/doc1 -[cites]-> documents/doc2")
```

**Grammar (v1):**

```
GET <collection>/<key> [ASOF <time>]
PUT <collection>/<key> <json-value>
DELETE <collection>/<key>

FIND <collection>
  [WHERE <comparison> {AND <comparison>}]
  [ASOF <time>]
  [ORDER BY <field> {ASC|DESC}]
  [LIMIT <n>]

SEARCH <collection> WITH '<text-query>' K <n> [ASOF <time>]

RELATE <collection>/<key> -[<edge-type>]-{<direction>} <collection>/<key>
UNRELATE <collection>/<key> -[<edge-type>]-> <collection>/<key>
```

### HTTP Server

```go
import "oneshottest/internal/server"

// Runs on configured port (default 8080)

// POST /query — execute TQL
// Request body: plain TQL text (newline-separated for batch)
// Response: JSON result

// GET /healthz — liveness check
// Response: 200 OK
```

**Example:**

```bash
curl -X POST http://localhost:8080/query \
  -d "GET users/alice"

curl http://localhost:8080/healthz
```

### Go Client

```go
import "oneshottest/client"

c := client.New("http://localhost:8080")

result, err := c.Query(ctx, "GET users/alice")
// result.Type: "get" | "find" | "search" | "relate" | "error"
// result.Rows: []map[string]any
// result.Error: error message if Type=="error"
```

### CLI REPL

Interactive prompt:

```
temporaldb> GET users/alice
[{"key":"alice","value":"{...}","valid_from":"2026-09-03T10:00:00Z"}]

temporaldb> FIND users WHERE age > 25 LIMIT 5
[{...}, {...}, ...]

temporaldb> .help
.help       — show commands
.exit       — quit
.history    — show command history
```

### MCP Server

Exposes 5 tools:

- **`tql_query`** — execute a TQL statement
- **`tql_get`** — GET shorthand (collection, key, optional asOf)
- **`tql_put`** — PUT shorthand (collection, key, value)
- **`tql_history`** — fetch full history of a collection/key
- **`tql_search`** — SEARCH with optional K limit

## Configuration

Create `.env` file in the working directory (optional; env vars take precedence):

```bash
TEMPORALDB_DATA_DIR=/var/lib/temporaldb
TEMPORALDB_PORT=8080
TEMPORALDB_QDRANT_URL=http://localhost:6334
TEMPORALDB_TEI_URL=http://localhost:8080
```

## Design Decisions

See `docs/adr/ADR-001-the-event-log-is-the-database.md` for:

- Why SQLite + no ORM (§D1)
- Why append-only log + projection (§D2)
- Bitemporal interval model (§D3)
- TQL grammar decisions (§D4)
- Vector search optionality (§D6)
- Backup strategy (§D7)
- Single-writer concurrency model (§D13)

## Limitations (v1)

- **No transactions across collections** — each event is atomic within its collection
- **Vector search requires external services** (Qdrant, TEI) — falls back to no-op if unconfigured
- **No authentication** — designed for trusted internal use
- **No replication** — single SQLite file (backup is via event log shipping)
- **Sub-millisecond precision** — timestamps rounded to milliseconds

## Testing

```bash
CGO_ENABLED=0 go test ./... -v
CGO_ENABLED=0 go test ./... -race  # concurrent safety
```

Each task (T1–T9) has its own test fence verified by `adr-verify`. See `docs/adr/ADR-001-the-event-log-is-the-database/tasks/README.md` for verification status.

## Development

### Adding a Migration

```bash
# Edit internal/storage/migrations/NNNN_description.sql
# Goose runs them automatically on storage.Open()
```

### Adding a TQL Verb

1. Extend lexer/parser in `internal/tql/lexer.go` and `internal/tql/parser.go`
2. Add AST node in `internal/tql/ast/ast.go`
3. Implement executor arm in `internal/tql/executor.go`
4. Add tests in `internal/tql/*_test.go`

### Adding a Server Route

1. Implement handler in `internal/server/handler.go`
2. Register in `internal/server/server.go` (chi router)
3. Test with `client.Query()` or `curl`

## License

Unlicensed (example/educational)

## Status

**ADR-001 Accepted, Implementation Complete**

- ✅ T1: Storage core + config + migrations
- ✅ T2: Event log + live projection
- ✅ T3: Bitemporal semantics (redesigned Wave 3 R3)
- ✅ T4: TQL parser
- ✅ T5: TQL executor + graph
- ✅ T6: Server + client
- ✅ T7: Backup
- ✅ T8: CLI
- ✅ T9: MCP server

All acceptance tests pass. All prescribed mutants killed. Ready for production hardening.

---

**Questions?** See `docs/adr/ADR-001-the-event-log-is-the-database.md` for the design rationale, or run the server and try `./cmd/temporaldb-cli/temporaldb-cli` for an interactive walk-through.
