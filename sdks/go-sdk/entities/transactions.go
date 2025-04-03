package entities

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	libTransaction "github.com/LerianStudio/lib-commons/commons/transaction"
	"github.com/LerianStudio/midaz/sdks/go-sdk/internal/api"
	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
)

// TransactionsService defines the interface for transaction-related operations.
// It provides methods to create, read, update, and commit transactions
// within a ledger and organization.
type TransactionsService interface {
	// CreateTransaction creates a new transaction using the standard format.
	// The orgID and ledgerID parameters specify which organization and ledger to create the transaction in.
	// The input parameter contains the transaction details such as entries, metadata, and external ID.
	// Returns the created transaction, or an error if the operation fails.
	CreateTransaction(ctx context.Context, orgID, ledgerID string, input *models.CreateTransactionInput) (*models.Transaction, error)

	// CreateTransactionWithDSL creates a new transaction using the DSL format.
	// The orgID and ledgerID parameters specify which organization and ledger to create the transaction in.
	// The input parameter contains the transaction DSL script and optional metadata.
	// Returns the created transaction, or an error if the operation fails.
	CreateTransactionWithDSL(ctx context.Context, orgID, ledgerID string, input *models.TransactionDSLInput) (*models.Transaction, error)

	// CreateTransactionWithDSLFile creates a new transaction using a DSL file.
	// The orgID and ledgerID parameters specify which organization and ledger to create the transaction in.
	// The dslContent parameter contains the raw DSL file content as bytes.
	// Returns the created transaction, or an error if the operation fails.
	CreateTransactionWithDSLFile(ctx context.Context, orgID, ledgerID string, dslContent []byte) (*models.Transaction, error)

	// GetTransaction retrieves a specific transaction by its ID.
	// The orgID and ledgerID parameters specify which organization and ledger the transaction belongs to.
	// The transactionID parameter is the unique identifier of the transaction to retrieve.
	// Returns the transaction if found, or an error if the operation fails or the transaction doesn't exist.
	GetTransaction(ctx context.Context, orgID, ledgerID, transactionID string) (*models.Transaction, error)

	// ListTransactions retrieves a paginated list of transactions for a ledger with optional filters.
	// The orgID and ledgerID parameters specify which organization and ledger to query.
	// The opts parameter can be used to specify pagination, sorting, and filtering options.
	// Returns a ListResponse containing the transactions and pagination information, or an error if the operation fails.
	ListTransactions(ctx context.Context, orgID, ledgerID string, opts *models.ListOptions) (*models.ListResponse[models.Transaction], error)

	// UpdateTransaction updates an existing transaction.
	// The orgID and ledgerID parameters specify which organization and ledger the transaction belongs to.
	// The transactionID parameter is the unique identifier of the transaction to update.
	// The input parameter contains the transaction details to update, which can be of various types.
	// Returns the updated transaction, or an error if the operation fails.
	UpdateTransaction(ctx context.Context, orgID, ledgerID, transactionID string, input any) (*models.Transaction, error)

	// CommitTransaction commits a transaction, making it final and immutable.
	// The orgID and ledgerID parameters specify which organization and ledger the transaction belongs to.
	// The transactionID parameter is the unique identifier of the transaction to commit.
	// Returns the committed transaction, or an error if the operation fails.
	CommitTransaction(ctx context.Context, orgID, ledgerID, transactionID string) (*models.Transaction, error)

	// CommitTransactionWithExternalID commits a transaction using an external ID instead of the internal transaction ID.
	// The orgID and ledgerID parameters specify which organization and ledger the transaction belongs to.
	// The externalID parameter is the external identifier of the transaction to commit.
	// Returns the committed transaction, or an error if the operation fails.
	CommitTransactionWithExternalID(ctx context.Context, orgID, ledgerID, externalID string) (*models.Transaction, error)
}

