// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"strings"

	fetcher "github.com/LerianStudio/fetcher/pkg/engine"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/constant"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/model"
	"github.com/Masterminds/squirrel"
)

// postgresFactory builds PostgreSQL connectors. Build is side-effect free with
// respect to the network: it resolves the per-tenant connection but defers the
// connectivity check to Connector.TestConnection, per the engine's factory
// contract.
type postgresFactory struct {
	resolver TenantResolver
	breaker  CircuitBreaker
}

// Compile-time check that postgresFactory satisfies the engine port.
var _ fetcher.ConnectorFactory = (*postgresFactory)(nil)

// Build resolves the per-tenant PostgreSQL connection from the tenant ID stamped
// in the descriptor's HostAttributes and constructs a connector over it. It
// opens no network connection. A malformed descriptor (missing config name, or
// missing tenant in multi-tenant mode) yields a CategoryValidation error.
func (f *postgresFactory) Build(ctx context.Context, descriptor fetcher.ConnectionDescriptor) (fetcher.Connector, error) {
	if descriptor.ConfigName == "" {
		return nil, NewEngineValidationError("connection descriptor is missing config name")
	}

	tenantID := tenantIDFromDescriptor(descriptor)

	handle, err := f.resolver.ResolvePostgres(ctx, tenantID, descriptor.ConfigName)
	if err != nil {
		return nil, err
	}

	// Symmetric with mongoFactory.Build: a resolver that returns a nil handle
	// without an error would otherwise yield a connector that nil-derefs on the
	// first PingContext/QueryContext inside the breaker. The single-tenant
	// resolver also rejects a concrete nil *sql.DB at its boundary (where the
	// typed-nil is still visible), since a nil *sql.DB wrapped in sqlQuerier is
	// not == nil here.
	if handle.db == nil {
		return nil, NewEngineUnavailableError("resolved postgres connection is nil", nil)
	}

	schemas := schemaOverrideFromDescriptor(descriptor)
	if len(schemas) == 0 {
		schemas = handle.schemas
	}

	if len(schemas) == 0 {
		schemas = []string{"public"}
	}

	return &postgresConnector{
		configName: descriptor.ConfigName,
		db:         handle.db,
		schemas:    schemas,
		breaker:    f.breaker,
	}, nil
}

// postgresConnector is a single-flight PostgreSQL connector over an already
// resolved (tenant-scoped) read surface. It reuses the reporter's existing
// connection pool — it never opens its own pool — so embedding inherits the
// host's pooling and circuit-breaker policy.
type postgresConnector struct {
	configName string
	db         sqlQuerier
	schemas    []string
	breaker    CircuitBreaker
}

// Compile-time check that postgresConnector satisfies the engine port.
var _ fetcher.Connector = (*postgresConnector)(nil)

// TestConnection performs the connectivity check via PingContext, run through
// the per-datasource circuit breaker.
func (c *postgresConnector) TestConnection(ctx context.Context) error {
	_, err := c.breaker.Execute(c.configName, func() (any, error) {
		return nil, c.db.PingContext(ctx)
	})
	if err != nil {
		return NewEngineConnectError("postgres connectivity check failed", err)
	}

	return nil
}

// DiscoverSchema reads the datasource's tables and columns from
// information_schema across the configured schemas and returns a secret-free
// snapshot. It reuses the same information_schema introspection the reporter's
// postgres repository performs, qualified-table-named to match extraction
// requests.
func (c *postgresConnector) DiscoverSchema(ctx context.Context) (fetcher.SchemaSnapshot, error) {
	result, err := c.breaker.Execute(c.configName, func() (any, error) {
		return c.discoverSchema(ctx)
	})
	if err != nil {
		return fetcher.SchemaSnapshot{}, err
	}

	snapshot, _ := result.(fetcher.SchemaSnapshot)

	return snapshot, nil
}

func (c *postgresConnector) discoverSchema(ctx context.Context) (fetcher.SchemaSnapshot, error) {
	const q = `
		SELECT table_schema, table_name, column_name
		FROM information_schema.columns
		WHERE table_schema = ANY($1)
		ORDER BY table_schema, table_name, ordinal_position
	`

	rows, err := c.db.QueryContext(ctx, q, pqStringArray(c.schemas))
	if err != nil {
		return fetcher.SchemaSnapshot{}, classifyQueryError(ctx, "postgres schema discovery failed", err)
	}
	defer func() { _ = rows.Close() }()

	tables := make(map[string][]string)

	for rows.Next() {
		var schemaName, tableName, columnName string
		if err := rows.Scan(&schemaName, &tableName, &columnName); err != nil {
			return fetcher.SchemaSnapshot{}, NewEngineInternalError("failed to scan schema row", err)
		}

		qualified := schemaName + "." + tableName
		tables[qualified] = append(tables[qualified], columnName)
	}

	if err := rows.Err(); err != nil {
		return fetcher.SchemaSnapshot{}, classifyQueryError(ctx, "postgres schema iteration failed", err)
	}

	return buildSnapshot(c.configName, tables), nil
}

