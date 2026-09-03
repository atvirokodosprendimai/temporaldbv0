package tql

import (
	"fmt"
	"unicode"
	"unicode/utf8"
)

// TokenType represents the type of a token.
type TokenType int

const (
	TokenEOF TokenType = iota
	TokenError
	TokenKeyword
	TokenIdentifier
	TokenString
	TokenNumber
	TokenTimestamp
	TokenPunct // /, -, >, etc
	TokenJSON  // {...}
)

// Token represents a lexical token.
type Token struct {
	Type  TokenType
	Value string
	Pos   int
}

// Lexer tokenizes TQL input.
type Lexer struct {
	input  string
	pos    int
	start  int
	tokens []*Token
}

// NewLexer creates a new lexer for the given input.
func NewLexer(input string) *Lexer {
	return &Lexer{
		input:  input,
		pos:    0,
		start:  0,
		tokens: make([]*Token, 0),
	}
}

// Tokenize lexes the entire input and returns all tokens.
func (l *Lexer) Tokenize() ([]*Token, error) {
	for !l.isEOF() {
		l.skipWhitespace()
		if l.isEOF() {
			break
		}

		l.start = l.pos
		ch := l.peek()

		// Handle JSON objects
		if ch == '{' {
			if err := l.scanJSON(); err != nil {
				return nil, err
			}
			continue
		}

		// Handle quoted strings
		if ch == '"' {
			if err := l.scanString(); err != nil {
				return nil, err
			}
			continue
		}

		// Handle multi-character operators
		if ch == '!' {
			l.next()
			if l.peek() == '=' {
				l.next()
				l.emit(TokenKeyword)
			} else {
				return nil, fmt.Errorf("unexpected character: ! at position %d", l.start)
			}
			continue
		}

		if ch == '<' || ch == '>' {
			l.next()
			if l.peek() == '=' {
				l.next()
			}
			l.emit(TokenKeyword)
			continue
		}

		if ch == '=' {
			l.next()
			l.emit(TokenKeyword)
			continue
		}

		// Handle punctuation
		if ch == '/' || ch == '-' || ch == '>' || ch == '(' || ch == ')' || ch == '[' || ch == ']' {
			l.next()
			l.emit(TokenPunct)
			continue
		}

		// Handle numbers and timestamps
		if unicode.IsDigit(rune(ch)) {
			l.scanTimestampOrNumber()
			continue
		}

		// Handle identifiers and keywords
		if unicode.IsLetter(rune(ch)) || ch == '_' {
			l.scanIdentifier()
			continue
		}

		return nil, fmt.Errorf("unexpected character: %c at position %d", ch, l.pos)
	}

	l.tokens = append(l.tokens, &Token{Type: TokenEOF, Value: "", Pos: l.pos})
	return l.tokens, nil
}

func (l *Lexer) scanIdentifier() {
	for !l.isEOF() {
		ch := l.peek()
		if !unicode.IsLetter(rune(ch)) && !unicode.IsDigit(rune(ch)) && ch != '_' && ch != '.' {
			break
		}
		l.next()
	}

	value := l.input[l.start:l.pos]
	tokenType := TokenKeyword
	if !isKeyword(value) {
		tokenType = TokenIdentifier
	}
	l.tokens = append(l.tokens, &Token{Type: tokenType, Value: value, Pos: l.start})
}

func (l *Lexer) scanTimestampOrNumber() {
	for !l.isEOF() {
		ch := l.peek()
		if !unicode.IsDigit(rune(ch)) && ch != '.' && ch != '-' && ch != '+' && ch != 'T' && ch != 'Z' && ch != ':' &&
			(ch < 'a' || ch > 'z') && (ch < 'A' || ch > 'Z') {
			break
		}
		l.next()
	}

	value := l.input[l.start:l.pos]
	// Determine if this is a timestamp or number
	tokenType := TokenNumber
	if isTimestamp(value) {
		tokenType = TokenTimestamp
	}
	l.tokens = append(l.tokens, &Token{Type: tokenType, Value: value, Pos: l.start})
}