// transactionsEntity implements the TransactionsService interface.
// It handles the communication with the Midaz API for transaction-related operations.
type transactionsEntity struct {
	httpClient *http.Client
	authToken  string
	baseURLs   map[string]string
}

// NewTransactionsEntity creates a new transactions entity.
//
// Parameters:
//   - httpClient: The HTTP client used for API requests. Can be configured with custom timeouts
//     and transport options. If nil, a default client will be used.
//   - authToken: The authentication token for API authorization. Must be a valid JWT token
//     issued by the Midaz authentication service.
//   - baseURLs: Map of service names to base URLs. Must include a "transaction" key with
//     the URL of the transaction service (e.g., "https://api.midaz.io/v1").
//
// Returns:
//   - TransactionsService: An implementation of the TransactionsService interface that provides
//     methods for creating, retrieving, and managing transactions.
//
// Example:
//
//	// Create a transactions entity with default HTTP client
//	txEntity := entities.NewTransactionsEntity(
//	    &http.Client{Timeout: 30 * time.Second},
//	    "your-auth-token",
//	    map[string]string{"transaction": "https://api.midaz.io/v1"},
//	)
//
//	// Use the entity to create a transaction
//	tx, err := txEntity.CreateTransaction(
//	    context.Background(),
//	    "org-123",
//	    "ledger-456",
//	    &models.CreateTransactionInput{
//	        Description: "Payment for invoice #123",
//	        Entries: []models.Entry{
//	            {
//	                AccountAlias: "customer:john.doe",
//	                Direction:    models.DirectionCredit,
//	                Amount:       models.Amount{Value: 10000, Scale: 2},
//	                AssetCode:    "USD",
//	            },
//	        },
//	        Metadata: map[string]interface{}{
//	            "invoice_id": "inv-123",
//	            "customer_id": "cust-456",
//	        },
//	    },
//	)
//
//	if err != nil {
//	    log.Fatalf("Failed to create transaction: %v", err)
//	}
//
//	fmt.Printf("Transaction created: %s\n", tx.ID)
func NewTransactionsEntity(httpClient *http.Client, authToken string, baseURLs map[string]string) TransactionsService {
	return &transactionsEntity{
		httpClient: httpClient,
		authToken:  authToken,
		baseURLs:   baseURLs,
	}
}

