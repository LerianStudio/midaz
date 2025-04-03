# Midaz Go SDK Method Map

This document provides a comprehensive overview of all public methods available in the Midaz Go SDK, organized by package and purpose.

## Table of Contents

- [I. Client Package (`midaz`)](#i-client-package-midaz)
- [II. Entities Package (`midaz/entities`)](#ii-entities-package-midazentities)
- [III. Builders Package (`midaz/builders`)](#iii-builders-package-midazbuilders)
- [IV. Abstractions Package (`midaz/abstractions`)](#iv-abstractions-package-midazabstractions)
- [V. Models Package (`midaz/models`)](#v-models-package-midazmodels)

## I. Client Package (`midaz`)

The client package provides the main entry point for the Midaz SDK.

### Client

- `New()` - Creates a new Midaz client with the provided options
- `WithAuthToken()` - Sets the authentication token for the client
- `WithOnboardingURL()` - Sets the base URL for the onboarding API
- `WithTransactionURL()` - Sets the base URL for the transaction API
- `WithHTTPClient()` - Sets a custom HTTP client for the client
- `WithTimeout()` - Sets the timeout for requests made by the client
- `WithDebug()` - Enables or disables debug mode for the client
- `UseEntity()` - Enables the Entity API interface
- `UseBuilder()` - Enables the Builder API interface
- `UseAbstraction()` - Enables the Abstraction API interface
- `UseAllAPIs()` - Enables all API interfaces
- `Client.Entity` - Provides access to the Entity API interface
- `Client.Builder` - Provides access to the Builder API interface
- `Client.Abstraction` - Provides access to the Abstraction API interface

## II. Entities Package (`midaz/entities`)

The entities package provides direct access to Midaz API resources.

### Entity

- `NewEntity()` - Creates a new Entity instance that provides access to all service interfaces
- `Entity.Accounts` - Returns the AccountsService interface for account operations
- `Entity.Assets` - Returns the AssetsService interface for asset operations
- `Entity.AssetRates` - Returns the AssetRatesService interface for asset rate operations
- `Entity.Balances` - Returns the BalancesService interface for balance operations
- `Entity.Ledgers` - Returns the LedgersService interface for ledger operations
- `Entity.Operations` - Returns the OperationsService interface for operation operations
- `Entity.Organizations` - Returns the OrganizationsService interface for organization operations
- `Entity.Portfolios` - Returns the PortfoliosService interface for portfolio operations
- `Entity.Segments` - Returns the SegmentsService interface for segment operations
- `Entity.Transactions` - Returns the TransactionsService interface for transaction operations

### Accounts Service

- `AccountsService.List()` - Lists accounts for a ledger with pagination
- `AccountsService.Get()` - Gets an account by ID
- `AccountsService.GetByAlias()` - Gets an account by alias
- `AccountsService.Create()` - Creates a new account
- `AccountsService.Update()` - Updates an account
- `AccountsService.Delete()` - Deletes an account

### Assets Service

- `AssetsService.List()` - Lists assets for a ledger with pagination
- `AssetsService.Get()` - Gets an asset by ID
- `AssetsService.Create()` - Creates a new asset
- `AssetsService.Update()` - Updates an asset
- `AssetsService.Delete()` - Deletes an asset

### Asset Rates Service

- `AssetRatesService.List()` - Lists asset rates for a ledger with pagination
- `AssetRatesService.Get()` - Gets an asset rate by ID
- `AssetRatesService.Create()` - Creates a new asset rate
- `AssetRatesService.Update()` - Updates an asset rate
- `AssetRatesService.Delete()` - Deletes an asset rate

### Balances Service

- `BalancesService.List()` - Lists balances for a ledger with pagination
- `BalancesService.ListForAccount()` - Lists balances for a specific account
- `BalancesService.Get()` - Gets a balance by ID
- `BalancesService.Update()` - Updates a balance

### Ledgers Service

- `LedgersService.List()` - Lists ledgers for an organization with pagination
- `LedgersService.Get()` - Gets a ledger by ID
- `LedgersService.Create()` - Creates a new ledger
- `LedgersService.Update()` - Updates a ledger
- `LedgersService.Delete()` - Deletes a ledger

### Operations Service

- `OperationsService.List()` - Lists operations for a ledger with pagination
- `OperationsService.Get()` - Gets an operation by ID

### Organizations Service

- `OrganizationsService.List()` - Lists organizations with pagination
- `OrganizationsService.Get()` - Gets an organization by ID
- `OrganizationsService.Create()` - Creates a new organization
- `OrganizationsService.Update()` - Updates an organization
- `OrganizationsService.Delete()` - Deletes an organization

### Portfolios Service

- `PortfoliosService.List()` - Lists portfolios for a ledger with pagination
- `PortfoliosService.Get()` - Gets a portfolio by ID
- `PortfoliosService.Create()` - Creates a new portfolio
- `PortfoliosService.Update()` - Updates a portfolio
- `PortfoliosService.Delete()` - Deletes a portfolio

### Segments Service

- `SegmentsService.List()` - Lists segments for a portfolio with pagination
- `SegmentsService.Get()` - Gets a segment by ID
- `SegmentsService.Create()` - Creates a new segment
- `SegmentsService.Update()` - Updates a segment
- `SegmentsService.Delete()` - Deletes a segment

### Transactions Service

- `TransactionsService.List()` - Lists transactions for a ledger with pagination
- `TransactionsService.Get()` - Gets a transaction by ID
- `TransactionsService.Create()` - Creates a new transaction
- `TransactionsService.Commit()` - Commits a pending transaction
- `TransactionsService.Cancel()` - Cancels a pending transaction

## III. Builders Package (`midaz/builders`)

The builders package provides fluent interfaces for creating and updating resources.

### Organization Builders

- `NewOrganization()` - Creates a new organization builder
  - `WithLegalName()` - Sets the organization's legal name
  - `WithLegalDocument()` - Sets the organization's legal document
  - `WithStatus()` - Sets the organization status
  - `WithAddress()` - Sets the organization address
  - `WithMetadata()` - Sets additional metadata
  - `WithTag()` - Adds a single tag to the organization
  - `WithTags()` - Adds multiple tags to the organization
  - `Create()` - Executes the organization creation and returns the created organization

- `NewOrganizationUpdate()` - Creates a new organization update builder
  - `WithLegalName()` - Sets the updated organization name
  - `WithLegalDocument()` - Sets the updated legal document
  - `WithStatus()` - Sets the updated organization status
  - `WithAddress()` - Sets the updated organization address
  - `WithMetadata()` - Sets the updated metadata
  - `WithTag()` - Adds a tag to the organization
  - `WithTags()` - Adds multiple tags to the organization
  - `Update()` - Executes the organization update and returns the updated organization

### Ledger Builders

- `NewLedger()` - Creates a new ledger builder
  - `WithOrganization()` - Sets the organization ID
  - `WithName()` - Sets the ledger name
  - `WithStatus()` - Sets the ledger status
  - `WithMetadata()` - Sets additional metadata
  - `WithTag()` - Adds a single tag to the ledger
  - `WithTags()` - Adds multiple tags to the ledger
  - `Create()` - Executes the ledger creation and returns the created ledger

- `NewLedgerUpdate()` - Creates a new ledger update builder
  - `WithName()` - Sets the updated ledger name
  - `WithStatus()` - Sets the updated ledger status
  - `WithMetadata()` - Sets the updated metadata
  - `WithTag()` - Adds a tag to the ledger
  - `WithTags()` - Adds multiple tags to the ledger
  - `Update()` - Executes the ledger update and returns the updated ledger

### Account Builders

- `NewAccount()` - Creates a new account builder
  - `WithOrganization()` - Sets the organization ID
  - `WithLedger()` - Sets the ledger ID
  - `WithName()` - Sets the account name
  - `WithAssetCode()` - Sets the asset code
  - `WithType()` - Sets the account type
  - `WithParentAccount()` - Sets the parent account ID
  - `WithEntityID()` - Sets the entity ID
  - `WithPortfolio()` - Sets the portfolio ID
  - `WithSegment()` - Sets the segment ID
  - `WithAlias()` - Sets the account alias
  - `WithStatus()` - Sets the account status
  - `WithMetadata()` - Sets additional metadata
  - `WithTag()` - Adds a single tag to the account
  - `WithTags()` - Adds multiple tags to the account
  - `Create()` - Executes the account creation and returns the created account

- `NewAccountUpdate()` - Creates a new account update builder
  - `WithName()` - Sets the updated account name
  - `WithPortfolio()` - Sets the updated portfolio ID
  - `WithSegment()` - Sets the updated segment ID
  - `WithStatus()` - Sets the updated account status
  - `WithMetadata()` - Sets the updated metadata
  - `WithTag()` - Adds a tag to the account
  - `WithTags()` - Adds multiple tags to the account
  - `Update()` - Executes the account update and returns the updated account

### Asset Builders

- `NewAsset()` - Creates a new asset builder
  - `WithOrganization()` - Sets the organization ID
  - `WithLedger()` - Sets the ledger ID
  - `WithName()` - Sets the asset name
  - `WithCode()` - Sets the asset code
  - `WithType()` - Sets the asset type
  - `WithStatus()` - Sets the asset status
  - `WithMetadata()` - Sets additional metadata
  - `WithTag()` - Adds a single tag to the asset
  - `WithTags()` - Adds multiple tags to the asset
  - `Create()` - Executes the asset creation and returns the created asset

- `NewAssetUpdate()` - Creates a new asset update builder
  - `WithName()` - Sets the updated asset name
  - `WithStatus()` - Sets the updated asset status
  - `WithMetadata()` - Sets the updated metadata
  - `WithTag()` - Adds a tag to the asset
  - `WithTags()` - Adds multiple tags to the asset
  - `Update()` - Executes the asset update and returns the updated asset

### Asset Rate Builders

- `NewAssetRate()` - Creates a new asset rate builder
  - `WithOrganization()` - Sets the organization ID 
  - `WithLedger()` - Sets the ledger ID
  - `WithBaseAsset()` - Sets the base asset code
  - `WithQuoteAsset()` - Sets the quote asset code
  - `WithRate()` - Sets the exchange rate
  - `WithEffectiveAt()` - Sets when the rate becomes effective
  - `WithExpirationAt()` - Sets when the rate expires
  - `WithMetadata()` - Sets additional metadata
  - `WithTag()` - Adds a single tag to the asset rate
  - `WithTags()` - Adds multiple tags to the asset rate
  - `CreateOrUpdate()` - Executes the asset rate creation or update

### Portfolio Builders

- `NewPortfolio()` - Creates a new portfolio builder
  - `WithOrganization()` - Sets the organization ID
  - `WithLedger()` - Sets the ledger ID
  - `WithName()` - Sets the portfolio name
  - `WithStatus()` - Sets the portfolio status
  - `WithMetadata()` - Sets additional metadata
  - `WithTag()` - Adds a single tag to the portfolio
  - `WithTags()` - Adds multiple tags to the portfolio
  - `Create()` - Executes the portfolio creation and returns the created portfolio

- `NewPortfolioUpdate()` - Creates a new portfolio update builder
  - `WithName()` - Sets the updated portfolio name
  - `WithStatus()` - Sets the updated portfolio status
  - `WithMetadata()` - Sets the updated metadata
  - `WithTag()` - Adds a tag to the portfolio
  - `WithTags()` - Adds multiple tags to the portfolio
  - `Update()` - Executes the portfolio update and returns the updated portfolio

### Segment Builders

- `NewSegment()` - Creates a new segment builder
  - `WithOrganization()` - Sets the organization ID
  - `WithLedger()` - Sets the ledger ID
  - `WithPortfolio()` - Sets the portfolio ID
  - `WithName()` - Sets the segment name
  - `WithStatus()` - Sets the segment status
  - `WithMetadata()` - Sets additional metadata
  - `WithTag()` - Adds a single tag to the segment
  - `WithTags()` - Adds multiple tags to the segment
  - `Create()` - Executes the segment creation and returns the created segment

- `NewSegmentUpdate()` - Creates a new segment update builder
  - `WithName()` - Sets the updated segment name
  - `WithStatus()` - Sets the updated segment status
  - `WithMetadata()` - Sets the updated metadata
  - `WithTag()` - Adds a tag to the segment
  - `WithTags()` - Adds multiple tags to the segment
  - `Update()` - Executes the segment update and returns the updated segment

### Transaction Builders

- `NewTransaction()` - Creates a new transaction builder
  - `WithOrganization()` - Sets the organization ID
  - `WithLedger()` - Sets the ledger ID
  - `WithDescription()` - Sets the transaction description
  - `WithExternalID()` - Sets an external ID for the transaction
  - `WithMetadata()` - Sets additional metadata
  - `WithStatus()` - Sets the transaction status
  - `WithTag()` - Adds a single tag to the transaction
  - `WithTags()` - Adds multiple tags to the transaction
  - `AddOperation()` - Adds an operation to the transaction
  - `Create()` - Executes the transaction creation and returns the created transaction
  - `CreateAndCommit()` - Creates and commits the transaction in one step

- `NewDeposit()` - Creates a new deposit transaction builder
  - `WithOrganization()` - Sets the organization ID
  - `WithLedger()` - Sets the ledger ID
  - `WithDescription()` - Sets the transaction description
  - `WithExternalID()` - Sets an external ID for the transaction
  - `WithMetadata()` - Sets additional metadata
  - `WithSourceAccount()` - Sets the source account ID
  - `WithDestinationAccount()` - Sets the destination account ID
  - `WithAmount()` - Sets the deposit amount
  - `WithAssetCode()` - Sets the asset code
  - `WithTag()` - Adds a single tag to the transaction
  - `WithTags()` - Adds multiple tags to the transaction
  - `Execute()` - Executes the deposit and returns the created transaction

- `NewWithdrawal()` - Creates a new withdrawal transaction builder
  - `WithOrganization()` - Sets the organization ID
  - `WithLedger()` - Sets the ledger ID
  - `WithDescription()` - Sets the transaction description
  - `WithExternalID()` - Sets an external ID for the transaction
  - `WithMetadata()` - Sets additional metadata
  - `WithSourceAccount()` - Sets the source account ID
  - `WithDestinationAccount()` - Sets the destination account ID
  - `WithAmount()` - Sets the withdrawal amount
  - `WithAssetCode()` - Sets the asset code
  - `WithTag()` - Adds a single tag to the transaction
  - `WithTags()` - Adds multiple tags to the transaction
  - `Execute()` - Executes the withdrawal and returns the created transaction

- `NewTransfer()` - Creates a new transfer transaction builder
  - `WithOrganization()` - Sets the organization ID
  - `WithLedger()` - Sets the ledger ID
  - `WithDescription()` - Sets the transaction description
  - `WithExternalID()` - Sets an external ID for the transaction
  - `WithMetadata()` - Sets additional metadata
  - `WithSourceAccount()` - Sets the source account ID
  - `WithDestinationAccount()` - Sets the destination account ID
  - `WithAmount()` - Sets the transfer amount
  - `WithAssetCode()` - Sets the asset code
  - `WithTag()` - Adds a single tag to the transaction
  - `WithTags()` - Adds multiple tags to the transaction
  - `Execute()` - Executes the transfer and returns the created transaction

### Balance Builders

- `NewBalanceUpdate()` - Creates a new balance update builder
  - `WithAllowSending()` - Sets whether sending is allowed
  - `WithAllowReceiving()` - Sets whether receiving is allowed
  - `Update()` - Executes the balance update and returns the updated balance

## IV. Abstractions Package (`midaz/abstractions`)

The abstractions package provides high-level transaction operations that simplify the creation of common transaction types.

### Abstraction

- `NewAbstraction()` - Creates a new Abstraction instance with the provided transaction creation function
- `Abstraction.Deposits` - Returns the DepositService interface for deposit operations
- `Abstraction.Withdrawals` - Returns the WithdrawalService interface for withdrawal operations
- `Abstraction.Transfers` - Returns the TransferService interface for transfer operations

### Deposit Service

- `DepositService.CreateDeposit()` - Creates a deposit transaction, adding funds to an internal account from an external source

### Withdrawal Service

- `WithdrawalService.CreateWithdrawal()` - Creates a withdrawal transaction, removing funds from an internal account to an external destination

### Transfer Service

- `TransferService.CreateTransfer()` - Creates a transfer transaction between two internal accounts

### Transaction Options

- `WithMetadata()` - Adds structured metadata to a transaction
- `WithChartOfAccountsGroupName()` - Sets the chart of accounts group for a transaction
- `WithCode()` - Sets a custom transaction code for a transaction
- `WithPending()` - Marks a transaction as pending, requiring explicit commitment later
- `WithIdempotencyKey()` - Adds an idempotency key to ensure transaction uniqueness
- `WithExternalID()` - Sets an external ID for the transaction
- `WithNotes()` - Adds detailed notes to a transaction
- `WithRequestID()` - Attaches a unique request ID to the transaction
- `WithSendingOptions()` - Configures sending options for a transaction
- `WithFromTo()` - Adds a from/to entry to a transaction
- `WithShare()` - Adds a share configuration to a from/to entry
- `WithRate()` - Adds an exchange rate to a from/to entry

### Error Handling

- `IsNotFoundError()` - Checks if the error is a not found error
- `IsValidationError()` - Checks if the error is a validation error
- `IsAuthenticationError()` - Checks if the error is an authentication error
- `IsPermissionError()` - Checks if the error is a permission error
- `IsRateLimitError()` - Checks if the error is a rate limit error
- `IsInternalError()` - Checks if the error is an internal error
- `FormatTransactionError()` - Produces a standardized error message for transaction errors
- `GetTransactionErrorCode()` - Extracts the error code from a transaction error
- `GetTransactionErrorMessage()` - Extracts the error message from a transaction error

## V. Models Package (`midaz/models`)

The models package defines the data structures used throughout the SDK.

### Resource Models

- `Organization` - Represents an organization in the system
- `Ledger` - Represents a ledger in the system
- `Account` - Represents an account in the system
- `Asset` - Represents an asset in the system
- `AssetRate` - Represents an exchange rate between assets
- `Portfolio` - Represents a portfolio in the system
- `Segment` - Represents a segment in the system
- `Transaction` - Represents a transaction in the system
- `Operation` - Represents an operation within a transaction
- `Balance` - Represents an account balance

### Input Models

- `CreateOrganizationInput` - Input for creating an organization
- `UpdateOrganizationInput` - Input for updating an organization
- `CreateLedgerInput` - Input for creating a ledger
- `UpdateLedgerInput` - Input for updating a ledger
- `CreateAccountInput` - Input for creating an account
- `UpdateAccountInput` - Input for updating an account
- `CreateAssetInput` - Input for creating an asset
- `UpdateAssetInput` - Input for updating an asset
- `CreateAssetRateInput` - Input for creating an asset rate
- `UpdateAssetRateInput` - Input for updating an asset rate
- `CreatePortfolioInput` - Input for creating a portfolio
- `UpdatePortfolioInput` - Input for updating a portfolio
- `CreateSegmentInput` - Input for creating a segment
- `UpdateSegmentInput` - Input for updating a segment
- `CreateTransactionInput` - Input for creating a transaction
- `CreateOperationInput` - Input for creating an operation
- `UpdateBalanceInput` - Input for updating a balance

### Response Models

- `ListResponse` - Generic paginated response for list operations
- `ListOptions` - Options for list operations

### Common Types and Constants

- `Status` - Enum for resource status
- `Address` - Struct for address information
- `TransactionStatus` constants - Pending, Committed, Canceled
- `OperationType` constants - Debit, Credit
- `AssetType` constants - Currency, Stock, Crypto