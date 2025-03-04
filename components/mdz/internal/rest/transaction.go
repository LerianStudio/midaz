package rest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/pkg/mmodel"
)

type transaction struct {
	Factory *factory.Factory
}

// extractStatusCode extracts status code from transaction object
func extractStatusCode(tx *mmodel.Transaction) {
	if tx == nil {
		return
	}

	// Extract status code if the status is an object
	if statusObj, ok := tx.Status.(map[string]interface{}); ok {
		if code, exists := statusObj["code"]; exists {
			tx.StatusCode = fmt.Sprintf("%v", code)
		}
	} else if statusStr, ok := tx.Status.(string); ok {
		// If status is already a string, copy it to StatusCode
		tx.StatusCode = statusStr
	}
}

func (r *transaction) Create(
	organizationID, ledgerID string,
	inp mmodel.CreateTransactionInput,
) (*mmodel.Transaction, error) {
	jsonData, err := json.Marshal(inp)
	if err != nil {
		return nil, fmt.Errorf("marshalling JSON: %v", err)
	}

	// Debug log the JSON being sent
	fmt.Printf("DEBUG: Sending to API: %s\n", string(jsonData))

	body := bytes.NewReader(jsonData)

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/transactions/json",
		r.Factory.Env.URLAPITransaction, organizationID, ledgerID)

	// Debug log the URI
	fmt.Printf("DEBUG: API Endpoint: %s\n", uri)

	req, err := http.NewRequest(http.MethodPost, uri, body)
	if err != nil {
		return nil, errors.New("creating request: " + err.Error())
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.Factory.Token)
	req.Header.Set("X-TTL", "3600")
	req.Header.Set("X-Idempotency-Key", inp.IdempotencyKey)

	resp, err := r.Factory.HTTPClient.Do(req)
	if err != nil {
		return nil, errors.New("making POST request: " + err.Error())
	}

	defer resp.Body.Close()

	if err := checkResponse(resp, http.StatusCreated); err != nil {
		return nil, err
	}

	var transactionRest mmodel.Transaction
	if err := json.NewDecoder(resp.Body).Decode(&transactionRest); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	// Extract status code
	extractStatusCode(&transactionRest)

	return &transactionRest, nil
}

// CreateDSL creates a new transaction using DSL syntax
func (r *transaction) CreateDSL(
	organizationID, ledgerID string,
	inp mmodel.CreateTransactionDSLInput,
) (*mmodel.Transaction, error) {
	jsonData, err := json.Marshal(inp)
	if err != nil {
		return nil, fmt.Errorf("marshalling JSON: %v", err)
	}

	body := bytes.NewReader(jsonData)

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/transactions/dsl",
		r.Factory.Env.URLAPITransaction, organizationID, ledgerID)

	req, err := http.NewRequest(http.MethodPost, uri, body)
	if err != nil {
		return nil, errors.New("creating request: " + err.Error())
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.Factory.Token)
	req.Header.Set("X-TTL", "3600")
	req.Header.Set("X-Idempotency-Key", inp.IdempotencyKey)

	resp, err := r.Factory.HTTPClient.Do(req)
	if err != nil {
		return nil, errors.New("making POST request: " + err.Error())
	}

	defer resp.Body.Close()

	if err := checkResponse(resp, http.StatusCreated); err != nil {
		return nil, err
	}

	var transactionRest mmodel.Transaction
	if err := json.NewDecoder(resp.Body).Decode(&transactionRest); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	// Extract status code
	extractStatusCode(&transactionRest)

	return &transactionRest, nil
}

func (r *transaction) Get(
	organizationID, ledgerID string,
	limit, page int,
	sortOrder, startDate, endDate string,
) (*mmodel.Transactions, error) {
	baseURL := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/transactions",
		r.Factory.Env.URLAPITransaction, organizationID, ledgerID)

	reqURL, err := BuildPaginatedURL(baseURL, limit, page, sortOrder, startDate, endDate)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, errors.New("creating request: " + err.Error())
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.Factory.Token)

	resp, err := r.Factory.HTTPClient.Do(req)
	if err != nil {
		return nil, errors.New("making GET request: " + err.Error())
	}
	defer resp.Body.Close()

	if err := checkResponse(resp, http.StatusOK); err != nil {
		return nil, err
	}

	var transactionsResp mmodel.Transactions
	if err := json.NewDecoder(resp.Body).Decode(&transactionsResp); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	// Extract status code for each transaction
	for i := range transactionsResp.Items {
		extractStatusCode(&transactionsResp.Items[i])
	}
	for i := range transactionsResp.Data {
		extractStatusCode(transactionsResp.Data[i])
	}

	return &transactionsResp, nil
}

