// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package constant

// Parsing and splitting constants for key=value, schema.table, and similar patterns.
const (
	// SplitKeyValueParts is the number of parts when splitting key=value pairs (key and value).
	SplitKeyValueParts = 2

	// MinPathParts is the minimum number of path segments for schema.table or database.collection references.
	MinPathParts = 2

	// MinQuotedStringLength is the minimum length for a string to be considered quoted (e.g., "" or '').
	MinQuotedStringLength = 2

	// BetweenOperatorValues is the exact number of values required for a between filter operator.
	BetweenOperatorValues = 2

	// DateOnlyStringLength is the length of a YYYY-MM-DD date string.
	DateOnlyStringLength = 10

	// MaxSchemaPreviewKeys is the maximum number of keys to include in a schema preview.
	MaxSchemaPreviewKeys = 3

	// MatchGroupsWithByClause is the expected number of regex match groups for aggregation expressions with a "by" clause.
	MatchGroupsWithByClause = 4

	// MatchGroupsSimple is the expected number of regex match groups for simple aggregation expressions.
	MatchGroupsSimple = 3

	// MinByClauseArgs is the minimum number of arguments for an aggregation expression with a "by" clause.
	MinByClauseArgs = 4

	// PowerOperatorTokenLength is the number of characters consumed by the ** (power) operator.
	PowerOperatorTokenLength = 2
)
