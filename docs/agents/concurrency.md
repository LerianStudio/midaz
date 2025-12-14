# Concurrency & Safe Goroutine Handling Guide

## Overview

Midaz enforces **safe goroutine practices** through the `mruntime` package. The custom `panicguard` linter prevents bare `go` keyword usage and ensures all concurrent code includes panic recovery.

**Golden Rule**: NEVER use the bare `go` keyword. Always use `mruntime.SafeGoWithContextAndComponent()`.

## Why Safe Goroutines Matter

### Problems with Bare Goroutines

```go
// ❌ DANGEROUS - Bare goroutine
go func() {
    // If this panics, it crashes the entire application
    result := riskyOperation()
    process(result)
}()
```

**Issues**:
1. **No panic recovery** - Crashes entire application
2. **No observability** - Can't trace or monitor
3. **No context propagation** - Loses cancellation, deadlines, tracing
4. **No component identification** - Can't identify which part of system failed

### Safe Goroutine Benefits

```go
// ✅ SAFE - Using mruntime
mruntime.SafeGoWithContextAndComponent(
    ctx,
    "account-service",
    mruntime.KeepRunning,
    func(ctx context.Context) {
        // If this panics, it's recovered and logged
        // Context is properly propagated
        // Observability is maintained
        result := riskyOperation(ctx)
        process(ctx, result)
    },
)
```

**Benefits**:
1. **Automatic panic recovery** with configurable policy
2. **Context propagation** for cancellation and deadlines
3. **Observability integration** (metrics, traces, logs)
4. **Component identification** for debugging
5. **Graceful degradation** options

## mruntime Package

### Location: `pkg/mruntime/goroutine.go`

### Function Signature

```go
func SafeGoWithContextAndComponent(
    ctx context.Context,
    component string,
    policy PanicRecoveryPolicy,
    fn func(context.Context),
)
```

### Parameters

**ctx** (`context.Context`): Context for cancellation, deadlines, tracing
- Pass the request context or background context
- Context will be propagated to the goroutine function

**component** (`string`): Identifier for which component is running this goroutine
- Examples: `"account-service"`, `"transaction-processor"`, `"balance-calculator"`
- Used in logging and metrics to identify source of errors

**policy** (`PanicRecoveryPolicy`): What to do when panic occurs
- `mruntime.KeepRunning`: Log panic and continue (non-critical goroutine)
- `mruntime.CrashProcess`: Log panic and exit application (critical goroutine)

**fn** (`func(context.Context)`): The function to run in goroutine
- Receives propagated context
- Should respect context cancellation

### Panic Recovery Policies

**KeepRunning** - For non-critical operations
```go
// Example: Background cache refresh
mruntime.SafeGoWithContextAndComponent(
    ctx,
    "cache-refresher",
    mruntime.KeepRunning,  // Don't crash if this fails
    func(ctx context.Context) {
        refreshCache(ctx)
    },
)
```

**CrashProcess** - For critical operations
```go
// Example: Critical event processor
mruntime.SafeGoWithContextAndComponent(
    ctx,
    "transaction-processor",
    mruntime.CrashProcess,  // Crash if this fails - data integrity at risk
    func(ctx context.Context) {
        processTransaction(ctx, tx)
    },
)
```

## Usage Patterns

### Pattern 1: Background Worker

```go
func (s *Service) Start(ctx context.Context) {
    // Start background worker that processes messages
    mruntime.SafeGoWithContextAndComponent(
        ctx,
        "rabbitmq-consumer",
        mruntime.KeepRunning,
        func(ctx context.Context) {
            for {
                select {
                case <-ctx.Done():
                    // Context cancelled - graceful shutdown
                    logger.Info("Shutting down consumer")
                    return
                case msg := <-s.messageQueue:
                    // Process message
                    if err := s.processMessage(ctx, msg); err != nil {
                        logger.Errorf("Failed to process message: %v", err)
                    }
                }
            }
        },
    )
}
```

### Pattern 2: Async API Call

