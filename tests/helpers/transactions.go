package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

const (
	transactionHTTPStatusOK = 200
	// Operation types as defined in lib-commons/v2/commons/constants
	operationTypeCredit = "CREDIT"
	operationTypeDebit  = "DEBIT"
	// External account prefix for inflow/outflow detection
	externalAccountPrefix = "@external/"
)

var (
	// ErrFetchTransactionsFailed is returned when fetching transactions fails with a non-OK status
	ErrFetchTransactionsFailed = errors.New("fetch transactions failed")
	// ErrBalanceInconsistency is returned when actual balance doesn't match expected from history
	ErrBalanceInconsistency = errors.New("balance inconsistency detected")
)

// TransactionOperation represents an operation within a transaction
type TransactionOperation struct {
	ID           string          `json:"id"`
	Type         string          `json:"type"`         // DEBIT or CREDIT
	AccountAlias string          `json:"accountAlias"` // Account alias involved
	AssetCode    string          `json:"assetCode"`    // Asset code
	Amount       OperationAmount `json:"amount"`       // Amount details
	BalanceAfter OperationBal    `json:"balanceAfter"` // Balance after operation
	Status       OperationStatus `json:"status"`       // Operation status
}

// OperationAmount represents the amount in an operation
type OperationAmount struct {
	Value *decimal.Decimal `json:"value"`
}

// OperationBal represents balance info in an operation
type OperationBal struct {
	Available *decimal.Decimal `json:"available"`
}

// OperationStatus represents the status of an operation
type OperationStatus struct {
	Code string `json:"code"`
}

// TransactionRecord represents a transaction from the API
type TransactionRecord struct {
	ID          string                 `json:"id"`
	Status      TransactionStatus      `json:"status"`
	Amount      *decimal.Decimal       `json:"amount"`
	AssetCode   string                 `json:"assetCode"`
	Source      []string               `json:"source"`      // Source account aliases
	Destination []string               `json:"destination"` // Destination account aliases
	Operations  []TransactionOperation `json:"operations"`
	CreatedAt   string                 `json:"createdAt"`
}

// TransactionStatus represents the status of a transaction
type TransactionStatus struct {
	Code string `json:"code"`
}

// TransactionListResponse represents the paginated API response for transactions
type TransactionListResponse struct {
	Items      []TransactionRecord `json:"items"`
	NextCursor *string             `json:"next_cursor"`
	PrevCursor *string             `json:"prev_cursor"`
	Limit      int                 `json:"limit"`
}

// BalanceImpact represents the calculated balance impact from a transaction for an account
type BalanceImpact struct {
	TransactionID string
	AccountAlias  string
	Impact        decimal.Decimal // Positive = credit to account, Negative = debit from account
	Type          string          // "inflow", "outflow", "transfer_in", "transfer_out"
}

// TransactionHistorySummary provides summary info about fetched transaction history
type TransactionHistorySummary struct {
	TotalTransactions int
	TotalOperations   int
	InflowCount       int
	OutflowCount      int
	TransferInCount   int
	TransferOutCount  int
	NetImpact         decimal.Decimal
}

// transactionPageResult holds the result of fetching a single page of transactions
type transactionPageResult struct {
	items      []TransactionRecord
	nextCursor string
	err        error
}

// fetchTransactionPage fetches a single page of transactions
func fetchTransactionPage(ctx context.Context, trans *HTTPClient, path string, headers map[string]string) transactionPageResult {
	code, body, err := trans.Request(ctx, "GET", path, headers, nil)
	if err != nil {
		return transactionPageResult{err: fmt.Errorf("failed to fetch transactions: %w", err)}
	}

	if code != transactionHTTPStatusOK {
		return transactionPageResult{err: fmt.Errorf("%w: status %d: %s", ErrFetchTransactionsFailed, code, string(body))}
	}

	var response TransactionListResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return transactionPageResult{err: fmt.Errorf("failed to unmarshal transactions response: %w", err)}
	}

	nextCursor := ""
	if response.NextCursor != nil {
		nextCursor = *response.NextCursor
	}

	return transactionPageResult{items: response.Items, nextCursor: nextCursor}
}

