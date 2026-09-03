package ast

import (
	"testing"
	"time"
)

// TestStmtInterfaceImplementation verifies that all statement types implement the Stmt interface.
func TestStmtInterfaceImplementation(t *testing.T) {
	tests := []struct {
		name string
		stmt Stmt
	}{
		{"Get", &Get{Collection: "test", Key: "key1"}},
		{"Put", &Put{Collection: "test", Key: "key1", Value: "{}"}},
		{"Delete", &Delete{Collection: "test", Key: "key1"}},
		{"Find", &Find{Collection: "test"}},
		{"History", &History{Collection: "test", Key: "key1"}},
		{"Relate", &Relate{FromCollection: "a", FromKey: "k1", EdgeType: "edge", ToCollection: "b", ToKey: "k2"}},
		{"Unrelate", &Unrelate{FromCollection: "a", FromKey: "k1", EdgeType: "edge", ToCollection: "b", ToKey: "k2"}},
		{"Related", &Related{Collection: "test", Key: "key1"}},
		{"Search", &Search{Collection: "test", Text: "query"}},
		{"Purge", &Purge{Collection: "test"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.stmt == nil {
				t.Error("stmt is nil")
			}
			// The stmt() method is not exported, but we can verify the type
			// implements the interface by assignment.
			var _ Stmt = tt.stmt
		})
	}
}

// TestStatementFieldTypes verifies that statement fields have the correct types.
func TestStatementFieldTypes(t *testing.T) {
	now := time.Now()

	g := &Get{Collection: "col", Key: "k", AsOf: &now}
	if g.Collection != "col" || g.Key != "k" || g.AsOf == nil {
		t.Error("Get fields not set correctly")
	}

	p := &Put{Collection: "col", Key: "k", Value: "{}", At: &now}
	if p.Collection != "col" || p.Key != "k" || p.Value != "{}" || p.At == nil {
		t.Error("Put fields not set correctly")
	}

	where := &Where{
		Comparisons: []*Comparison{
			{Field: "age", Op: ">", Value: "21"},
		},
	}
	if len(where.Comparisons) != 1 || where.Comparisons[0].Field != "age" {
		t.Error("Where/Comparison fields not set correctly")
	}

	order := &OrderBy{Field: "name", Desc: false}
	if order.Field != "name" || order.Desc {
		t.Error("OrderBy fields not set correctly")
	}
}
