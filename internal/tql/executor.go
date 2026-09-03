package tql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"oneshottest/internal/event"
	"oneshottest/internal/temporal"
	"oneshottest/internal/tql/ast"
	"oneshottest/internal/vector"
)

// Searcher is the interface for vector search operations.
// It is optional; when nil, SEARCH returns a "feature not configured" error.
type Searcher interface {
	Search(ctx context.Context, collection string, text string, k int) ([]vector.Hit, error)
}

// Purger is the interface for backup sink purge operations (T7).
// It is optional; when nil, PURGE only clears events, not backed-up segments.
type Purger interface {
	Purge(ctx context.Context, before time.Time) error
}

// Executor compiles and executes TQL statements over the event log.
type Executor struct {
	store  *event.Store
	search Searcher
	purge  Purger
}

// NewExecutor creates a new TQL executor.
// search and purge are optional (nil is permitted).
func NewExecutor(store *event.Store, search Searcher, purge Purger) *Executor {
	return &Executor{
		store:  store,
		search: search,
		purge:  purge,
	}
}

// Exec executes a parsed TQL statement and returns a Result.
func (e *Executor) Exec(ctx context.Context, stmt ast.Stmt) (*Result, error) {
	switch s := stmt.(type) {
	case *ast.Get:
		return e.execGet(ctx, s)
	case *ast.Put:
		return e.execPut(ctx, s)
	case *ast.Delete:
		return e.execDelete(ctx, s)
	case *ast.Find:
		return e.execFind(ctx, s)
	case *ast.History:
		return e.execHistory(ctx, s)
	case *ast.Relate:
		return e.execRelate(ctx, s)
	case *ast.Unrelate:
		return e.execUnrelate(ctx, s)
	case *ast.Related:
		return e.execRelated(ctx, s)
	case *ast.Search:
		return e.execSearch(ctx, s)
	case *ast.Purge:
		return e.execPurge(ctx, s)
	default:
		return &Result{Error: "unknown statement type"}, nil
	}
}

// execPut handles PUT <collection>/<key> <json> [AT <time>]
func (e *Executor) execPut(ctx context.Context, stmt *ast.Put) (*Result, error) {
	// Parse the JSON value
	var value interface{}
	if err := json.Unmarshal([]byte(stmt.Value), &value); err != nil {
		return &Result{Error: fmt.Sprintf("invalid JSON value: %v", err)}, nil
	}

	valueBytes := []byte(stmt.Value)
	validFrom := time.Now().UTC()
	if stmt.At != nil {
		validFrom = *stmt.At
	}

	ev := event.Event{
		Collection: stmt.Collection,
		Key:        stmt.Key,
		Op:         event.OpPut,
		Value:      valueBytes,
		ValidFrom:  validFrom,
		TxTime:     time.Now().UTC(),
		Meta:       []byte("{}"),
	}

	seq, err := e.store.Append(ctx, ev)
	if err != nil {
		return &Result{Error: fmt.Sprintf("append: %v", err)}, nil
	}

	return &Result{Seq: seq}, nil
}

// execGet handles GET <collection>/<key> [AS OF <time>]
func (e *Executor) execGet(ctx context.Context, stmt *ast.Get) (*Result, error) {
	if stmt.AsOf != nil {
		// Time-travel query: replay events and apply temporal.Visible
		return e.execGetAsOf(ctx, stmt)
	}

	// Current state: read from live table
	var valueStr string
	var validFromStr, txTimeStr string
	err := e.store.QueryRowContext(ctx,
		`SELECT value, valid_from, tx_time FROM live WHERE collection = ? AND key = ? AND deleted = 0`,
		stmt.Collection, stmt.Key).Scan(&valueStr, &validFromStr, &txTimeStr)
	if err == sql.ErrNoRows {
		return &Result{Rows: []map[string]interface{}{}}, nil
	}
	if err != nil {
		return &Result{Error: fmt.Sprintf("query live: %v", err)}, nil
	}

	var value interface{}
	if err := json.Unmarshal([]byte(valueStr), &value); err != nil {
		return &Result{Error: fmt.Sprintf("unmarshal value: %v", err)}, nil
	}

	row := valueToMap(value)
	return &Result{Rows: []map[string]interface{}{row}, Count: 1}, nil
}