```go
func (h *Handler) CreateAccount(c *fiber.Ctx) error {
    ctx := c.Context()

    // Create account synchronously
    account, err := h.Command.CreateAccount(ctx, input)
    if err != nil {
        return http.WithError(c, err)
    }

    // Send notification asynchronously (fire and forget)
    mruntime.SafeGoWithContextAndComponent(
        context.Background(),  // New context - don't want HTTP timeout to cancel this
        "notification-sender",
        mruntime.KeepRunning,  // Non-critical - don't crash if notification fails
        func(ctx context.Context) {
            if err := h.notificationService.Send(ctx, account); err != nil {
                logger.Errorf("Failed to send notification: %v", err)
            }
        },
    )

    return http.Created(c, account)
}
```

### Pattern 3: Parallel Processing

```go
func (uc *UseCase) CalculateAccountBalances(ctx context.Context, accounts []*Account) error {
    var wg sync.WaitGroup
    errChan := make(chan error, len(accounts))

    for _, account := range accounts {
        wg.Add(1)

        // Launch parallel balance calculation
        mruntime.SafeGoWithContextAndComponent(
            ctx,
            "balance-calculator",
            mruntime.KeepRunning,
            func(ctx context.Context) {
                defer wg.Done()

                balance, err := uc.calculateBalance(ctx, account)
                if err != nil {
                    errChan <- fmt.Errorf("calculating balance for %s: %w", account.ID, err)
                    return
                }

                if err := uc.repo.UpdateBalance(ctx, account.ID, balance); err != nil {
                    errChan <- fmt.Errorf("updating balance for %s: %w", account.ID, err)
                }
            },
        )
    }

    // Wait for all goroutines
    wg.Wait()
    close(errChan)

    // Collect errors
    var errors []error
    for err := range errChan {
        errors = append(errors, err)
    }

    if len(errors) > 0 {
        return fmt.Errorf("balance calculation errors: %v", errors)
    }

    return nil
}
```

### Pattern 4: Timeout with Context

```go
func (uc *UseCase) ProcessWithTimeout(ctx context.Context, data Data) error {
    // Create context with timeout
    ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()

    resultChan := make(chan error, 1)

    // Launch processing with timeout context
    mruntime.SafeGoWithContextAndComponent(
        ctx,
        "data-processor",
        mruntime.KeepRunning,
        func(ctx context.Context) {
            // Long-running operation
            result, err := heavyComputation(ctx, data)
            if err != nil {
                resultChan <- err
                return
            }

            resultChan <- uc.repo.Save(ctx, result)
        },
    )

    // Wait for result or timeout
    select {
    case err := <-resultChan:
        return err
    case <-ctx.Done():
        return fmt.Errorf("processing timed out: %w", ctx.Err())
    }
}
```

## Custom Linter Integration

### panicguard Linter

**Location**: `pkg/mlint/panicguard/`

The custom `panicguard` linter enforces safe goroutine usage:

1. **Blocks bare `go` keyword**
   ```go
   // ❌ Caught by linter
   go func() { doWork() }()
   // Error: "bare go keyword forbidden - use mruntime.SafeGoWithContextAndComponent"
   ```

2. **Blocks misplaced `recover()`**
   ```go
   // ❌ Caught by linter
   func process() {
       recover()  // recover() only works in deferred function
   }
   // Error: "recover() must be called in defer statement"
   ```

3. **Enforces mruntime usage**
   ```go
   // ✅ Approved by linter
   mruntime.SafeGoWithContextAndComponent(ctx, "worker", policy, fn)
   ```

### Running the Linter

```bash
# Build and run custom linter
make lint

# The panicguard plugin is automatically included in golangci-lint config
```

## Context Cancellation Patterns

### Respecting Context Cancellation

```go
func (s *Service) ProcessBatch(ctx context.Context, items []Item) error {
    for _, item := range items {
        // Check if context was cancelled
        select {
        case <-ctx.Done():
            return ctx.Err()  // Stop processing, return cancellation error
        default:
            // Continue processing
        }

        if err := s.processItem(ctx, item); err != nil {
            return fmt.Errorf("processing item %s: %w", item.ID, err)
        }
    }
    return nil
}
```

