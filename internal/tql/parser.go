package tql

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"oneshottest/internal/tql/ast"
)

// ParseError represents an error during parsing with position information.
type ParseError struct {
	Pos    int
	Input  string
	Token  *Token
	Reason string
}

func (e *ParseError) Error() string {
	if e.Token != nil {
		return fmt.Sprintf("parse error at position %d: %s (token: %s)", e.Pos, e.Reason, e.Token.Value)
	}
	return fmt.Sprintf("parse error at position %d: %s", e.Pos, e.Reason)
}

// Parser parses a sequence of tokens into an AST.
type Parser struct {
	tokens []*Token
	pos    int
	input  string
}

// Parse parses a TQL string and returns an AST statement.
func Parse(input string) (ast.Stmt, error) {
	lexer := NewLexer(input)
	tokens, err := lexer.Tokenize()
	if err != nil {
		// Try to extract position from error message
		pos := 0
		if strings.Contains(err.Error(), "position") {
			// Error message format: "... at position N"
			parts := strings.Split(err.Error(), "position")
			if len(parts) > 1 {
				numPart := strings.TrimSpace(parts[len(parts)-1])
				if n, parseErr := strconv.Atoi(strings.Fields(numPart)[0]); parseErr == nil {
					pos = n
				}
			}
		}
		return nil, &ParseError{
			Pos:    pos,
			Input:  input,
			Reason: err.Error(),
		}
	}

	parser := &Parser{
		tokens: tokens,
		pos:    0,
		input:  input,
	}

	return parser.parseStatement()
}

func (p *Parser) parseStatement() (ast.Stmt, error) {
	if p.isEOF() {
		return nil, p.error("unexpected end of input")
	}

	keyword := p.peek().Value
	switch strings.ToUpper(keyword) {
	case "GET":
		return p.parseGet()
	case "FIND":
		return p.parseFind()
	case "PUT":
		return p.parsePut()
	case "DELETE":
		return p.parseDelete()
	case "HISTORY":
		return p.parseHistory()
	case "RELATE":
		return p.parseRelate()
	case "UNRELATE":
		return p.parseUnrelate()
	case "RELATED":
		return p.parseRelated()
	case "SEARCH":
		return p.parseSearch()
	case "PURGE":
		return p.parsePurge()
	default:
		return nil, p.error(fmt.Sprintf("unexpected keyword: %s", keyword))
	}
}

// GET <collection>/<key> [AS OF <time>]
func (p *Parser) parseGet() (ast.Stmt, error) {
	if !p.consume("GET") {
		return nil, p.error("expected GET")
	}

	collection, key, err := p.parseCollectionKey()
	if err != nil {
		return nil, err
	}

	stmt := &ast.Get{
		Collection: collection,
		Key:        key,
	}

	// Optional AS OF
	if p.match("AS") {
		if !p.consume("AS") {
			return nil, p.error("expected AS")
		}
		if !p.consume("OF") {
			return nil, p.error("expected OF")
		}
		asOf, err := p.parseTime()
		if err != nil {
			return nil, err
		}
		stmt.AsOf = asOf
	}

	return stmt, nil
}

// FIND <collection> [WHERE <expr>] [AS OF <time>] [ORDER BY <field> [ASC|DESC]] [LIMIT <n>]
func (p *Parser) parseFind() (ast.Stmt, error) {
	if !p.consume("FIND") {
		return nil, p.error("expected FIND")
	}

	collection, err := p.parseIdentifier()
	if err != nil {
		return nil, err
	}

	stmt := &ast.Find{
		Collection: collection,
	}

	// Optional WHERE
	if p.match("WHERE") {
		p.consume("WHERE")
		where, err := p.parseWhere()
		if err != nil {
			return nil, err
		}
		stmt.Where = where
	}

	// Optional AS OF
	if p.match("AS") {
		if !p.consume("AS") {
			return nil, p.error("expected AS")
		}
		if !p.consume("OF") {
			return nil, p.error("expected OF")
		}
		asOf, err := p.parseTime()
		if err != nil {
			return nil, err
		}
		stmt.AsOf = asOf
	}

	// Optional ORDER BY
	if p.match("ORDER") {
		p.consume("ORDER")
		p.consume("BY")
		field, err := p.parseIdentifier()
		if err != nil {
			return nil, err
		}
		desc := false
		if p.match("DESC") {
			p.consume("DESC")
			desc = true
		} else if p.match("ASC") {
			p.consume("ASC")
		}
		stmt.OrderBy = &ast.OrderBy{
			Field: field,
			Desc:  desc,
		}
	}

	// Optional LIMIT
	if p.match("LIMIT") {
		p.consume("LIMIT")
		limit, err := p.parseNumber()
		if err != nil {
			return nil, err
		}
		stmt.Limit = &limit
	}

	return stmt, nil
}