// execGetAsOf handles GET with AS OF (time-travel query)
func (e *Executor) execGetAsOf(ctx context.Context, stmt *ast.Get) (*Result, error) {
	// Get all events for this key
	events, err := e.store.History(ctx, stmt.Collection, stmt.Key)
	if err != nil {
		return &Result{Error: fmt.Sprintf("query history: %v", err)}, nil
	}

	// Apply temporal.Visible to find the one visible version
	// Note: asOf=time.Time{} means unbounded (all events considered for validity)
	// validAt is the business time query
	visibleEv, ok := temporal.Visible(events, time.Time{}, *stmt.AsOf)
	if !ok {
		return &Result{Rows: []map[string]interface{}{}}, nil
	}

	var value interface{}
	if err := json.Unmarshal(visibleEv.Value, &value); err != nil {
		return &Result{Error: fmt.Sprintf("unmarshal value: %v", err)}, nil
	}

	row := valueToMap(value)
	return &Result{Rows: []map[string]interface{}{row}, Count: 1}, nil
}

// execDelete handles DELETE <collection>/<key>
func (e *Executor) execDelete(ctx context.Context, stmt *ast.Delete) (*Result, error) {
	now := time.Now().UTC()

	ev := event.Event{
		Collection: stmt.Collection,
		Key:        stmt.Key,
		Op:         event.OpDelete,
		Value:      nil,
		ValidFrom:  now,
		TxTime:     now,
		Meta:       []byte("{}"),
	}

	seq, err := e.store.Append(ctx, ev)
	if err != nil {
		return &Result{Error: fmt.Sprintf("append: %v", err)}, nil
	}

	return &Result{Seq: seq}, nil
}

// execFind handles FIND <collection> [WHERE <expr>] [ORDER BY <field> [ASC|DESC]] [LIMIT <n>]
func (e *Executor) execFind(ctx context.Context, stmt *ast.Find) (*Result, error) {
	if stmt.AsOf != nil {
		return e.execFindAsOf(ctx, stmt)
	}

	// Current state: query live table
	query := `SELECT key, value FROM live WHERE collection = ? AND deleted = 0`
	args := []interface{}{stmt.Collection}

	// Compile WHERE clause to SQL predicates
	if stmt.Where != nil {
		for _, comp := range stmt.Where.Comparisons {
			pred, err := comparisonToPredicate(comp)
			if err != nil {
				return &Result{Error: fmt.Sprintf("compile WHERE: %v", err)}, nil
			}
			query += " AND " + pred
			// Append the value as a parameter (bound, not concatenated)
			args = append(args, parseValue(comp.Value))
		}
	}

	// Handle ORDER BY
	if stmt.OrderBy != nil {
		orderDir := "ASC"
		if stmt.OrderBy.Desc {
			orderDir = "DESC"
		}
		query += fmt.Sprintf(` ORDER BY json_extract(value, '$.%s') %s`, stmt.OrderBy.Field, orderDir)
	}

	// Handle LIMIT
	if stmt.Limit != nil {
		query += fmt.Sprintf(` LIMIT %d`, *stmt.Limit)
	}

	rows, err := e.store.QueryContext(ctx, query, args...)
	if err != nil {
		return &Result{Error: fmt.Sprintf("query: %v", err)}, nil
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var key string
		var valueStr string
		if err := rows.Scan(&key, &valueStr); err != nil {
			return &Result{Error: fmt.Sprintf("scan: %v", err)}, nil
		}

		var value interface{}
		if err := json.Unmarshal([]byte(valueStr), &value); err != nil {
			return &Result{Error: fmt.Sprintf("unmarshal: %v", err)}, nil
		}

		row := valueToMap(value)
		results = append(results, row)
	}

	if err := rows.Err(); err != nil {
		return &Result{Error: fmt.Sprintf("rows error: %v", err)}, nil
	}

	if results == nil {
		results = []map[string]interface{}{}
	}

	return &Result{Rows: results, Count: int64(len(results))}, nil
}

