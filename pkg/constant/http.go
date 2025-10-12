package constant

// UUIDPathParameters contains the canonical list of URL path parameter names
// that should be validated as UUIDs across services.
var UUIDPathParameters = []string{
	"id",
	"organization_id",
	"ledger_id",
	"asset_id",
	"portfolio_id",
	"segment_id",
	"account_id",
	"transaction_id",
	"operation_id",
	"asset_rate_id",
	"external_id",
	"audit_id",
	"balance_id",
	"operation_route_id",
	"transaction_route_id",
}

// XTotalCount is the standard HTTP header used to return total items count in
// paginated responses.
const XTotalCount = "X-Total-Count"

// ContentLength is the standard HTTP header representing payload length in
// bytes.
const ContentLength = "Content-Length"