### Graceful Shutdown

```go
type Server struct {
    httpServer *fiber.App
    workers    []Worker
}

func (s *Server) Start(ctx context.Context) error {
    // Launch all background workers
    for _, worker := range s.workers {
        mruntime.SafeGoWithContextAndComponent(
            ctx,
            worker.Name(),
            mruntime.KeepRunning,
            worker.Run,
        )
    }

    // Start HTTP server
    return s.httpServer.Listen(":8080")
}

func (s *Server) Shutdown() error {
    // Context cancellation will stop all workers
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    // Graceful HTTP shutdown
    if err := s.httpServer.ShutdownWithContext(ctx); err != nil {
        return fmt.Errorf("shutting down HTTP server: %w", err)
    }

    // Workers will stop when context is cancelled
    return nil
}
```

## Observability Integration

### Tracing in Goroutines

```go
func (uc *UseCase) AsyncOperation(ctx context.Context, data Data) error {
    // Start span in parent goroutine
    ctx, span := tracer.Start(ctx, "usecase.async_operation")
    defer span.End()

    // Propagate traced context to goroutine
    mruntime.SafeGoWithContextAndComponent(
        ctx,  // Traced context propagated
        "async-worker",
        mruntime.KeepRunning,
        func(ctx context.Context) {
            // Create child span in goroutine
            ctx, childSpan := tracer.Start(ctx, "async_worker.process")
            defer childSpan.End()

            if err := process(ctx, data); err != nil {
                childSpan.RecordError(err)
                logger.Errorf("Async processing failed: %v", err)
            }
        },
    )

    return nil
}
```

### Metrics for Goroutines

```go
var (
    goroutineStarted = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "goroutine_started_total",
            Help: "Number of goroutines started",
        },
        []string{"component"},
    )

    goroutinePanics = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "goroutine_panics_total",
            Help: "Number of goroutine panics",
        },
        []string{"component"},
    )
)

// mruntime automatically increments these metrics
```

## Common Patterns & Best Practices

### ✅ DO: Use Context for Cancellation

```go
mruntime.SafeGoWithContextAndComponent(
    ctx,
    "worker",
    mruntime.KeepRunning,
    func(ctx context.Context) {
        ticker := time.NewTicker(1 * time.Second)
        defer ticker.Stop()

        for {
            select {
            case <-ctx.Done():
                return  // Graceful exit
            case <-ticker.C:
                doWork(ctx)
            }
        }
    },
)
```

### ✅ DO: Use WaitGroup for Coordination

```go
var wg sync.WaitGroup

for i := 0; i < 10; i++ {
    wg.Add(1)
    mruntime.SafeGoWithContextAndComponent(
        ctx,
        "worker",
        mruntime.KeepRunning,
        func(ctx context.Context) {
            defer wg.Done()
            doWork(ctx)
        },
    )
}

wg.Wait()  // Wait for all workers
```

### ✅ DO: Use Channels for Communication

```go
results := make(chan Result, 10)

mruntime.SafeGoWithContextAndComponent(
    ctx,
    "producer",
    mruntime.KeepRunning,
    func(ctx context.Context) {
        for _, item := range items {
            result := process(ctx, item)
            results <- result
        }
        close(results)
    },
)

// Consume results
for result := range results {
    handleResult(result)
}
```

### ❌ DON'T: Use Bare Go Keyword

```go
// ❌ FORBIDDEN - Will be caught by linter
go func() {
    doWork()
}()
```

### ❌ DON'T: Ignore Context

```go
// ❌ BAD - Ignores context cancellation
mruntime.SafeGoWithContextAndComponent(
    ctx,
    "worker",
    mruntime.KeepRunning,
    func(ctx context.Context) {
        for {
            doWork()  // Never checks ctx.Done() - can't be cancelled!
        }
    },
)
```

### ❌ DON'T: Use Panic for Control Flow

