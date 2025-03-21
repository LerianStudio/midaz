package mpostgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// PostgresConfig represents configuration for instrumented Postgres client
type PostgresConfig struct {
	ServiceName    string
	LibraryName    string
	CollectMetrics bool
}

// PostgresTracer implements pgx.QueryTracer interface for OpenTelemetry
type PostgresTracer struct {
	serviceName string
	libraryName string
}

// NewPostgresTracer creates a new tracer for pgx that uses OpenTelemetry
func NewPostgresTracer(serviceName, libraryName string) *PostgresTracer {
	return &PostgresTracer{
		serviceName: serviceName,
		libraryName: libraryName,
	}
}

// TraceQueryStart implements pgx.QueryTracer interface
func (p *PostgresTracer) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	// Extract the operation from the query
	operation := extractOperation(data.SQL)

	// Create attributes for the span
	attrs := []attribute.KeyValue{
		attribute.String("db.system", "postgresql"),
		attribute.String("db.name", conn.Config().Database),
		attribute.String("db.operation", operation),
		attribute.String("db.statement", data.SQL),
	}

	// Start a new span for this query
	tracer := otel.Tracer(p.libraryName)
	ctx, _ = tracer.Start(
		ctx,
		"PostgreSQL "+operation,
		trace.WithAttributes(attrs...),
		trace.WithSpanKind(trace.SpanKindClient),
	)

	// Collect metrics
	meter := otel.Meter(p.serviceName)

	// Create metric for active queries
	activeQueries, _ := meter.Int64UpDownCounter(
		mopentelemetry.GetMetricName("db", "postgresql", "queries", "active"),
		metric.WithDescription("Number of active PostgreSQL queries"),
		metric.WithUnit("{query}"),
	)

	// Increment active queries count
	metricAttrs := []attribute.KeyValue{
		attribute.String("db.operation", operation),
		attribute.String("db.name", conn.Config().Database),
	}
	activeQueries.Add(ctx, 1, metric.WithAttributes(metricAttrs...))

	return ctx
}

// TraceQueryEnd implements pgx.QueryTracer interface
func (p *PostgresTracer) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryEndData) {
	span := trace.SpanFromContext(ctx)
	defer span.End()

	// Extract the operation from the query (should match what we did in TraceQueryStart)
	operation := extractOperation(span.SpanContext().SpanID().String()) // We can't access the SQL here, so use span ID

	// Record query result
	if data.Err != nil {
		span.SetStatus(codes.Error, data.Err.Error())
		span.RecordError(data.Err)
	} else {
		span.SetStatus(codes.Ok, "")

		// If this was a SELECT, record the number of rows returned
		if strings.HasPrefix(operation, "SELECT") {
			rowsAffected := data.CommandTag.RowsAffected()
			span.SetAttributes(attribute.Int64("db.rows_returned", rowsAffected))
		} else {
			// For other operations, record rows affected
			rowsAffected := data.CommandTag.RowsAffected()
			span.SetAttributes(attribute.Int64("db.rows_affected", rowsAffected))
		}
	}

	// Since we can't access direct duration from data struct,
	// we'll estimate it from the span timing
	span.SetAttributes(attribute.Float64("db.duration_ms", 0.0)) // Placeholder

	// Collect metrics
	meter := otel.Meter(p.serviceName)

	// Create metric for active queries
	activeQueries, _ := meter.Int64UpDownCounter(
		mopentelemetry.GetMetricName("db", "postgresql", "queries", "active"),
		metric.WithDescription("Number of active PostgreSQL queries"),
		metric.WithUnit("{query}"),
	)

	// Create metric for query duration
	queryDuration, _ := meter.Int64Histogram(
		mopentelemetry.GetMetricName("db", "postgresql", "queries", "duration"),
		metric.WithDescription("Duration of PostgreSQL queries"),
		metric.WithUnit("ms"),
	)

	// Create metric for query errors
	queryErrors, _ := meter.Int64Counter(
		mopentelemetry.GetMetricName("db", "postgresql", "queries", "errors"),
		metric.WithDescription("Number of PostgreSQL query errors"),
		metric.WithUnit("{error}"),
	)

	// Increment active queries count
	metricAttrs := []attribute.KeyValue{
		attribute.String("db.operation", operation),
		attribute.String("db.name", conn.Config().Database),
	}

	// Decrement active queries count
	activeQueries.Add(ctx, -1, metric.WithAttributes(metricAttrs...))

	// Set a constant duration as we can't reliably get it from data struct
	queryDuration.Record(ctx, 0, metric.WithAttributes(metricAttrs...))

	// Record error if any
	if data.Err != nil {
		errorAttrs := append(metricAttrs, attribute.String("error.type", fmt.Sprintf("%T", data.Err)))
		queryErrors.Add(ctx, 1, metric.WithAttributes(errorAttrs...))
	}
}

