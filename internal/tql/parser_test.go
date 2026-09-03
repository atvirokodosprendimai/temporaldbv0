package tql

import (
	"testing"
	"time"

	"oneshottest/internal/tql/ast"
)

func TestParseEachVerbProducesItsNode(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    ast.Stmt
		wantErr bool
	}{
		{
			name:  "GET with collection/key",
			input: "GET users/alice",
			want: &ast.Get{
				Collection: "users",
				Key:        "alice",
			},
		},
		{
			name:  "GET with AS OF",
			input: "GET users/alice AS OF 2026-09-03T10:00:00Z",
			want: &ast.Get{
				Collection: "users",
				Key:        "alice",
				AsOf:       parseTime("2026-09-03T10:00:00Z"),
			},
		},
		{
			name:  "FIND collection only",
			input: "FIND users",
			want: &ast.Find{
				Collection: "users",
			},
		},
		{
			name:  "FIND with WHERE",
			input: "FIND users WHERE age > 21",
			want: &ast.Find{
				Collection: "users",
				Where: &ast.Where{
					Comparisons: []*ast.Comparison{
						{Field: "age", Op: ">", Value: "21"},
					},
				},
			},
		},
		{
			name:  "FIND with multiple WHERE conditions (AND)",
			input: "FIND users WHERE age > 21 AND city = NewYork",
			want: &ast.Find{
				Collection: "users",
				Where: &ast.Where{
					Comparisons: []*ast.Comparison{
						{Field: "age", Op: ">", Value: "21"},
						{Field: "city", Op: "=", Value: "NewYork"},
					},
				},
			},
		},
		{
			name:  "FIND with ORDER BY",
			input: "FIND users ORDER BY age DESC",
			want: &ast.Find{
				Collection: "users",
				OrderBy: &ast.OrderBy{
					Field: "age",
					Desc:  true,
				},
			},
		},
		{
			name:  "FIND with LIMIT",
			input: "FIND users LIMIT 10",
			want: &ast.Find{
				Collection: "users",
				Limit:      pInt(10),
			},
		},
		{
			name:  "FIND with AS OF",
			input: "FIND users AS OF 2026-09-03T10:00:00Z",
			want: &ast.Find{
				Collection: "users",
				AsOf:       parseTime("2026-09-03T10:00:00Z"),
			},
		},
		{
			name:  "FIND with all clauses",
			input: "FIND users WHERE age > 21 AS OF 2026-09-03T10:00:00Z ORDER BY age ASC LIMIT 5",
			want: &ast.Find{
				Collection: "users",
				Where: &ast.Where{
					Comparisons: []*ast.Comparison{
						{Field: "age", Op: ">", Value: "21"},
					},
				},
				AsOf: parseTime("2026-09-03T10:00:00Z"),
				OrderBy: &ast.OrderBy{
					Field: "age",
					Desc:  false,
				},
				Limit: pInt(5),
			},
		},
		{
			name:  "PUT with JSON object",
			input: `PUT users/alice {"name":"Alice","age":30}`,
			want: &ast.Put{
				Collection: "users",
				Key:        "alice",
				Value:      `{"name":"Alice","age":30}`,
			},
		},
		{
			name:  "PUT with AT timestamp",
			input: `PUT users/alice {"name":"Alice"} AT 2026-09-01T00:00:00Z`,
			want: &ast.Put{
				Collection: "users",
				Key:        "alice",
				Value:      `{"name":"Alice"}`,
				At:         parseTime("2026-09-01T00:00:00Z"),
			},
		},
		{
			name:  "DELETE",
			input: "DELETE users/alice",
			want: &ast.Delete{
				Collection: "users",
				Key:        "alice",
			},
		},
		{
			name:  "HISTORY with key only",
			input: "HISTORY users/alice",
			want: &ast.History{
				Collection: "users",
				Key:        "alice",
			},
		},
		{
			name:  "HISTORY with BETWEEN",
			input: "HISTORY users/alice BETWEEN 2026-09-01T00:00:00Z AND 2026-09-03T00:00:00Z",
			want: &ast.History{
				Collection: "users",
				Key:        "alice",
				From:       parseTime("2026-09-01T00:00:00Z"),
				To:         parseTime("2026-09-03T00:00:00Z"),
			},
		},
		{
			name:  "RELATE with edge",
			input: "RELATE users/alice -knows-> users/bob",
			want: &ast.Relate{
				FromCollection: "users",
				FromKey:        "alice",
				EdgeType:       "knows",
				ToCollection:   "users",
				ToKey:          "bob",
			},
		},
		{
			name:  "RELATE with properties",
			input: `RELATE users/alice -knows-> users/bob {"since":2020}`,
			want: &ast.Relate{
				FromCollection: "users",
				FromKey:        "alice",
				EdgeType:       "knows",
				ToCollection:   "users",
				ToKey:          "bob",
				Props:          `{"since":2020}`,
			},
		},
		{
			name:  "UNRELATE",
			input: "UNRELATE users/alice -knows-> users/bob",
			want: &ast.Unrelate{
				FromCollection: "users",
				FromKey:        "alice",
				EdgeType:       "knows",
				ToCollection:   "users",
				ToKey:          "bob",
			},
		},
		{
			name:  "RELATED with key only",
			input: "RELATED users/alice",
			want: &ast.Related{
				Collection: "users",
				Key:        "alice",
			},
		},
		{
			name:  "RELATED with edge type",
			input: "RELATED users/alice -knows->",
			want: &ast.Related{
				Collection: "users",
				Key:        "alice",
				EdgeType:   pString("knows"),
			},
		},
		{
			name:  "RELATED with AS OF",
			input: "RELATED users/alice AS OF 2026-09-03T10:00:00Z",
			want: &ast.Related{
				Collection: "users",
				Key:        "alice",
				AsOf:       parseTime("2026-09-03T10:00:00Z"),
			},
		},
		{
			name:  "RELATED with LIMIT",
			input: "RELATED users/alice LIMIT 10",
			want: &ast.Related{
				Collection: "users",
				Key:        "alice",
				Limit:      pInt(10),
			},
		},
		{
			name:  "SEARCH with text",
			input: `SEARCH documents NEAR "machine learning"`,
			want: &ast.Search{
				Collection: "documents",
				Text:       "machine learning",
			},
		},
		{
			name:  "SEARCH with WHERE",
			input: `SEARCH documents NEAR "ai" WHERE category = research`,
			want: &ast.Search{
				Collection: "documents",
				Text:       "ai",
				Where: &ast.Where{
					Comparisons: []*ast.Comparison{
						{Field: "category", Op: "=", Value: "research"},
					},
				},
			},
		},
		{
			name:  "SEARCH with LIMIT",
			input: `SEARCH documents NEAR "ai" LIMIT 20`,
			want: &ast.Search{
				Collection: "documents",
				Text:       "ai",
				Limit:      pInt(20),
			},
		},
		{
			name:  "PURGE",
			input: "PURGE users BEFORE 2026-01-01T00:00:00Z",
			want: &ast.Purge{
				Collection: "users",
				Before:     parseTime("2026-01-01T00:00:00Z"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}
			if !stmtEqual(got, tt.want) {
				t.Errorf("Parse() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestParseAsOfAndValidAtClauses(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "FIND with AS OF",
			input:   "FIND users AS OF 2026-09-03T10:00:00Z",
			wantErr: false,
		},
		{
			name:    "GET with AS OF",
			input:   "GET users/alice AS OF 2026-09-03T10:00:00Z",
			wantErr: false,
		},
		{
			name:    "RELATED with AS OF",
			input:   "RELATED users/alice AS OF 2026-09-03T10:00:00Z",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseRejectsOrInWhere(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "FIND with OR in WHERE",
			input:   "FIND users WHERE age > 21 OR city = NewYork",
			wantErr: true,
		},
		{
			name:    "FIND with AND succeeds",
			input:   "FIND users WHERE age > 21 AND city = NewYork",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && err != nil {
				// Verify the error mentions OR
				if !contains(err.Error(), "OR") {
					t.Errorf("error should mention OR: %v", err)
				}
			}
		})
	}
}

func TestParseErrorCarriesPosition(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantError bool
	}{
		{
			name:      "invalid syntax",
			input:     "GET users alice",
			wantError: true,
		},
		{
			name:      "unterminated JSON",
			input:     `PUT users/alice {"unclosed`,
			wantError: true,
		},
		{
			name:      "invalid timestamp",
			input:     "GET users/alice AS OF invalid",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.input)
			if (err != nil) != tt.wantError {
				t.Errorf("Parse() error = %v, wantError %v", err, tt.wantError)
				return
			}
			if err != nil {
				// Check that error is a ParseError with position info
				pe, ok := err.(*ParseError)
				if !ok {
					t.Errorf("expected ParseError, got %T: %v", err, err)
					return
				}
				if pe.Pos == 0 && len(pe.Input) > 0 {
					t.Errorf("ParseError.Pos should be set, got %#v", pe)
				}
			}
		})
	}
}

