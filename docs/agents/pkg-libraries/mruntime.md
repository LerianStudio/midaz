# pkg/mruntime - Safe Goroutine Handling

**Location**: `pkg/mruntime/`
**Priority**: ⚠️ REQUIRED - Enforced by panicguard linter
**Status**: Production-ready with full observability

Policy-based panic recovery for goroutines with OpenTelemetry integration (metrics, tracing, error reporting).

## Design Philosophy

**Never use bare `go` keyword** - Enforced by custom panicguard linter
**Always use `mruntime.SafeGo*()`** - With panic recovery and observability
**Choose recovery policy** - KeepRunning for handlers, CrashProcess for critical sections

## Panic Policies

```go
const (
    // KeepRunning - log panic and continue execution
    // Use for: HTTP/gRPC handlers, worker goroutines
    KeepRunning PanicPolicy = iota

    // CrashProcess - log panic and re-panic to crash
    // Use for: Critical invariant violations where continuing would corrupt data
    CrashProcess
)
```

## Core Functions

### SafeGoWithContextAndComponent (Recommended)

Launch goroutine with full observability (context, component name, panic recovery):

```go
func SafeGoWithContextAndComponent(
    ctx context.Context,
    logger Logger,
    component, name string,
    policy PanicPolicy,
    fn func(context.Context),
)
```

**Usage:**
```go
// Recommended - full observability
mruntime.SafeGoWithContextAndComponent(ctx, logger, "transaction", "balance-sync",
    mruntime.KeepRunning, func(ctx context.Context) {
        syncBalances(ctx)
    })

// Background worker with KeepRunning
mruntime.SafeGoWithContextAndComponent(ctx, logger, "onboarding", "email-sender",
    mruntime.KeepRunning, func(ctx context.Context) {
        sendWelcomeEmail(ctx, userID)
    })

// Critical section with CrashProcess
mruntime.SafeGoWithContextAndComponent(ctx, logger, "transaction", "balance-commit",
    mruntime.CrashProcess, func(ctx context.Context) {
        commitBalanceUpdate(ctx)  // If this panics, crash the process
    })
```

### SafeGoWithContext

Launch goroutine with context (no component label):

```go
func SafeGoWithContext(
    ctx context.Context,
    logger Logger,
    name string,
    policy PanicPolicy,
    fn func(context.Context),
)
```

**Usage:**
```go
mruntime.SafeGoWithContext(ctx, logger, "background-task", mruntime.KeepRunning,
    func(ctx context.Context) {
        doWork(ctx)
    })
```

### SafeGo (Basic)

Launch goroutine without observability (not recommended for production):

```go
func SafeGo(
    logger Logger,
    name string,
    policy PanicPolicy,
    fn func(),
)
```

**Usage:**
```go
// Not recommended - no observability
mruntime.SafeGo(logger, "background-task", mruntime.KeepRunning, func() {
    doWork()
})
```

## Deferred Recovery Functions

Use these in `defer` statements for panic recovery:

### RecoverWithPolicyAndContext (Recommended)

```go
func RecoverWithPolicyAndContext(
    ctx context.Context,
    logger Logger,
    component, name string,
    policy PanicPolicy,
)
```

**Usage:**
```go
func (h *AccountHandler) CreateAccount(c *fiber.Ctx) error {
    ctx := c.UserContext()
    logger, _, _, _ := libCommons.NewTrackingFromContext(ctx)

    defer mruntime.RecoverWithPolicyAndContext(ctx, logger, "onboarding",
        "handler.create_account", mruntime.KeepRunning)

    // Handler logic - panics will be logged and converted to 500 response
    // ...
}
```

### RecoverAndLogWithContext

Always use KeepRunning policy:

```go
func RecoverAndLogWithContext(
    ctx context.Context,
    logger Logger,
    component, name string,
)
```

**Usage:**
```go
func (r *AccountRepository) Create(ctx context.Context, acc *Account) (*Account, error) {
    logger, _, _, _ := libCommons.NewTrackingFromContext(ctx)

    defer mruntime.RecoverAndLogWithContext(ctx, logger, "onboarding", "postgres.create_account")

    // Repository logic - panics will be logged but not crash the process
    // ...
}
```

