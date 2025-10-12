// Package mongodb provides MongoDB repository implementation for metadata persistence.
//
// This package implements flexible, schema-less metadata storage for all entities:
// - Organizations, Ledgers, Accounts, Assets, Portfolios, Segments
// - Supports arbitrary key-value pairs without schema migrations
// - Efficient batch operations for metadata enrichment
// - Query capabilities for metadata-driven filtering
//
// Metadata is stored separately from relational data to avoid PostgreSQL bloat
// and enable flexible attribute extension.
package mongodb
