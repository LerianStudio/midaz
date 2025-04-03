# Midaz Go SDK Method Map

This document provides an overview of all public methods available in the Midaz Go SDK, organized by purpose and package.

## I. High-Level APIs

These are the primary entry points for most users. They provide a simplified interface to the Midaz APIs and handle common use cases.

### Top-Level Package (`midaz`)

- **client.go**
  - `NewClient()` - Creates a new Midaz client with the given configuration options

- **options.go**
  - `WithAuthToken()` - Sets the authentication token for the client
  - `WithOnboardingURL()` - Sets the onboarding API URL
  - `WithTransactionURL()` - Sets the transaction API URL
  - `WithDebug()` - Enables or disables debug mode
  - `WithTimeout()` - Sets the HTTP request timeout in seconds

### Resource Services (`midaz`)

- **Organizations**
  - `Client.List()` - Lists all organizations
  - `Client.Get()` - Gets an organization by ID
  - `Client.Create()` - Creates a new organization
  - `Client.Update()` - Updates an organization
  - `Client.Delete()` - Deletes an organization

- **Ledgers**
  - `Client.List()` - Lists ledgers for an organization
  - `Client.Get()` - Gets a ledger by ID
  - `Client.Create()` - Creates a new ledger
  - `Client.Update()` - Updates a ledger
  - `Client.Delete()` - Deletes a ledger

- **Accounts**
  - `Client.List()` - Lists accounts for a ledger
  - `Client.Get()` - Gets an account by ID
  - `Client.Create()` - Creates a new account
  - `Client.Update()` - Updates an account
  - `Client.Delete()` - Deletes an account

- **Assets**
  - `Client.List()` - Lists assets for a ledger
  - `Client.Get()` - Gets an asset by ID
  - `Client.Create()` - Creates a new asset
  - `Client.Update()` - Updates an asset
  - `Client.Delete()` - Deletes an asset

- **Portfolios**
  - `Client.List()` - Lists portfolios for a ledger
  - `Client.Get()` - Gets a portfolio by ID
  - `Client.Create()` - Creates a new portfolio
  - `Client.Update()` - Updates a portfolio
  - `Client.Delete()` - Deletes a portfolio

- **Transactions**
  - `Client.List()` - Lists transactions for a ledger
  - `Client.Get()` - Gets a transaction by ID
  - `Client.Create()` - Creates a new transaction
  - `Client.Update()` - Updates a transaction
  - `Client.Delete()` - Deletes a transaction

- **Balances**
  - `Client.List()` - Lists balances for a ledger
  - `Client.Get()` - Gets a balance by ID
  - `Client.Update()` - Updates a balance

## II. Builder APIs (`midaz/builders`)

The builders package provides fluent builder interfaces for creating and updating resources. These builders simplify complex API interactions through method chaining and progressive disclosure of options.

### Organization Builders

- `NewOrganization()` - Creates a new organization builder
  - `WithLegalName()` - Sets the organization's legal name
  - `WithLegalDocument()` - Sets the organization's legal document (e.g., tax ID)
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

### Balance Builders

- `NewBalanceUpdate()` - Creates a new balance update builder
  - `WithAllowSending()` - Sets whether sending is allowed
  - `WithAllowReceiving()` - Sets whether receiving is allowed
  - `Update()` - Executes the balance update and returns the updated balance

### Transaction Builders

- `NewDeposit()` - Creates a new deposit transaction builder
  - `WithOrganization()` - Sets the organization ID
  - `WithLedger()` - Sets the ledger ID
  - `WithAmount()` - Sets the amount and scale
  - `WithAssetCode()` - Sets the asset code
  - `WithDescription()` - Sets a description for the transaction
  - `WithMetadata()` - Sets additional metadata
  - `WithTag()` - Adds a single tag to the transaction
  - `WithTags()` - Adds multiple tags to the transaction
  - `WithExternalID()` - Sets an external ID for the transaction
  - `WithIdempotencyKey()` - Sets an idempotency key
  - `ToAccount()` - Sets the destination account
  - `Execute()` - Executes the deposit and returns the created transaction

