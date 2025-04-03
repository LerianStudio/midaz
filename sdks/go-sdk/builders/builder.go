// Package builders provides fluent builder interfaces for the Midaz SDK.
// It implements the builder pattern to simplify the creation and manipulation
// of Midaz resources through a chainable API.
package builders

// Builder provides a centralized access point to all builder types in the Midaz SDK.
// It acts as a factory for creating specific builders for different resource types
// and operations.
type Builder struct {
	// Client interfaces for different resource types
	accountClient      AccountClientInterface
	assetClient        AssetClientInterface
	assetRateClient    AssetRateClientInterface
	balanceClient      BalanceClientInterface
	ledgerClient       LedgerClientInterface
	organizationClient OrganizationClientInterface
	portfolioClient    PortfolioClientInterface
	segmentClient      SegmentClientInterface
	transactionClient  ClientInterface
}

// NewBuilder creates a new Builder instance with the provided client interfaces.
// This constructor initializes a Builder that provides access to all builder types
// in the Midaz SDK.
//
// Parameters:
//   - accountClient: Client interface for account operations
//   - assetClient: Client interface for asset operations
//   - assetRateClient: Client interface for asset rate operations
//   - balanceClient: Client interface for balance operations
//   - ledgerClient: Client interface for ledger operations
//   - organizationClient: Client interface for organization operations
//   - portfolioClient: Client interface for portfolio operations
//   - segmentClient: Client interface for segment operations
//   - transactionClient: Client interface for transaction operations
//
// Returns:
//   - *Builder: A pointer to the newly created Builder, ready to create specific builders
//
// Example - Creating a builder with a client:
//
//	// Initialize the builder with a client
//	builder := builders.NewBuilder(
//	    client, // AccountClientInterface
//	    client, // AssetClientInterface
//	    client, // AssetRateClientInterface
//	    client, // BalanceClientInterface
//	    client, // LedgerClientInterface
//	    client, // OrganizationClientInterface
//	    client, // PortfolioClientInterface
//	    client, // SegmentClientInterface
//	    client, // ClientInterface (for transactions)
//	)
//
// Example - Using the builder to create an account:
//
//	// After creating the builder, use it to create an account
//	account, err := builder.
//	    NewAccount().
//	    WithOrganization("org-123").
//	    WithLedger("ledger-456").
//	    WithName("Checking Account").
//	    WithAssetCode("USD").
//	    WithType("ASSET").
//	    Create(context.Background())
func NewBuilder(
	accountClient AccountClientInterface,
	assetClient AssetClientInterface,
	assetRateClient AssetRateClientInterface,
	balanceClient BalanceClientInterface,
	ledgerClient LedgerClientInterface,
	organizationClient OrganizationClientInterface,
	portfolioClient PortfolioClientInterface,
	segmentClient SegmentClientInterface,
	transactionClient ClientInterface,
) *Builder {
	return &Builder{
		accountClient:      accountClient,
		assetClient:        assetClient,
		assetRateClient:    assetRateClient,
		balanceClient:      balanceClient,
		ledgerClient:       ledgerClient,
		organizationClient: organizationClient,
		portfolioClient:    portfolioClient,
		segmentClient:      segmentClient,
		transactionClient:  transactionClient,
	}
}

// NewAccount creates a new builder for creating accounts.
//
// Returns:
//   - AccountBuilder: A builder interface for configuring and creating account resources
//
// Example:
//
//	account, err := builder.
//	    NewAccount().
//	    WithOrganization("org-123").
//	    WithLedger("ledger-456").
//	    WithName("Checking Account").
//	    WithAssetCode("USD").
//	    WithType("ASSET").
//	    Create(context.Background())
func (b *Builder) NewAccount() AccountBuilder {
	return NewAccount(b.accountClient)
}

// NewAccountUpdate creates a new builder for updating accounts.
//
// Parameters:
//   - orgID: The organization ID that the account belongs to
//   - ledgerID: The ledger ID that the account belongs to
//   - accountID: The ID of the account to update
//
// Returns:
//   - AccountUpdateBuilder: A builder interface for configuring and updating account resources
//
// Example:
//
//	updatedAccount, err := builder.
//	    NewAccountUpdate("org-123", "ledger-456", "account-789").
//	    WithName("Updated Account Name").
//	    WithStatus("INACTIVE").
//	    Update(context.Background())
func (b *Builder) NewAccountUpdate(orgID, ledgerID, accountID string) AccountUpdateBuilder {
	return NewAccountUpdate(b.accountClient, orgID, ledgerID, accountID)
}

