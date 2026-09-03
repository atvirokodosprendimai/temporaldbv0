package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"oneshottest/internal/tql"
)

// renderer handles output formatting (table or JSON).
type renderer struct {
	useTable bool
}

// newRenderer creates a renderer that outputs in the specified format.
func newRenderer(useTable bool) *renderer {
	return &renderer{useTable: useTable}
}

// render outputs the result in the configured format (table or JSON).
func (r *renderer) render(result *tql.Result) error {
	if r.useTable {
		return r.renderTable(result)
	}
	return r.renderJSON(result)
}

// renderTable outputs the result as a formatted table for a TTY.
func (r *renderer) renderTable(result *tql.Result) error {
	if len(result.Rows) == 0 {
		fmt.Printf("(empty result, count=%d, seq=%d)\n", result.Count, result.Seq)
		return nil
	}

	// Extract all keys from the first row (assuming consistent schema).
	firstRow := result.Rows[0]
	var keys []string
	for k := range firstRow {
		keys = append(keys, k)
	}

	// Print header.
	fmt.Println(strings.Join(keys, "\t"))

	// Print rows.
	for _, row := range result.Rows {
		var values []string
		for _, k := range keys {
			v := row[k]
			values = append(values, fmt.Sprintf("%v", v))
		}
		fmt.Println(strings.Join(values, "\t"))
	}

	return nil
}

// renderJSON outputs the result as JSON.
func (r *renderer) renderJSON(result *tql.Result) error {
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
