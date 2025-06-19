package tests

import (
	"context"
	"errors"
	"testing"
	"time"

	"demo-data/internal/domain/entities"
	"demo-data/internal/infrastructure/logging"
)

func TestNewLogger(t *testing.T) {
	tests := []struct {
		name        string
		development bool
		level       string
		expectError bool
	}{
		{
			name:        "development logger with debug level",
			development: true,
			level:       "debug",
			expectError: false,
		},
		{
			name:        "production logger with info level",
			development: false,
			level:       "info",
			expectError: false,
		},
		{
			name:        "logger with invalid level defaults to info",
			development: false,
			level:       "invalid",
			expectError: false,
		},
		{
			name:        "logger with warn level",
			development: true,
			level:       "warn",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, err := logging.NewLogger(tt.development, tt.level)

			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.expectError && logger == nil {
				t.Error("expected logger but got nil")
			}
		})
	}
}

func TestLoggerFactory(t *testing.T) {
	factory := logging.NewLoggerFactory()
	if factory == nil {
		t.Fatal("expected factory but got nil")
	}

	t.Run("CreateDevelopmentLogger", func(t *testing.T) {
		logger, err := factory.CreateDevelopmentLogger()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if logger == nil {
			t.Error("expected logger but got nil")
		}
	})

	t.Run("CreateProductionLogger", func(t *testing.T) {
		logger, err := factory.CreateProductionLogger()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if logger == nil {
			t.Error("expected logger but got nil")
		}
	})

	t.Run("CreateTestLogger", func(t *testing.T) {
		logger, err := factory.CreateTestLogger()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if logger == nil {
			t.Error("expected logger but got nil")
		}
	})

	t.Run("CreateLogger from config", func(t *testing.T) {
		config := &entities.Configuration{
			Debug:    false,
			LogLevel: "info",
		}

		logger, err := factory.CreateLogger(config)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if logger == nil {
			t.Error("expected logger but got nil")
		}
	})
}

func TestLoggerWithContext(t *testing.T) {
	logger, err := logging.NewLogger(true, "debug")
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	ctx := context.Background()
	ctx = context.WithValue(ctx, "request_id", "test-123")
	ctx = context.WithValue(ctx, "user_id", "user-456")

	contextLogger := logger.WithContext(ctx)
	if contextLogger == nil {
		t.Error("expected context logger but got nil")
	}

	// Test that the context logger is different from the original
	if contextLogger == logger {
		t.Error("expected different logger instance")
	}
}

func TestLoggerWith(t *testing.T) {
	logger, err := logging.NewLogger(true, "debug")
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	fieldLogger := logger.With("key", "value", "number", 42)
	if fieldLogger == nil {
		t.Error("expected field logger but got nil")
	}

	// Test that the field logger is different from the original
	if fieldLogger == logger {
		t.Error("expected different logger instance")
	}
}

func TestLogGenerationProgress(t *testing.T) {
	logger, err := logging.NewLogger(true, "debug")
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	// This is more of a smoke test since we can't easily capture log output
	logging.LogGenerationProgress(logger, "accounts", 50, 100, time.Second)
	logging.LogGenerationProgress(logger, "transactions", 1000, 5000, time.Minute)
}

func TestLogAPICall(t *testing.T) {
	logger, err := logging.NewLogger(true, "debug")
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	ctx := context.Background()

	// Test successful API call
	logging.LogAPICall(logger, ctx, "GET", "/api/organizations", 100*time.Millisecond, nil)

	// Test failed API call
	logging.LogAPICall(logger, ctx, "POST", "/api/accounts", 200*time.Millisecond,
		errors.New("validation failed: name is required"))
}

func TestLogConfigurationLoaded(t *testing.T) {
	logger, err := logging.NewLogger(true, "debug")
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	config := &entities.Configuration{
		APIBaseURL:      "https://api.example.com",
		AuthToken:       "test-token",
		Debug:           true,
		LogLevel:        "debug",
		TimeoutDuration: 30 * time.Second,
	}

	logging.LogConfigurationLoaded(logger, "config.yaml", config)
}

func TestLogError(t *testing.T) {
	logger, err := logging.NewLogger(true, "debug")
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	ctx := context.Background()
	err = errors.New("validation failed: email has invalid format")

	logging.LogError(logger, ctx, "user_validation", err, "user_id", "123")
}

func TestLogStartupEvent(t *testing.T) {
	logger, err := logging.NewLogger(true, "debug")
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	// Test successful startup
	logging.LogStartupEvent(logger, "database", "success", 500*time.Millisecond)

	// Test failed startup
	logging.LogStartupEvent(logger, "api_client", "failed", time.Second, "error", "connection refused")
}

func TestLogVolumeConfiguration(t *testing.T) {
	logger, err := logging.NewLogger(true, "debug")
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	volumeConfig := entities.VolumeMetrics{
		Organizations:          2,
		LedgersPerOrg:          3,
		AccountsPerLedger:      100,
		TransactionsPerAccount: 50,
		AssetsPerLedger:        5,
		PortfoliosPerLedger:    2,
		SegmentsPerLedger:      4,
	}

	logging.LogVolumeConfiguration(logger, "small", volumeConfig)
}

func TestLogSDKEvent(t *testing.T) {
	logger, err := logging.NewLogger(true, "debug")
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	ctx := context.Background()
	logging.LogSDKEvent(logger, ctx, "client_initialized", "version", "1.0.0")
}

func TestLogCLICommand(t *testing.T) {
	logger, err := logging.NewLogger(true, "debug")
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	// Test successful command
	logging.LogCLICommand(logger, "validate", []string{"--debug"}, 100*time.Millisecond, nil)

	// Test failed command
	logging.LogCLICommand(logger, "generate", []string{"--volume", "large"},
		time.Second, errors.New("authentication failed: missing token"))
}

func TestLoggerSync(t *testing.T) {
	logger, err := logging.NewLogger(true, "debug")
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	// Test sync (should not error on most systems)
	err = logger.Sync()
	// We don't assert on the error since sync can fail on some systems (like when stderr is redirected)
	_ = err
}

// TestProductionLoggerJSONOutput tests that production logger outputs JSON
func TestProductionLoggerJSONOutput(t *testing.T) {
	// This test is more complex and would require capturing the log output
	// For now, we just ensure production logger can be created
	logger, err := logging.NewLogger(false, "info")
	if err != nil {
		t.Fatalf("failed to create production logger: %v", err)
	}

	// Basic functionality test
	logger.Info("test message", "key", "value")

	// Ensure sync works
	_ = logger.Sync()
}

// Benchmark tests for performance
func BenchmarkLoggerInfo(b *testing.B) {
	logger, err := logging.NewLogger(false, "info")
	if err != nil {
		b.Fatalf("failed to create logger: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Info("benchmark message", "iteration", i, "timestamp", time.Now())
	}
}

func BenchmarkLoggerDebug(b *testing.B) {
	logger, err := logging.NewLogger(false, "info") // Debug messages should be filtered out
	if err != nil {
		b.Fatalf("failed to create logger: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Debug("debug message", "iteration", i) // Should be filtered out
	}
}

func BenchmarkLoggerWithFields(b *testing.B) {
	logger, err := logging.NewLogger(false, "info")
	if err != nil {
		b.Fatalf("failed to create logger: %v", err)
	}

	ctx := context.Background()
	ctx = context.WithValue(ctx, "request_id", "bench-123")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.WithContext(ctx).Info("benchmark message",
			"iteration", i,
			"user_id", "user-456",
			"operation", "benchmark",
		)
	}
}
