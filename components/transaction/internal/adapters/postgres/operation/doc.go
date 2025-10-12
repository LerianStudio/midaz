// Package operation provides PostgreSQL repository implementation for operation persistence.
//
// Operations represent individual debits and credits in double-entry accounting.
// This package implements:
// - Operation creation with before/after balance snapshots
// - DEBIT/CREDIT/ON_HOLD/RELEASE operation types
// - Link between transactions and specific account balance changes
// - Complete audit trail with balance states at time of operation
package operation
