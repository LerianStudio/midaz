// Package transactionroute provides PostgreSQL repository implementation for transaction route persistence.
//
// Transaction routes combine multiple operation routes into validated transaction flows:
// - Define complete accounting entries (sources + destinations)
// - Cached in Redis for fast transaction validation
// - Enable enforcement of accounting rules and entry patterns
// - Support standard journal entry templates
package transactionroute