// PUT <collection>/<key> <json-object> [AT <time>]
func (p *Parser) parsePut() (ast.Stmt, error) {
	if !p.consume("PUT") {
		return nil, p.error("expected PUT")
	}

	collection, key, err := p.parseCollectionKey()
	if err != nil {
		return nil, err
	}

	// Expect JSON object
	if !p.isTokenType(TokenJSON) {
		return nil, p.error("expected JSON object after PUT")
	}
	value := p.advance().Value

	stmt := &ast.Put{
		Collection: collection,
		Key:        key,
		Value:      value,
	}

	// Optional AT
	if p.match("AT") {
		p.consume("AT")
		at, err := p.parseTime()
		if err != nil {
			return nil, err
		}
		stmt.At = at
	}

	return stmt, nil
}

// DELETE <collection>/<key>
func (p *Parser) parseDelete() (ast.Stmt, error) {
	if !p.consume("DELETE") {
		return nil, p.error("expected DELETE")
	}

	collection, key, err := p.parseCollectionKey()
	if err != nil {
		return nil, err
	}

	return &ast.Delete{
		Collection: collection,
		Key:        key,
	}, nil
}

// HISTORY <collection>/<key> [BETWEEN <time> AND <time>]
func (p *Parser) parseHistory() (ast.Stmt, error) {
	if !p.consume("HISTORY") {
		return nil, p.error("expected HISTORY")
	}

	collection, key, err := p.parseCollectionKey()
	if err != nil {
		return nil, err
	}

	stmt := &ast.History{
		Collection: collection,
		Key:        key,
	}

	// Optional BETWEEN
	if p.match("BETWEEN") {
		if !p.consume("BETWEEN") {
			return nil, p.error("expected BETWEEN")
		}
		from, err := p.parseTime()
		if err != nil {
			return nil, err
		}
		stmt.From = from

		if !p.consume("AND") {
			return nil, p.error("expected AND in BETWEEN clause")
		}

		to, err := p.parseTime()
		if err != nil {
			return nil, err
		}
		stmt.To = to
	}

	return stmt, nil
}

// RELATE <collection>/<key> -<edge-type>-> <collection>/<key> [<json-object>]
func (p *Parser) parseRelate() (ast.Stmt, error) {
	if !p.consume("RELATE") {
		return nil, p.error("expected RELATE")
	}

	fromColl, fromKey, err := p.parseCollectionKey()
	if err != nil {
		return nil, err
	}

	// Parse -<edge-type>->
	if !p.consume("-") {
		return nil, p.error("expected - in RELATE")
	}

	edgeType, err := p.parseIdentifier()
	if err != nil {
		return nil, err
	}

	if !p.consume("-") {
		return nil, p.error("expected - after edge type in RELATE")
	}
	if !p.consume(">") {
		return nil, p.error("expected > after edge type in RELATE")
	}

	toColl, toKey, err := p.parseCollectionKey()
	if err != nil {
		return nil, err
	}

	stmt := &ast.Relate{
		FromCollection: fromColl,
		FromKey:        fromKey,
		EdgeType:       edgeType,
		ToCollection:   toColl,
		ToKey:          toKey,
	}

	// Optional JSON object for properties
	if p.isTokenType(TokenJSON) {
		stmt.Props = p.advance().Value
	}

	return stmt, nil
}

// UNRELATE <collection>/<key> -<edge-type>-> <collection>/<key>
func (p *Parser) parseUnrelate() (ast.Stmt, error) {
	if !p.consume("UNRELATE") {
		return nil, p.error("expected UNRELATE")
	}

	fromColl, fromKey, err := p.parseCollectionKey()
	if err != nil {
		return nil, err
	}

	// Parse -<edge-type>->
	if !p.consume("-") {
		return nil, p.error("expected - in UNRELATE")
	}

	edgeType, err := p.parseIdentifier()
	if err != nil {
		return nil, err
	}

	if !p.consume("-") {
		return nil, p.error("expected - after edge type in UNRELATE")
	}
	if !p.consume(">") {
		return nil, p.error("expected > after edge type in UNRELATE")
	}

	toColl, toKey, err := p.parseCollectionKey()
	if err != nil {
		return nil, err
	}

	return &ast.Unrelate{
		FromCollection: fromColl,
		FromKey:        fromKey,
		EdgeType:       edgeType,
		ToCollection:   toColl,
		ToKey:          toKey,
	}, nil
}

