# Observability Guide

## Observability Stack (LGTM)

Midaz uses the **Grafana LGTM stack** for comprehensive observability:

- **L**oki: Log aggregation and querying
- **G**rafana: Visualization and dashboards
- **T**empo: Distributed tracing
- **M**imir: Metrics and alerting (Prometheus-compatible)

All integrated via **OpenTelemetry** for standardized instrumentation.

## OpenTelemetry Integration

### Tracer Setup

Every component initializes OpenTelemetry tracing:

```go
import (
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("component-name")

func init() {
    // Initialize OpenTelemetry provider
    provider := initTraceProvider()
    otel.SetTracerProvider(provider)
}
```

### Creating Spans

**Pattern**: Create spans for all significant operations

```go
func (uc *UseCase) CreateAccount(ctx context.Context, input Input) (*Account, error) {
    // Start span with descriptive name
    ctx, span := tracer.Start(ctx, "usecase.create_account")
    defer span.End()

    // Add attributes to span
    span.SetAttributes(
        attribute.String("account.name", input.Name),
        attribute.String("account.type", input.Type),
        attribute.String("organization.id", input.OrganizationID.String()),
    )

    // Business logic
    account, err := uc.AccountRepo.Create(ctx, account)
    if err != nil {
        // Record error in span
        span.RecordError(err)
        span.SetStatus(codes.Error, "Failed to create account")
        return nil, fmt.Errorf("creating account: %w", err)
    }

    // Mark span as successful
    span.SetStatus(codes.Ok, "Account created successfully")
    return account, nil
}
```

### Span Hierarchy

```
HTTP Request [handler.create_account]
  ↓
UseCase [usecase.create_account]
  ↓
Repository [repository.create]
    ↓
Database Query [postgresql.exec]
```

Each layer creates a child span, building a complete trace.

### Business Error Events

**Don't mark business errors as span errors**:

```go
// Business error (expected) - Use event, not error status
if account == nil {
    err := pkg.EntityNotFoundError{EntityType: "Account", ID: accountID}
    libOpentelemetry.HandleSpanBusinessErrorEvent(span, err, "Account not found")
    return nil, err
}

// Technical error (unexpected) - Mark span as error
if err := db.Query(ctx, query); err != nil {
    span.RecordError(err)
    span.SetStatus(codes.Error, "Database query failed")
    return nil, err
}
```

### Context Propagation

**Always propagate context through call chain**:

```go
// ✅ GOOD - Context flows through all layers
func (h *Handler) CreateAccount(c *fiber.Ctx) error {
    ctx := c.Context()  // Extract from HTTP request

    ctx, span := tracer.Start(ctx, "handler.create_account")
    defer span.End()

    // Pass context to use case
    account, err := h.Command.CreateAccount(ctx, input)
    ...
}

func (uc *UseCase) CreateAccount(ctx context.Context, input Input) (*Account, error) {
    ctx, span := tracer.Start(ctx, "usecase.create_account")
    defer span.End()

    // Pass context to repository
    return uc.AccountRepo.Create(ctx, account)
}

func (r *Repository) Create(ctx context.Context, account *Account) error {
    // Context includes trace information
    _, err := r.db.ExecContext(ctx, query, args...)
    return err
}
```

## Structured Logging

### Tracking Context

Extract tracking context at handler level:

```go
import libCommons "github.com/lerianstudio/lib-commons"

func (h *Handler) CreateAccount(c *fiber.Ctx) error {
    ctx := c.Context()

    // Extract tracking context (includes logger, tracer, metrics)
    tracking := libCommons.NewTrackingFromContext(ctx)

    // Use structured logger
    tracking.Logger.WithFields(map[string]interface{}{
        "handler":         "CreateAccount",
        "organization_id": organizationID,
        "ledger_id":       ledgerID,
    }).Info("Processing account creation request")

    account, err := h.Command.CreateAccount(ctx, organizationID, ledgerID, input)
    if err != nil {
        tracking.Logger.WithFields(map[string]interface{}{
            "error":           err.Error(),
            "organization_id": organizationID,
            "ledger_id":       ledgerID,
        }).Error("Failed to create account")
        return http.WithError(c, err)
    }

    tracking.Logger.WithFields(map[string]interface{}{
        "account_id":      account.ID,
        "account_name":    account.Name,
        "organization_id": organizationID,
    }).Info("Account created successfully")

    return http.Created(c, account)
}
```

### Log Levels