// FetchAllTransactionsForLedger fetches all transactions for a ledger with pagination handling
// NOTE: The API defaults to returning only transactions from the last month when no date range
// is specified. We explicitly set start_date to epoch to fetch ALL transactions.
func FetchAllTransactionsForLedger(ctx context.Context, trans *HTTPClient, orgID, ledgerID string, headers map[string]string) ([]TransactionRecord, error) {
	var allTransactions []TransactionRecord

	cursor := ""
	startDate := "1970-01-01"
	endDate := time.Now().Format("2006-01-02")

	for {
		path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions?limit=100&start_date=%s&end_date=%s",
			orgID, ledgerID, startDate, endDate)
		if cursor != "" {
			path += "&cursor=" + cursor
		}

		result := fetchTransactionPage(ctx, trans, path, headers)
		if result.err != nil {
			return nil, result.err
		}

		if len(allTransactions) == 0 && len(result.items) == 0 && cursor == "" {
			fmt.Printf("[DEBUG] FetchAllTransactionsForLedger: First page returned 0 items for ledger=%s (path=%s)\n", ledgerID, path)
		}

		allTransactions = append(allTransactions, result.items...)

		if result.nextCursor == "" {
			break
		}

		cursor = result.nextCursor
	}

	return allTransactions, nil
}

// FetchTransactionByID fetches a single transaction with its operations
func FetchTransactionByID(ctx context.Context, trans *HTTPClient, orgID, ledgerID, txnID string, headers map[string]string) (*TransactionRecord, error) {
	path := fmt.Sprintf("/v1/organizations/%s/ledgers/%s/transactions/%s", orgID, ledgerID, txnID)

	code, body, err := trans.Request(ctx, "GET", path, headers, nil)
	if err != nil {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return nil, fmt.Errorf("failed to fetch transaction %s: %w", txnID, err)
	}

	if code != transactionHTTPStatusOK {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return nil, fmt.Errorf("%w: transaction %s status %d: %s", ErrFetchTransactionsFailed, txnID, code, string(body))
	}

	var txn TransactionRecord
	if err := json.Unmarshal(body, &txn); err != nil {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return nil, fmt.Errorf("failed to unmarshal transaction %s: %w", txnID, err)
	}

	return &txn, nil
}

// FilterTransactionsByAccount filters transactions that involve the given account alias
func FilterTransactionsByAccount(transactions []TransactionRecord, accountAlias string) []TransactionRecord {
	var filtered []TransactionRecord

	for _, txn := range transactions {
		if transactionInvolvesAccount(txn, accountAlias) {
			filtered = append(filtered, txn)
		}
	}

	return filtered
}

// transactionInvolvesAccount checks if a transaction involves the given account
func transactionInvolvesAccount(txn TransactionRecord, accountAlias string) bool {
	// Check source list
	for _, src := range txn.Source {
		if aliasMatches(src, accountAlias) {
			return true
		}
	}
	// Check destination list
	for _, dst := range txn.Destination {
		if aliasMatches(dst, accountAlias) {
			return true
		}
	}
	// Also check operations
	for _, op := range txn.Operations {
		if aliasMatches(op.AccountAlias, accountAlias) {
			return true
		}
	}

	return false
}

// aliasMatches checks if two account aliases match (handles variations like with/without key suffix)
func aliasMatches(alias1, alias2 string) bool {
	// Direct match
	if alias1 == alias2 {
		return true
	}
	// Strip any key suffix (format: alias#key)
	base1 := strings.Split(alias1, "#")[0]
	base2 := strings.Split(alias2, "#")[0]

	return base1 == base2
}

// CalculateBalanceImpactFromOperations calculates the balance impact for an account from transaction operations
// CREDIT operations mean money flowing INTO the account (positive impact)
// DEBIT operations mean money flowing OUT of the account (negative impact)
func CalculateBalanceImpactFromOperations(txn TransactionRecord, accountAlias string, assetCode string) decimal.Decimal {
	impact := decimal.Zero

	for _, op := range txn.Operations {
		// Skip if not the right asset
		if op.AssetCode != assetCode {
			continue
		}

		// Skip if not the account we're tracking
		if !aliasMatches(op.AccountAlias, accountAlias) {
			continue
		}

		// Skip if no amount
		if op.Amount.Value == nil {
			continue
		}

		// CREDIT = money in (positive), DEBIT = money out (negative)
		switch op.Type {
		case operationTypeCredit:
			impact = impact.Add(*op.Amount.Value)
		case operationTypeDebit:
			impact = impact.Sub(*op.Amount.Value)
		}
	}

	return impact
}