// RELATED <collection>/<key> [-<edge-type>->] [AS OF <time>] [LIMIT <n>]
func (p *Parser) parseRelated() (ast.Stmt, error) {
	if !p.consume("RELATED") {
		return nil, p.error("expected RELATED")
	}

	collection, key, err := p.parseCollectionKey()
	if err != nil {
		return nil, err
	}

	stmt := &ast.Related{
		Collection: collection,
		Key:        key,
	}

	// Optional -<edge-type>->
	if p.match("-") {
		p.consume("-")
		edgeType, err := p.parseIdentifier()
		if err != nil {
			return nil, err
		}
		if !p.consume("-") {
			return nil, p.error("expected - after edge type in RELATED")
		}
		if !p.consume(">") {
			return nil, p.error("expected > after edge type in RELATED")
		}
		stmt.EdgeType = &edgeType
	}

	// Optional AS OF
	if p.match("AS") {
		if !p.consume("AS") {
			return nil, p.error("expected AS")
		}
		if !p.consume("OF") {
			return nil, p.error("expected OF")
		}
		asOf, err := p.parseTime()
		if err != nil {
			return nil, err
		}
		stmt.AsOf = asOf
	}

	// Optional LIMIT
	if p.match("LIMIT") {
		p.consume("LIMIT")
		limit, err := p.parseNumber()
		if err != nil {
			return nil, err
		}
		stmt.Limit = &limit
	}

	return stmt, nil
}

// SEARCH <collection> NEAR "<text>" [WHERE <expr>] [LIMIT <n>]
func (p *Parser) parseSearch() (ast.Stmt, error) {
	if !p.consume("SEARCH") {
		return nil, p.error("expected SEARCH")
	}

	collection, err := p.parseIdentifier()
	if err != nil {
		return nil, err
	}

	if !p.consume("NEAR") {
		return nil, p.error("expected NEAR in SEARCH")
	}

	// Expect quoted string
	if !p.isTokenType(TokenString) {
		return nil, p.error("expected quoted string after NEAR")
	}
	text := p.parseQuotedString()

	stmt := &ast.Search{
		Collection: collection,
		Text:       text,
	}

	// Optional WHERE
	if p.match("WHERE") {
		p.consume("WHERE")
		where, err := p.parseWhere()
		if err != nil {
			return nil, err
		}
		stmt.Where = where
	}

	// Optional LIMIT
	if p.match("LIMIT") {
		p.consume("LIMIT")
		limit, err := p.parseNumber()
		if err != nil {
			return nil, err
		}
		stmt.Limit = &limit
	}

	return stmt, nil
}

// PURGE <collection> BEFORE <time>
func (p *Parser) parsePurge() (ast.Stmt, error) {
	if !p.consume("PURGE") {
		return nil, p.error("expected PURGE")
	}

	collection, err := p.parseIdentifier()
	if err != nil {
		return nil, err
	}

	if !p.consume("BEFORE") {
		return nil, p.error("expected BEFORE in PURGE")
	}

	before, err := p.parseTime()
	if err != nil {
		return nil, err
	}

	return &ast.Purge{
		Collection: collection,
		Before:     before,
	}, nil
}

// parseWhere parses: <field> <op> <value> [AND <field> <op> <value>]*
func (p *Parser) parseWhere() (*ast.Where, error) {
	where := &ast.Where{
		Comparisons: make([]*ast.Comparison, 0),
	}

	for {
		// Parse one comparison
		field, err := p.parseIdentifier()
		if err != nil {
			return nil, err
		}

		// Parse operator
		op, err := p.parseOperator()
		if err != nil {
			return nil, err
		}

		// Parse value
		value, err := p.parseValue()
		if err != nil {
			return nil, err
		}

		where.Comparisons = append(where.Comparisons, &ast.Comparison{
			Field: field,
			Op:    op,
			Value: value,
		})

		// Check for AND or OR
		if p.match("OR") {
			return nil, p.error("OR is not supported in v1 TQL; use AND to combine conditions")
		}

		if !p.match("AND") {
			break
		}
		p.consume("AND")
	}

	return where, nil
}

// parseCollectionKey parses: <collection>/<key>
func (p *Parser) parseCollectionKey() (string, string, error) {
	collection, err := p.parseIdentifier()
	if err != nil {
		return "", "", err
	}

	if !p.consume("/") {
		return "", "", p.error("expected / after collection name")
	}

	key, err := p.parseIdentifier()
	if err != nil {
		return "", "", err
	}

	return collection, key, nil
}

