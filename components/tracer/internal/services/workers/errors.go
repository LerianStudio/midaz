// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package workers

import "errors"

// Shared sentinel errors for worker components.
// These errors are used across multiple workers for consistent error handling.
var (
	// ErrNilLogger is returned when a required logger dependency is nil.
	ErrNilLogger = errors.New("logger cannot be nil")
	// ErrNilRepository is returned when a required repository dependency is nil.
	ErrNilRepository = errors.New("repository cannot be nil")
	// ErrInvalidCleanupInterval is returned when cleanup interval is not positive.
	ErrInvalidCleanupInterval = errors.New("cleanup interval must be positive")
	// ErrNilRuleCache is returned when the required rule cache dependency is nil.
	ErrNilRuleCache = errors.New("rule cache cannot be nil")
	// ErrNilExpressionCompiler is returned when the required expression compiler dependency is nil.
	ErrNilExpressionCompiler = errors.New("expression compiler cannot be nil")
	// ErrInvalidPollInterval is returned when poll interval is not positive.
	ErrInvalidPollInterval = errors.New("poll interval must be positive")
	// ErrInvalidStalenessThreshold is returned when staleness threshold is not positive.
	ErrInvalidStalenessThreshold = errors.New("staleness threshold must be positive")
	// ErrInvalidOverlapBuffer is returned when overlap buffer is negative.
	ErrInvalidOverlapBuffer = errors.New("overlap buffer must be non-negative")
	// ErrNilCircuitBreaker is returned when the required circuit breaker dependency is nil.
	ErrNilCircuitBreaker = errors.New("circuit breaker cannot be nil")
	// ErrTenantCapReached is returned by WorkerSupervisor.EnsureWorkers when the
	// active tenant count has reached MaxTenants. The HTTP lazy-spawn middleware
	// matches on this sentinel to surface 503 + Retry-After to the client so
	// cap events are visible to operators (M18).
	ErrTenantCapReached = errors.New("supervisor: tenant worker cap reached")
)