// CalculateExpectedBalanceFromHistory calculates the expected balance by summing all transaction impacts
func CalculateExpectedBalanceFromHistory(seedBalance decimal.Decimal, transactions []TransactionRecord, accountAlias, assetCode string) decimal.Decimal {
	balance := seedBalance

	for _, txn := range transactions {
		// Only consider approved/created transactions (not pending or canceled)
		if txn.Status.Code != "APPROVED" && txn.Status.Code != "CREATED" {
			continue
		}

		impact := CalculateBalanceImpactFromOperations(txn, accountAlias, assetCode)
		balance = balance.Add(impact)
	}

	return balance
}

// processOperationForSummary processes a single operation and updates the summary
func processOperationForSummary(summary *TransactionHistorySummary, op TransactionOperation, txn TransactionRecord) {
	if op.Amount.Value == nil {
		return
	}

	isFromExternal := hasExternalSource(txn)
	isToExternal := hasExternalDestination(txn)

	switch op.Type {
	case operationTypeCredit:
		summary.NetImpact = summary.NetImpact.Add(*op.Amount.Value)
		if isFromExternal {
			summary.InflowCount++
		} else {
			summary.TransferInCount++
		}
	case operationTypeDebit:
		summary.NetImpact = summary.NetImpact.Sub(*op.Amount.Value)
		if isToExternal {
			summary.OutflowCount++
		} else {
			summary.TransferOutCount++
		}
	}
}

// isValidTransactionStatus checks if transaction has a valid status for processing
func isValidTransactionStatus(code string) bool {
	return code == "APPROVED" || code == "CREATED"
}

// GetTransactionHistorySummary provides a summary of transaction history for an account
func GetTransactionHistorySummary(transactions []TransactionRecord, accountAlias, assetCode string) TransactionHistorySummary {
	summary := TransactionHistorySummary{}

	for _, txn := range transactions {
		if !isValidTransactionStatus(txn.Status.Code) {
			continue
		}

		summary.TotalTransactions++

		for _, op := range txn.Operations {
			if op.AssetCode != assetCode || !aliasMatches(op.AccountAlias, accountAlias) {
				continue
			}

			summary.TotalOperations++
			processOperationForSummary(&summary, op, txn)
		}
	}

	return summary
}

// hasExternalSource checks if transaction has external source (inflow)
func hasExternalSource(txn TransactionRecord) bool {
	for _, src := range txn.Source {
		if strings.HasPrefix(src, externalAccountPrefix) {
			return true
		}
	}

	return false
}

// hasExternalDestination checks if transaction has external destination (outflow)
func hasExternalDestination(txn TransactionRecord) bool {
	for _, dst := range txn.Destination {
		if strings.HasPrefix(dst, externalAccountPrefix) {
			return true
		}
	}

	return false
}

// VerifyBalanceConsistency verifies that actual balance matches calculated balance from transaction history
// Returns nil if consistent, error with details if not
func VerifyBalanceConsistency(ctx context.Context, trans *HTTPClient, orgID, ledgerID, accountAlias, assetCode string, seedBalance decimal.Decimal, headers map[string]string) error {
	// 1. Fetch actual balance
	actualBalance, err := GetAvailableSumByAlias(ctx, trans, orgID, ledgerID, accountAlias, assetCode, headers)
	if err != nil {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return fmt.Errorf("failed to get actual balance for %s: %w", accountAlias, err)
	}

	// 2. Fetch all transactions for the ledger
	allTransactions, err := FetchAllTransactionsForLedger(ctx, trans, orgID, ledgerID, headers)
	if err != nil {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return fmt.Errorf("failed to fetch transactions: %w", err)
	}

	// 3. Filter to transactions involving this account
	accountTransactions := FilterTransactionsByAccount(allTransactions, accountAlias)

	// 4. Calculate expected balance from transaction history
	expectedBalance := CalculateExpectedBalanceFromHistory(seedBalance, accountTransactions, accountAlias, assetCode)

	// 5. Get summary for logging
	summary := GetTransactionHistorySummary(accountTransactions, accountAlias, assetCode)

	// 6. Compare
	if !actualBalance.Equal(expectedBalance) {
		//nolint:wrapcheck // Error already wrapped with context for test helpers
		return fmt.Errorf("%w: account=%s actual=%s expected_from_history=%s (diff=%s, txn_count=%d, inflows=%d, outflows=%d, transfers_in=%d, transfers_out=%d, net_impact=%s)",
			ErrBalanceInconsistency,
			accountAlias,
			actualBalance.String(),
			expectedBalance.String(),
			actualBalance.Sub(expectedBalance).String(),
			summary.TotalTransactions,
			summary.InflowCount,
			summary.OutflowCount,
			summary.TransferInCount,
			summary.TransferOutCount,
			summary.NetImpact.String())
	}

	return nil
}