```go
// ❌ BAD - Using panic for business logic
mruntime.SafeGoWithContextAndComponent(
    ctx,
    "worker",
    mruntime.KeepRunning,
    func(ctx context.Context) {
        if !valid {
            panic("invalid data")  // Use error returns instead!
        }
    },
)
```

## Race Condition Prevention

### Shared State Protection

```go
type SafeCounter struct {
    mu    sync.Mutex
    count int
}

func (c *SafeCounter) Increment() {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.count++
}

func (c *SafeCounter) Value() int {
    c.mu.Lock()
    defer c.mu.Unlock()
    return c.count
}

// Usage
counter := &SafeCounter{}

for i := 0; i < 10; i++ {
    mruntime.SafeGoWithContextAndComponent(
        ctx,
        "counter",
        mruntime.KeepRunning,
        func(ctx context.Context) {
            counter.Increment()  // Thread-safe
        },
    )
}
```

### Using sync.Map for Concurrent Access

```go
type Cache struct {
    data sync.Map
}

func (c *Cache) Set(key string, value interface{}) {
    c.data.Store(key, value)  // Thread-safe
}

func (c *Cache) Get(key string) (interface{}, bool) {
    return c.data.Load(key)  // Thread-safe
}
```

### Atomic Operations

```go
import "sync/atomic"

type Stats struct {
    requests int64
    errors   int64
}

func (s *Stats) IncrementRequests() {
    atomic.AddInt64(&s.requests, 1)  // Thread-safe increment
}

func (s *Stats) IncrementErrors() {
    atomic.AddInt64(&s.errors, 1)
}

func (s *Stats) GetRequests() int64 {
    return atomic.LoadInt64(&s.requests)
}
```

## Testing Concurrent Code

### Testing with Race Detector

```bash
# Run tests with race detector
go test -race ./...

# Via Makefile
make test GOFLAGS=-race
```

### Testing Goroutine Behavior

```go
func TestConcurrentProcessing(t *testing.T) {
    t.Parallel()

    ctx := context.Background()
    var wg sync.WaitGroup
    results := make(chan int, 100)

    // Launch multiple goroutines
    for i := 0; i < 100; i++ {
        wg.Add(1)
        i := i  // Capture loop variable
        mruntime.SafeGoWithContextAndComponent(
            ctx,
            "test-worker",
            mruntime.KeepRunning,
            func(ctx context.Context) {
                defer wg.Done()
                results <- i * 2
            },
        )
    }

    // Wait and close
    go func() {
        wg.Wait()
        close(results)
    }()

    // Verify all results received
    count := 0
    for range results {
        count++
    }
    assert.Equal(t, 100, count)
}
```

## Deprecated Patterns

### ❌ Old mruntime.SafeGoWithContext (Deprecated)

```go
// ❌ DEPRECATED - Missing component parameter
mruntime.SafeGoWithContext(ctx, policy, fn)
```

### ✅ Use SafeGoWithContextAndComponent Instead

```go
// ✅ CURRENT - Includes component identification
mruntime.SafeGoWithContextAndComponent(ctx, "component-name", policy, fn)
```

The linter will flag usage of the deprecated function.

## Concurrency Checklist

✅ **Always use mruntime.SafeGoWithContextAndComponent()** - Never bare `go` keyword

✅ **Choose appropriate panic recovery policy** - KeepRunning vs CrashProcess

✅ **Propagate context** - Pass ctx to goroutine function

✅ **Respect context cancellation** - Check `ctx.Done()` in loops

✅ **Identify component** - Provide meaningful component name

✅ **Protect shared state** - Use mutexes, sync.Map, or atomic operations

✅ **Use WaitGroup** for coordinating multiple goroutines

✅ **Use channels** for communication between goroutines

✅ **Test with race detector** - `go test -race`

✅ **Handle goroutine errors** - Don't silently ignore failures

✅ **Graceful shutdown** - Cancel context to stop background workers

## Related Documentation

- Error Handling: `docs/agents/error-handling.md`
- Observability: `docs/agents/observability.md`
- Testing: `docs/agents/testing.md`
- Architecture: `docs/agents/architecture.md`
