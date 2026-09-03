// Package tql implements the Temporal Query Language (TQL) executor.
package tql

// Result represents the outcome of executing a TQL statement.
// It is the type that the server and client marshal (D4, D8, D9).
type Result struct {
	// For GET, PUT, DELETE, HISTORY, RELATE, UNRELATE, RELATED, SEARCH, PURGE.
	// Each statement returns a different shape; clients interpret based on the statement type.
	Rows []map[string]interface{} `json:"rows,omitempty"`
	// Seq is the sequence number for writes (PUT, DELETE, RELATE, UNRELATE).
	Seq int64 `json:"seq,omitempty"`
	// Count is the number of rows affected or returned.
	Count int64 `json:"count,omitempty"`
	// Error, if non-empty, indicates the statement failed.
	Error string `json:"error,omitempty"`
}