// execFindAsOf handles FIND with AS OF (time-travel query)
func (e *Executor) execFindAsOf(ctx context.Context, stmt *ast.Find) (*Result, error) {
	// Replay the state as of the given time
	state, err := e.store.ReplayAsOf(ctx, stmt.Collection, *stmt.AsOf)
	if err != nil {
		return &Result{Error: fmt.Sprintf("replay: %v", err)}, nil
	}

	// Filter by WHERE clause
	var results []map[string]interface{}
	for _, ev := range state {
		if stmt.Where != nil {
			match, err := matchesWhere(ev.Value, stmt.Where)
			if err != nil {
				return &Result{Error: fmt.Sprintf("match WHERE: %v", err)}, nil
			}
			if !match {
				continue
			}
		}

		var value interface{}
		if err := json.Unmarshal(ev.Value, &value); err != nil {
			return &Result{Error: fmt.Sprintf("unmarshal: %v", err)}, nil
		}

		row := valueToMap(value)
		results = append(results, row)
	}

	// Sort and limit (TODO: implement ORDER BY sorting if needed)
	if stmt.Limit != nil && len(results) > *stmt.Limit {
		results = results[:*stmt.Limit]
	}

	if results == nil {
		results = []map[string]interface{}{}
	}

	return &Result{Rows: results, Count: int64(len(results))}, nil
}

// execHistory handles HISTORY <collection>/<key> [BETWEEN <time> AND <time>]
func (e *Executor) execHistory(ctx context.Context, stmt *ast.History) (*Result, error) {
	events, err := e.store.History(ctx, stmt.Collection, stmt.Key)
	if err != nil {
		return &Result{Error: fmt.Sprintf("query history: %v", err)}, nil
	}

	// Filter by time range if specified
	if stmt.From != nil || stmt.To != nil {
		var filtered []event.Event
		for _, ev := range events {
			if stmt.From != nil && ev.ValidFrom.Before(*stmt.From) {
				continue
			}
			if stmt.To != nil && ev.ValidFrom.After(*stmt.To) {
				continue
			}
			filtered = append(filtered, ev)
		}
		events = filtered
	}

	var results []map[string]interface{}
	for _, ev := range events {
		row := map[string]interface{}{
			"key":        ev.Key,
			"op":         string(ev.Op),
			"seq":        ev.Seq,
			"valid_from": ev.ValidFrom.Format(time.RFC3339Nano),
			"tx_time":    ev.TxTime.Format(time.RFC3339Nano),
		}
		if ev.Value != nil {
			var value interface{}
			if err := json.Unmarshal(ev.Value, &value); err == nil {
				row["value"] = value
			}
		}
		results = append(results, row)
	}

	if results == nil {
		results = []map[string]interface{}{}
	}

	return &Result{Rows: results, Count: int64(len(results))}, nil
}

// execRelate handles RELATE <from> -<type>-> <to> [<json>]
func (e *Executor) execRelate(ctx context.Context, stmt *ast.Relate) (*Result, error) {
	// Relations are stored as events in the __edges__ collection
	// Key is a hash of from/type/to
	key := edgeKey(stmt.FromCollection+"/"+stmt.FromKey, stmt.EdgeType, stmt.ToCollection+"/"+stmt.ToKey)

	// Meta contains the edge metadata: {from, type, to, props}
	meta := map[string]interface{}{
		"from": stmt.FromCollection + "/" + stmt.FromKey,
		"type": stmt.EdgeType,
		"to":   stmt.ToCollection + "/" + stmt.ToKey,
	}

	if stmt.Props != "" {
		var props interface{}
		if err := json.Unmarshal([]byte(stmt.Props), &props); err != nil {
			return &Result{Error: fmt.Sprintf("invalid props JSON: %v", err)}, nil
		}
		meta["props"] = props
	}

	metaBytes, _ := json.Marshal(meta)

	now := time.Now().UTC()
	ev := event.Event{
		Collection: "__edges__",
		Key:        key,
		Op:         event.OpPut,
		Value:      []byte(`{}`),
		ValidFrom:  now,
		TxTime:     now,
		Meta:       metaBytes,
	}

	seq, err := e.store.Append(ctx, ev)
	if err != nil {
		return &Result{Error: fmt.Sprintf("append edge: %v", err)}, nil
	}

	return &Result{Seq: seq}, nil
}

