package query

import (
	"testing"
)

func TestLexer(t *testing.T) {
	tests := []struct {
		input    string
		expected []TokenType
	}{
		// Simple field expression
		{"status = open", []TokenType{TokenIdent, TokenEq, TokenIdent, TokenEOF}},
		// Operators
		{"priority <= P1", []TokenType{TokenIdent, TokenLte, TokenIdent, TokenEOF}},
		{"points >= 5", []TokenType{TokenIdent, TokenGte, TokenNumber, TokenEOF}},
		{"status != closed", []TokenType{TokenIdent, TokenNeq, TokenIdent, TokenEOF}},
		{"title ~ auth", []TokenType{TokenIdent, TokenContains, TokenIdent, TokenEOF}},
		{"title !~ test", []TokenType{TokenIdent, TokenNotContains, TokenIdent, TokenEOF}},
		// Boolean operators
		{"a AND b", []TokenType{TokenIdent, TokenAnd, TokenIdent, TokenEOF}},
		{"a OR b", []TokenType{TokenIdent, TokenOr, TokenIdent, TokenEOF}},
		{"NOT a", []TokenType{TokenNot, TokenIdent, TokenEOF}},
		{"a && b", []TokenType{TokenIdent, TokenAnd, TokenIdent, TokenEOF}},
		{"a || b", []TokenType{TokenIdent, TokenOr, TokenIdent, TokenEOF}},
		{"-status = open", []TokenType{TokenNot, TokenIdent, TokenEq, TokenIdent, TokenEOF}},
		// Quoted strings
		{`title ~ "hello world"`, []TokenType{TokenIdent, TokenContains, TokenString, TokenEOF}},
		{`title ~ 'single quotes'`, []TokenType{TokenIdent, TokenContains, TokenString, TokenEOF}},
		// Special values
		{"implementer = @me", []TokenType{TokenIdent, TokenEq, TokenAtMe, TokenEOF}},
		{"labels = EMPTY", []TokenType{TokenIdent, TokenEq, TokenEmpty, TokenEOF}},
		// Dates
		{"created >= 2024-01-15", []TokenType{TokenIdent, TokenGte, TokenDate, TokenEOF}},
		{"updated >= -7d", []TokenType{TokenIdent, TokenGte, TokenDate, TokenEOF}},
		{"created = today", []TokenType{TokenIdent, TokenEq, TokenDate, TokenEOF}},
		{"updated >= this_week", []TokenType{TokenIdent, TokenGte, TokenDate, TokenEOF}},
		// Dot notation
		{"log.message ~ fix", []TokenType{TokenIdent, TokenDot, TokenIdent, TokenContains, TokenIdent, TokenEOF}},
		// Functions
		{"has(labels)", []TokenType{TokenIdent, TokenLParen, TokenIdent, TokenRParen, TokenEOF}},
		{"is(open)", []TokenType{TokenIdent, TokenLParen, TokenIdent, TokenRParen, TokenEOF}},
		// Grouping
		{"(a AND b) OR c", []TokenType{TokenLParen, TokenIdent, TokenAnd, TokenIdent, TokenRParen, TokenOr, TokenIdent, TokenEOF}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			lexer := NewLexer(tt.input)
			tokens, err := lexer.Tokenize()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(tokens) != len(tt.expected) {
				t.Fatalf("expected %d tokens, got %d: %v", len(tt.expected), len(tokens), tokens)
			}

			for i, tok := range tokens {
				if tok.Type != tt.expected[i] {
					t.Errorf("token %d: expected %v, got %v", i, tt.expected[i], tok.Type)
				}
			}
		})
	}
}

func TestParser(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
	}{
		// Simple expressions
		{"status = open", false},
		{"priority <= P1", false},
		{"points >= 5", false},
		{"title ~ auth", false},

		// Boolean expressions
		{"status = open AND type = bug", false},
		{"status = open OR status = blocked", false},
		{"NOT status = closed", false},
		{"-status = closed", false},

		// Complex expressions
		{"status = open AND type = bug AND priority <= P1", false},
		{"(status = open OR status = blocked) AND type = bug", false},
		{"type = bug AND (priority = P0 OR priority = P1)", false},

		// Functions
		{"has(labels)", false},
		{"is(open)", false},
		{"any(type, bug, feature)", false},
		{"descendant_of(td-abc123)", false},

		// Cross-entity
		{"log.message ~ fixed", false},
		{"log.type = blocker", false},
		{"comment.text ~ approved", false},

		// Text search
		{`"authentication"`, false},
		{"auth", false}, // bare word becomes text search

		// Special values
		{"implementer = @me", false},
		{"labels = EMPTY", false},

		// Dates
		{"created >= 2024-01-15", false},
		{"updated >= -7d", false},
		{"created = today", false},

		// Implicit AND
		{"status = open type = bug", false},

		// Edge cases
		{"", false}, // empty query
		{"((status = open))", false},

		// Errors
		{"status = ", true},     // missing value
		{"(status = open", true}, // unclosed paren
		{"= open", true},        // missing field
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			query, err := Parse(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if query == nil {
					t.Errorf("expected query, got nil")
				}
			}
		})
	}
}