// QueryStream translates the request's per-table field selection into one
// SELECT per table and returns a cursor that streams rows lazily. It reads
// Fields for this datasource's config name; an empty selection yields an empty
// cursor. The cursor opens the first table's rows eagerly so connectivity and
// SQL errors surface synchronously, then advances table-by-table as it drains —
// peak memory stays bounded to one row plus the driver's own buffer.
func (c *postgresConnector) QueryStream(ctx context.Context, request fetcher.ExtractionRequest) (fetcher.RowCursor, error) {
	filters, err := filtersForDatasource(c.configName, request.Filters)
	if err != nil {
		return nil, err
	}

	selection := request.MappedFields[c.configName]

	tables := make([]string, 0, len(selection))
	for table := range selection {
		tables = append(tables, table)
	}

	sort.Strings(tables)

	// Discover the schema once so per-table filter field references can be
	// validated against the real columns; an unknown field must error loudly
	// rather than silently widen the result. Only paid when filters are present.
	var snapshot fetcher.SchemaSnapshot
	if len(filters) > 0 {
		snapshot, err = c.DiscoverSchema(ctx)
		if err != nil {
			return nil, err
		}
	}

	cursor := &postgresCursor{
		connector: c,
		ctx:       ctx,
		selection: selection,
		filters:   filters,
		schema:    snapshot,
		tables:    tables,
	}

	// Prime the first table so a SQL/connectivity failure is returned from
	// QueryStream rather than swallowed on the first Next.
	if err := cursor.openNextTable(); err != nil {
		_ = cursor.Close(ctx)
		return nil, err
	}

	return cursor, nil
}

// Close releases the connector. The underlying pool is owned by the host
// (reporter datasources / tenant manager), so Close is a no-op here: closing a
// shared, tenant-scoped pool on a single extraction would tear down a resource
// other in-flight extractions share. It is idempotent and double-close safe.
func (c *postgresConnector) Close(_ context.Context) error { return nil }

// postgresCursor streams rows across the request's selected tables, one row at a
// time, opening each table's *sql.Rows only when the previous table is
// exhausted. It is single-flight: not safe for concurrent use, matching the
// engine's RowCursor contract.
type postgresCursor struct {
	connector *postgresConnector
	ctx       context.Context
	selection fetcher.FieldSelection
	filters   datasourceFilters
	schema    fetcher.SchemaSnapshot
	tables    []string
	tableIdx  int

	rows       *sql.Rows
	columns    []string
	currentTbl string
	currentRow map[string]any
	hasRow     bool
	exhausted  bool
	err        error
}

// Compile-time check that postgresCursor satisfies the engine port.
var _ fetcher.RowCursor = (*postgresCursor)(nil)

// Next advances to the next row, transparently rolling over to the next selected
// table when the current table's rows are exhausted. It honors context
// cancellation, recording the error so Err can distinguish a cancelled stream
// from a clean end-of-stream.
func (c *postgresCursor) Next(ctx context.Context) bool {
	if c.err != nil || c.exhausted {
		return false
	}

	if ctx.Err() != nil {
		c.fail(classifyQueryError(ctx, "postgres stream canceled", ctx.Err()))
		return false
	}

	for {
		if c.rows == nil {
			c.exhausted = true
			c.hasRow = false

			return false
		}

		if c.rows.Next() {
			row, err := c.scanRow()
			if err != nil {
				c.fail(err)
				return false
			}

			c.currentRow = row
			c.hasRow = true

			return true
		}

		// Current table drained: surface any iteration error, then advance.
		if err := c.rows.Err(); err != nil {
			c.fail(classifyQueryError(ctx, "postgres row iteration failed", err))
			return false
		}

		_ = c.rows.Close()
		c.rows = nil

		if err := c.openNextTable(); err != nil {
			c.fail(err)
			return false
		}
	}
}

// Row returns the current (qualified table, row) pair. It is valid only after
// Next returned true.
func (c *postgresCursor) Row() (string, map[string]any) {
	if !c.hasRow {
		return "", nil
	}

	return c.currentTbl, c.currentRow
}

// Err returns the first non-EOF error encountered (including a context
// cancellation observed mid-stream), or nil for a clean drain.
func (c *postgresCursor) Err() error { return c.err }

// Close releases the open *sql.Rows. It is idempotent and double-close safe. The
// shared pool is not closed (see postgresConnector.Close).
func (c *postgresCursor) Close(_ context.Context) error {
	if c.rows != nil {
		err := c.rows.Close()
		c.rows = nil

		if err != nil && c.err == nil {
			return err
		}
	}

	c.hasRow = false
	c.exhausted = true

	return nil
}

