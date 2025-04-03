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

type account struct {
	Factory *factory.Factory
}

// func (r *account) Create( performs an operation
func (r *account) Create(
	organizationID, ledgerID string,
	inp mmodel.CreateAccountInput,
) (*mmodel.Account, error) {
	jsonData, err := json.Marshal(inp)

	if err != nil {
		return nil, fmt.Errorf("marshalling JSON: %v", err)
	}

	body := bytes.NewReader(jsonData)

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/accounts",
		r.Factory.Env.URLAPILedger, organizationID, ledgerID)

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

	var accountRest mmodel.Account

	if err := json.NewDecoder(resp.Body).Decode(&accountRest); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return &accountRest, nil
}

// func (r *account) Get( performs an operation
func (r *account) Get(
	organizationID, ledgerID string,
	limit, page int,
	sortOrder, startDate, endDate string,
) (*mmodel.Accounts, error) {
	baseURL := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/accounts",
		r.Factory.Env.URLAPILedger, organizationID, ledgerID)

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
		return nil, errors.New("making POST request: " + err.Error())
	}

	defer resp.Body.Close()

	if err := checkResponse(resp, http.StatusOK); err != nil {
		return nil, err
	}

	var accountsResp mmodel.Accounts

	if err := json.NewDecoder(resp.Body).Decode(&accountsResp); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return &accountsResp, nil
}

// func (r *account) GetByID( performs an operation
func (r *account) GetByID(
	organizationID, ledgerID, accountID string) (*mmodel.Account, error) {
	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/accounts/%s",
		r.Factory.Env.URLAPILedger, organizationID, ledgerID, accountID)

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

	var accountResp mmodel.Account

	if err := json.NewDecoder(resp.Body).Decode(&accountResp); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return &accountResp, nil
}

// func (r *account) Update( performs an operation
func (r *account) Update(
	organizationID, ledgerID, accountID string,
	inp mmodel.UpdateAccountInput,
) (*mmodel.Account, error) {
	jsonData, err := json.Marshal(inp)

	if err != nil {
		return nil, fmt.Errorf("marshalling JSON: %v", err)
	}

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/accounts/%s",
		r.Factory.Env.URLAPILedger, organizationID, ledgerID, accountID)

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

	var respStr mmodel.Account

	if err := json.NewDecoder(resp.Body).Decode(&respStr); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return &respStr, nil
}

// func (r *account) Delete(organizationID, ledgerID, accountID string) error { performs an operation
func (r *account) Delete(organizationID, ledgerID, accountID string) error {
	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/accounts/%s",
		r.Factory.Env.URLAPILedger, organizationID, ledgerID, accountID)

	req, err := http.NewRequest(http.MethodDelete, uri, nil)

	if err != nil {
		return errors.New("creating request: " + err.Error())
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.Factory.Token)

	resp, err := r.Factory.HTTPClient.Do(req)

	if err != nil {
		return errors.New("making GET request: " + err.Error())
	}

	defer resp.Body.Close()

	if err := checkResponse(resp, http.StatusNoContent); err != nil {
		return err
	}

	return nil
}

// \1 performs an operation
func NewAccount(f *factory.Factory) *account {
	return &account{f}
}