- `NewWithdrawal()` - Creates a new withdrawal transaction builder
  - `WithOrganization()` - Sets the organization ID
  - `WithLedger()` - Sets the ledger ID
  - `WithAmount()` - Sets the amount and scale
  - `WithAssetCode()` - Sets the asset code
  - `WithDescription()` - Sets a description for the transaction
  - `WithMetadata()` - Sets additional metadata
  - `WithTag()` - Adds a single tag to the transaction
  - `WithTags()` - Adds multiple tags to the transaction
  - `WithExternalID()` - Sets an external ID for the transaction
  - `WithIdempotencyKey()` - Sets an idempotency key
  - `FromAccount()` - Sets the source account
  - `Execute()` - Executes the withdrawal and returns the created transaction

- `NewTransfer()` - Creates a new transfer transaction builder
  - `WithOrganization()` - Sets the organization ID
  - `WithLedger()` - Sets the ledger ID
  - `WithAmount()` - Sets the amount and scale
  - `WithAssetCode()` - Sets the asset code
  - `WithDescription()` - Sets a description for the transaction
  - `WithMetadata()` - Sets additional metadata
  - `WithTag()` - Adds a single tag to the transaction
  - `WithTags()` - Adds multiple tags to the transaction
  - `WithExternalID()` - Sets an external ID for the transaction
  - `WithIdempotencyKey()` - Sets an idempotency key
  - `FromAccount()` - Sets the source account
  - `ToAccount()` - Sets the destination account
  - `Execute()` - Executes the transfer and returns the created transaction

## III. Error Handling (`midaz/errors`)

The errors package provides standardized error types and utilities for handling errors in a consistent way.

### Standard Error Types

- `ErrNotFound` - Returned when a resource is not found
- `ErrValidation` - Returned when a request fails validation
- `ErrTimeout` - Returned when a request times out
- `ErrAuthentication` - Returned when authentication fails
- `ErrPermission` - Returned when the user does not have permission
- `ErrRateLimit` - Returned when the API rate limit is exceeded
- `ErrInternal` - Returned when an unexpected error occurs
- `ErrAccountEligibility` - Returned when accounts are not eligible for a transaction
- `ErrAssetMismatch` - Returned when accounts have different asset types
- `ErrInsufficientBalance` - Returned when a transaction would result in a negative balance
- `ErrExternalAccountFormat` - Returned when an external account reference has an invalid format
- `ErrIdempotencyKey` - Returned when there's an issue with idempotency key handling

### Error Handling Utilities

- `MidazError` - Detailed error type with code, message, and resource information
  - `Error()` - Implements the error interface with formatted error message
  - `Unwrap()` - Returns the underlying error for use with errors.Is() and errors.As()
  - `Is()` - Checks if the error matches a target error

- `NewError()` - Creates a new MidazError with the given code and error
- `NewErrorf()` - Creates a new MidazError with the given code and formatted message
- `APIErrorToError()` - Converts an internal API error type to a public error

### Error Type Checking

- `IsNotFoundError()` - Checks if the error is a not found error
- `IsValidationError()` - Checks if the error is a validation error
- `IsAccountEligibilityError()` - Checks if the error is related to account eligibility
- `IsInsufficientBalanceError()` - Checks if the error is an insufficient balance error
- `IsAssetMismatchError()` - Checks if the error is related to asset mismatch
- `IsIdempotencyError()` - Checks if the error is related to idempotency key handling
- `IsTimeoutError()` - Checks if the error is related to timeout
- `IsAuthenticationError()` - Checks if the error is related to authentication
- `IsPermissionError()` - Checks if the error is related to permissions
- `IsRateLimitError()` - Checks if the error is related to rate limiting
- `IsInternalError()` - Checks if the error is an internal error

### Transaction Error Utilities

- `FormatTransactionError()` - Produces a standardized error message for transaction errors
- `CategorizeTransactionError()` - Provides the error category as a string
- `GetTransactionErrorContext()` - Returns detailed context information for transaction errors
- `TransactionErrorContext` - Struct containing detailed error context information
- `IsNonTransactionError()` - Checks if an error is not related to transaction processing
- `IsTransactionRetryable()` - Determines if a transaction error can be safely retried
- `FormatErrorWithResourceContext()` - Adds context about the affected resource to an error message