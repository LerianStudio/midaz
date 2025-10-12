// Package operationroute provides PostgreSQL repository implementation for operation route persistence.
//
// Operation routes define validation rules for transaction operations:
// - Which accounts can participate as sources (debits)
// - Which accounts can participate as destinations (credits)
// - Validation by account alias or account type
// - Reusable accounting policy enforcement
package operationroute
