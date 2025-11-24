package in

// Handlers aggregates all HTTP handlers to simplify router wiring.
type Handlers struct {
	// Onboarding-context handlers
	Account      *AccountHandler
	Portfolio    *PortfolioHandler
	Ledger       *LedgerHandler
	Asset        *AssetHandler
	Organization *OrganizationHandler
	Segment      *SegmentHandler
	AccountType  *AccountTypeHandler

	// Transaction-context handlers
	Transaction      *TransactionHandler
	Operation        *OperationHandler
	AssetRate        *AssetRateHandler
	Balance          *BalanceHandler
	OperationRoute   *OperationRouteHandler
	TransactionRoute *TransactionRouteHandler
}