func TestLexerHandlesQuotedStringsAndTimestamps(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "quoted string with escapes",
			input:   `SEARCH docs NEAR "hello \"world\""`,
			wantErr: false,
		},
		{
			name:    "RFC3339 timestamp",
			input:   "GET users/alice AS OF 2026-09-03T10:00:00Z",
			wantErr: false,
		},
		{
			name:    "RFC3339 timestamp with nanoseconds",
			input:   "GET users/alice AS OF 2026-09-03T10:00:00.123456789Z",
			wantErr: false,
		},
		{
			name:    "JSON with nested quotes",
			input:   `PUT users/alice {"msg":"say \"hi\""}`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// Helper functions for test comparison

func pInt(v int) *int {
	return &v
}

func pString(v string) *string {
	return &v
}

func parseTime(s string) *time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		panic(err)
	}
	return &t
}

func contains(s, substr string) bool {
	for i := 0; i < len(s)-len(substr)+1; i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func stmtEqual(a, b ast.Stmt) bool {
	switch aa := a.(type) {
	case *ast.Get:
		bb, ok := b.(*ast.Get)
		return ok && aa.Collection == bb.Collection && aa.Key == bb.Key && timeEqual(aa.AsOf, bb.AsOf)
	case *ast.Find:
		bb, ok := b.(*ast.Find)
		return ok && aa.Collection == bb.Collection && whereEqual(aa.Where, bb.Where) &&
			timeEqual(aa.AsOf, bb.AsOf) && orderByEqual(aa.OrderBy, bb.OrderBy) && intEqual(aa.Limit, bb.Limit)
	case *ast.Put:
		bb, ok := b.(*ast.Put)
		return ok && aa.Collection == bb.Collection && aa.Key == bb.Key && aa.Value == bb.Value && timeEqual(aa.At, bb.At)
	case *ast.Delete:
		bb, ok := b.(*ast.Delete)
		return ok && aa.Collection == bb.Collection && aa.Key == bb.Key
	case *ast.History:
		bb, ok := b.(*ast.History)
		return ok && aa.Collection == bb.Collection && aa.Key == bb.Key && timeEqual(aa.From, bb.From) && timeEqual(aa.To, bb.To)
	case *ast.Relate:
		bb, ok := b.(*ast.Relate)
		return ok && aa.FromCollection == bb.FromCollection && aa.FromKey == bb.FromKey &&
			aa.EdgeType == bb.EdgeType && aa.ToCollection == bb.ToCollection &&
			aa.ToKey == bb.ToKey && aa.Props == bb.Props
	case *ast.Unrelate:
		bb, ok := b.(*ast.Unrelate)
		return ok && aa.FromCollection == bb.FromCollection && aa.FromKey == bb.FromKey &&
			aa.EdgeType == bb.EdgeType && aa.ToCollection == bb.ToCollection && aa.ToKey == bb.ToKey
	case *ast.Related:
		bb, ok := b.(*ast.Related)
		return ok && aa.Collection == bb.Collection && aa.Key == bb.Key &&
			stringPtrEqual(aa.EdgeType, bb.EdgeType) && timeEqual(aa.AsOf, bb.AsOf) && intEqual(aa.Limit, bb.Limit)
	case *ast.Search:
		bb, ok := b.(*ast.Search)
		return ok && aa.Collection == bb.Collection && aa.Text == bb.Text &&
			whereEqual(aa.Where, bb.Where) && intEqual(aa.Limit, bb.Limit)
	case *ast.Purge:
		bb, ok := b.(*ast.Purge)
		return ok && aa.Collection == bb.Collection && timeEqual(aa.Before, bb.Before)
	default:
		return false
	}
}

func timeEqual(a, b *time.Time) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Equal(*b)
}

func intEqual(a, b *int) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func stringPtrEqual(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func whereEqual(a, b *ast.Where) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if len(a.Comparisons) != len(b.Comparisons) {
		return false
	}
	for i := range a.Comparisons {
		if a.Comparisons[i].Field != b.Comparisons[i].Field ||
			a.Comparisons[i].Op != b.Comparisons[i].Op ||
			a.Comparisons[i].Value != b.Comparisons[i].Value {
			return false
		}
	}
	return true
}

func orderByEqual(a, b *ast.OrderBy) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Field == b.Field && a.Desc == b.Desc
}
