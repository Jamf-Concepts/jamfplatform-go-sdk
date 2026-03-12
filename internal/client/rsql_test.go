// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package client

import "testing"

func TestBuildRSQLExpression(t *testing.T) {
	tests := []struct {
		name     string
		clauses  []RSQLClause
		expected string
	}{
		{
			name:     "empty clauses",
			clauses:  nil,
			expected: "",
		},
		{
			name: "single clause default operator",
			clauses: []RSQLClause{
				{Selector: "name", Argument: "test"},
			},
			expected: `name==test`,
		},
		{
			name: "single clause custom operator",
			clauses: []RSQLClause{
				{Selector: "name", Operator: "!=", Argument: "test"},
			},
			expected: `name!=test`,
		},
		{
			name: "two clauses default and join",
			clauses: []RSQLClause{
				{Selector: "name", Argument: "a"},
				{Selector: "type", Argument: "b"},
			},
			expected: `name==a and type==b`,
		},
		{
			name: "or join",
			clauses: []RSQLClause{
				{Selector: "name", Argument: "a"},
				{Selector: "type", Argument: "b", JoinWith: "or"},
			},
			expected: `name==a or type==b`,
		},
		{
			name: "parentheses",
			clauses: []RSQLClause{
				{Selector: "a", Argument: "1", HasOpeningParenthesis: true},
				{Selector: "b", Argument: "2", JoinWith: "or", HasClosingParenthesis: true},
				{Selector: "c", Argument: "3"},
			},
			expected: `(a==1 or b==2) and c==3`,
		},
		{
			name: "skip empty selector",
			clauses: []RSQLClause{
				{Selector: "", Argument: "val"},
				{Selector: "name", Argument: "test"},
			},
			expected: `name==test`,
		},
		{
			name: "skip empty argument",
			clauses: []RSQLClause{
				{Selector: "name", Argument: ""},
				{Selector: "type", Argument: "ok"},
			},
			expected: `type==ok`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildRSQLExpression(tt.clauses)
			if got != tt.expected {
				t.Errorf("BuildRSQLExpression() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestFormatArgument(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty", "", ""},
		{"whitespace only", "   ", ""},
		{"simple value", "hello", "hello"},
		{"value with space", "hello world", `"hello world"`},
		{"already double quoted", `"hello"`, `"hello"`},
		{"already single quoted", `'hello'`, `'hello'`},
		{"list argument", "(a,b,c)", "(a,b,c)"},
		{"value with comma", "a,b", `"a,b"`},
		{"value with parens", "a(b)", `"a(b)"`},
		{"value with embedded quote", `say "hi"`, `"say \"hi\""`},
		{"value with single backslash", `a\b`, `a\b`},
		{"value with double backslash", `a\\b`, `a\\\\b`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatArgument(tt.input)
			if got != tt.expected {
				t.Errorf("FormatArgument(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
