// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

// CircuitBreaker is the resilience seam the connectors run datasource I/O
// through. It is satisfied by the reporter's existing
// pkg/reporter.CircuitBreakerManager (via the Execute method), so the embedded
// engine inherits the same per-datasource breaker policy the remote-fetcher
// path used — no second resilience implementation. Declaring the narrow
// interface here keeps this package unit-testable and inverts the dependency
// onto the breaker the host already wires.
type CircuitBreaker interface {
	// Execute runs fn through the per-datasource breaker. A breaker in the open
	// state fast-fails without invoking fn.
	Execute(datasourceName string, fn func() (any, error)) (any, error)
}

// noopBreaker runs every operation directly, with no breaker semantics. It is
// the default when the host wires no breaker (e.g. unit tests), so connector
// code never needs a nil check.
type noopBreaker struct{}

func (noopBreaker) Execute(_ string, fn func() (any, error)) (any, error) { return fn() }