### RecoverAndCrash

Always use CrashProcess policy:

```go
func RecoverAndCrash(logger Logger, name string)
```

**Usage:**
```go
func CommitBalanceUpdate(balance *Balance) {
    defer mruntime.RecoverAndCrash(logger, "commit_balance_update")

    // Critical operation - if this panics, crash the process
    // because continuing could corrupt financial data
    // ...
}
```

### Additional Recovery Functions

#### RecoverAndLog
Basic recovery without context:
```go
func RecoverAndLog(logger Logger, name string)
```

#### RecoverAndCrashWithContext
Context-aware crash recovery:
```go
func RecoverAndCrashWithContext(ctx context.Context, logger Logger, component, name string)
```

#### RecoverWithPolicy
Policy-based recovery without context:
```go
func RecoverWithPolicy(logger Logger, name string, policy PanicPolicy)
```

## Usage Patterns

### HTTP Handler Pattern

```go
func (h *AccountHandler) CreateAccount(c *fiber.Ctx) error {
    ctx := c.UserContext()
    logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

    // Recover from panics and convert to 500 response
    defer mruntime.RecoverAndLogWithContext(ctx, logger, "onboarding", "handler.create_account")

    // Handler logic
    // If assert.That() panics here, mruntime catches it and logs it
    assert.NotNil(input, "input must not be nil")

    account, err := h.Command.CreateAccount(ctx, organizationID, ledgerID, input)
    // ...
}
```

### Background Worker Pattern

```go
func StartBalanceSyncWorker(ctx context.Context) {
    logger, _, _, _ := libCommons.NewTrackingFromContext(ctx)

    // Launch worker with KeepRunning policy
    mruntime.SafeGoWithContextAndComponent(ctx, logger, "transaction", "balance-sync-worker",
        mruntime.KeepRunning, func(ctx context.Context) {
            ticker := time.NewTicker(5 * time.Minute)
            defer ticker.Stop()

            for {
                select {
                case <-ctx.Done():
                    return
                case <-ticker.C:
                    syncBalances(ctx)  // Panics here won't crash the process
                }
            }
        })
}
```

### Critical Section Pattern

```go
func (r *BalanceRepository) UpdateBalance(ctx context.Context, balance *Balance) error {
    logger, _, _, _ := libCommons.NewTrackingFromContext(ctx)

    // Crash if this panics - balance corruption is unacceptable
    defer mruntime.RecoverWithPolicyAndContext(ctx, logger, "transaction",
        "update_balance", mruntime.CrashProcess)

    // Critical balance update
    // If assert.That() panics here, the process crashes
    assert.That(balance.Amount >= 0, "balance must not be negative",
        "balance", balance.Amount)

    // ... update logic
    return nil
}
```

## Observability Integration

### Metrics

Records `panic_recovered_total` counter with labels:
- `component` - Service component (e.g., "transaction", "onboarding")
- `goroutine_name` - Goroutine identifier
- `policy` - Panic policy (KeepRunning, CrashProcess)

**Initialization:**
```go
tl := opentelemetry.InitializeTelemetry(cfg)
mruntime.InitPanicMetrics(tl.MetricsFactory)
```

### Tracing

Records `panic.recovered` span events with:
- Panic message
- Stack trace
- Component and goroutine name
- Sets span status to Error

**Automatic Integration:**
```go
// Just pass context - tracing is automatic
mruntime.SafeGoWithContextAndComponent(ctx, logger, "transaction", "worker",
    mruntime.KeepRunning, func(ctx context.Context) {
        // Panics here create span events automatically
    })
```

### Error Reporting

Optional integration with Sentry or similar services:

```go
// Initialize error reporter
mruntime.SetErrorReporter(sentryReporter)

// Now all panics are reported to Sentry
mruntime.SafeGoWithContextAndComponent(ctx, logger, "transaction", "worker",
    mruntime.KeepRunning, func(ctx context.Context) {
        panic("something went wrong")  // Reported to Sentry
    })
```

