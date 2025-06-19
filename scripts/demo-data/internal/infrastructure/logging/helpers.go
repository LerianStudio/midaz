package logging

import (
	"context"
	"time"

	"demo-data/internal/domain/entities"
)

// LogGenerationProgress logs progress with structured fields for data generation operations
func LogGenerationProgress(logger Logger, entityType string, current, total int, duration time.Duration) {
	progressPercent := float64(current) / float64(total) * 100
	var ratePerSecond float64
	if duration.Seconds() > 0 {
		ratePerSecond = float64(current) / duration.Seconds()
	}

	logger.Info("Generation progress",
		"entity_type", entityType,
		"current", current,
		"total", total,
		"progress_percent", progressPercent,
		"duration_ms", duration.Milliseconds(),
		"rate_per_second", ratePerSecond,
	)
}

// LogAPICall logs API calls with timing and status information
func LogAPICall(logger Logger, ctx context.Context, method, endpoint string, duration time.Duration, err error) {
	fields := []any{
		"method", method,
		"endpoint", endpoint,
		"duration_ms", duration.Milliseconds(),
	}

	if err != nil {
		fields = append(fields, "error", err.Error())
		logger.WithContext(ctx).Error("API call failed", fields...)
	} else {
		logger.WithContext(ctx).Debug("API call completed", fields...)
	}
}

// LogConfigurationLoaded logs configuration loading events with key configuration details
func LogConfigurationLoaded(logger Logger, source string, config *entities.Configuration) {
	logger.Info("Configuration loaded",
		"source", source,
		"api_url", config.APIBaseURL,
		"debug", config.Debug,
		"log_level", config.LogLevel,
		"timeout_ms", config.TimeoutDuration.Milliseconds(),
		"auth_configured", config.AuthToken != "",
	)
}

// LogError logs errors with structured context for better error tracking
func LogError(logger Logger, ctx context.Context, operation string, err error, additionalFields ...any) {
	fields := []any{
		"operation", operation,
		"error", err.Error(),
		"error_type", getErrorType(err),
	}
	fields = append(fields, additionalFields...)

	logger.WithContext(ctx).Error("Operation failed", fields...)
}

// LogStartupEvent logs application startup events
func LogStartupEvent(logger Logger, component string, status string, duration time.Duration, additionalFields ...any) {
	fields := []any{
		"component", component,
		"status", status,
		"startup_duration_ms", duration.Milliseconds(),
	}
	fields = append(fields, additionalFields...)

	if status == "success" {
		logger.Info("Component started", fields...)
	} else {
		logger.Error("Component startup failed", fields...)
	}
}

// LogVolumeConfiguration logs volume configuration details for debugging
func LogVolumeConfiguration(logger Logger, volumeType string, config entities.VolumeMetrics) {
	totalLedgers := config.Organizations * config.LedgersPerOrg
	totalAccounts := totalLedgers * config.AccountsPerLedger
	totalTransactions := totalAccounts * config.TransactionsPerAccount

	logger.Debug("Volume configuration",
		"volume_type", volumeType,
		"organizations", config.Organizations,
		"ledgers_per_org", config.LedgersPerOrg,
		"accounts_per_ledger", config.AccountsPerLedger,
		"transactions_per_account", config.TransactionsPerAccount,
		"estimated_total_ledgers", totalLedgers,
		"estimated_total_accounts", totalAccounts,
		"estimated_total_transactions", totalTransactions,
	)
}

// LogSDKEvent logs SDK-related events for debugging and monitoring
func LogSDKEvent(logger Logger, ctx context.Context, event string, additionalFields ...any) {
	fields := []any{
		"sdk_event", event,
		"component", "midaz_sdk",
	}
	fields = append(fields, additionalFields...)

	logger.WithContext(ctx).Debug("SDK event", fields...)
}

// LogCLICommand logs CLI command execution events
func LogCLICommand(logger Logger, command string, args []string, duration time.Duration, err error) {
	fields := []any{
		"command", command,
		"args", args,
		"execution_duration_ms", duration.Milliseconds(),
	}

	if err != nil {
		fields = append(fields, "error", err.Error())
		logger.Error("CLI command failed", fields...)
	} else {
		logger.Info("CLI command completed", fields...)
	}
}

// getErrorType extracts error type information for better error categorization
func getErrorType(err error) string {
	if err == nil {
		return "none"
	}

	// Check for common error patterns
	errStr := err.Error()
	switch {
	case containsAny(errStr, "timeout", "deadline exceeded"):
		return "timeout"
	case containsAny(errStr, "connection refused", "no route to host"):
		return "network"
	case containsAny(errStr, "unauthorized", "forbidden"):
		return "auth"
	case containsAny(errStr, "not found"):
		return "not_found"
	case containsAny(errStr, "validation", "invalid"):
		return "validation"
	default:
		return "unknown"
	}
}

// containsAny checks if a string contains any of the given substrings
func containsAny(s string, substrings ...string) bool {
	for _, substring := range substrings {
		if len(s) >= len(substring) {
			for i := 0; i <= len(s)-len(substring); i++ {
				if s[i:i+len(substring)] == substring {
					return true
				}
			}
		}
	}
	return false
}