// execUnrelate handles UNRELATE <from> -<type>-> <to>
func (e *Executor) execUnrelate(ctx context.Context, stmt *ast.Unrelate) (*Result, error) {
	key := edgeKey(stmt.FromCollection+"/"+stmt.FromKey, stmt.EdgeType, stmt.ToCollection+"/"+stmt.ToKey)

	now := time.Now().UTC()
	ev := event.Event{
		Collection: "__edges__",
		Key:        key,
		Op:         event.OpDelete,
		Value:      nil,
		ValidFrom:  now,
		TxTime:     now,
		Meta:       []byte("{}"),
	}

	seq, err := e.store.Append(ctx, ev)
	if err != nil {
		return &Result{Error: fmt.Sprintf("append unrelate: %v", err)}, nil
	}

	return &Result{Seq: seq}, nil
}

// execRelated handles RELATED <collection>/<key> [-<edge-type>->] [AS OF <time>] [LIMIT <n>]
func (e *Executor) execRelated(ctx context.Context, stmt *ast.Related) (*Result, error) {
	fromKey := stmt.Collection + "/" + stmt.Key

	if stmt.AsOf != nil {
		return e.execRelatedAsOf(ctx, stmt, fromKey)
	}

	// Current state: query live table for edges
	query := `SELECT key, meta FROM live WHERE collection = '__edges__' AND deleted = 0`
	args := []interface{}{}

	// Filter by edge type if specified
	if stmt.EdgeType != nil {
		query += ` AND json_extract(meta, '$.type') = ?`
		args = append(args, *stmt.EdgeType)
	}

	// Filter by from
	query += ` AND json_extract(meta, '$.from') = ?`
	args = append(args, fromKey)

	// Handle LIMIT
	if stmt.Limit != nil {
		query += fmt.Sprintf(` LIMIT %d`, *stmt.Limit)
	}

	rows, err := e.store.QueryContext(ctx, query, args...)
	if err != nil {
		return &Result{Error: fmt.Sprintf("query: %v", err)}, nil
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var key string
		var metaStr string
		if err := rows.Scan(&key, &metaStr); err != nil {
			return &Result{Error: fmt.Sprintf("scan: %v", err)}, nil
		}

		var meta map[string]interface{}
		if err := json.Unmarshal([]byte(metaStr), &meta); err != nil {
			return &Result{Error: fmt.Sprintf("unmarshal meta: %v", err)}, nil
		}

		results = append(results, meta)
	}

	if err := rows.Err(); err != nil {
		return &Result{Error: fmt.Sprintf("rows error: %v", err)}, nil
	}

	if results == nil {
		results = []map[string]interface{}{}
	}

	return &Result{Rows: results, Count: int64(len(results))}, nil
}

// execRelatedAsOf handles RELATED with AS OF (time-travel)
func (e *Executor) execRelatedAsOf(ctx context.Context, stmt *ast.Related, fromKey string) (*Result, error) {
	// Replay edges as of the given time
	state, err := e.store.ReplayAsOf(ctx, "__edges__", *stmt.AsOf)
	if err != nil {
		return &Result{Error: fmt.Sprintf("replay edges: %v", err)}, nil
	}

	var results []map[string]interface{}
	for _, ev := range state {
		var meta map[string]interface{}
		if err := json.Unmarshal(ev.Meta, &meta); err != nil {
			return &Result{Error: fmt.Sprintf("unmarshal meta: %v", err)}, nil
		}

		// Filter by from
		from, ok := meta["from"].(string)
		if !ok || from != fromKey {
			continue
		}

		// Filter by edge type if specified
		if stmt.EdgeType != nil {
			edgeType, ok := meta["type"].(string)
			if !ok || edgeType != *stmt.EdgeType {
				continue
			}
		}

		results = append(results, meta)
	}

	// Handle LIMIT
	if stmt.Limit != nil && len(results) > *stmt.Limit {
		results = results[:*stmt.Limit]
	}

	if results == nil {
		results = []map[string]interface{}{}
	}

	return &Result{Rows: results, Count: int64(len(results))}, nil
}