// NewAsset creates a new builder for creating assets.
//
// Returns:
//   - AssetBuilder: A builder interface for configuring and creating asset resources
//
// Example:
//
//	asset, err := builder.
//	    NewAsset().
//	    WithOrganization("org-123").
//	    WithLedger("ledger-456").
//	    WithName("US Dollar").
//	    WithCode("USD").
//	    Create(context.Background())
func (b *Builder) NewAsset() AssetBuilder {
	return NewAsset(b.assetClient)
}

// NewAssetUpdate creates a new builder for updating assets.
//
// Parameters:
//   - orgID: The organization ID that the asset belongs to
//   - ledgerID: The ledger ID that the asset belongs to
//   - assetID: The ID of the asset to update
//
// Returns:
//   - AssetUpdateBuilder: A builder interface for configuring and updating asset resources
//
// Example:
//
//	updatedAsset, err := builder.
//	    NewAssetUpdate("org-123", "ledger-456", "asset-789").
//	    WithName("Updated Asset Name").
//	    Update(context.Background())
func (b *Builder) NewAssetUpdate(orgID, ledgerID, assetID string) AssetUpdateBuilder {
	return NewAssetUpdate(b.assetClient, orgID, ledgerID, assetID)
}

// NewAssetRate creates a new builder for creating asset rates.
//
// Returns:
//   - AssetRateBuilder: A builder interface for configuring and creating asset rate resources
//
// Example:
//
//	assetRate, err := builder.
//	    NewAssetRate().
//	    WithOrganization("org-123").
//	    WithLedger("ledger-456").
//	    WithBaseAsset("USD").
//	    WithQuoteAsset("EUR").
//	    WithRate(0.85).
//	    Create(context.Background())
func (b *Builder) NewAssetRate() AssetRateBuilder {
	return NewAssetRate(b.assetRateClient)
}

// NewLedger creates a new builder for creating ledgers.
//
// Returns:
//   - LedgerBuilder: A builder interface for configuring and creating ledger resources
//
// Example:
//
//	ledger, err := builder.
//	    NewLedger().
//	    WithOrganization("org-123").
//	    WithName("Main Ledger").
//	    Create(context.Background())
func (b *Builder) NewLedger() LedgerBuilder {
	return NewLedger(b.ledgerClient)
}

// NewLedgerUpdate creates a new builder for updating ledgers.
//
// Parameters:
//   - orgID: The organization ID that the ledger belongs to
//   - ledgerID: The ID of the ledger to update
//
// Returns:
//   - LedgerUpdateBuilder: A builder interface for configuring and updating ledger resources
//
// Example:
//
//	updatedLedger, err := builder.
//	    NewLedgerUpdate("org-123", "ledger-456").
//	    WithName("Updated Ledger Name").
//	    Update(context.Background())
func (b *Builder) NewLedgerUpdate(orgID, ledgerID string) LedgerUpdateBuilder {
	return NewLedgerUpdate(b.ledgerClient, orgID, ledgerID)
}

// NewOrganization creates a new builder for creating organizations.
//
// Returns:
//   - OrganizationBuilder: A builder interface for configuring and creating organization resources
//
// Example:
//
//	organization, err := builder.
//	    NewOrganization().
//	    WithName("ACME Corporation").
//	    Create(context.Background())
func (b *Builder) NewOrganization() OrganizationBuilder {
	return NewOrganization(b.organizationClient)
}

// NewOrganizationUpdate creates a new builder for updating organizations.
//
// Parameters:
//   - orgID: The ID of the organization to update
//
// Returns:
//   - OrganizationUpdateBuilder: A builder interface for configuring and updating organization resources
//
// Example:
//
//	updatedOrg, err := builder.
//	    NewOrganizationUpdate("org-123").
//	    WithName("Updated Organization Name").
//	    Update(context.Background())
func (b *Builder) NewOrganizationUpdate(orgID string) OrganizationUpdateBuilder {
	return NewOrganizationUpdate(b.organizationClient, orgID)
}

// NewPortfolio creates a new builder for creating portfolios.
//
// Returns:
//   - PortfolioBuilder: A builder interface for configuring and creating portfolio resources
//
// Example:
//
//	portfolio, err := builder.
//	    NewPortfolio().
//	    WithOrganization("org-123").
//	    WithLedger("ledger-456").
//	    WithName("Investment Portfolio").
//	    Create(context.Background())
func (b *Builder) NewPortfolio() PortfolioBuilder {
	return NewPortfolio(b.portfolioClient)
}

