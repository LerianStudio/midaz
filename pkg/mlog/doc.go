// Package mlog provides wide event / canonical log line utilities for Midaz.
//
// Wide Events Pattern:
// Instead of scattered log statements throughout a request lifecycle,
// emit ONE comprehensive structured event per request containing all
// context needed for debugging.
//
// Benefits:
//   - Single queryable event per request
//   - All business context in one place
//   - Supports queries like "show failed transactions for premium users"
//   - Correlates with OpenTelemetry traces via trace_id
//
// Usage:
//
//	// In middleware - initialize and defer emission
//	event := mlog.NewWideEvent(c)
//	defer event.Emit(logger)
//
//	// In handlers - enrich with business context
//	event := mlog.GetWideEvent(c)
//	event.SetTransaction(txnID, amount, assetCode)
//	event.SetUser(userID, orgID, role)
//
// Reference: https://loggingsucks.com/
package mlog
