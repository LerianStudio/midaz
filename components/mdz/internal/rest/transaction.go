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

func (r *transaction) Create(
	organizationID, ledgerID string,
	inp mmodel.CreateTransactionInput,
) (*mmodel.Transaction, error) {
	jsonData, err := json.Marshal(inp)
	if err != nil {
		return nil, fmt.Errorf("marshalling JSON: %v", err)
	}

	body := bytes.NewReader(jsonData)

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/transactions",
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

	var transactionRest mmodel.Transaction
	if err := json.NewDecoder(resp.Body).Decode(&transactionRest); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return &transactionRest, nil
}

func (r *transaction) CreateDSL(
	organizationID, ledgerID string,
	dslContent string,
) (*mmodel.Transaction, error) {
	body := bytes.NewReader([]byte(dslContent))

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/transactions/dsl",
		r.Factory.Env.URLAPITransaction, organizationID, ledgerID)

	req, err := http.NewRequest(http.MethodPost, uri, body)
	if err != nil {
		return nil, errors.New("creating request: " + err.Error())
	}

	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Authorization", "Bearer "+r.Factory.Token)

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

	return &transactionResp, nil
}

func (r *transaction) Revert(
	organizationID, ledgerID, transactionID string) (*mmodel.Transaction, error) {
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

	return &transactionResp, nil
}

func NewTransaction(f *factory.Factory) *transaction {
	return &transaction{f}
}
