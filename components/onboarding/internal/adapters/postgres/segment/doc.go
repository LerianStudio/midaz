// Package segment provides PostgreSQL repository implementation for segment persistence.
//
// Handles segment storage for dimensional categorization with:
// - Name uniqueness enforcement within ledgers
// - Soft delete support
// - Cascade handling for associated accounts
package segment