```go
// DEBUG - Detailed information for debugging
tracking.Logger.Debugf("Validating account input: %+v", input)

// INFO - General informational messages
tracking.Logger.Infof("Account %s created", accountID)

// WARN - Warning messages (something unusual but not an error)
tracking.Logger.Warnf("Account balance below threshold: %f", balance)

// ERROR - Error messages (operation failed)
tracking.Logger.Errorf("Failed to create account: %v", err)

// FATAL - Critical errors (application cannot continue)
tracking.Logger.Fatalf("Database connection lost: %v", err)
```

### Structured Logging Best Practices

```go
// ✅ GOOD - Structured with key-value pairs
tracking.Logger.WithFields(map[string]interface{}{
    "account_id":   accountID,
    "balance":      balance,
    "currency":     currency,
    "timestamp":    time.Now(),
}).Info("Balance updated")

// ❌ BAD - String interpolation loses structure
tracking.Logger.Infof("Balance updated for account %s to %f %s", accountID, balance, currency)
```

### PII Redaction

**Critical**: Personal Identifiable Information (PII) must be redacted:

```go
// Automatic redaction in logs for sensitive fields
tracking.Logger.WithFields(map[string]interface{}{
    "user_email":    "[REDACTED]",     // Don't log email addresses
    "account_name":  account.Name,      // Safe to log
    "ssn":           "[REDACTED]",     // Never log SSN
    "account_id":    account.ID,        // UUIDs are safe
}).Info("User account created")
```

## Metrics

### Prometheus Metrics

```go
import "github.com/prometheus/client_golang/prometheus"

var (
    // Counter - Monotonically increasing value
    accountsCreated = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "accounts_created_total",
            Help: "Total number of accounts created",
        },
        []string{"organization_id", "account_type"},
    )

    // Gauge - Value that can go up or down
    activeAccounts = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "active_accounts",
            Help: "Number of active accounts",
        },
        []string{"organization_id"},
    )

    // Histogram - Distribution of values
    requestDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "http_request_duration_seconds",
            Help:    "HTTP request duration in seconds",
            Buckets: prometheus.DefBuckets,
        },
        []string{"method", "path", "status"},
    )

    // Summary - Similar to histogram but calculates quantiles
    balanceAmount = prometheus.NewSummaryVec(
        prometheus.SummaryOpts{
            Name:       "account_balance_amount",
            Help:       "Account balance amounts",
            Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
        },
        []string{"account_type"},
    )
)

func init() {
    // Register metrics
    prometheus.MustRegister(accountsCreated)
    prometheus.MustRegister(activeAccounts)
    prometheus.MustRegister(requestDuration)
    prometheus.MustRegister(balanceAmount)
}
```

### Recording Metrics

```go
func (uc *UseCase) CreateAccount(ctx context.Context, input Input) (*Account, error) {
    // Increment counter
    accountsCreated.WithLabelValues(
        input.OrganizationID.String(),
        input.Type,
    ).Inc()

    // Update gauge
    activeAccounts.WithLabelValues(
        input.OrganizationID.String(),
    ).Inc()

    // Record value in summary
    balanceAmount.WithLabelValues(input.Type).Observe(float64(input.InitialBalance))

    // ... business logic
}
```

### HTTP Middleware Metrics

```go
func MetricsMiddleware() fiber.Handler {
    return func(c *fiber.Ctx) error {
        start := time.Now()

        // Process request
        err := c.Next()

        // Record duration
        duration := time.Since(start).Seconds()
        requestDuration.WithLabelValues(
            c.Method(),
            c.Path(),
            strconv.Itoa(c.Response().StatusCode()),
        ).Observe(duration)

        return err
    }
}
```

### Exposing Metrics Endpoint

```go
import "github.com/gofiber/adaptor/v2"

func setupMetrics(app *fiber.App) {
    // Prometheus metrics endpoint
    app.Get("/metrics", adaptor.HTTPHandler(promhttp.Handler()))
}

// Access metrics at: http://localhost:3000/metrics
```

## Health Checks

### Health Check Endpoint

```go
type HealthResponse struct {
    Status  string            `json:"status"`
    Checks  map[string]Check  `json:"checks"`
    Version string            `json:"version"`
}

type Check struct {
    Status  string  `json:"status"`
    Message string  `json:"message,omitempty"`
}

func (h *Handler) HealthCheck(c *fiber.Ctx) error {
    ctx := c.Context()

    health := HealthResponse{
        Status:  "healthy",
        Version: version.Version,
        Checks:  make(map[string]Check),
    }

    // Check PostgreSQL
    if err := h.db.PingContext(ctx); err != nil {
        health.Status = "unhealthy"
        health.Checks["postgres"] = Check{
            Status:  "down",
            Message: err.Error(),
        }
    } else {
        health.Checks["postgres"] = Check{Status: "up"}
    }

    // Check MongoDB
    if err := h.mongo.Ping(ctx, nil); err != nil {
        health.Status = "unhealthy"
        health.Checks["mongodb"] = Check{
            Status:  "down",
            Message: err.Error(),
        }
    } else {
        health.Checks["mongodb"] = Check{Status: "up"}
    }

    // Check RabbitMQ
    if !h.rabbitmq.IsConnected() {
        health.Status = "unhealthy"
        health.Checks["rabbitmq"] = Check{Status: "down"}
    } else {
        health.Checks["rabbitmq"] = Check{Status: "up"}
    }

    statusCode := http.StatusOK
    if health.Status == "unhealthy" {
        statusCode = http.StatusServiceUnavailable
    }

    return c.Status(statusCode).JSON(health)
}
```