func TestParserAST(t *testing.T) {
	tests := []struct {
		input    string
		checkAST func(t *testing.T, n Node)
	}{
		{
			input: "status = open",
			checkAST: func(t *testing.T, n Node) {
				fe, ok := n.(*FieldExpr)
				if !ok {
					t.Fatalf("expected FieldExpr, got %T", n)
				}
				if fe.Field != "status" {
					t.Errorf("field: expected 'status', got %q", fe.Field)
				}
				if fe.Operator != "=" {
					t.Errorf("operator: expected '=', got %q", fe.Operator)
				}
				if fe.Value != "open" {
					t.Errorf("value: expected 'open', got %v", fe.Value)
				}
			},
		},
		{
			input: "status = open AND type = bug",
			checkAST: func(t *testing.T, n Node) {
				be, ok := n.(*BinaryExpr)
				if !ok {
					t.Fatalf("expected BinaryExpr, got %T", n)
				}
				if be.Op != "AND" {
					t.Errorf("op: expected 'AND', got %q", be.Op)
				}
			},
		},
		{
			input: "NOT status = closed",
			checkAST: func(t *testing.T, n Node) {
				ue, ok := n.(*UnaryExpr)
				if !ok {
					t.Fatalf("expected UnaryExpr, got %T", n)
				}
				if ue.Op != "NOT" {
					t.Errorf("op: expected 'NOT', got %q", ue.Op)
				}
			},
		},
		{
			input: "has(labels)",
			checkAST: func(t *testing.T, n Node) {
				fn, ok := n.(*FunctionCall)
				if !ok {
					t.Fatalf("expected FunctionCall, got %T", n)
				}
				if fn.Name != "has" {
					t.Errorf("name: expected 'has', got %q", fn.Name)
				}
				if len(fn.Args) != 1 {
					t.Errorf("args: expected 1, got %d", len(fn.Args))
				}
			},
		},
		{
			input: "log.message ~ fixed",
			checkAST: func(t *testing.T, n Node) {
				fe, ok := n.(*FieldExpr)
				if !ok {
					t.Fatalf("expected FieldExpr, got %T", n)
				}
				if fe.Field != "log.message" {
					t.Errorf("field: expected 'log.message', got %q", fe.Field)
				}
			},
		},
		{
			input: `"search text"`,
			checkAST: func(t *testing.T, n Node) {
				ts, ok := n.(*TextSearch)
				if !ok {
					t.Fatalf("expected TextSearch, got %T", n)
				}
				if ts.Text != "search text" {
					t.Errorf("text: expected 'search text', got %q", ts.Text)
				}
			},
		},
		{
			input: "implementer = @me",
			checkAST: func(t *testing.T, n Node) {
				fe, ok := n.(*FieldExpr)
				if !ok {
					t.Fatalf("expected FieldExpr, got %T", n)
				}
				sv, ok := fe.Value.(*SpecialValue)
				if !ok {
					t.Fatalf("expected SpecialValue, got %T", fe.Value)
				}
				if sv.Type != "me" {
					t.Errorf("type: expected 'me', got %q", sv.Type)
				}
			},
		},
		{
			input: "created >= -7d",
			checkAST: func(t *testing.T, n Node) {
				fe, ok := n.(*FieldExpr)
				if !ok {
					t.Fatalf("expected FieldExpr, got %T", n)
				}
				dv, ok := fe.Value.(*DateValue)
				if !ok {
					t.Fatalf("expected DateValue, got %T", fe.Value)
				}
				if dv.Raw != "-7d" {
					t.Errorf("raw: expected '-7d', got %q", dv.Raw)
				}
				if !dv.Relative {
					t.Error("expected Relative to be true")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			query, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if query.Root == nil {
				t.Fatal("expected non-nil root")
			}
			tt.checkAST(t, query.Root)
		})
	}
}

func TestQueryValidation(t *testing.T) {
	tests := []struct {
		input      string
		wantErrors int
	}{
		// Valid queries
		{"status = open", 0},
		{"type = bug", 0},
		{"priority <= P1", 0},
		{"log.message ~ fix", 0},
		{"has(labels)", 0},

		// Invalid field
		{"stauts = open", 1}, // typo
		{"foo = bar", 1},    // unknown field

		// Invalid enum value
		{"status = unknown", 1},
		{"type = unknown", 1},

		// Invalid function
		{"unknown_func(x)", 1},
		{"has()", 1}, // missing arg
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			query, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			errs := query.Validate()
			if len(errs) != tt.wantErrors {
				t.Errorf("expected %d errors, got %d: %v", tt.wantErrors, len(errs), errs)
			}
		})
	}
}

func TestOperatorPrecedence(t *testing.T) {
	// NOT > AND > OR
	tests := []struct {
		input    string
		expected string // String representation showing structure
	}{
		{
			input:    "a OR b AND c",
			expected: `("a" OR ("b" AND "c"))`, // AND binds tighter
		},
		{
			input:    "NOT a AND b",
			expected: `((NOT "a") AND "b")`, // NOT binds tightest
		},
		{
			input:    "(a OR b) AND c",
			expected: `(("a" OR "b") AND "c")`, // parens override
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			query, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			got := query.String()
			if got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}
