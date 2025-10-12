// Package balance provides PostgreSQL repository implementation for balance persistence.
//
// This is a **critical financial package** implementing:
// - Atomic balance updates with optimistic locking (version field)
// - Available and OnHold amount tracking
// - Permission flags (allowSending, allowReceiving)
// - Batch balance updates for transaction processing
// - Version-based concurrency control to prevent lost updates
//
// All balance modifications must use the version field to prevent race conditions.
package balance
