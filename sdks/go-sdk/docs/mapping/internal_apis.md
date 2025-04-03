# Midaz Go SDK Internal API Map

This document provides an overview of the internal APIs used by the Midaz Go SDK, organized by package and purpose.

## I. Client Package (`midaz/client`)

The client package provides the foundation for all API interactions.

### HTTP Client

- `httpClient` - Handles all HTTP requests to the Midaz API
  - `Do()` - Executes an HTTP request with retries and error handling
  - `Get()` - Performs an HTTP GET request
  - `Post()` - Performs an HTTP POST request
  - `Put()` - Performs an HTTP PUT request
  - `Delete()` - Performs an HTTP DELETE request
  - `Patch()` - Performs an HTTP PATCH request

### API Client

- `apiClient` - Base client for all API operations
  - `SetAuthToken()` - Sets the authentication token
  - `SetOnboardingURL()` - Sets the onboarding API URL
  - `SetTransactionURL()` - Sets the transaction API URL
  - `SetTimeout()` - Sets the request timeout
  - `SetDebug()` - Enables or disables debug mode

## II. Resource Clients (`midaz/client`)

These clients handle specific resource operations and are used by the builders.

### Organization Client

- `organizationClient` - Handles organization-related API operations
  - `List()` - Lists organizations
  - `Get()` - Gets an organization by ID
  - `Create()` - Creates a new organization
  - `Update()` - Updates an organization
  - `Delete()` - Deletes an organization

### Ledger Client

- `ledgerClient` - Handles ledger-related API operations
  - `List()` - Lists ledgers for an organization
  - `Get()` - Gets a ledger by ID
  - `Create()` - Creates a new ledger
  - `Update()` - Updates a ledger
  - `Delete()` - Deletes a ledger

### Account Client

- `accountClient` - Handles account-related API operations
  - `List()` - Lists accounts for a ledger
  - `Get()` - Gets an account by ID
  - `GetByAlias()` - Gets an account by alias
  - `Create()` - Creates a new account
  - `Update()` - Updates an account
  - `Delete()` - Deletes an account

### Asset Client

- `assetClient` - Handles asset-related API operations
  - `List()` - Lists assets for a ledger
  - `Get()` - Gets an asset by ID
  - `Create()` - Creates a new asset
  - `Update()` - Updates an asset
  - `Delete()` - Deletes an asset

### Asset Rate Client

- `assetRateClient` - Handles asset rate-related API operations
  - `List()` - Lists asset rates for a ledger
  - `Get()` - Gets an asset rate by ID
  - `Create()` - Creates a new asset rate
  - `Update()` - Updates an asset rate
  - `Delete()` - Deletes an asset rate

### Portfolio Client

- `portfolioClient` - Handles portfolio-related API operations
  - `List()` - Lists portfolios for a ledger
  - `Get()` - Gets a portfolio by ID
  - `Create()` - Creates a new portfolio
  - `Update()` - Updates a portfolio
  - `Delete()` - Deletes a portfolio

### Segment Client

- `segmentClient` - Handles segment-related API operations
  - `List()` - Lists segments for a portfolio
  - `Get()` - Gets a segment by ID
  - `Create()` - Creates a new segment
  - `Update()` - Updates a segment
  - `Delete()` - Deletes a segment

### Transaction Client

- `transactionClient` - Handles transaction-related API operations
  - `List()` - Lists transactions for a ledger
  - `Get()` - Gets a transaction by ID
  - `Create()` - Creates a new transaction
  - `Update()` - Updates a transaction
  - `Delete()` - Deletes a transaction

### Balance Client

- `balanceClient` - Handles balance-related API operations
  - `List()` - Lists balances for a ledger
  - `Get()` - Gets a balance by ID
  - `Update()` - Updates a balance

## III. Builder Implementations (`midaz/builders`)

The builders package provides fluent interfaces for creating and updating resources, implemented as private structs with public interfaces.

### Organization Builders

- `baseOrganizationBuilder` - Base implementation for organization builders
  - Contains common fields and methods for organization creation and updates
  - Implements the `WithLegalName()`, `WithLegalDocument()`, etc. methods

- `organizationBuilder` - Implementation of the OrganizationBuilder interface
  - Extends baseOrganizationBuilder
  - Implements `Create()` to create a new organization

- `organizationUpdateBuilder` - Implementation of the OrganizationUpdateBuilder interface
  - Extends baseOrganizationBuilder
  - Tracks which fields have been updated
  - Implements `Update()` to update an existing organization

### Ledger Builders

