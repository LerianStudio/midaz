package rest

import (
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

func NewOperation(f *factory.Factory) *operation {
	return &operation{f}
}