// execSearch handles SEARCH <collection> NEAR "<text>" [WHERE <expr>] [LIMIT <n>]
func (e *Executor) execSearch(ctx context.Context, stmt *ast.Search) (*Result, error) {
	// Check if vector search is configured
	if e.search == nil {
		return &Result{Error: "feature not configured"}, nil
	}

	// Default limit
	k := 10
	if stmt.Limit != nil {
		k = *stmt.Limit
	}

	// Search the vector index
	hits, err := e.search.Search(ctx, stmt.Collection, stmt.Text, k)
	if err != nil {
		return &Result{Error: fmt.Sprintf("search: %v", err)}, nil
	}

	// Hydrate from live table
	var results []map[string]interface{}
	for _, hit := range hits {
		var valueStr string
		err := e.store.QueryRowContext(ctx,
			`SELECT value FROM live WHERE collection = ? AND key = ? AND deleted = 0`,
			stmt.Collection, hit.Key).Scan(&valueStr)
		if err == sql.ErrNoRows {
			continue // Skip if not found
		}
		if err != nil {
			return &Result{Error: fmt.Sprintf("hydrate: %v", err)}, nil
		}

		var value interface{}
		if err := json.Unmarshal([]byte(valueStr), &value); err != nil {
			return &Result{Error: fmt.Sprintf("unmarshal: %v", err)}, nil
		}

		// Apply WHERE filter if specified
		if stmt.Where != nil {
			match, err := matchesWhere([]byte(valueStr), stmt.Where)
			if err != nil {
				return &Result{Error: fmt.Sprintf("match WHERE: %v", err)}, nil
			}
			if !match {
				continue
			}
		}

		row := valueToMap(value)
		row["_score"] = hit.Score
		results = append(results, row)
	}

	if results == nil {
		results = []map[string]interface{}{}
	}

	return &Result{Rows: results, Count: int64(len(results))}, nil
}

// execPurge handles PURGE <collection> BEFORE <time>
func (e *Executor) execPurge(ctx context.Context, stmt *ast.Purge) (*Result, error) {
	before := stmt.Before.Format(time.RFC3339Nano)

	// Delete from events table
	result, err := e.store.ExecContext(ctx,
		`DELETE FROM events WHERE collection = ? AND valid_from < ?`,
		stmt.Collection, before)
	if err != nil {
		return &Result{Error: fmt.Sprintf("delete events: %v", err)}, nil
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return &Result{Error: fmt.Sprintf("rows affected: %v", err)}, nil
	}

	// Notify backup sink to purge if configured
	if e.purge != nil {
		if err := e.purge.Purge(ctx, *stmt.Before); err != nil {
			return &Result{Error: fmt.Sprintf("purge backup: %v", err)}, nil
		}
	}

	return &Result{Count: affected}, nil
}

// --- Helpers ---

// comparisonToPredicate converts a WHERE comparison to a SQL fragment with parameter placeholder.
// Note: does NOT include the parameter value; the caller must append it to args.
func comparisonToPredicate(comp *ast.Comparison) (string, error) {
	field := comp.Field
	op := strings.ToUpper(comp.Op)

	// Build json_extract path for nested fields
	path := fmt.Sprintf(`$.%s`, field)

	switch op {
	case "=":
		return fmt.Sprintf(`json_extract(value, '%s') = ?`, path), nil
	case "!=":
		return fmt.Sprintf(`json_extract(value, '%s') != ?`, path), nil
	case "<":
		return fmt.Sprintf(`json_extract(value, '%s') < ?`, path), nil
	case "<=":
		return fmt.Sprintf(`json_extract(value, '%s') <= ?`, path), nil
	case ">":
		return fmt.Sprintf(`json_extract(value, '%s') > ?`, path), nil
	case ">=":
		return fmt.Sprintf(`json_extract(value, '%s') >= ?`, path), nil
	case "IN":
		// IN operator expects an array; convert to JSON comparison
		return fmt.Sprintf(`json_extract(value, '%s') IN (SELECT json_each.value FROM json_each(?))`, path), nil
	case "CONTAINS":
		// CONTAINS for array membership
		return fmt.Sprintf(`json_array_length(json_extract(value, '%s')) > 0`, path), nil
	default:
		return "", fmt.Errorf("unsupported operator: %s", op)
	}
}