- `baseLedgerBuilder` - Base implementation for ledger builders
  - Contains common fields and methods for ledger creation and updates
  - Implements the `WithName()`, `WithOrganization()`, etc. methods

- `ledgerBuilder` - Implementation of the LedgerBuilder interface
  - Extends baseLedgerBuilder
  - Implements `Create()` to create a new ledger

- `ledgerUpdateBuilder` - Implementation of the LedgerUpdateBuilder interface
  - Extends baseLedgerBuilder
  - Tracks which fields have been updated
  - Implements `Update()` to update an existing ledger

### Account Builders

- `baseAccountBuilder` - Base implementation for account builders
  - Contains common fields and methods for account creation and updates
  - Implements the `WithName()`, `WithAssetCode()`, etc. methods

- `accountBuilder` - Implementation of the AccountBuilder interface
  - Extends baseAccountBuilder
  - Implements `Create()` to create a new account

- `accountUpdateBuilder` - Implementation of the AccountUpdateBuilder interface
  - Extends baseAccountBuilder
  - Tracks which fields have been updated
  - Implements `Update()` to update an existing account

### Asset Builders

- `baseAssetBuilder` - Base implementation for asset builders
  - Contains common fields and methods for asset creation and updates
  - Implements the `WithName()`, `WithCode()`, etc. methods

- `assetBuilder` - Implementation of the AssetBuilder interface
  - Extends baseAssetBuilder
  - Implements `Create()` to create a new asset

- `assetUpdateBuilder` - Implementation of the AssetUpdateBuilder interface
  - Extends baseAssetBuilder
  - Tracks which fields have been updated
  - Implements `Update()` to update an existing asset

### Asset Rate Builders

- `assetRateBuilder` - Implementation of the AssetRateBuilder interface
  - Contains fields for asset rate creation and updates
  - Implements the `WithBaseAsset()`, `WithQuoteAsset()`, etc. methods
  - Implements `CreateOrUpdate()` to create or update an asset rate

### Portfolio Builders

- `basePortfolioBuilder` - Base implementation for portfolio builders
  - Contains common fields and methods for portfolio creation and updates
  - Implements the `WithName()`, `WithOrganization()`, etc. methods

- `portfolioBuilder` - Implementation of the PortfolioBuilder interface
  - Extends basePortfolioBuilder
  - Implements `Create()` to create a new portfolio

- `portfolioUpdateBuilder` - Implementation of the PortfolioUpdateBuilder interface
  - Extends basePortfolioBuilder
  - Tracks which fields have been updated
  - Implements `Update()` to update an existing portfolio

### Segment Builders

- `baseSegmentBuilder` - Base implementation for segment builders
  - Contains common fields and methods for segment creation and updates
  - Implements the `WithName()`, `WithPortfolio()`, etc. methods

- `segmentBuilder` - Implementation of the SegmentBuilder interface
  - Extends baseSegmentBuilder
  - Implements `Create()` to create a new segment

- `segmentUpdateBuilder` - Implementation of the SegmentUpdateBuilder interface
  - Extends baseSegmentBuilder
  - Tracks which fields have been updated
  - Implements `Update()` to update an existing segment

### Balance Builders

- `balanceUpdateBuilder` - Implementation of the BalanceUpdateBuilder interface
  - Contains fields for balance updates
  - Implements the `WithAllowSending()`, `WithAllowReceiving()` methods
  - Tracks which fields have been updated
  - Implements `Update()` to update an existing balance

### Transaction Builders

- `baseTransactionBuilder` - Base implementation for transaction builders
  - Contains common fields and methods for all transaction types
  - Implements the `WithAmount()`, `WithAssetCode()`, etc. methods

- `depositBuilder` - Implementation of the DepositBuilder interface
  - Extends baseTransactionBuilder
  - Implements `ToAccount()` to set the destination account
  - Implements `Execute()` to create a deposit transaction

- `withdrawalBuilder` - Implementation of the WithdrawalBuilder interface
  - Extends baseTransactionBuilder
  - Implements `FromAccount()` to set the source account
  - Implements `Execute()` to create a withdrawal transaction

- `transferBuilder` - Implementation of the TransferBuilder interface
  - Extends baseTransactionBuilder
  - Implements `FromAccount()` and `ToAccount()` to set source and destination
  - Implements `Execute()` to create a transfer transaction

## IV. Error Handling (`midaz/errors`)

The errors package provides standardized error types and utilities for handling errors in a consistent way.

### Error Types

- `MidazError` - Custom error type that includes error code and resource information
  - `Error()` - Returns the formatted error message
  - `Unwrap()` - Returns the underlying error for use with errors.Is() and errors.As()
  - `Is()` - Checks if the error matches a target error