func (l *Lexer) scanString() error {
	l.next() // consume opening quote

	for !l.isEOF() {
		ch := l.peek()
		if ch == '"' {
			l.next() // consume closing quote
			value := l.input[l.start:l.pos]
			l.tokens = append(l.tokens, &Token{Type: TokenString, Value: value, Pos: l.start})
			return nil
		}
		if ch == '\\' {
			l.next() // consume backslash
			if l.isEOF() {
				return fmt.Errorf("unterminated string at position %d", l.start)
			}
			l.next() // consume escaped character
		} else {
			l.next()
		}
	}

	return fmt.Errorf("unterminated string at position %d", l.start)
}

func (l *Lexer) scanJSON() error {
	depth := 1 // We've already started with the opening brace
	l.next()   // consume opening brace

	for !l.isEOF() {
		ch := l.peek()
		if ch == '{' {
			depth++
			l.next()
		} else if ch == '}' {
			l.next()
			depth--
			if depth == 0 {
				value := l.input[l.start:l.pos]
				l.tokens = append(l.tokens, &Token{Type: TokenJSON, Value: value, Pos: l.start})
				return nil
			}
		} else if ch == '"' {
			// Handle strings inside JSON
			l.next() // consume opening quote
			for !l.isEOF() {
				if l.peek() == '"' {
					l.next()
					break
				}
				if l.peek() == '\\' {
					l.next()
					if !l.isEOF() {
						l.next()
					}
				} else {
					l.next()
				}
			}
		} else {
			l.next()
		}
	}

	return fmt.Errorf("unterminated JSON object at position %d", l.start)
}

func (l *Lexer) emit(t TokenType) {
	value := l.input[l.start:l.pos]
	l.tokens = append(l.tokens, &Token{Type: t, Value: value, Pos: l.start})
}

func (l *Lexer) skipWhitespace() {
	for !l.isEOF() && unicode.IsSpace(rune(l.peek())) {
		l.next()
	}
}

func (l *Lexer) peek() byte {
	if l.isEOF() {
		return 0
	}
	return l.input[l.pos]
}

func (l *Lexer) next() {
	if !l.isEOF() {
		_, width := utf8.DecodeRuneInString(l.input[l.pos:])
		l.pos += width
	}
}

func (l *Lexer) isEOF() bool {
	return l.pos >= len(l.input)
}

// isKeyword returns true if the word is a TQL keyword.
func isKeyword(word string) bool {
	keywords := map[string]bool{
		"GET":      true,
		"FIND":     true,
		"PUT":      true,
		"DELETE":   true,
		"HISTORY":  true,
		"RELATE":   true,
		"UNRELATE": true,
		"RELATED":  true,
		"SEARCH":   true,
		"PURGE":    true,
		"WHERE":    true,
		"AS":       true,
		"OF":       true,
		"AT":       true,
		"BETWEEN":  true,
		"AND":      true,
		"OR":       true,
		"LIMIT":    true,
		"ORDER":    true,
		"BY":       true,
		"ASC":      true,
		"DESC":     true,
		"IN":       true,
		"CONTAINS": true,
		"NEAR":     true,
		"BEFORE":   true,
	}
	return keywords[word]
}

// isTimestamp returns true if the string looks like an RFC3339 timestamp.
func isTimestamp(s string) bool {
	if len(s) < 19 { // minimum length for 2026-09-03T10:00:00Z
		return false
	}
	// Check for RFC3339 format: YYYY-MM-DDTHH:MM:SS[.nnnnnnnnn]Z
	if s[4] == '-' && s[7] == '-' && s[10] == 'T' && s[13] == ':' && s[16] == ':' {
		if s[len(s)-1] == 'Z' || (len(s) > 19 && s[19] == '.') {
			return true
		}
	}
	return false
}