// CreateTransaction creates a new transaction using the standard format.
//
// This method creates a transaction using the standard format, which involves specifying
// a list of entries (debits and credits) that make up the transaction. Each entry specifies
// an account, direction (debit or credit), amount, and asset code.
//
// Parameters:
//   - ctx: Context for the request, which can be used for cancellation and timeout.
//   - orgID: The ID of the organization that owns the ledger. Must be a valid organization ID.
//   - ledgerID: The ID of the ledger where the transaction will be created. Must be a valid ledger ID.
//   - input: The transaction details, including entries, description, metadata, and other properties.
//     The input must contain at least one entry, and the transaction must be balanced
//     (total debits must equal total credits for each asset).
//
// Returns:
//   - *models.Transaction: The created transaction if successful, containing the transaction ID,
//     status, entries, and other properties.
//   - error: An error if the operation fails. Possible errors include:
//   - Invalid input (missing required fields, unbalanced transaction)
//   - Authentication failure (invalid auth token)
//   - Authorization failure (insufficient permissions)
//   - Resource not found (invalid organization or ledger ID)
//   - Network or server errors
//
// Example:
//
//	// Create a simple payment transaction
//	tx, err := txService.CreateTransaction(
//	    context.Background(),
//	    "org-123",
//	    "ledger-456",
//	    &models.CreateTransactionInput{
//	        Description: "Payment for invoice #123",
//	        Entries: []models.Entry{
//	            {
//	                // Debit the customer's account (decrease balance)
//	                AccountAlias: "customer:john.doe",
//	                Direction:    models.DirectionDebit,
//	                Amount:       models.Amount{Value: 10000, Scale: 2}, // $100.00
//	                AssetCode:    "USD",
//	            },
//	            {
//	                // Credit the revenue account (increase balance)
//	                AccountAlias: "revenue:payments",
//	                Direction:    models.DirectionCredit,
//	                Amount:       models.Amount{Value: 10000, Scale: 2}, // $100.00
//	                AssetCode:    "USD",
//	            },
//	        },
//	        Metadata: map[string]interface{}{
//	            "invoice_id": "inv-123",
//	            "customer_id": "cust-456",
//	        },
//	    },
//	)
//
//	if err != nil {
//	    // Handle error
//	    return err
//	}
//
//	// Use the transaction
//	fmt.Printf("Transaction created: %s (status: %s)\n", tx.ID, tx.Status)
func (e *transactionsEntity) CreateTransaction(ctx context.Context, orgID, ledgerID string, input *models.CreateTransactionInput) (*models.Transaction, error) {
	if input == nil {
		return nil, fmt.Errorf("transaction input cannot be nil")
	}

	// Validate required parameters
	if orgID == "" {
		return nil, fmt.Errorf("organization ID cannot be empty")
	}

	if ledgerID == "" {
		return nil, fmt.Errorf("ledger ID cannot be empty")
	}

	// Validate the input using the model's Validate method
	if err := input.Validate(); err != nil {
		return nil, fmt.Errorf("invalid transaction input: %v", err)
	}

	// Convert to lib-commons format
	libTransaction := input.ToLibTransaction()

	url := e.buildURL(orgID, ledgerID, "")

	body, err := json.Marshal(libTransaction)

	if err != nil {
		return nil, fmt.Errorf("internal error: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))

	if err != nil {
		return nil, fmt.Errorf("internal error: %w", err)
	}

	e.setCommonHeaders(req)

	resp, err := e.httpClient.Do(req)

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	// Check if the response status code indicates an error
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, api.ErrorFromResponse(resp, respBody)
	}

	var transaction models.Transaction

	if err := json.NewDecoder(resp.Body).Decode(&transaction); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &transaction, nil
}

// CreateTransactionWithDSL creates a new transaction using the DSL format.
//
// This method creates a transaction using the Domain-Specific Language (DSL) format,
// which provides a more flexible way to define complex transactions. The DSL format
// allows for more advanced transaction logic, including conditional operations and
// multi-step transactions.
//
// Parameters:
//   - ctx: Context for the request, which can be used for cancellation and timeout.
//   - orgID: The ID of the organization that owns the ledger. Must be a valid organization ID.
//   - ledgerID: The ID of the ledger where the transaction will be created. Must be a valid ledger ID.
//   - input: The transaction DSL input, including the DSL script and optional metadata.
//     The DSL script must follow the Midaz transaction DSL syntax and must define a balanced
//     transaction (total debits must equal total credits for each asset).
//
// Returns:
//   - *models.Transaction: The created transaction if successful, containing the transaction ID,
//     status, operations, and other properties.
//   - error: An error if the operation fails. Possible errors include:
//   - Invalid DSL script (syntax errors, unbalanced transaction)
//   - Authentication failure (invalid auth token)
//   - Authorization failure (insufficient permissions)
//   - Resource not found (invalid organization or ledger ID)
//   - Network or server errors
//
// Example:
//
//	// Create a transaction using DSL
//	tx, err := txService.CreateTransactionWithDSL(
//	    context.Background(),
//	    "org-123",
//	    "ledger-456",
//	    &models.TransactionDSLInput{
//	        DSL: `
//	            // Define a simple transfer transaction
//	            transaction {
//	                description = "Transfer from savings to checking"
//
//	                // Debit the savings account
//	                debit {
//	                    account = "customer:john.doe:savings"
//	                    asset = "USD"
//	                    amount = 50000 // $500.00 with scale 2
//	                }
//
//	                // Credit the checking account
//	                credit {
//	                    account = "customer:john.doe:checking"
//	                    asset = "USD"
//	                    amount = 50000 // $500.00 with scale 2
//	                }
//	            }
//	        `,
//	        Metadata: map[string]interface{}{
//	            "transfer_id": "transfer-123",
//	            "initiated_by": "mobile-app",
//	        },
//	    },
//	)
//
//	if err != nil {
//	    // Handle error
//	    return err
//	}
//
//	// Use the transaction
//	fmt.Printf("Transaction created: %s (status: %s)\n", tx.ID, tx.Status)
func (e *transactionsEntity) CreateTransactionWithDSL(ctx context.Context, orgID, ledgerID string, input *models.TransactionDSLInput) (*models.Transaction, error) {
	if input == nil {
		return nil, fmt.Errorf("transaction DSL input cannot be nil")
	}

	// Validate required parameters
	if orgID == "" {
		return nil, fmt.Errorf("organization ID cannot be empty")
	}

	if ledgerID == "" {
		return nil, fmt.Errorf("ledger ID cannot be empty")
	}

	// Validate the input using the model's Validate method
	if err := input.Validate(); err != nil {
		return nil, fmt.Errorf("invalid transaction DSL input: %v", err)
	}

	// Convert the DSL input to lib-commons format before sending to API
	libTransaction := input.ToLibTransaction()

	url := fmt.Sprintf("%s/dsl", e.buildURL(orgID, ledgerID, ""))

	body, err := json.Marshal(libTransaction)

	if err != nil {
		return nil, fmt.Errorf("internal error: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))

	if err != nil {
		return nil, fmt.Errorf("internal error: %w", err)
	}

	e.setCommonHeaders(req)

	resp, err := e.httpClient.Do(req)

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	// Check if the response status code indicates an error
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, api.ErrorFromResponse(resp, respBody)
	}

	var transaction models.Transaction

	if err := json.NewDecoder(resp.Body).Decode(&transaction); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &transaction, nil
}

// CreateTransactionWithDSLFile creates a new transaction using a DSL file.
func (e *transactionsEntity) CreateTransactionWithDSLFile(ctx context.Context, orgID, ledgerID string, dslContent []byte) (*models.Transaction, error) {
	if orgID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	if ledgerID == "" {
		return nil, fmt.Errorf("ledger ID is required")
	}

	if len(dslContent) == 0 {
		return nil, fmt.Errorf("DSL content is required")
	}

	url := fmt.Sprintf("%s/dsl/file", e.buildURL(orgID, ledgerID, ""))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(dslContent))

	if err != nil {
		return nil, fmt.Errorf("internal error: %w", err)
	}

	e.setCommonHeaders(req)

	resp, err := e.httpClient.Do(req)

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	// Check if the response status code indicates an error
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, api.ErrorFromResponse(resp, respBody)
	}

	var transaction models.Transaction

	if err := json.NewDecoder(resp.Body).Decode(&transaction); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &transaction, nil
}

