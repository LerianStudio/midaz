// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	fetcher "github.com/LerianStudio/fetcher/pkg/engine"
)

// The engine boundary contract (fetcher's *EngineError) requires every error
// crossing back into the engine to be a category-classified, credential-free
// value. These constructors wrap the raw cause for errors.As/errors.Is
// transparency WITHOUT rendering it, so the public string stays the safe
// message while the host can still recognize a typed error it raised. They are
// the single place this adapter mints boundary errors so categorisation stays
// consistent across the connector, factory, and resolver.

// NewEngineValidationError builds a CategoryValidation engine error. Use it for
// malformed descriptors, missing tenant identity, and similar caller faults.
func NewEngineValidationError(message string) *fetcher.EngineError {
	return fetcher.NewEngineError(fetcher.CategoryValidation, message)
}

// NewEngineConnectError builds a CategoryConnect engine error, preserving the
// cause for inspection. Use it when establishing connectivity fails (the
// TestConnection stage), distinct from a read failure on a live connection.
func NewEngineConnectError(message string, cause error) *fetcher.EngineError {
	return fetcher.NewWrappedEngineError(fetcher.CategoryConnect, message, cause)
}

// NewEngineUnavailableError builds a CategoryUnavailable engine error,
// preserving the cause. Use it when a dependency is reachable-but-failing or a
// read against a live connection fails.
func NewEngineUnavailableError(message string, cause error) *fetcher.EngineError {
	return fetcher.NewWrappedEngineError(fetcher.CategoryUnavailable, message, cause)
}

// NewEngineInternalError builds a CategoryInternal engine error, preserving the
// cause. Use it for unexpected failures (scan errors, decode faults) that are
// neither caller faults nor dependency outages.
func NewEngineInternalError(message string, cause error) *fetcher.EngineError {
	return fetcher.NewWrappedEngineError(fetcher.CategoryInternal, message, cause)
}