// openNextTable opens the *sql.Rows for the next selected table, skipping tables
// with no selected fields. It sets c.rows to nil and returns nil when no tables
// remain (a clean end-of-stream).
func (c *postgresCursor) openNextTable() error {
	for c.tableIdx < len(c.tables) {
		qualified := c.tables[c.tableIdx]
		c.tableIdx++

		fields := c.selection[qualified]
		if len(fields) == 0 {
			continue
		}

		query, args, err := buildPostgresSelect(qualified, fields, c.filters.tableFilters(qualified), c.schema)
		if err != nil {
			return NewEngineValidationError("failed to build postgres select: " + err.Error())
		}

		result, execErr := c.connector.breaker.Execute(c.connector.configName, func() (any, error) {
			return c.connector.db.QueryContext(c.ctx, query, args...)
		})
		if execErr != nil {
			return classifyQueryError(c.ctx, "postgres query failed", execErr)
		}

		rows, _ := result.(*sql.Rows)

		cols, colErr := rows.Columns()
		if colErr != nil {
			_ = rows.Close()
			return NewEngineInternalError("failed to read postgres columns", colErr)
		}

		c.rows = rows
		c.columns = cols
		c.currentTbl = qualified

		return nil
	}

	c.rows = nil

	return nil
}

// scanRow scans the current *sql.Rows into a column->value map, decoding JSONB
// payloads the same way the reporter's existing scan path does so embedded
// extraction produces identical row shapes.
func (c *postgresCursor) scanRow() (map[string]any, error) {
	values := make([]any, len(c.columns))
	pointers := make([]any, len(c.columns))

	for i := range values {
		pointers[i] = &values[i]
	}

	if err := c.rows.Scan(pointers...); err != nil {
		return nil, NewEngineInternalError("failed to scan postgres row", err)
	}

	row := make(map[string]any, len(c.columns))
	for i, col := range c.columns {
		row[col] = decodeMaybeJSON(values[i])
	}

	return row, nil
}

func (c *postgresCursor) fail(err error) {
	if c.err == nil {
		c.err = err
	}

	c.exhausted = true
	c.hasRow = false

	if c.rows != nil {
		_ = c.rows.Close()
		c.rows = nil
	}
}

// buildPostgresSelect assembles a SELECT over the qualified table using
// squirrel with PostgreSQL dollar placeholders. Field selection projects only
// the root column for nested JSONB paths (e.g. "fee_charge.totalAmount" selects
// "fee_charge"), mirroring the reporter's existing projection. A "*" selection
// projects all columns. Per-field filter conditions are translated into WHERE
// clauses via applyPostgresFilters, validated against the discovered schema.
func buildPostgresSelect(qualified string, fields []string, filters map[string]model.FilterCondition, schema fetcher.SchemaSnapshot) (string, []any, error) {
	schemaName, tableName := splitQualified(qualified)
	target := quoteIdentifier(tableName)

	if schemaName != "" {
		target = quoteIdentifier(schemaName) + "." + quoteIdentifier(tableName)
	}

	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)

	cols := projectColumns(fields)
	if len(cols) == 0 {
		cols = []string{"*"}
	}

	builder := psql.Select(cols...).From(target)

	builder, err := applyPostgresFilters(builder, qualified, filters, schema)
	if err != nil {
		return "", nil, err
	}

	return builder.ToSql()
}

// applyPostgresFilters translates FilterCondition criteria into WHERE clauses on
// the squirrel builder, reusing the exact operator semantics of
// pkg/reporter/postgres applyAdvancedFilter: Equals (single -> Eq, multi -> IN),
// Gt/GtOrEq/Lt/LtOrEq, Between with date-range upper-bound expansion, In -> Eq,
// NotIn -> NotEq. Every filter field is validated against the discovered schema
// columns; an unknown field is a loud error so a mis-referenced filter can never
// silently widen the result. Fields are applied in sorted order for
// deterministic SQL. Empty conditions are skipped.
func applyPostgresFilters(builder squirrel.SelectBuilder, qualified string, filters map[string]model.FilterCondition, schema fetcher.SchemaSnapshot) (squirrel.SelectBuilder, error) {
	if len(filters) == 0 {
		return builder, nil
	}

	validColumns := validColumnSet(schema, qualified)

	fields := make([]string, 0, len(filters))
	for field := range filters {
		fields = append(fields, field)
	}

	sort.Strings(fields)

	for _, field := range fields {
		condition := filters[field]
		if isPostgresFilterEmpty(condition) {
			continue
		}

		// Charset whitelist BEFORE the field reaches the squirrel map key (emitted
		// verbatim and unquoted): the root-column check below validates only the
		// dotted root, so an injection-shaped path must be rejected here.
		if err := validateFilterField(field); err != nil {
			return builder, err
		}

		if _, ok := validColumns[rootField(field)]; !ok {
			return builder, NewEngineValidationError("unknown filter field " + field + " for table " + qualified)
		}

		if err := validateFilterCondition(field, condition); err != nil {
			return builder, err
		}

		builder = applyPostgresFilterCondition(builder, field, condition)
	}

	return builder, nil
}