// VerifyBalanceConsistencyWithInfo verifies balance and returns detailed info for logging
func VerifyBalanceConsistencyWithInfo(ctx context.Context, trans *HTTPClient, orgID, ledgerID, accountAlias, assetCode string, seedBalance decimal.Decimal, headers map[string]string) (actualBalance, expectedBalance decimal.Decimal, summary TransactionHistorySummary, err error) {
	// 1. Fetch actual balance
	actualBalance, err = GetAvailableSumByAlias(ctx, trans, orgID, ledgerID, accountAlias, assetCode, headers)
	if err != nil {
		err = fmt.Errorf("failed to get actual balance for %s: %w", accountAlias, err)
		return
	}

	// 2. Fetch all transactions for the ledger
	allTransactions, fetchErr := FetchAllTransactionsForLedger(ctx, trans, orgID, ledgerID, headers)
	if fetchErr != nil {
		err = fmt.Errorf("failed to fetch transactions: %w", fetchErr)
		return
	}

	// 3. Filter to transactions involving this account
	accountTransactions := FilterTransactionsByAccount(allTransactions, accountAlias)

	// 4. Calculate expected balance from transaction history
	expectedBalance = CalculateExpectedBalanceFromHistory(seedBalance, accountTransactions, accountAlias, assetCode)

	// 5. Get summary for logging
	summary = GetTransactionHistorySummary(accountTransactions, accountAlias, assetCode)

	return
}

// CompareHTTPCountsWithActual compares HTTP 201 counts with actual committed transactions
// Returns ghost transaction count and details
// Ghost transactions = committed but no HTTP 201 recorded (actual > HTTP)
// Missing transactions = HTTP 201 recorded but not committed (HTTP > actual)
func CompareHTTPCountsWithActual(httpCounts map[string]int, summary TransactionHistorySummary) (ghostCount int, details string) {
	// Calculate expected from HTTP counts (what we thought succeeded)
	httpInflowCount := httpCounts["inflow"]
	httpOutflowCount := httpCounts["outflow"]
	httpTransferCount := httpCounts["transfer"]

	// Calculate actual from transaction history
	actualInflowCount := summary.InflowCount
	actualOutflowCount := summary.OutflowCount
	actualTransferIn := summary.TransferInCount
	actualTransferOut := summary.TransferOutCount

	// Ghost transactions = actual - HTTP counted (only when actual > HTTP)
	// These are transactions that committed but we didn't record an HTTP 201
	ghostInflows := max(0, actualInflowCount-httpInflowCount)
	ghostOutflows := max(0, actualOutflowCount-httpOutflowCount)
	ghostTransfersIn := max(0, actualTransferIn-httpTransferCount)
	ghostTransfersOut := max(0, actualTransferOut-httpTransferCount)

	ghostCount = ghostInflows + ghostOutflows + ghostTransfersIn + ghostTransfersOut

	// Missing transactions = HTTP counted - actual (only when HTTP > actual)
	// These are transactions we got HTTP 201 for but didn't actually commit
	missingInflows := max(0, httpInflowCount-actualInflowCount)
	missingOutflows := max(0, httpOutflowCount-actualOutflowCount)
	missingTransfersIn := max(0, httpTransferCount-actualTransferIn)
	missingTransfersOut := max(0, httpTransferCount-actualTransferOut)

	missingCount := missingInflows + missingOutflows + missingTransfersIn + missingTransfersOut

	details = fmt.Sprintf("HTTP_201_counts: inflows=%d outflows=%d transfers=%d | Actual_committed: inflows=%d outflows=%d transfers_in=%d transfers_out=%d | Ghost_transactions: %d | Missing_transactions: %d",
		httpInflowCount, httpOutflowCount, httpTransferCount,
		actualInflowCount, actualOutflowCount, actualTransferIn, actualTransferOut,
		ghostCount, missingCount)

	return ghostCount, details
}
