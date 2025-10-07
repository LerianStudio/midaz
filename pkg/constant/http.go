// Package constant provides system-wide constant values used across the Midaz ledger system.
// This file contains HTTP-related constants including path parameters and header names.
package constant

// UUIDPathParameters defines the list of path parameter names that should be validated as UUIDs.
// This slice is used by HTTP middleware to automatically validate that path parameters
// containing these names conform to UUID format (RFC 4122).
//
// Parameters in this list are validated to ensure they are valid UUIDs before being
// passed to handlers, preventing invalid data from reaching business logic layers.
var UUIDPathParameters = []string{
	"id",                   // Generic entity identifier
	"organization_id",      // Organization identifier
	"ledger_id",            // Ledger identifier
	"asset_id",             // Asset identifier
	"portfolio_id",         // Portfolio identifier
	"segment_id",           // Segment identifier
	"account_id",           // Account identifier
	"transaction_id",       // Transaction identifier
	"operation_id",         // Operation identifier
	"asset_rate_id",        // Asset rate identifier
	"external_id",          // External entity identifier
	"audit_id",             // Audit record identifier
	"balance_id",           // Balance identifier
	"operation_route_id",   // Operation route identifier
	"transaction_route_id", // Transaction route identifier
}

// XTotalCount is the HTTP header name used to return the total count of resources
// in paginated responses. This header allows clients to know the total number of
// resources available without having to fetch all pages.
const XTotalCount = "X-Total-Count"

// ContentLength is the standard HTTP header name for indicating the size of the
// response body in bytes. This constant is used for setting and reading content
// length headers in HTTP requests and responses.
const ContentLength = "Content-Length"