### Readiness vs Liveness

```go
// Liveness - Is the application running?
func (h *Handler) LivenessCheck(c *fiber.Ctx) error {
    return c.JSON(fiber.Map{"status": "alive"})
}

// Readiness - Is the application ready to serve traffic?
func (h *Handler) ReadinessCheck(c *fiber.Ctx) error {
    // Check dependencies are available
    if err := h.checkDependencies(); err != nil {
        return c.Status(503).JSON(fiber.Map{
            "status": "not_ready",
            "error":  err.Error(),
        })
    }

    return c.JSON(fiber.Map{"status": "ready"})
}
```

## Distributed Tracing Examples

### Tracing Across Services

**Onboarding Service** (creates trace):
```go
func (h *Handler) CreateAccount(c *fiber.Ctx) error {
    ctx := c.Context()

    // Start trace
    ctx, span := tracer.Start(ctx, "onboarding.create_account")
    defer span.End()

    // Call transaction service via gRPC
    // Context automatically propagates trace ID
    _, err := h.transactionClient.ValidateAccount(ctx, &pb.ValidateRequest{
        AccountID: accountID,
    })

    return nil
}
```

**Transaction Service** (continues trace):
```go
func (s *Server) ValidateAccount(ctx context.Context, req *pb.ValidateRequest) (*pb.ValidateResponse, error) {
    // Span is automatically created as child of onboarding span
    ctx, span := tracer.Start(ctx, "transaction.validate_account")
    defer span.End()

    // Trace ID flows through both services
    // Can correlate logs and spans across services

    return &pb.ValidateResponse{Valid: true}, nil
}
```

### Trace Visualization

In Grafana/Tempo, you'll see:
```
[HTTP Request: POST /accounts] (200ms)
  ├─ [onboarding.create_account] (180ms)
  │   ├─ [repository.create] (50ms)
  │   │   └─ [postgresql.insert] (45ms)
  │   └─ [grpc.call.transaction] (120ms)
  │       └─ [transaction.validate_account] (115ms)
  │           └─ [repository.find] (110ms)
  └─ [response] (20ms)
```

## Observability Checklist

✅ **Create spans** for all significant operations

✅ **Add attributes** to spans for context

✅ **Propagate context** through all layers

✅ **Use structured logging** with key-value pairs

✅ **Log at boundaries** (handlers), not every function

✅ **Redact PII** from logs

✅ **Distinguish business vs technical errors** in spans

✅ **Record metrics** for key operations

✅ **Expose /metrics endpoint** for Prometheus

✅ **Implement health checks** (/health, /live, /ready)

✅ **Use descriptive span names** (usecase.create_account)

✅ **Include trace IDs** in log entries for correlation

## Grafana Dashboard Examples

### Key Metrics to Monitor

```promql
# Request rate
rate(http_requests_total[5m])

# Error rate
rate(http_requests_total{status=~"5.."}[5m])

# P95 latency
histogram_quantile(0.95, rate(http_request_duration_seconds_bucket[5m]))

# Account creation rate
rate(accounts_created_total[5m])

# Active accounts by type
sum by (account_type) (active_accounts)

# Database connection pool
pg_connections_active
pg_connections_idle
```

### Alerts

```yaml
groups:
  - name: midaz_alerts
    rules:
      - alert: HighErrorRate
        expr: rate(http_requests_total{status=~"5.."}[5m]) > 0.05
        annotations:
          summary: "High error rate detected"

      - alert: HighLatency
        expr: histogram_quantile(0.95, rate(http_request_duration_seconds_bucket[5m])) > 1
        annotations:
          summary: "P95 latency above 1 second"

      - alert: DatabaseDown
        expr: up{job="postgres"} == 0
        annotations:
          summary: "PostgreSQL is down"
```

## Related Documentation

- Architecture: `docs/agents/architecture.md`
- Error Handling: `docs/agents/error-handling.md`
- Concurrency: `docs/agents/concurrency.md`
- API Design: `docs/agents/api-design.md`
