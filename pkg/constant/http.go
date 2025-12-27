package constant

// UUIDPathParameters is a list of path parameter names that are expected to contain UUIDs.
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
	"holder_id",
	"alias_id",
}

// XTotalCount is the HTTP header for the total count of items in a paginated response.
const (
	XTotalCount   = "X-Total-Count"
	ContentLength = "Content-Length"
)
