package mmongo

import (
	"context"
	"fmt"
	"time"

	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/event"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// MongoConfig represents configuration for instrumented MongoDB client
type MongoConfig struct {
	ServiceName    string
	LibraryName    string
	CollectMetrics bool
}

// CommandMonitor captures command events for MongoDB
type CommandMonitor struct {
	serviceName string
	libraryName string
}

// NewCommandMonitor creates a new MongoDB command monitor
func NewCommandMonitor(serviceName, libraryName string) *CommandMonitor {
	return &CommandMonitor{
		serviceName: serviceName,
		libraryName: libraryName,
	}
}

// Started is called when a command starts
func (m *CommandMonitor) Started(ctx context.Context, evt *event.CommandStartedEvent) {
	// Extract operation information
	operation := evt.CommandName

	// Create span attributes
	attrs := []attribute.KeyValue{
		attribute.String("db.system", "mongodb"),
		attribute.String("db.name", evt.DatabaseName),
		attribute.String("db.operation", operation),
		attribute.String("db.collection", extractCollection(evt.Command)),
		attribute.String("db.statement", evt.Command.String()),
	}

	// Start a span for this operation
	tracer := otel.Tracer(m.libraryName)
	spanCtx, span := tracer.Start(
		ctx,
		"MongoDB "+operation,
		trace.WithAttributes(attrs...),
		trace.WithSpanKind(trace.SpanKindClient),
	)

	// Store the span in the context - not used but needed for correct tracing behavior
	_ = trace.ContextWithSpan(ctx, span)

	// Collect metrics
	meter := otel.Meter(m.serviceName)

	// Metric for active operations
	activeOps, _ := meter.Int64UpDownCounter(
		"db.mongodb.active_operations",
		metric.WithDescription("Number of active MongoDB operations"),
		metric.WithUnit("{operation}"),
	)

	// Metric attributes
	metricAttrs := []attribute.KeyValue{
		attribute.String("db.operation", operation),
		attribute.String("db.name", evt.DatabaseName),
		attribute.String("db.collection", extractCollection(evt.Command)),
	}

	// Increment active operations
	activeOps.Add(spanCtx, 1, metric.WithAttributes(metricAttrs...))
}

// Succeeded is called when a command completes successfully
func (m *CommandMonitor) Succeeded(ctx context.Context, evt *event.CommandSucceededEvent) {
	// Get the current span
	span := trace.SpanFromContext(ctx)
	defer span.End()

	// Record duration
	duration := evt.Duration
	span.SetAttributes(attribute.Int64("db.duration_ms", duration.Milliseconds()))

	// Set success status
	span.SetStatus(codes.Ok, "")

	// Extract operation
	operation := evt.CommandName

	// Collect metrics
	meter := otel.Meter(m.serviceName)

	// Metric for active operations
	activeOps, _ := meter.Int64UpDownCounter(
		"db.mongodb.active_operations",
		metric.WithDescription("Number of active MongoDB operations"),
		metric.WithUnit("{operation}"),
	)

	// Metric for operation duration
	opDuration, _ := meter.Int64Histogram(
		"db.mongodb.operation_duration",
		metric.WithDescription("Duration of MongoDB operations"),
		metric.WithUnit("ms"),
	)

	// Create metric attributes
	metricAttrs := []attribute.KeyValue{
		attribute.String("db.operation", operation),
	}

	// Decrement active operations
	activeOps.Add(ctx, -1, metric.WithAttributes(metricAttrs...))

	// Record duration
	opDuration.Record(ctx, duration.Milliseconds(), metric.WithAttributes(metricAttrs...))
}

// Failed is called when a command fails
func (m *CommandMonitor) Failed(ctx context.Context, evt *event.CommandFailedEvent) {
	// Get the current span
	span := trace.SpanFromContext(ctx)
	defer span.End()

	// Record error
	errorMsg := fmt.Errorf("%s", evt.Failure)
	mopentelemetry.HandleSpanError(&span, "MongoDB command failed: "+evt.CommandName, errorMsg)

	// Record duration
	duration := evt.Duration
	span.SetAttributes(attribute.Int64("db.duration_ms", duration.Milliseconds()))

	// Extract operation
	operation := evt.CommandName

	// Collect metrics
	meter := otel.Meter(m.serviceName)

	// Metric for active operations
	activeOps, _ := meter.Int64UpDownCounter(
		"db.mongodb.active_operations",
		metric.WithDescription("Number of active MongoDB operations"),
		metric.WithUnit("{operation}"),
	)

	// Metric for operation duration
	opDuration, _ := meter.Int64Histogram(
		"db.mongodb.operation_duration",
		metric.WithDescription("Duration of MongoDB operations"),
		metric.WithUnit("ms"),
	)

	// Metric for operation errors
	opErrors, _ := meter.Int64Counter(
		"db.mongodb.operation_errors",
		metric.WithDescription("Number of MongoDB operation errors"),
		metric.WithUnit("{error}"),
	)

	// Create metric attributes
	metricAttrs := []attribute.KeyValue{
		attribute.String("db.operation", operation),
	}

	// Add error type to attributes
	errorAttrs := append(metricAttrs, attribute.String("error.message", evt.Failure))

	// Decrement active operations
	activeOps.Add(ctx, -1, metric.WithAttributes(metricAttrs...))

	// Record duration
	opDuration.Record(ctx, duration.Milliseconds(), metric.WithAttributes(metricAttrs...))

	// Record error
	opErrors.Add(ctx, 1, metric.WithAttributes(errorAttrs...))
}

// InstrumentMongoClient adds OpenTelemetry instrumentation to a MongoDB client
func InstrumentMongoClient(clientOptions *options.ClientOptions, config MongoConfig) {
	// Create a command monitor
	monitor := NewCommandMonitor(config.ServiceName, config.LibraryName)

	// Set the monitor directly on the client options
	clientOptions.SetMonitor(&event.CommandMonitor{
		Started:   monitor.Started,
		Succeeded: monitor.Succeeded,
		Failed:    monitor.Failed,
	})
}

// CollectMongoMetrics starts a goroutine to collect MongoDB client metrics
func CollectMongoMetrics(client *mongo.Client, config MongoConfig) {
	if !config.CollectMetrics {
		return
	}

	go collectConnectionMetrics(client, config)
}

// collectConnectionMetrics collects metrics about MongoDB connections
// This function is separated to reduce cognitive complexity
func collectConnectionMetrics(client *mongo.Client, config MongoConfig) {
	ctx := context.Background()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	meter := otel.Meter(config.ServiceName)

	// Pool metrics
	poolSize, _ := meter.Int64Gauge(
		"db.mongodb.connection_pool.size",
		metric.WithDescription("MongoDB connection pool size"),
		metric.WithUnit("{connection}"),
	)

	inUseConnections, _ := meter.Int64Gauge(
		"db.mongodb.connection_pool.in_use",
		metric.WithDescription("MongoDB in-use connections"),
		metric.WithUnit("{connection}"),
	)

	maxSize, _ := meter.Int64Gauge(
		"db.mongodb.connection_pool.max",
		metric.WithDescription("MongoDB connection pool max size"),
		metric.WithUnit("{connection}"),
	)

	for range ticker.C {
		recordConnectionMetrics(ctx, client, poolSize, inUseConnections, maxSize)
	}
}

// recordConnectionMetrics collects and records connection metrics for all databases
// Further reduces cognitive complexity by extracting the inner loop
func recordConnectionMetrics(ctx context.Context, client *mongo.Client, poolSize, inUseConnections, maxSize metric.Int64Gauge) {
	// Get all databases to collect metrics for each
	databases, err := client.ListDatabaseNames(ctx, bson.M{})
	if err != nil {
		return
	}

	for _, dbName := range databases {
		// Get server status
		result := client.Database(dbName).RunCommand(ctx, bson.D{{Key: "serverStatus", Value: 1}})

		var serverStatus bson.M
		if err := result.Decode(&serverStatus); err != nil {
			continue
		}

		// Try to extract connection pool info
		if connections, ok := serverStatus["connections"].(bson.M); ok {
			recordMetricsForDatabase(ctx, dbName, connections, poolSize, inUseConnections, maxSize)
		}
	}
}

// recordMetricsForDatabase records metrics for a single database
// Further reduces complexity by extracting the metrics recording logic
func recordMetricsForDatabase(ctx context.Context, dbName string, connections bson.M,
	poolSize, inUseConnections, maxSize metric.Int64Gauge) {
	attrs := []attribute.KeyValue{
		attribute.String("db.name", dbName),
	}

	// Extract metrics
	if current, ok := connections["current"].(int32); ok {
		poolSize.Record(ctx, int64(current), metric.WithAttributes(attrs...))
	}

	if active, ok := connections["active"].(int32); ok {
		inUseConnections.Record(ctx, int64(active), metric.WithAttributes(attrs...))
	}

	if maxConns, ok := connections["max"].(int32); ok {
		maxSize.Record(ctx, int64(maxConns), metric.WithAttributes(attrs...))
	}
}

// extractCollection tries to extract the collection name from the command
func extractCollection(cmd bson.Raw) string {
	// For common commands, try to extract the collection
	if collectionVal, err := cmd.LookupErr(cmd.Index(0).Key()); err == nil {
		if collection, ok := collectionVal.StringValueOK(); ok {
			return collection
		}
	}

	// If the collection name couldn't be determined, return "unknown"
	return "unknown"
}