### Error Creation

- `NewError()` - Creates a new MidazError with the given code and error
- `NewErrorf()` - Creates a new MidazError with the given code and formatted message
- `APIErrorToError()` - Converts an internal API error type to a public error

### Error Type Constants

- `ErrNotFound` - Error code for resource not found
- `ErrValidation` - Error code for validation failures
- `ErrTimeout` - Error code for request timeouts
- `ErrAuthentication` - Error code for authentication failures
- `ErrPermission` - Error code for permission issues
- `ErrRateLimit` - Error code for rate limiting
- `ErrInternal` - Error code for internal server errors
- `ErrAccountEligibility` - Error code for account eligibility issues
- `ErrAssetMismatch` - Error code for asset mismatch issues
- `ErrInsufficientBalance` - Error code for insufficient balance
- `ErrExternalAccountFormat` - Error code for external account format issues
- `ErrIdempotencyKey` - Error code for idempotency key issues

### Error Type Checking

- `IsNotFoundError()` - Checks if an error is a not found error
- `IsValidationError()` - Checks if an error is a validation error
- `IsAccountEligibilityError()` - Checks if an error is related to account eligibility
- `IsInsufficientBalanceError()` - Checks if an error is an insufficient balance error
- `IsAssetMismatchError()` - Checks if an error is related to asset mismatch
- `IsIdempotencyError()` - Checks if an error is related to idempotency key handling
- `IsTimeoutError()` - Checks if an error is related to timeout
- `IsAuthenticationError()` - Checks if an error is related to authentication
- `IsPermissionError()` - Checks if an error is related to permissions
- `IsRateLimitError()` - Checks if an error is related to rate limiting
- `IsInternalError()` - Checks if an error is an internal error

### Transaction Error Handling

- `TransactionErrorContext` - Struct containing detailed error context information
  - `ResourceType` - Type of resource affected (e.g., "account", "transaction")
  - `ResourceID` - ID of the affected resource
  - `Operation` - Operation that failed (e.g., "create", "update")
  - `Details` - Additional error details

- `FormatTransactionError()` - Produces a standardized error message for transaction errors
- `CategorizeTransactionError()` - Provides the error category as a string
- `GetTransactionErrorContext()` - Returns detailed context information for transaction errors
- `IsNonTransactionError()` - Checks if an error is not related to transaction processing
- `IsTransactionRetryable()` - Determines if a transaction error can be safely retried
- `FormatErrorWithResourceContext()` - Adds context about the affected resource to an error message

## V. Models Package (`midaz/models`)

The models package defines the data structures used throughout the SDK.

### Resource Models

- `Organization` - Represents an organization
- `Ledger` - Represents a ledger
- `Account` - Represents an account
- `Asset` - Represents an asset
- `AssetRate` - Represents an asset exchange rate
- `Portfolio` - Represents a portfolio
- `Segment` - Represents a segment
- `Transaction` - Represents a transaction
- `Balance` - Represents an account balance

### Request/Response Models

- `CreateOrganizationRequest` - Request to create an organization
- `CreateOrganizationResponse` - Response from creating an organization
- `UpdateOrganizationRequest` - Request to update an organization
- `UpdateOrganizationResponse` - Response from updating an organization
- (Similar patterns for other resources)

### Pagination Models

- `PaginationParams` - Parameters for paginated requests
- `PaginatedResponse` - Generic paginated response

## VI. Utility Functions (`midaz/utils`)

The utils package provides helper functions used throughout the SDK.

### HTTP Utilities

- `BuildURL()` - Builds a URL with path parameters
- `BuildQueryParams()` - Builds query parameters for a request
- `DecodeResponse()` - Decodes an HTTP response into a struct
- `EncodeRequest()` - Encodes a struct into an HTTP request body

### Validation Utilities

- `ValidateRequired()` - Validates that a required field is not empty
- `ValidateFormat()` - Validates that a field matches a specific format
- `ValidateLength()` - Validates that a field has a specific length
- `ValidateRange()` - Validates that a numeric field is within a range

### Conversion Utilities

- `ToSnakeCase()` - Converts a string to snake_case
- `ToCamelCase()` - Converts a string to camelCase
- `ToTitleCase()` - Converts a string to TitleCase
- `FormatAmount()` - Formats an amount with the specified scale
- `ParseAmount()` - Parses an amount string into a numeric value and scale

### Time Utilities

- `FormatTime()` - Formats a time in the specified format
- `ParseTime()` - Parses a time string in the specified format
- `IsZeroTime()` - Checks if a time is the zero value
- `Now()` - Returns the current time in UTC