// applyPostgresFilterCondition applies a single FilterCondition to the builder,
// mirroring pkg/reporter/postgres applyAdvancedFilter operator-for-operator.
func applyPostgresFilterCondition(builder squirrel.SelectBuilder, field string, condition model.FilterCondition) squirrel.SelectBuilder {
	if len(condition.Equals) == 1 {
		builder = builder.Where(squirrel.Eq{field: condition.Equals[0]})
	} else if len(condition.Equals) > 1 {
		builder = builder.Where(squirrel.Eq{field: condition.Equals})
	}

	if len(condition.GreaterThan) > 0 {
		builder = builder.Where(squirrel.Gt{field: condition.GreaterThan[0]})
	}

	if len(condition.GreaterOrEqual) > 0 {
		builder = builder.Where(squirrel.GtOrEq{field: condition.GreaterOrEqual[0]})
	}

	if len(condition.LessThan) > 0 {
		builder = builder.Where(squirrel.Lt{field: condition.LessThan[0]})
	}

	if len(condition.LessOrEqual) > 0 {
		builder = builder.Where(squirrel.LtOrEq{field: condition.LessOrEqual[0]})
	}

	if len(condition.Between) == constant.BetweenOperatorValues {
		endValue := applyDateRangeUpperBound(field, condition.Between[0], condition.Between[1])
		builder = builder.Where(squirrel.GtOrEq{field: condition.Between[0]}).Where(squirrel.LtOrEq{field: endValue})
	}

	if len(condition.In) > 0 {
		builder = builder.Where(squirrel.Eq{field: condition.In})
	}

	if len(condition.NotIn) > 0 {
		builder = builder.Where(squirrel.NotEq{field: condition.NotIn})
	}

	return builder
}

// isPostgresFilterEmpty reports whether a FilterCondition carries no active
// operator, mirroring the legacy isFilterConditionEmpty.
func isPostgresFilterEmpty(condition model.FilterCondition) bool {
	return len(condition.Equals) == 0 &&
		len(condition.GreaterThan) == 0 &&
		len(condition.GreaterOrEqual) == 0 &&
		len(condition.LessThan) == 0 &&
		len(condition.LessOrEqual) == 0 &&
		len(condition.Between) == 0 &&
		len(condition.In) == 0 &&
		len(condition.NotIn) == 0
}

// projectColumns reduces a field selection to the distinct root columns to
// SELECT. A "*" selection returns nil (caller selects all). Nested JSONB paths
// collapse to their root column; duplicates are removed; output is sorted for
// deterministic SQL.
func projectColumns(fields []string) []string {
	for _, f := range fields {
		if f == "*" {
			return nil
		}
	}

	seen := make(map[string]struct{}, len(fields))

	var cols []string

	for _, f := range fields {
		root := f
		if dot := strings.Index(f, "."); dot != -1 {
			root = f[:dot]
		}

		if root == "" {
			continue
		}

		if _, ok := seen[root]; ok {
			continue
		}

		seen[root] = struct{}{}

		cols = append(cols, quoteIdentifier(root))
	}

	sort.Strings(cols)

	return cols
}

// splitQualified splits a "schema.table" name into its parts. An unqualified
// name returns an empty schema.
func splitQualified(qualified string) (schemaName, tableName string) {
	if dot := strings.Index(qualified, "."); dot != -1 {
		return qualified[:dot], qualified[dot+1:]
	}

	return "", qualified
}

// quoteIdentifier double-quotes a SQL identifier and escapes embedded quotes,
// so a projected column or table name can never break out of its quoting.
func quoteIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// decodeMaybeJSON unmarshals a []byte value that may carry a JSONB payload into
// its Go shape (object, array, or string), falling back to the raw value when
// it is not valid JSON. It mirrors the reporter's existing JSONB handling.
func decodeMaybeJSON(value any) any {
	bytes, ok := value.([]byte)
	if !ok {
		return value
	}

	var obj map[string]any
	if err := json.Unmarshal(bytes, &obj); err == nil {
		return obj
	}

	var arr []any
	if err := json.Unmarshal(bytes, &arr); err == nil {
		return arr
	}

	var str string
	if err := json.Unmarshal(bytes, &str); err == nil {
		return str
	}

	return value
}
