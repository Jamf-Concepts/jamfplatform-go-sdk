// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MIT

package client

import "strings"

// RSQLClause represents a single RSQL filter clause.
type RSQLClause struct {
	Selector              string
	Operator              string // defaults to "==" if empty
	Argument              string
	JoinWith              string // "and" or "or", defaults to "and"
	HasOpeningParenthesis bool
	HasClosingParenthesis bool
}

// BuildRSQLExpression concatenates filter clauses into an RSQL query string.
func BuildRSQLExpression(clauses []RSQLClause) string {
	if len(clauses) == 0 {
		return ""
	}

	var builtClauses []string
	var joiners []string

	for _, clause := range clauses {
		if clause.Selector == "" || clause.Argument == "" {
			continue
		}

		operator := clause.Operator
		if operator == "" {
			operator = "=="
		}

		c := clause.Selector + operator + FormatArgument(clause.Argument)
		if clause.HasOpeningParenthesis {
			c = "(" + c
		}
		if clause.HasClosingParenthesis {
			c = c + ")"
		}
		builtClauses = append(builtClauses, c)

		joinWith := "and"
		if strings.ToLower(clause.JoinWith) == "or" {
			joinWith = "or"
		}
		joiners = append(joiners, joinWith)
	}

	if len(builtClauses) == 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString(builtClauses[0])
	for i := 1; i < len(builtClauses); i++ {
		logic := "and"
		if i < len(joiners) {
			logic = joiners[i]
		}
		builder.WriteString(" ")
		builder.WriteString(logic)
		builder.WriteString(" ")
		builder.WriteString(builtClauses[i])
	}

	return builder.String()
}

// FormatArgument prepares an RSQL argument value, adding quotes/escapes when needed.
func FormatArgument(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if alreadyWrappedArgument(trimmed) || looksLikeListArgument(trimmed) {
		return trimmed
	}
	escaped := strings.ReplaceAll(trimmed, `\\`, `\\\\`)
	escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
	if argumentNeedsQuoting(trimmed) {
		return "\"" + escaped + "\""
	}
	return escaped
}

// alreadyWrappedArgument checks if the value is already enclosed in quotes.
func alreadyWrappedArgument(value string) bool {
	if len(value) < 2 {
		return false
	}
	return (strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"")) ||
		(strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'"))
}

// looksLikeListArgument checks if the value appears to be a list argument enclosed in parentheses.
func looksLikeListArgument(value string) bool {
	return strings.HasPrefix(value, "(") && strings.HasSuffix(value, ")")
}

// argumentNeedsQuoting determines if the argument contains characters that require it to be quoted.
func argumentNeedsQuoting(value string) bool {
	return strings.ContainsAny(value, " ,;()\t")
}
