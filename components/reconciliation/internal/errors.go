// Package internal provides internal types and errors for the reconciliation component.
package internal

import "errors"

// Sentinel errors for reconciliation operations.
var (
	// ErrQueryFailed indicates a database query failed.
	ErrQueryFailed = errors.New("query failed")

	// ErrScanFailed indicates a row scan operation failed.
	ErrScanFailed = errors.New("row scan failed")

	// ErrIterationFailed indicates row iteration failed.
	ErrIterationFailed = errors.New("row iteration failed")

	// ErrNoReport indicates no reconciliation report is available.
	ErrNoReport = errors.New("no reconciliation report available")

	// ErrReconciliationInProgress indicates a reconciliation is already running.
	ErrReconciliationInProgress = errors.New("reconciliation already in progress")
)
