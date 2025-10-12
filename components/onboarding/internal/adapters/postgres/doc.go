// Package postgres provides PostgreSQL repository implementations for all
// onboarding service domain entities. Each sub-package contains a repository
// handling persistence for a specific aggregate root (Organization, Ledger,
// Account, etc.) with support for transactions, soft deletes, and optimistic
// locking where applicable.
package postgres