func (r *transaction) GetByID(
	organizationID, ledgerID, transactionID string) (*mmodel.Transaction, error) {
	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/transactions/%s",
		r.Factory.Env.URLAPITransaction, organizationID, ledgerID, transactionID)

	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, errors.New("creating request: " + err.Error())
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.Factory.Token)

	resp, err := r.Factory.HTTPClient.Do(req)
	if err != nil {
		return nil, errors.New("making GET request: " + err.Error())
	}
	defer resp.Body.Close()

	if err := checkResponse(resp, http.StatusOK); err != nil {
		return nil, err
	}

	var transactionResp mmodel.Transaction
	if err := json.NewDecoder(resp.Body).Decode(&transactionResp); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	// Extract status code
	extractStatusCode(&transactionResp)

	return &transactionResp, nil
}

func (r *transaction) GetByParentID(
	organizationID, ledgerID, parentID string,
) (*mmodel.Transaction, error) {
	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/transactions/parent/%s",
		r.Factory.Env.URLAPITransaction, organizationID, ledgerID, parentID)

	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, errors.New("creating request: " + err.Error())
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.Factory.Token)

	resp, err := r.Factory.HTTPClient.Do(req)
	if err != nil {
		return nil, errors.New("making GET request: " + err.Error())
	}
	defer resp.Body.Close()

	if err := checkResponse(resp, http.StatusOK); err != nil {
		return nil, err
	}

	var transactionResp mmodel.Transaction
	if err := json.NewDecoder(resp.Body).Decode(&transactionResp); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	// Extract status code
	extractStatusCode(&transactionResp)

	return &transactionResp, nil
}

// GetByParentIDPaginated gets child transactions for a parent transaction with pagination
func (r *transaction) GetByParentIDPaginated(
	organizationID, ledgerID, parentID string,
	limit, page int,
	sortOrder, startDate, endDate string,
) (*mmodel.Transactions, error) {
	baseURL := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/transactions/parent/%s",
		r.Factory.Env.URLAPITransaction, organizationID, ledgerID, parentID)

	reqURL, err := BuildPaginatedURL(baseURL, limit, page, sortOrder, startDate, endDate)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, errors.New("creating request: " + err.Error())
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.Factory.Token)

	resp, err := r.Factory.HTTPClient.Do(req)
	if err != nil {
		return nil, errors.New("making GET request: " + err.Error())
	}
	defer resp.Body.Close()

	if err := checkResponse(resp, http.StatusOK); err != nil {
		return nil, err
	}

	var transactionsResp mmodel.Transactions
	if err := json.NewDecoder(resp.Body).Decode(&transactionsResp); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	// Extract status code for each transaction
	for i := range transactionsResp.Items {
		extractStatusCode(&transactionsResp.Items[i])
	}
	for i := range transactionsResp.Data {
		extractStatusCode(transactionsResp.Data[i])
	}

	return &transactionsResp, nil
}

func (r *transaction) Update(
	organizationID, ledgerID, transactionID string,
	inp mmodel.UpdateTransactionInput,
) (*mmodel.Transaction, error) {
	jsonData, err := json.Marshal(inp)
	if err != nil {
		return nil, fmt.Errorf("marshalling JSON: %v", err)
	}

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/transactions/%s",
		r.Factory.Env.URLAPITransaction, organizationID, ledgerID, transactionID)

	req, err := http.NewRequest(http.MethodPatch, uri, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, errors.New("creating request: " + err.Error())
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.Factory.Token)

	resp, err := r.Factory.HTTPClient.Do(req)
	if err != nil {
		return nil, errors.New("making PATCH request: " + err.Error())
	}
	defer resp.Body.Close()

	if err := checkResponse(resp, http.StatusOK); err != nil {
		return nil, err
	}

	var respStr mmodel.Transaction
	if err := json.NewDecoder(resp.Body).Decode(&respStr); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	// Extract status code
	extractStatusCode(&respStr)

	return &respStr, nil
}

