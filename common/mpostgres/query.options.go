package mpostgres

import (
	"fmt"
	"strings"
)

// DefaultMaxLimit is the default max limit for any query using db.LimitOffsetQuery
// It can be overrideable by LimitOffsetQuery.MaxLimit
const DefaultMaxLimit int64 = 50

// WithLimit adds limit to a SQL query builder
var WithLimit = func(limit int64) SQLQueryBuilderOption {
	return func(b *SQLQueryBuilder) {
		b.Limit = fmt.Sprintf("LIMIT %d", limit)
	}
}

// WithLimitOffset adds limit and offset pagination style to a SQL query builder
func WithLimitOffset(limit, offset int64) SQLQueryBuilderOption {
	return func(b *SQLQueryBuilder) {
		b.Limit = fmt.Sprintf("LIMIT %d", limit)
		b.Offset = fmt.Sprintf("OFFSET %d", offset)
	}
}

// WithSort sorts the results by given sort argument
var WithSort = func(field string, order string) SQLQueryBuilderOption {
	return func(b *SQLQueryBuilder) {
		b.Sorts = append(b.Sorts, fmt.Sprintf("%s %s", field, strings.ToUpper(order)))
	}
}

// WithTextSearch searches for docs using PostgreSQL full-text search
var WithTextSearch = func(field, text string) SQLQueryBuilderOption {
	return func(b *SQLQueryBuilder) {
		if text != "" {
			b.Where = append(b.Where, fmt.Sprintf("%s @@ to_tsquery('%s')", field, text))
		}
	}
}

// WithFilter adds a new filter condition to the query builder
func WithFilter(column string, value any) SQLQueryBuilderOption {
	return func(b *SQLQueryBuilder) {
		b.Params = append(b.Params, value)
		b.Where = append(b.Where, fmt.Sprintf("%s = $%d", column, len(b.Params)))
	}
}