// GetTransaction retrieves a specific transaction by its ID.
//
// This method fetches a transaction by its unique identifier from the specified organization
// and ledger. It returns the complete transaction details, including all operations,
// metadata, and status information.
//
// Parameters:
//   - ctx: Context for the request, which can be used for cancellation and timeout.
//   - orgID: The ID of the organization that owns the ledger. Must be a valid organization ID.
//   - ledgerID: The ID of the ledger where the transaction exists. Must be a valid ledger ID.
//   - transactionID: The unique identifier of the transaction to retrieve. Must be a valid
//     transaction ID previously returned from a transaction creation method.
//
// Returns:
//   - *models.Transaction: The retrieved transaction if found, containing the transaction ID,
//     status, operations, metadata, and other properties.
//   - error: An error if the operation fails. Possible errors include:
//   - Authentication failure (invalid auth token)
//   - Authorization failure (insufficient permissions)
//   - Resource not found (invalid organization, ledger, or transaction ID)
//   - Network or server errors
//
// Example:
//
//	// Retrieve a transaction by ID
//	tx, err := txService.GetTransaction(
//	    context.Background(),
//	    "org-123",
//	    "ledger-456",
//	    "tx-789",
//	)
//
//	if err != nil {
//	    // Handle error
//	    if errors.Is(err, errors.ErrNotFound) {
//	        fmt.Println("Transaction not found")
//	        return
//	    }
//	    return err
//	}
//
//	// Use the transaction data
//	fmt.Printf("Transaction: %s\n", tx.ID)
//	fmt.Printf("Status: %s\n", tx.Status)
//	fmt.Printf("Description: %s\n", tx.Description)
//	fmt.Printf("Created At: %s\n", tx.CreatedAt.Format(time.RFC3339))
//
//	// Process operations
//	for i, op := range tx.Operations {
//	    fmt.Printf("Operation %d: %s %s %d %s on account %s\n",
//	        i+1,
//	        op.Type,
//	        op.AssetCode,
//	        op.Amount.Value,
//	        op.AccountID,
//	    )
//	}
func (e *transactionsEntity) GetTransaction(ctx context.Context, orgID, ledgerID, transactionID string) (*models.Transaction, error) {
	// Validate required parameters
	if orgID == "" {
		return nil, fmt.Errorf("organization ID cannot be empty")
	}

	if ledgerID == "" {
		return nil, fmt.Errorf("ledger ID cannot be empty")
	}

	if transactionID == "" {
		return nil, fmt.Errorf("transaction ID cannot be empty")
	}

	url := e.buildURL(orgID, ledgerID, transactionID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)

	if err != nil {
		return nil, fmt.Errorf("internal error: %w", err)
	}

	e.setCommonHeaders(req)

	resp, err := e.httpClient.Do(req)

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	// Check if the response status code indicates an error
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, api.ErrorFromResponse(resp, respBody)
	}

	// Decode the response into a lib-commons Transaction first
	var libTransaction libTransaction.Transaction
	if err := json.NewDecoder(resp.Body).Decode(&libTransaction); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Convert to SDK Transaction
	transaction := &models.Transaction{}
	transaction.FromLibTransaction(&libTransaction)

	return transaction, nil
}

