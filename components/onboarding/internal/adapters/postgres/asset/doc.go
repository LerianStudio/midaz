// Package asset provides PostgreSQL repository implementation for asset persistence.
//
// Handles storage for financial assets (currencies, cryptocurrencies, commodities)
// with support for:
// - Name and code uniqueness enforcement
// - Asset type validation (currency, crypto, commodities, others)
// - Status management and soft deletes
// - Balance remaining checks before deletion
package asset
