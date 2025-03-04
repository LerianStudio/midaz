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

type operation struct {
	Factory *factory.Factory
}

func (r *operation) Create(
	organizationID, ledgerID string,
	operation *mmodel.Operation,
) (*mmodel.Operation, error) {
	jsonData, err := json.Marshal(operation)
	if err != nil {
		return nil, fmt.Errorf("marshalling JSON: %v", err)
	}

	body := bytes.NewReader(jsonData)

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/operations",
		r.Factory.Env.URLAPITransaction, organizationID, ledgerID)

	req, err := http.NewRequest(http.MethodPost, uri, body)
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

	if err := checkResponse(resp, http.StatusCreated); err != nil {
		return nil, err
	}

	var operationResp mmodel.Operation
	if err := json.NewDecoder(resp.Body).Decode(&operationResp); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return &operationResp, nil
}

func (r *operation) Get(
	organizationID, ledgerID string,
	limit, page int,
	sortOrder, startDate, endDate string,
) (*mmodel.Operations, error) {
	baseURL := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/operations",
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

	var operationsResp mmodel.Operations
	if err := json.NewDecoder(resp.Body).Decode(&operationsResp); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return &operationsResp, nil
}

func (r *operation) GetByID(
	organizationID, ledgerID, operationID string) (*mmodel.Operation, error) {
	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/operations/%s",
		r.Factory.Env.URLAPITransaction, organizationID, ledgerID, operationID)

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

	var operationResp mmodel.Operation
	if err := json.NewDecoder(resp.Body).Decode(&operationResp); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return &operationResp, nil
}

func (r *operation) GetByAccount(
	organizationID, ledgerID, accountID string,
	limit, page int,
	sortOrder, startDate, endDate string,
) (*mmodel.Operations, error) {
	baseURL := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/accounts/%s/operations",
		r.Factory.Env.URLAPITransaction, organizationID, ledgerID, accountID)

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

	var operationsResp mmodel.Operations
	if err := json.NewDecoder(resp.Body).Decode(&operationsResp); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return &operationsResp, nil
}

func (r *operation) GetByAccountAndID(
	organizationID, ledgerID, accountID, operationID string) (*mmodel.Operation, error) {
	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/accounts/%s/operations/%s",
		r.Factory.Env.URLAPITransaction, organizationID, ledgerID, accountID, operationID)

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

	var operationResp mmodel.Operation
	if err := json.NewDecoder(resp.Body).Decode(&operationResp); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return &operationResp, nil
}

func (r *operation) Update(
	organizationID, ledgerID, transactionID, operationID string,
	inp mmodel.UpdateOperationInput,
) (*mmodel.Operation, error) {
	jsonData, err := json.Marshal(inp)
	if err != nil {
		return nil, fmt.Errorf("marshalling JSON: %v", err)
	}

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/transactions/%s/operations/%s",
		r.Factory.Env.URLAPITransaction, organizationID, ledgerID, transactionID, operationID)

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

	var respStr mmodel.Operation
	if err := json.NewDecoder(resp.Body).Decode(&respStr); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return &respStr, nil
}

func (r *operation) GetByTransaction(
	organizationID, ledgerID, transactionID string,
	limit, page int,
	sortOrder, startDate, endDate string,
) (*mmodel.Operations, error) {
	baseURL := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/transactions/%s/operations",
		r.Factory.Env.URLAPITransaction, organizationID, ledgerID, transactionID)

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

	var operationsResp mmodel.Operations
	if err := json.NewDecoder(resp.Body).Decode(&operationsResp); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return &operationsResp, nil
}

func (r *operation) Delete(organizationID, ledgerID, operationID string) error {
	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/operations/%s",
		r.Factory.Env.URLAPITransaction, organizationID, ledgerID, operationID)

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

func (r *operation) ListByIDs(organizationID, ledgerID string, ids []string) ([]*mmodel.Operation, error) {
	// Convert IDs to query parameters
	if len(ids) == 0 {
		return []*mmodel.Operation{}, nil
	}

	// Use comma-separated IDs in a query parameter
	idsStr := ""
	for i, id := range ids {
		if i > 0 {
			idsStr += ","
		}
		idsStr += id
	}

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/operations/list?ids=%s",
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

	var operationsResp []*mmodel.Operation
	if err := json.NewDecoder(resp.Body).Decode(&operationsResp); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return operationsResp, nil
}

// NewOperation creates a new operation REST client
func NewOperation(f *factory.Factory) *operation {
	return &operation{f}
}