## Custom Linter Integration

The `mlint/panicguard` custom linter enforces safe goroutine usage:

**Blocked:**
```go
// ❌ COMPILATION ERROR - caught by panicguard linter
go func() {
    doWork()
}()
```

**Required:**
```go
// ✅ ALLOWED - uses mruntime
mruntime.SafeGoWithContextAndComponent(ctx, logger, "component", "name",
    mruntime.KeepRunning, func(ctx context.Context) {
        doWork()
    })
```

See [`specialized.md`](./specialized.md#mlint) for linter details.

## Policy Decision Guide

| Scenario | Policy | Rationale |
|----------|--------|-----------|
| HTTP/gRPC handlers | KeepRunning | Don't crash on single request failure |
| Background workers | KeepRunning | Don't crash on single task failure |
| Queue consumers | KeepRunning | Don't crash on single message failure |
| Balance updates | CrashProcess | Financial data corruption is unacceptable |
| Transaction commits | CrashProcess | Partial commits corrupt ledger state |
| Database migrations | CrashProcess | Partial migrations corrupt schema |

**Default: KeepRunning** - Use CrashProcess only for critical invariants.

## Integration with assert

Assertion panics are designed to be caught by mruntime:

```go
func (h *Handler) Process(c *fiber.Ctx) error {
    defer mruntime.RecoverAndLogWithContext(ctx, logger, "component", "handler")

    // If this panics, mruntime catches it:
    // - Logs full panic message with stack trace
    // - Records metric (panic_recovered_total)
    // - Creates span event (panic.recovered)
    // - Applies policy (KeepRunning → continues, CrashProcess → crashes)
    assert.NotNil(input, "input must not be nil")

    // ... handler logic
}
```

See [`assert.md`](./assert.md) for assertion patterns.

## Anti-Patterns

### ❌ Don't Use Bare go Keyword

```go
// ❌ COMPILATION ERROR - panicguard linter blocks this
go func() {
    doWork()
}()
```

### ❌ Don't Ignore Context

```go
// ❌ BAD - loses observability
mruntime.SafeGo(logger, "worker", mruntime.KeepRunning, func() {
    doWork()  // No context, no tracing
})

// ✅ GOOD - includes context
mruntime.SafeGoWithContextAndComponent(ctx, logger, "component", "worker",
    mruntime.KeepRunning, func(ctx context.Context) {
        doWork(ctx)  // Context propagated, tracing works
    })
```

### ❌ Don't Use CrashProcess for Non-Critical Code

```go
// ❌ BAD - crashes on email failure
mruntime.SafeGoWithContextAndComponent(ctx, logger, "notification", "email-sender",
    mruntime.CrashProcess, func(ctx context.Context) {
        sendEmail(ctx)  // Email failure shouldn't crash the process
    })

// ✅ GOOD - logs and continues on email failure
mruntime.SafeGoWithContextAndComponent(ctx, logger, "notification", "email-sender",
    mruntime.KeepRunning, func(ctx context.Context) {
        sendEmail(ctx)  // Logs failure but continues
    })
```

## References

- **Source**: `pkg/mruntime/goroutine.go:1`, `pkg/mruntime/recover.go:1`
- **Documentation**: `pkg/mruntime/doc.go:1`
- **Policy**: `pkg/mruntime/policy.go:1`
- **Tests**: `pkg/mruntime/goroutine_test.go:1`
- **Custom Linter**: `pkg/mlint/panicguard/`
- **Related**: [`assert.md`](./assert.md) for assertions
- **Related**: `docs/agents/concurrency.md` for concurrency patterns

## Summary

`pkg/mruntime` provides safe goroutine handling:

1. **Always use SafeGo functions** - Never bare `go` keyword (enforced by linter)
2. **Prefer SafeGoWithContextAndComponent** - Full observability
3. **Choose policy carefully** - KeepRunning (default) vs CrashProcess (critical)
4. **Integrate with observability** - Metrics, tracing, error reporting
5. **Combine with assert** - Assertions + mruntime = observable bugs without crashes

The combination of enforced safety + panic recovery + observability = reliable concurrent systems.
