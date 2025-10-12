// Package account provides PostgreSQL repository implementation for account persistence.
//
// This package implements the account repository interface with support for:
// - CRUD operations with soft deletes
// - Alias-based lookups for transaction routing
// - Parent-child account hierarchies
// - Portfolio and segment scoping
// - Concurrent-safe operations
//
// All queries exclude soft-deleted records unless explicitly requested.
package account
