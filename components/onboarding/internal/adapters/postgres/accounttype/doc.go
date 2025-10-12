// Package accounttype provides PostgreSQL repository implementation for account type persistence.
//
// Implements chart of accounts type definitions with:
// - KeyValue uniqueness enforcement (the account type identifier)
// - Account type validation rules for account creation
// - Support for custom accounting structures per ledger
package accounttype
