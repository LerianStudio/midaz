package mruntime_test

import (
	"context"
	"fmt"
	"log"

	"github.com/LerianStudio/midaz/v3/pkg/mruntime"
)

// simpleLogger is a minimal logger for examples.
type simpleLogger struct{}

func (l *simpleLogger) Errorf(format string, args ...any) {
	log.Printf("[ERROR] "+format, args...)
}

func ExampleSafeGoWithContext() {
	ctx := context.Background()
	logger := &simpleLogger{}

	// Launch a goroutine with panic recovery and observability
	done := make(chan struct{})
	mruntime.SafeGoWithContextAndComponent(ctx, logger, "transaction", "example-worker", mruntime.KeepRunning,
		func(ctx context.Context) {
			defer close(done)
			fmt.Println("Worker started")
			// Work happens here...
			fmt.Println("Worker completed")
		})

	<-done
	// Output:
	// Worker started
	// Worker completed
}

func ExampleRecoverAndLogWithContext() {
	ctx := context.Background()
	logger := &simpleLogger{}

	func() {
		defer mruntime.RecoverAndLogWithContext(ctx, logger, "example", "handler")
		fmt.Println("Before panic")
		// If a panic occurred here, it would be recovered and logged
		fmt.Println("After (no panic)")
	}()

	fmt.Println("Function completed normally")
	// Output:
	// Before panic
	// After (no panic)
	// Function completed normally
}

func ExampleInitPanicMetrics() {
	// During application startup, after telemetry initialization:
	// tl := opentelemetry.InitializeTelemetry(cfg)
	// mruntime.InitPanicMetrics(tl.MetricsFactory)

	// Nil is safe (no-op):
	mruntime.InitPanicMetrics(nil)

	// Metrics remain uninitialized until properly configured.
	pm := mruntime.GetPanicMetrics()
	fmt.Printf("Metrics initialized: %v\n", pm != nil)
	// Output:
	// Metrics initialized: false
}

func ExampleSetErrorReporter() {
	// Create a custom error reporter (e.g., for Sentry)
	reporter := &customReporter{}

	// Configure during startup
	mruntime.SetErrorReporter(reporter)

	// Later, panics will be reported automatically
	fmt.Println("Error reporter configured")

	// Clean up
	mruntime.SetErrorReporter(nil)
	// Output:
	// Error reporter configured
}

type customReporter struct{}

func (r *customReporter) CaptureException(ctx context.Context, err error, tags map[string]string) {
	// In a real implementation, this would send to Sentry or similar
	log.Printf("Captured exception: %v, tags: %v", err, tags)
}
