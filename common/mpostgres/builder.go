package mpostgres

// SQLQueryBuilderOption is a kind of interface for build options
type SQLQueryBuilderOption func(b *SQLQueryBuilder)

// SQLQueryBuilder builds a query for PostgreSQL
type SQLQueryBuilder struct {
	Params []any
	Where  []string
	Sorts  []string
	Table  string
	Limit  string
	Offset string
}

// NewSQLQueryBuilder creates an instance of SQLQueryBuilder
func NewSQLQueryBuilder(table string, opts ...SQLQueryBuilderOption) *SQLQueryBuilder {
	builder := &SQLQueryBuilder{
		Table: table,
	}
	for _, opt := range opts {
		opt(builder)
	}

	return builder
}

// With adds a new option to query builder
func (q *SQLQueryBuilder) With(opt SQLQueryBuilderOption) {
	opt(q)
}
