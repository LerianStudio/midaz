package entities

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	libTransaction "github.com/LerianStudio/lib-commons/commons/transaction"
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
// It takes the HTTP client, auth token, and base URLs which are used to make HTTP requests to the Midaz API.
// Returns an implementation of the TransactionsService interface.
func NewTransactionsEntity(httpClient *http.Client, authToken string, baseURLs map[string]string) TransactionsService {
	return &transactionsEntity{
		httpClient: httpClient,
		authToken:  authToken,
		baseURLs:   baseURLs,
	}
}

// CreateTransaction creates a new transaction using the standard format.
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

	// Handle error responses
	if resp.StatusCode >= 400 {
		var errResp struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
			return nil, fmt.Errorf("API error (status %d): failed to decode error response", resp.StatusCode)
		}
		return nil, fmt.Errorf("API error: %s - %s", errResp.Error, errResp.Message)
	}

	var transaction models.Transaction

	if err := json.NewDecoder(resp.Body).Decode(&transaction); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &transaction, nil
}

// CreateTransactionWithDSL creates a new transaction using the DSL format.
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

	// Handle error responses
	if resp.StatusCode >= 400 {
		var errResp struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
			return nil, fmt.Errorf("API error (status %d): failed to decode error response", resp.StatusCode)
		}
		return nil, fmt.Errorf("API error: %s - %s", errResp.Error, errResp.Message)
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

	// Handle error responses
	if resp.StatusCode >= 400 {
		var errResp struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
			return nil, fmt.Errorf("API error (status %d): failed to decode error response", resp.StatusCode)
		}
		return nil, fmt.Errorf("API error: %s - %s", errResp.Error, errResp.Message)
	}

	var transaction models.Transaction

	if err := json.NewDecoder(resp.Body).Decode(&transaction); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &transaction, nil
}

// GetTransaction gets a transaction by ID.
func (e *transactionsEntity) GetTransaction(ctx context.Context, orgID, ledgerID, transactionID string) (*models.Transaction, error) {
	if transactionID == "" {
		return nil, fmt.Errorf("transaction ID is required")
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

	// Handle error responses
	if resp.StatusCode >= 400 {
		var errResp struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
			return nil, fmt.Errorf("API error (status %d): failed to decode error response", resp.StatusCode)
		}
		return nil, fmt.Errorf("API error: %s - %s", errResp.Error, errResp.Message)
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

// ListTransactions lists transactions for a ledger with optional filters.
func (e *transactionsEntity) ListTransactions(ctx context.Context, orgID, ledgerID string, opts *models.ListOptions) (*models.ListResponse[models.Transaction], error) {
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

	// Handle error responses
	if resp.StatusCode >= 400 {
		var errResp struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
			return nil, fmt.Errorf("API error (status %d): failed to decode error response", resp.StatusCode)
		}
		return nil, fmt.Errorf("API error: %s - %s", errResp.Error, errResp.Message)
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

	// Handle error responses
	if resp.StatusCode >= 400 {
		var errResp struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
			return nil, fmt.Errorf("API error (status %d): failed to decode error response", resp.StatusCode)
		}
		return nil, fmt.Errorf("API error: %s - %s", errResp.Error, errResp.Message)
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

// CommitTransaction commits a transaction.
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

	// Handle error responses
	if resp.StatusCode >= 400 {
		var errResp struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
			return nil, fmt.Errorf("API error (status %d): failed to decode error response", resp.StatusCode)
		}
		return nil, fmt.Errorf("API error: %s - %s", errResp.Error, errResp.Message)
	}

	var transaction models.Transaction

	if err := json.NewDecoder(resp.Body).Decode(&transaction); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &transaction, nil
}

// CommitTransactionWithExternalID commits a transaction using an external ID.
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
