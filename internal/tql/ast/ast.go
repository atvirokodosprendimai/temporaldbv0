// Package ast defines abstract syntax tree node types for TQL (Temporal Query Language).
package ast

import "time"

// Stmt is the interface for all TQL statements.
type Stmt interface {
	stmt()
}

// Get represents a GET statement: GET <collection>/<key> [AS OF <time>]
type Get struct {
	Collection string
	Key        string
	AsOf       *time.Time
}

func (Get) stmt() {}

// Find represents a FIND statement: FIND <collection> [WHERE <expr>] [AS OF <time>] [ORDER BY <field> [ASC|DESC]] [LIMIT <n>]
type Find struct {
	Collection string
	Where      *Where
	AsOf       *time.Time
	OrderBy    *OrderBy
	Limit      *int
}

func (Find) stmt() {}

// Put represents a PUT statement: PUT <collection>/<key> <json-object> [AT <time>]
type Put struct {
	Collection string
	Key        string
	Value      string // JSON string
	At         *time.Time
}

func (Put) stmt() {}

// Delete represents a DELETE statement: DELETE <collection>/<key>
type Delete struct {
	Collection string
	Key        string
}

func (Delete) stmt() {}

// History represents a HISTORY statement: HISTORY <collection>/<key> [BETWEEN <time> AND <time>]
type History struct {
	Collection string
	Key        string
	From       *time.Time
	To         *time.Time
}

func (History) stmt() {}

// Relate represents a RELATE statement: RELATE <collection>/<key> -<edge-type>-> <collection>/<key> [<json-object>]
type Relate struct {
	FromCollection string
	FromKey        string
	EdgeType       string
	ToCollection   string
	ToKey          string
	Props          string // JSON string, optional
}

func (Relate) stmt() {}

// Unrelate represents an UNRELATE statement: UNRELATE <collection>/<key> -<edge-type>-> <collection>/<key>
type Unrelate struct {
	FromCollection string
	FromKey        string
	EdgeType       string
	ToCollection   string
	ToKey          string
}

func (Unrelate) stmt() {}

// Related represents a RELATED statement: RELATED <collection>/<key> [-<edge-type>->] [AS OF <time>] [LIMIT <n>]
type Related struct {
	Collection string
	Key        string
	EdgeType   *string // optional
	AsOf       *time.Time
	Limit      *int
}

func (Related) stmt() {}

// Search represents a SEARCH statement: SEARCH <collection> NEAR "<text>" [WHERE <expr>] [LIMIT <n>]
type Search struct {
	Collection string
	Text       string
	Where      *Where
	Limit      *int
}

func (Search) stmt() {}

// Purge represents a PURGE statement: PURGE <collection> BEFORE <time>
type Purge struct {
	Collection string
	Before     *time.Time
}

func (Purge) stmt() {}

// Where represents a WHERE clause: <field> <op> <value> [AND <expr>]*
type Where struct {
	Comparisons []*Comparison
}

// Comparison represents a single comparison: <field> <op> <value>
type Comparison struct {
	Field string
	Op    string // one of: =, !=, <, <=, >, >=, IN, CONTAINS
	Value string
}

// OrderBy represents an ORDER BY clause: ORDER BY <field> [ASC|DESC]
type OrderBy struct {
	Field string
	Desc  bool // true for DESC, false for ASC
}