// NewPortfolioUpdate creates a new builder for updating portfolios.
//
// Parameters:
//   - orgID: The organization ID that the portfolio belongs to
//   - ledgerID: The ledger ID that the portfolio belongs to
//   - portfolioID: The ID of the portfolio to update
//
// Returns:
//   - PortfolioUpdateBuilder: A builder interface for configuring and updating portfolio resources
//
// Example:
//
//	updatedPortfolio, err := builder.
//	    NewPortfolioUpdate("org-123", "ledger-456", "portfolio-789").
//	    WithName("Updated Portfolio Name").
//	    Update(context.Background())
func (b *Builder) NewPortfolioUpdate(orgID, ledgerID, portfolioID string) PortfolioUpdateBuilder {
	return NewPortfolioUpdate(b.portfolioClient, orgID, ledgerID, portfolioID)
}

// NewSegment creates a new builder for creating segments.
//
// Returns:
//   - SegmentBuilder: A builder interface for configuring and creating segment resources
//
// Example:
//
//	segment, err := builder.
//	    NewSegment().
//	    WithOrganization("org-123").
//	    WithLedger("ledger-456").
//	    WithName("Retail Segment").
//	    Create(context.Background())
func (b *Builder) NewSegment() SegmentBuilder {
	return NewSegment(b.segmentClient)
}

// NewSegmentUpdate creates a new builder for updating segments.
//
// Parameters:
//   - orgID: The organization ID that the segment belongs to
//   - ledgerID: The ledger ID that the segment belongs to
//   - portfolioID: The portfolio ID that the segment belongs to
//   - segmentID: The ID of the segment to update
//
// Returns:
//   - SegmentUpdateBuilder: A builder interface for configuring and updating segment resources
//
// Example:
//
//	updatedSegment, err := builder.
//	    NewSegmentUpdate("org-123", "ledger-456", "portfolio-789", "segment-abc").
//	    WithName("Updated Segment Name").
//	    WithStatus("INACTIVE").
//	    Update(context.Background())
func (b *Builder) NewSegmentUpdate(orgID, ledgerID, portfolioID, segmentID string) SegmentUpdateBuilder {
	return NewSegmentUpdate(b.segmentClient, orgID, ledgerID, portfolioID, segmentID)
}

// NewDeposit creates a new builder for deposit transactions.
//
// Returns:
//   - DepositBuilder: A builder interface for configuring and executing deposit transactions
//
// Example:
//
//	tx, err := builder.
//	    NewDeposit().
//	    WithOrganization("org-123").
//	    WithLedger("ledger-456").
//	    WithAmount(10000, 2). // $100.00
//	    WithAssetCode("USD").
//	    WithDescription("Customer deposit").
//	    ToAccount("customer:john.doe").
//	    Execute(context.Background())
func (b *Builder) NewDeposit() DepositBuilder {
	return NewDeposit(b.transactionClient)
}

// NewWithdrawal creates a new builder for withdrawal transactions.
//
// Returns:
//   - WithdrawalBuilder: A builder interface for configuring and executing withdrawal transactions
//
// Example:
//
//	tx, err := builder.
//	    NewWithdrawal().
//	    WithOrganization("org-123").
//	    WithLedger("ledger-456").
//	    WithAmount(5000, 2). // $50.00
//	    WithAssetCode("USD").
//	    WithDescription("Customer withdrawal").
//	    FromAccount("customer:john.doe").
//	    Execute(context.Background())
func (b *Builder) NewWithdrawal() WithdrawalBuilder {
	return NewWithdrawal(b.transactionClient)
}

// NewTransfer creates a new builder for transfer transactions.
//
// Returns:
//   - TransferBuilder: A builder interface for configuring and executing transfer transactions
//
// Example:
//
//	tx, err := builder.
//	    NewTransfer().
//	    WithOrganization("org-123").
//	    WithLedger("ledger-456").
//	    WithAmount(2500, 2). // $25.00
//	    WithAssetCode("USD").
//	    WithDescription("Transfer between accounts").
//	    FromAccount("customer:john.doe").
//	    ToAccount("merchant:acme").
//	    Execute(context.Background())
func (b *Builder) NewTransfer() TransferBuilder {
	return NewTransfer(b.transactionClient)
}
