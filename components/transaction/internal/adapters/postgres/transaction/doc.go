// Package transaction provides PostgreSQL repository implementation for transaction persistence.
//
// Implements transaction header storage including:
// - Gold DSL body preservation for audit trails
// - Status lifecycle management (CREATED, PENDING, APPROVED, CANCELED, NOTED)
// - Parent-child transaction relationships (reversals, corrections)
// - Pagination and filtering support for transaction queries
package transaction