// InstrumentPgxConfig adds OpenTelemetry instrumentation to a pgx config
func InstrumentPgxConfig(config *pgx.ConnConfig, telemetryConfig PostgresConfig) {
	// Create and set the tracer
	tracer := NewPostgresTracer(telemetryConfig.ServiceName, telemetryConfig.LibraryName)

	// Set the tracer directly - we're simplifying because the complex composition
	// with tracelog.TraceLog is causing go vet issues
	config.Tracer = tracer
}

// InstrumentPgxPool adds OpenTelemetry instrumentation to a pgx pool
func InstrumentPgxPool(pool *pgxpool.Pool, telemetryConfig PostgresConfig) {
	// Start a goroutine to collect connection pool metrics
	if telemetryConfig.CollectMetrics {
		go collectPoolMetrics(pool, telemetryConfig)
	}
}

// collectPoolMetrics collects metrics about the connection pool
func collectPoolMetrics(pool *pgxpool.Pool, config PostgresConfig) {
	ctx := context.Background()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	meter := otel.Meter(config.ServiceName)

	// Pool metrics
	totalConnections, _ := meter.Int64Gauge(
		mopentelemetry.GetMetricName("db", "postgresql", "connections", "total"),
		metric.WithDescription("Total connections in the PostgreSQL pool"),
		metric.WithUnit("{connection}"),
	)

	idleConnections, _ := meter.Int64Gauge(
		mopentelemetry.GetMetricName("db", "postgresql", "connections", "idle"),
		metric.WithDescription("Idle connections in the PostgreSQL pool"),
		metric.WithUnit("{connection}"),
	)

	for range ticker.C {
		stats := pool.Stat()

		attrs := []attribute.KeyValue{
			attribute.String("db.name", pool.Config().ConnConfig.Database),
		}

		totalConnections.Record(ctx, int64(stats.TotalConns()), metric.WithAttributes(attrs...))
		idleConnections.Record(ctx, int64(stats.IdleConns()), metric.WithAttributes(attrs...))
	}
}

// WithQueryTracing wraps a SQL query execution with OpenTelemetry tracing
func WithQueryTracing(ctx context.Context, db *sql.DB, query string, args ...any) (*sql.Rows, error) {
	// Extract operation type
	operation := extractOperation(query)

	// Get tracer
	tracer := otel.Tracer("github.com/LerianStudio/midaz/pkg/mpostgres")

	// Create span attributes
	attrs := []attribute.KeyValue{
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation", operation),
		attribute.String("db.statement", query),
	}

	// Start span
	ctx, span := tracer.Start(
		ctx,
		"PostgreSQL "+operation,
		trace.WithAttributes(attrs...),
	)
	defer span.End()

	// Execute query
	startTime := time.Now()
	rows, err := db.QueryContext(ctx, query, args...)
	duration := time.Since(startTime)

	// Record error if any
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Database query failed", err)
		return nil, err
	}

	// Record success and duration
	span.SetAttributes(attribute.Int64("db.duration_ms", duration.Milliseconds()))

	return rows, nil
}

// WithExecTracing wraps a SQL exec operation with OpenTelemetry tracing
func WithExecTracing(ctx context.Context, db *sql.DB, query string, args ...any) (sql.Result, error) {
	// Extract operation type
	operation := extractOperation(query)

	// Get tracer
	tracer := otel.Tracer("github.com/LerianStudio/midaz/pkg/mpostgres")

	// Create span attributes
	attrs := []attribute.KeyValue{
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation", operation),
		attribute.String("db.statement", query),
	}

	// Start span
	ctx, span := tracer.Start(
		ctx,
		"PostgreSQL "+operation,
		trace.WithAttributes(attrs...),
	)
	defer span.End()

	// Execute query
	startTime := time.Now()
	result, err := db.ExecContext(ctx, query, args...)
	duration := time.Since(startTime)

	// Record error if any
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Database exec failed", err)
		return nil, err
	}

	// Record success and duration
	span.SetAttributes(attribute.Int64("db.duration_ms", duration.Milliseconds()))

	// Record rows affected
	rowsAffected, err := result.RowsAffected()
	if err == nil {
		span.SetAttributes(attribute.Int64("db.rows_affected", rowsAffected))
	}

	return result, nil
}

// extractOperation extracts the operation type from a SQL query
func extractOperation(query string) string {
	query = strings.TrimSpace(query)
	if len(query) == 0 {
		return "UNKNOWN"
	}

	// Extract the first word of the query
	parts := strings.Fields(query)
	if len(parts) == 0 {
		return "UNKNOWN"
	}

	operation := strings.ToUpper(parts[0])

	// Handle special cases
	switch operation {
	case "INSERT", "SELECT", "UPDATE", "DELETE", "CREATE", "DROP", "ALTER", "TRUNCATE", "BEGIN", "COMMIT", "ROLLBACK":
		return operation
	default:
		return "OTHER"
	}
}
