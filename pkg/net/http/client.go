package http

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// InstrumentedClient wraps the standard http.Client with telemetry
type InstrumentedClient struct {
	client        *http.Client
	serviceName   string
	libraryName   string
	defaultLabels []attribute.KeyValue
}

// ClientOption is a function that configures an InstrumentedClient
type ClientOption func(*InstrumentedClient)

// WithDefaultLabels adds default labels to all telemetry from this client
func WithDefaultLabels(labels ...attribute.KeyValue) ClientOption {
	return func(c *InstrumentedClient) {
		c.defaultLabels = append(c.defaultLabels, labels...)
	}
}

// WithClient sets the underlying HTTP client
func WithClient(client *http.Client) ClientOption {
	return func(c *InstrumentedClient) {
		c.client = client
	}
}

// NewInstrumentedClient creates a new HTTP client with telemetry instrumentation
func NewInstrumentedClient(serviceName, libraryName string, options ...ClientOption) *InstrumentedClient {
	client := &InstrumentedClient{
		client:      http.DefaultClient,
		serviceName: serviceName,
		libraryName: libraryName,
		defaultLabels: []attribute.KeyValue{
			attribute.String("service.name", serviceName),
		},
	}

	// Apply options
	for _, opt := range options {
		opt(client)
	}

	// Track connection pool metrics
	go client.trackConnectionPoolMetrics()

	return client
}

// Do performs an HTTP request with telemetry instrumentation
func (c *InstrumentedClient) Do(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	tracer := otel.Tracer(c.libraryName)

	// Create attributes for this request
	attrs := append(c.defaultLabels,
		attribute.String("http.method", req.Method),
		attribute.String("http.url", req.URL.String()),
		attribute.String("http.host", req.URL.Host),
	)

	// Start a span for this request
	var span trace.Span
	ctx, span = tracer.Start(ctx, req.Method+" "+req.URL.Path,
		trace.WithAttributes(attrs...),
		trace.WithSpanKind(trace.SpanKindClient),
	)
	defer span.End()

	// Inject trace context into the outgoing request
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))

	// Set Midaz ID in header
	midazID := pkg.NewMidazIDFromContext(ctx)
	if midazID != "" {
		req.Header.Set(HeaderMidazID, midazID)
	}

	// Record metrics for in-flight requests
	meter := otel.Meter(c.serviceName)
	inFlightRequests, _ := meter.Int64UpDownCounter(
		mopentelemetry.GetMetricName("http", "client", "requests", "in_flight"),
		metric.WithDescription("Number of in-flight HTTP requests"),
		metric.WithUnit("{request}"),
	)
	inFlightRequests.Add(ctx, 1, metric.WithAttributes(attrs...))

	defer inFlightRequests.Add(ctx, -1, metric.WithAttributes(attrs...))

	// Create request counter
	requestCounter, _ := meter.Int64Counter(
		mopentelemetry.GetMetricName("http", "client", "requests", "total"),
		metric.WithDescription("Number of HTTP requests"),
		metric.WithUnit("{request}"),
	)

	// Start timestamp
	startTime := time.Now()

	// Execute the request
	resp, err := c.client.Do(req.WithContext(ctx))

	// Calculate duration
	duration := time.Since(startTime).Milliseconds()

	// Create duration histogram
	httpDuration, _ := meter.Int64Histogram(
		mopentelemetry.GetMetricName("http", "client", "duration", "milliseconds"),
		metric.WithDescription("Duration of HTTP requests"),
		metric.WithUnit("ms"),
	)

	// Handle errors
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)

		// Record failed request
		failedAttrs := append(attrs, attribute.String("status_code", "error"))
		requestCounter.Add(ctx, 1, metric.WithAttributes(failedAttrs...))
		httpDuration.Record(ctx, duration, metric.WithAttributes(failedAttrs...))

		return nil, err
	}

	// Record response status code
	span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))

	// Check if this was an error response
	if resp.StatusCode >= 400 {
		span.SetStatus(codes.Error, "HTTP error "+strconv.Itoa(resp.StatusCode))
	} else {
		span.SetStatus(codes.Ok, "")
	}

	// Record successful request with status code
	responseAttrs := append(attrs, attribute.String("status_code", strconv.Itoa(resp.StatusCode)))
	requestCounter.Add(ctx, 1, metric.WithAttributes(responseAttrs...))
	httpDuration.Record(ctx, duration, metric.WithAttributes(responseAttrs...))

	return resp, nil
}

// Get performs an HTTP GET request with telemetry
func (c *InstrumentedClient) Get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	return c.Do(req)
}

// Post performs an HTTP POST request with telemetry
func (c *InstrumentedClient) Post(ctx context.Context, url string, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", contentType)

	return c.Do(req)
}

// trackConnectionPoolMetrics collects metrics about the connection pool
// Note: This requires a custom http.Transport to fully implement
func (c *InstrumentedClient) trackConnectionPoolMetrics() {
	ctx := context.Background()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	meter := otel.Meter(c.serviceName)

	// Connection pool metrics
	poolSizeGauge, _ := meter.Int64Gauge(
		mopentelemetry.GetMetricName("http", "client", "connection_pool", "size"),
		metric.WithDescription("HTTP client connection pool size"),
		metric.WithUnit("{connection}"),
	)

	idleConnGauge, _ := meter.Int64Gauge(
		mopentelemetry.GetMetricName("http", "client", "connection_pool", "idle"),
		metric.WithDescription("HTTP client idle connections"),
		metric.WithUnit("{connection}"),
	)

	for range ticker.C {
		transport, ok := c.client.Transport.(*http.Transport)
		if !ok {
			// If not using http.Transport, we can't get connection stats
			// Use default values to indicate this
			poolSizeGauge.Record(ctx, 0, metric.WithAttributes(c.defaultLabels...))
			idleConnGauge.Record(ctx, 0, metric.WithAttributes(c.defaultLabels...))

			continue
		}

		// Get connection stats from the transport
		// Note: This doesn't give perfect information about connection pool size
		// but is a reasonable approximation
		stats := transport.Clone()

		// Estimate pool size based on max idle connections per host
		// This is just an approximation as Go doesn't expose actual pool size
		poolSize := stats.MaxIdleConnsPerHost * 2 // Rough estimate
		poolSizeGauge.Record(ctx, int64(poolSize), metric.WithAttributes(c.defaultLabels...))

		// Idle connections can be estimated but is an incomplete measure
		// Full instrumentation would require a custom transport implementation
		idleConnGauge.Record(ctx, int64(stats.MaxIdleConns), metric.WithAttributes(c.defaultLabels...))
	}
}