func (r *transaction) Delete(organizationID, ledgerID, transactionID string) error {
	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/transactions/%s",
		r.Factory.Env.URLAPITransaction, organizationID, ledgerID, transactionID)

	req, err := http.NewRequest(http.MethodDelete, uri, nil)
	if err != nil {
		return errors.New("creating request: " + err.Error())
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.Factory.Token)

	resp, err := r.Factory.HTTPClient.Do(req)
	if err != nil {
		return errors.New("making DELETE request: " + err.Error())
	}

	defer resp.Body.Close()

	if err := checkResponse(resp, http.StatusNoContent); err != nil {
		return err
	}

	return nil
}

// Commit marks a transaction as committed
func (r *transaction) Commit(organizationID, ledgerID, transactionID string) (*mmodel.Transaction, error) {
	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/transactions/%s/commit",
		r.Factory.Env.URLAPITransaction, organizationID, ledgerID, transactionID)

	req, err := http.NewRequest(http.MethodPost, uri, nil)
	if err != nil {
		return nil, errors.New("creating request: " + err.Error())
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.Factory.Token)

	resp, err := r.Factory.HTTPClient.Do(req)
	if err != nil {
		return nil, errors.New("making POST request: " + err.Error())
	}
	defer resp.Body.Close()

	if err := checkResponse(resp, http.StatusOK); err != nil {
		return nil, err
	}

	var transactionResp mmodel.Transaction
	if err := json.NewDecoder(resp.Body).Decode(&transactionResp); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	// Extract status code
	extractStatusCode(&transactionResp)

	return &transactionResp, nil
}

// Revert marks a transaction as reverted
func (r *transaction) Revert(organizationID, ledgerID, transactionID string) (*mmodel.Transaction, error) {
	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/transactions/%s/revert",
		r.Factory.Env.URLAPITransaction, organizationID, ledgerID, transactionID)

	req, err := http.NewRequest(http.MethodPost, uri, nil)
	if err != nil {
		return nil, errors.New("creating request: " + err.Error())
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.Factory.Token)

	resp, err := r.Factory.HTTPClient.Do(req)
	if err != nil {
		return nil, errors.New("making POST request: " + err.Error())
	}
	defer resp.Body.Close()

	if err := checkResponse(resp, http.StatusOK); err != nil {
		return nil, err
	}

	var transactionResp mmodel.Transaction
	if err := json.NewDecoder(resp.Body).Decode(&transactionResp); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	// Extract status code
	extractStatusCode(&transactionResp)

	return &transactionResp, nil
}

func (r *transaction) ListByIDs(organizationID, ledgerID string, ids []string) ([]*mmodel.Transaction, error) {
	// Convert IDs to query parameters
	if len(ids) == 0 {
		return []*mmodel.Transaction{}, nil
	}

	// Use comma-separated IDs in a query parameter
	idsStr := ""
	for i, id := range ids {
		if i > 0 {
			idsStr += ","
		}
		idsStr += id
	}

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/transactions/list?ids=%s",
		r.Factory.Env.URLAPITransaction, organizationID, ledgerID, idsStr)

	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, errors.New("creating request: " + err.Error())
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.Factory.Token)

	resp, err := r.Factory.HTTPClient.Do(req)
	if err != nil {
		return nil, errors.New("making GET request: " + err.Error())
	}
	defer resp.Body.Close()

	if err := checkResponse(resp, http.StatusOK); err != nil {
		return nil, err
	}

	var transactionsResp []*mmodel.Transaction
	if err := json.NewDecoder(resp.Body).Decode(&transactionsResp); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	// Extract status code for each transaction
	for _, tx := range transactionsResp {
		extractStatusCode(tx)
	}

	return transactionsResp, nil
}

// NewTransaction creates a new transaction REST client
func NewTransaction(f *factory.Factory) *transaction {
	return &transaction{f}
}
