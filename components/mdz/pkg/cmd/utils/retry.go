package utils

import (
	"context"
	"fmt"
	"time"

	"github.com/LerianStudio/midaz/components/mdz/pkg/errors"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
)

// ExecuteWithRetry executes a command with retry logic and proper error handling
func ExecuteWithRetry(ctx context.Context, f *factory.Factory, fn func() error) error {
	config := &errors.RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Second,
		MaxDelay:     10 * time.Second,
		Multiplier:   2.0,
		OnRetry: func(attempt int, delay time.Duration, err error) {
			if !f.NoColor {
				fmt.Fprintf(f.IOStreams.Err, "\033[33mAttempt %d failed: %v\033[0m\n", attempt, err)
				fmt.Fprintf(f.IOStreams.Err, "\033[33mRetrying in %v...\033[0m\n", delay)
			} else {
				fmt.Fprintf(f.IOStreams.Err, "Attempt %d failed: %v\n", attempt, err)
				fmt.Fprintf(f.IOStreams.Err, "Retrying in %v...\n", delay)
			}
		},
		RetryCondition: func(err error) bool {
			return errors.IsRetryable(err)
		},
	}

	retryableFn := func(ctx context.Context) error {
		return fn()
	}

	return errors.Retry(ctx, retryableFn, config)
}

// HandleError handles errors with enhanced formatting and suggestions
func HandleError(f *factory.Factory, err error) {
	if err == nil {
		return
	}

	// Extract enhanced error information
	enhancedErr, ok := err.(*errors.Error)
	if ok {
		// Print main error message
		if !f.NoColor {
			fmt.Fprintf(f.IOStreams.Err, "\033[31mError: %s\033[0m\n", enhancedErr.Message)
		} else {
			fmt.Fprintf(f.IOStreams.Err, "Error: %s\n", enhancedErr.Message)
		}

		// Print suggestions if available
		suggestions := errors.GetSuggestions(err)
		if len(suggestions) > 0 {
			fmt.Fprintln(f.IOStreams.Err, "\nSuggestions:")

			for _, suggestion := range suggestions {
				if !f.NoColor {
					fmt.Fprintf(f.IOStreams.Err, "  \033[32m• %s\033[0m\n", suggestion)
				} else {
					fmt.Fprintf(f.IOStreams.Err, "  • %s\n", suggestion)
				}
			}
		}

		// Print context if available
		if len(enhancedErr.Context) > 0 {
			fmt.Fprintln(f.IOStreams.Err, "\nContext:")

			for key, value := range enhancedErr.Context {
				fmt.Fprintf(f.IOStreams.Err, "  %s: %s\n", key, value)
			}
		}
	} else {
		// Fallback for regular errors
		if !f.NoColor {
			fmt.Fprintf(f.IOStreams.Err, "\033[31mError: %v\033[0m\n", err)
		} else {
			fmt.Fprintf(f.IOStreams.Err, "Error: %v\n", err)
		}
	}
}