// parseOperator parses: =, !=, <, <=, >, >=, IN, CONTAINS
func (p *Parser) parseOperator() (string, error) {
	if p.isEOF() {
		return "", p.error("unexpected end of input while parsing operator")
	}

	token := p.peek()
	opVal := strings.ToUpper(token.Value)
	if opVal == "IN" || opVal == "CONTAINS" {
		p.advance()
		return opVal, nil
	}

	// Check for symbolic operators
	switch token.Value {
	case "=", "!=", "<", "<=", ">", ">=":
		p.advance()
		return token.Value, nil
	default:
		return "", p.error(fmt.Sprintf("unexpected operator: %s", token.Value))
	}
}

// parseValue parses a value: identifier, number, string, or time
func (p *Parser) parseValue() (string, error) {
	if p.isEOF() {
		return "", p.error("unexpected end of input while parsing value")
	}

	token := p.peek()
	if token.Type == TokenString {
		return p.parseQuotedString(), nil
	}
	if token.Type == TokenNumber || token.Type == TokenTimestamp {
		p.advance()
		return token.Value, nil
	}
	if token.Type == TokenIdentifier {
		p.advance()
		return token.Value, nil
	}

	return "", p.error(fmt.Sprintf("unexpected value: %s", token.Value))
}

// parseIdentifier parses an identifier (alphanumeric, underscores, dots)
func (p *Parser) parseIdentifier() (string, error) {
	if p.isEOF() {
		return "", p.error("unexpected end of input while parsing identifier")
	}

	if p.isTokenType(TokenIdentifier) {
		return p.advance().Value, nil
	}

	// Allow keywords to be used as identifiers in certain contexts
	if p.isTokenType(TokenKeyword) {
		return p.advance().Value, nil
	}

	return "", p.error(fmt.Sprintf("expected identifier, got %s", p.peek().Value))
}

// parseTime parses an RFC3339 timestamp
func (p *Parser) parseTime() (*time.Time, error) {
	if p.isEOF() {
		return nil, p.error("unexpected end of input while parsing time")
	}

	if !p.isTokenType(TokenTimestamp) {
		return nil, p.error(fmt.Sprintf("expected timestamp, got %s", p.peek().Value))
	}

	timeStr := p.advance().Value
	t, err := time.Parse(time.RFC3339Nano, timeStr)
	if err != nil {
		return nil, p.error(fmt.Sprintf("invalid timestamp: %s", timeStr))
	}

	return &t, nil
}

// parseNumber parses an integer
func (p *Parser) parseNumber() (int, error) {
	if p.isEOF() {
		return 0, p.error("unexpected end of input while parsing number")
	}

	if !p.isTokenType(TokenNumber) {
		return 0, p.error(fmt.Sprintf("expected number, got %s", p.peek().Value))
	}

	numStr := p.advance().Value
	num, err := strconv.Atoi(numStr)
	if err != nil {
		return 0, p.error(fmt.Sprintf("invalid number: %s", numStr))
	}

	return num, nil
}

// parseQuotedString parses a quoted string, removing quotes and handling escapes
func (p *Parser) parseQuotedString() string {
	if p.isTokenType(TokenString) {
		value := p.advance().Value
		// Remove quotes
		if len(value) >= 2 {
			return value[1 : len(value)-1]
		}
		return value
	}
	return ""
}

// Helper methods

func (p *Parser) peek() *Token {
	if p.isEOF() {
		return &Token{Type: TokenEOF}
	}
	return p.tokens[p.pos]
}

func (p *Parser) advance() *Token {
	token := p.peek()
	if !p.isEOF() {
		p.pos++
	}
	return token
}

func (p *Parser) match(value string) bool {
	if p.isEOF() {
		return false
	}
	return strings.EqualFold(p.peek().Value, value)
}

func (p *Parser) consume(value string) bool {
	if p.match(value) {
		p.advance()
		return true
	}
	return false
}

func (p *Parser) isTokenType(t TokenType) bool {
	if p.isEOF() {
		return false
	}
	return p.peek().Type == t
}

func (p *Parser) isEOF() bool {
	return p.pos >= len(p.tokens)
}

func (p *Parser) error(reason string) *ParseError {
	token := p.peek()
	return &ParseError{
		Pos:    token.Pos,
		Token:  token,
		Input:  p.input,
		Reason: reason,
	}
}