// ListTransactions retrieves a paginated list of transactions for a ledger with optional filters.
//
// This method fetches a list of transactions from the specified organization and ledger,
// with support for pagination, sorting, and filtering. The results are returned as a paginated
// list that includes the total count and links to navigate between pages.
//
// Parameters:
//   - ctx: Context for the request, which can be used for cancellation and timeout.
//   - orgID: The ID of the organization that owns the ledger. Must be a valid organization ID.
//   - ledgerID: The ID of the ledger to query. Must be a valid ledger ID.
//   - opts: Optional parameters for pagination, sorting, and filtering. Can be nil for default behavior.
//     Supported options include:
//   - Page: The page number to retrieve (starting from 1)
//   - PageSize: The number of items per page (default is 20)
//   - Sort: The field to sort by (e.g., "created_at")
//   - Order: The sort order ("asc" or "desc")
//   - Filter: Additional filtering criteria as key-value pairs
//
// Returns:
//   - *models.ListResponse[models.Transaction]: A paginated response containing:
//   - Items: The list of transactions for the current page
//   - Pagination: Metadata about the pagination, including total count and links
//   - error: An error if the operation fails. Possible errors include:
//   - Authentication failure (invalid auth token)
//   - Authorization failure (insufficient permissions)
//   - Resource not found (invalid organization or ledger ID)
//   - Invalid parameters (negative page number, etc.)
//   - Network or server errors
//
// Example - Basic pagination:
//
//	// List transactions with default pagination (first page, 20 items)
//	response, err := txService.ListTransactions(
//	    context.Background(),
//	    "org-123",
//	    "ledger-456",
//	    nil, // Use default options
//	)
//
//	if err != nil {
//	    // Handle error
//	    return err
//	}
//
//	// Process the transactions
//	fmt.Printf("Found %d transactions (total: %d)\n",
//	    len(response.Items), response.Pagination.Total)
//
//	for _, tx := range response.Items {
//	    fmt.Printf("Transaction %s: %s (created: %s)\n",
//	        tx.ID, tx.Description, tx.CreatedAt.Format(time.RFC3339))
//	}
//
// Example - With filtering and pagination:
//
//	// List transactions with custom pagination and filtering
//	response, err := txService.ListTransactions(
//	    context.Background(),
//	    "org-123",
//	    "ledger-456",
//	    &models.ListOptions{
//	        Page:     2,
//	        PageSize: 10,
//	        Sort:     "created_at",
//	        Order:    "desc",
//	        Filter: map[string]interface{}{
//	            "status": "completed",
//	            "asset_code": "USD",
//	        },
//	    },
//	)
//
//	if err != nil {
//	    // Handle error
//	    return err
//	}
//
//	// Process the transactions
//	fmt.Printf("Page %d of %d (items per page: %d, total items: %d)\n",
//	    response.Pagination.Page,
//	    response.Pagination.TotalPages,
//	    response.Pagination.PageSize,
//	    response.Pagination.Total)
//
//	// Iterate through transactions
//	for _, tx := range response.Items {
//	    fmt.Printf("Transaction %s: %s\n", tx.ID, tx.Description)
//	}
func (e *transactionsEntity) ListTransactions(ctx context.Context, orgID, ledgerID string, opts *models.ListOptions) (*models.ListResponse[models.Transaction], error) {
	// Validate required parameters
	if orgID == "" {
		return nil, fmt.Errorf("organization ID cannot be empty")
	}

	if ledgerID == "" {
		return nil, fmt.Errorf("ledger ID cannot be empty")
	}

	url := e.buildURL(orgID, ledgerID, "")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)

	if err != nil {
		return nil, fmt.Errorf("internal error: %w", err)
	}

	// Add query parameters if options are provided
	if opts != nil {
		q := req.URL.Query()
		if opts.Page > 0 {
			q.Add("page", fmt.Sprintf("%d", opts.Page))
		}
		if opts.Limit > 0 {
			q.Add("limit", fmt.Sprintf("%d", opts.Limit))
		}
		if opts.Filters != nil {
			for k, v := range opts.Filters {
				q.Add(k, v)
			}
		}
		req.URL.RawQuery = q.Encode()
	}

	e.setCommonHeaders(req)

	resp, err := e.httpClient.Do(req)

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	// Check if the response status code indicates an error
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, api.ErrorFromResponse(resp, respBody)
	}

	// First decode into a partial response to get the structure
	var rawResponse struct {
		Items      []json.RawMessage `json:"items"`
		Pagination struct {
			Limit      int    `json:"limit"`
			Offset     int    `json:"offset"`
			Total      int    `json:"total"`
			PrevCursor string `json:"prevCursor,omitempty"`
			NextCursor string `json:"nextCursor,omitempty"`
		} `json:"pagination,omitempty"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&rawResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Now process each transaction
	transactions := make([]models.Transaction, 0, len(rawResponse.Items))
	for _, rawItem := range rawResponse.Items {
		// Decode each item into a lib-commons transaction
		var libTx libTransaction.Transaction
		if err := json.Unmarshal(rawItem, &libTx); err != nil {
			return nil, fmt.Errorf("failed to decode transaction item: %w", err)
		}

		// Convert to SDK transaction
		tx := models.Transaction{}
		tx.FromLibTransaction(&libTx)
		transactions = append(transactions, tx)
	}

	// Create the final response
	response := &models.ListResponse[models.Transaction]{
		Items: transactions,
		Pagination: models.Pagination{
			Limit:      rawResponse.Pagination.Limit,
			Offset:     rawResponse.Pagination.Offset,
			Total:      rawResponse.Pagination.Total,
			PrevCursor: rawResponse.Pagination.PrevCursor,
			NextCursor: rawResponse.Pagination.NextCursor,
		},
	}

	return response, nil
}

// UpdateTransaction updates an existing transaction.
func (e *transactionsEntity) UpdateTransaction(ctx context.Context, orgID, ledgerID, transactionID string, input any) (*models.Transaction, error) {
	if transactionID == "" {
		return nil, fmt.Errorf("transaction ID is required")
	}

	url := e.buildURL(orgID, ledgerID, transactionID)

	body, err := json.Marshal(input)

	if err != nil {
		return nil, fmt.Errorf("internal error: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bytes.NewBuffer(body))

	if err != nil {
		return nil, fmt.Errorf("internal error: %w", err)
	}

	e.setCommonHeaders(req)

	resp, err := e.httpClient.Do(req)

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	// Check if the response status code indicates an error
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, api.ErrorFromResponse(resp, respBody)
	}

	// Decode the response into a lib-commons Transaction first
	var libTransaction libTransaction.Transaction
	if err := json.NewDecoder(resp.Body).Decode(&libTransaction); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Convert to SDK Transaction
	transaction := &models.Transaction{}
	transaction.FromLibTransaction(&libTransaction)

	return transaction, nil
}

// CommitTransaction commits a transaction, making it final and immutable.
//
// This method finalizes a pending transaction, making it permanent and immutable.
// Once a transaction is committed, it cannot be modified or deleted. This is typically
// used for transactions that were created with the pending flag set to true.
//
// Parameters:
//   - ctx: Context for the request, which can be used for cancellation and timeout.
//   - orgID: The ID of the organization that owns the ledger. Must be a valid organization ID.
//   - ledgerID: The ID of the ledger where the transaction exists. Must be a valid ledger ID.
//   - transactionID: The unique identifier of the transaction to commit. Must be a valid
//     transaction ID previously returned from a transaction creation method.
//
// Returns:
//   - *models.Transaction: The committed transaction if successful, containing the updated
//     transaction status and other properties.
//   - error: An error if the operation fails. Possible errors include:
//   - Authentication failure (invalid auth token)
//   - Authorization failure (insufficient permissions)
//   - Resource not found (invalid organization, ledger, or transaction ID)
//   - Invalid state (transaction already committed or in an invalid state)
//   - Network or server errors
//
// Example:
//
//	// Create a pending transaction
//	pendingTx, err := txService.CreateTransaction(
//	    context.Background(),
//	    "org-123",
//	    "ledger-456",
//	    &models.CreateTransactionInput{
//	        Description: "Payment for invoice #123",
//	        Entries: []models.Entry{
//	            // Transaction entries...
//	        },
//	        Pending: true, // Create as pending
//	    },
//	)
//
//	if err != nil {
//	    // Handle error
//	    return err
//	}
//
//	fmt.Printf("Created pending transaction: %s\n", pendingTx.ID)
//
//	// Later, commit the transaction
//	committedTx, err := txService.CommitTransaction(
//	    context.Background(),
//	    "org-123",
//	    "ledger-456",
//	    pendingTx.ID,
//	)
//
//	if err != nil {
//	    // Handle error
//	    return err
//	}
//
//	fmt.Printf("Transaction committed: %s (status: %s)\n",
//	    committedTx.ID, committedTx.Status)
func (e *transactionsEntity) CommitTransaction(ctx context.Context, orgID, ledgerID, transactionID string) (*models.Transaction, error) {
	if transactionID == "" {
		return nil, fmt.Errorf("transaction ID is required")
	}

	url := fmt.Sprintf("%s/commit", e.buildURL(orgID, ledgerID, transactionID))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)

	if err != nil {
		return nil, fmt.Errorf("internal error: %w", err)
	}

	e.setCommonHeaders(req)

	resp, err := e.httpClient.Do(req)

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	// Check if the response status code indicates an error
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, api.ErrorFromResponse(resp, respBody)
	}

	var transaction models.Transaction

	if err := json.NewDecoder(resp.Body).Decode(&transaction); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &transaction, nil
}

// CommitTransactionWithExternalID commits a transaction using an external ID instead of the internal transaction ID.
//
// This method finalizes a pending transaction identified by its external ID rather than its
// internal transaction ID. This is useful when you have the external ID but not the internal
// transaction ID, or when you want to ensure idempotency across commit operations.
//
// Parameters:
//   - ctx: Context for the request, which can be used for cancellation and timeout.
//   - orgID: The ID of the organization that owns the ledger. Must be a valid organization ID.
//   - ledgerID: The ID of the ledger where the transaction exists. Must be a valid ledger ID.
//   - externalID: The external ID of the transaction to commit. This must match the external ID
//     provided when creating the transaction.
//
// Returns:
//   - *models.Transaction: The committed transaction if successful, containing the updated
//     transaction status and other properties.
//   - error: An error if the operation fails. Possible errors include:
//   - Authentication failure (invalid auth token)
//   - Authorization failure (insufficient permissions)
//   - Resource not found (invalid organization, ledger, or external ID)
//   - Invalid state (transaction already committed or in an invalid state)
//   - Network or server errors
//
// Example:
//
//	// Create a pending transaction with an external ID
//	pendingTx, err := txService.CreateTransaction(
//	    context.Background(),
//	    "org-123",
//	    "ledger-456",
//	    &models.CreateTransactionInput{
//	        Description: "Payment for invoice #123",
//	        Entries: []models.Entry{
//	            // Transaction entries...
//	        },
//	        Pending: true, // Create as pending
//	        ExternalID: "payment-inv123-20230401", // Custom external ID
//	    },
//	)
//
//	if err != nil {
//	    // Handle error
//	    return err
//	}
//
//	fmt.Printf("Created pending transaction with external ID: %s\n", pendingTx.ExternalID)
//
//	// Later, commit the transaction using the external ID
//	committedTx, err := txService.CommitTransactionWithExternalID(
//	    context.Background(),
//	    "org-123",
//	    "ledger-456",
//	    "payment-inv123-20230401", // Use the external ID instead of transaction ID
//	)
//
//	if err != nil {
//	    // Handle error
//	    return err
//	}
//
//	fmt.Printf("Transaction committed: %s (status: %s)\n",
//	    committedTx.ID, committedTx.Status)
func (e *transactionsEntity) CommitTransactionWithExternalID(ctx context.Context, orgID, ledgerID, externalID string) (*models.Transaction, error) {
	if externalID == "" {
		return nil, fmt.Errorf("external ID is required")
	}

	url := fmt.Sprintf("%s/commit-external/%s", e.buildURL(orgID, ledgerID, ""), externalID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)

	if err != nil {
		return nil, fmt.Errorf("internal error: %w", err)
	}

	e.setCommonHeaders(req)

	resp, err := e.httpClient.Do(req)

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	// Check if the response status code indicates an error
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, api.ErrorFromResponse(resp, respBody)
	}

	var transaction models.Transaction

	if err := json.NewDecoder(resp.Body).Decode(&transaction); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &transaction, nil
}

// buildURL builds the URL for transactions API calls.
func (e *transactionsEntity) buildURL(orgID, ledgerID, transactionID string) string {
	base := e.baseURLs["transaction"]
	if transactionID == "" {
		return fmt.Sprintf("%s/organizations/%s/ledgers/%s/transactions", base, orgID, ledgerID)
	}
	return fmt.Sprintf("%s/organizations/%s/ledgers/%s/transactions/%s", base, orgID, ledgerID, transactionID)
}

// setCommonHeaders sets common headers for API requests.
func (e *transactionsEntity) setCommonHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", e.authToken))
}