// matchesWhere checks if a JSON value matches a WHERE clause.
func matchesWhere(valueBytes []byte, where *ast.Where) (bool, error) {
	var value interface{}
	if err := json.Unmarshal(valueBytes, &value); err != nil {
		return false, err
	}

	doc, ok := value.(map[string]interface{})
	if !ok {
		return false, nil
	}

	for _, comp := range where.Comparisons {
		if !matchComparison(doc, comp) {
			return false, nil
		}
	}

	return true, nil
}

// matchComparison checks if a document matches a single comparison.
func matchComparison(doc map[string]interface{}, comp *ast.Comparison) bool {
	fieldValue := getFieldValue(doc, comp.Field)
	if fieldValue == nil {
		return false
	}

	op := strings.ToUpper(comp.Op)
	expectedValue := parseValue(comp.Value)

	switch op {
	case "=":
		return fieldValue == expectedValue
	case "!=":
		return fieldValue != expectedValue
	case "<":
		return compare(fieldValue, expectedValue) < 0
	case "<=":
		return compare(fieldValue, expectedValue) <= 0
	case ">":
		return compare(fieldValue, expectedValue) > 0
	case ">=":
		return compare(fieldValue, expectedValue) >= 0
	case "IN":
		// Check if fieldValue is in the array
		arr, ok := expectedValue.([]interface{})
		if !ok {
			return false
		}
		for _, v := range arr {
			if fieldValue == v {
				return true
			}
		}
		return false
	case "CONTAINS":
		// Check if fieldValue array contains a value
		arr, ok := fieldValue.([]interface{})
		if !ok {
			return false
		}
		for _, v := range arr {
			if v == expectedValue {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// getFieldValue extracts a value from a document by field path (supports nested fields like "address.city").
func getFieldValue(doc map[string]interface{}, fieldPath string) interface{} {
	parts := strings.Split(fieldPath, ".")
	current := interface{}(doc)

	for _, part := range parts {
		switch v := current.(type) {
		case map[string]interface{}:
			current = v[part]
		default:
			return nil
		}
	}

	return current
}

// parseValue parses a string value into the appropriate Go type.
func parseValue(s string) interface{} {
	// Try integer
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return float64(i)
	}

	// Try float
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}

	// Try boolean
	if s == "true" {
		return true
	}
	if s == "false" {
		return false
	}

	// Try JSON array
	var arr []interface{}
	if err := json.Unmarshal([]byte(s), &arr); err == nil {
		return arr
	}

	// Try JSON object
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(s), &obj); err == nil {
		return obj
	}

	// Default to string (strip quotes if present)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}

	return s
}

// compare returns -1, 0, or 1 for a<b, a==b, a>b.
// Assumes both values are numeric after parseValue.
func compare(a, b interface{}) int {
	aFloat := toFloat(a)
	bFloat := toFloat(b)

	if aFloat < bFloat {
		return -1
	}
	if aFloat > bFloat {
		return 1
	}
	return 0
}

// toFloat converts a value to float64 for comparison.
func toFloat(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case string:
		f, _ := strconv.ParseFloat(val, 64)
		return f
	default:
		return 0
	}
}

// valueToMap converts a JSON value to a map (or extracts it if it's a map).
func valueToMap(value interface{}) map[string]interface{} {
	if m, ok := value.(map[string]interface{}); ok {
		return m
	}
	return map[string]interface{}{"value": value}
}

// edgeKey computes a deterministic key for an edge (from/type/to).
// Uses length-prefixed concatenation to avoid collisions.
func edgeKey(from, edgeType, to string) string {
	// Simple deterministic key: use hex encoding of lengths to avoid ambiguity
	return fmt.Sprintf("%d:%s:%d:%s:%d:%s", len(from), from, len(edgeType), edgeType, len(to), to)
}
