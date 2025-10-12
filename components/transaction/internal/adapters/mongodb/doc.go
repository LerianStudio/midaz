// Package mongodb provides MongoDB repository implementation for metadata persistence.
//
// Implements flexible metadata storage for:
// - Transactions and their custom attributes
// - Operations and their custom attributes
// - Operation routes and transaction routes
// - Arbitrary key-value pairs without schema constraints
//
// Metadata enables extending financial records with business-specific attributes
// without modifying the core PostgreSQL schema.
package mongodb
