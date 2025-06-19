package mt001_test

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"demo-data/internal/adapters/primary/cli"
	"demo-data/internal/adapters/secondary/config"
	"demo-data/internal/infrastructure/di"
	"demo-data/internal/infrastructure/logging"
)

func TestPerformanceBaselines(t *testing.T) {
	t.Run("CLI startup time", func(t *testing.T) {
		start := time.Now()

		// Initialize complete application
		container := di.NewContainer()
		logger, err := logging.NewLogger(false, "error") // Minimal logging for performance
		require.NoError(t, err)
		container.SetLogger(logger)

		configAdapter := config.NewViperConfigAdapter()
		container.SetConfigurationPort(configAdapter)

		cliAdapter := cli.NewCLIAdapter(container)

		elapsed := time.Since(start)

		// CLI should start in under 500ms as per acceptance criteria
		assert.Less(t, elapsed, 500*time.Millisecond,
			"CLI startup took %v, should be under 500ms", elapsed)

		// Verify it actually works
		assert.NotNil(t, cliAdapter)
	})

	t.Run("configuration loading performance", func(t *testing.T) {
		configAdapter := config.NewViperConfigAdapter()
		ctx := context.Background()

		start := time.Now()
		config, err := configAdapter.Load(ctx)
		elapsed := time.Since(start)

		require.NoError(t, err)
		assert.NotNil(t, config)

		// Configuration loading should be fast
		assert.Less(t, elapsed, 100*time.Millisecond,
			"Configuration loading took %v, should be under 100ms", elapsed)
	})

	t.Run("logger creation performance", func(t *testing.T) {
		start := time.Now()

		logger, err := logging.NewLogger(false, "info")

		elapsed := time.Since(start)

		require.NoError(t, err)
		assert.NotNil(t, logger)

		// Logger creation should be very fast
		assert.Less(t, elapsed, 50*time.Millisecond,
			"Logger creation took %v, should be under 50ms", elapsed)
	})

	t.Run("memory usage stability", func(t *testing.T) {
		var m1, m2 runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&m1)

		// Create and dispose of multiple components
		for i := 0; i < 100; i++ {
			container := di.NewContainer()
			logger, _ := logging.NewLogger(false, "error")
			container.SetLogger(logger)

			configAdapter := config.NewViperConfigAdapter()
			container.SetConfigurationPort(configAdapter)

			_ = cli.NewCLIAdapter(container)
		}

		runtime.GC()
		runtime.ReadMemStats(&m2)

		// Memory usage should not grow excessively
		memoryGrowth := m2.Alloc - m1.Alloc
		assert.Less(t, memoryGrowth, uint64(10*1024*1024), // 10MB threshold
			"Memory usage grew by %d bytes, should be under 10MB", memoryGrowth)
	})
}

func BenchmarkCLIOperations(b *testing.B) {
	// Setup shared components
	container := di.NewContainer()
	logger, _ := logging.NewLogger(false, "error") // Minimal logging for benchmarks
	container.SetLogger(logger)

	configAdapter := config.NewViperConfigAdapter()
	container.SetConfigurationPort(configAdapter)

	cliAdapter := cli.NewCLIAdapter(container)
	ctx := context.Background()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		err := cliAdapter.ExecuteWithArgs(ctx, []string{"version"})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkConfigurationLoading(b *testing.B) {
	configAdapter := config.NewViperConfigAdapter()
	ctx := context.Background()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		config, err := configAdapter.Load(ctx)
		if err != nil {
			b.Fatal(err)
		}
		if config == nil {
			b.Fatal("config is nil")
		}
	}
}

func BenchmarkLoggerCreation(b *testing.B) {
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		logger, err := logging.NewLogger(false, "info")
		if err != nil {
			b.Fatal(err)
		}
		if logger == nil {
			b.Fatal("logger is nil")
		}
	}
}

func BenchmarkDependencyInjection(b *testing.B) {
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		container := di.NewContainer()

		logger, err := logging.NewLogger(false, "error")
		if err != nil {
			b.Fatal(err)
		}
		container.SetLogger(logger)

		configAdapter := config.NewViperConfigAdapter()
		container.SetConfigurationPort(configAdapter)

		if container.GetConfigurationPort() == nil {
			b.Fatal("configuration port is nil")
		}
		if container.GetLogger() == nil {
			b.Fatal("logger is nil")
		}
	}
}

func BenchmarkCompleteWorkflow(b *testing.B) {
	// This benchmark tests the complete workflow from container creation to CLI execution
	ctx := context.Background()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Full initialization
		container := di.NewContainer()

		logger, err := logging.NewLogger(false, "error")
		if err != nil {
			b.Fatal(err)
		}
		container.SetLogger(logger)

		configAdapter := config.NewViperConfigAdapter()
		container.SetConfigurationPort(configAdapter)

		cliAdapter := cli.NewCLIAdapter(container)

		// Execute command
		err = cliAdapter.ExecuteWithArgs(ctx, []string{"version"})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func TestConcurrency(t *testing.T) {
	t.Run("concurrent CLI creation", func(t *testing.T) {
		const numGoroutines = 5

		// Test concurrent creation of CLI adapters (not execution)
		done := make(chan error, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				// Each goroutine creates its own container and CLI
				container := di.NewContainer()
				logger, err := logging.NewLogger(false, "error")
				if err != nil {
					done <- err
					return
				}
				container.SetLogger(logger)

				configAdapter := config.NewViperConfigAdapter()
				container.SetConfigurationPort(configAdapter)

				cliAdapter := cli.NewCLIAdapter(container)
				if cliAdapter == nil {
					done <- fmt.Errorf("CLI adapter is nil")
					return
				}

				done <- nil
			}(i)
		}

		// Wait for all goroutines to complete
		for i := 0; i < numGoroutines; i++ {
			select {
			case err := <-done:
				assert.NoError(t, err, "Concurrent creation should not fail")
			case <-time.After(10 * time.Second):
				t.Fatal("Concurrent test timed out")
			}
		}
	})
}
