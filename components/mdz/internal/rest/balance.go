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

type balance struct {
	Factory *factory.Factory
}

// Create creates a new balance
func (r *balance) Create(
	organizationID, ledgerID, accountID string,
	inp mmodel.CreateBalanceInput,
) (*mmodel.Balance, error) {
	jsonData, err := json.Marshal(inp)
	if err != nil {
		return nil, fmt.Errorf("marshalling JSON: %v", err)
	}

	body := bytes.NewReader(jsonData)

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/accounts/%s/balances",
		r.Factory.Env.URLAPITransaction, organizationID, ledgerID, accountID)

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

	var balanceResp mmodel.Balance
	if err := json.NewDecoder(resp.Body).Decode(&balanceResp); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return &balanceResp, nil
}

func (r *balance) Get(
	organizationID, ledgerID string,
	limit, page int,
	sortOrder, startDate, endDate string,
) (*mmodel.Balances, error) {
	baseURL := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/balances",
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

	var balancesResp mmodel.Balances
	if err := json.NewDecoder(resp.Body).Decode(&balancesResp); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return &balancesResp, nil
}

func (r *balance) GetByID(
	organizationID, ledgerID, balanceID string) (*mmodel.Balance, error) {
	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/balances/%s",
		r.Factory.Env.URLAPITransaction, organizationID, ledgerID, balanceID)

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

	var balanceResp mmodel.Balance
	if err := json.NewDecoder(resp.Body).Decode(&balanceResp); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return &balanceResp, nil
}

func (r *balance) GetByAccount(
	organizationID, ledgerID, accountID string,
	limit, page int,
	sortOrder, startDate, endDate string,
) (*mmodel.Balances, error) {
	baseURL := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/accounts/%s/balances",
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

	var balancesResp mmodel.Balances
	if err := json.NewDecoder(resp.Body).Decode(&balancesResp); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return &balancesResp, nil
}

func (r *balance) Update(
	organizationID, ledgerID, balanceID string,
	inp mmodel.UpdateBalance,
) (*mmodel.Balance, error) {
	jsonData, err := json.Marshal(inp)
	if err != nil {
		return nil, fmt.Errorf("marshalling JSON: %v", err)
	}

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/balances/%s",
		r.Factory.Env.URLAPITransaction, organizationID, ledgerID, balanceID)

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

	var respStr mmodel.Balance
	if err := json.NewDecoder(resp.Body).Decode(&respStr); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return &respStr, nil
}

func (r *balance) Delete(organizationID, ledgerID, balanceID string) error {
	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/balances/%s",
		r.Factory.Env.URLAPITransaction, organizationID, ledgerID, balanceID)

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

func (r *balance) ListByAccountIDs(organizationID, ledgerID string, accountIDs []string) ([]*mmodel.Balance, error) {
	// Convert account IDs to query parameters
	if len(accountIDs) == 0 {
		return []*mmodel.Balance{}, nil
	}

	// Use comma-separated account IDs in a query parameter
	accountIDsStr := ""
	for i, id := range accountIDs {
		if i > 0 {
			accountIDsStr += ","
		}
		accountIDsStr += id
	}

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/balances/accounts?accountIds=%s",
		r.Factory.Env.URLAPITransaction, organizationID, ledgerID, accountIDsStr)

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

	var balancesResp []*mmodel.Balance
	if err := json.NewDecoder(resp.Body).Decode(&balancesResp); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return balancesResp, nil
}

func (r *balance) ListByAliases(organizationID, ledgerID string, aliases []string) ([]*mmodel.Balance, error) {
	// Convert aliases to query parameters
	if len(aliases) == 0 {
		return []*mmodel.Balance{}, nil
	}

	// Use comma-separated aliases in a query parameter
	aliasesStr := ""
	for i, alias := range aliases {
		if i > 0 {
			aliasesStr += ","
		}
		aliasesStr += alias
	}

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/balances/aliases?aliases=%s",
		r.Factory.Env.URLAPITransaction, organizationID, ledgerID, aliasesStr)

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

	var balancesResp []*mmodel.Balance
	if err := json.NewDecoder(resp.Body).Decode(&balancesResp); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return balancesResp, nil
}

// NewBalance creates a new balance REST client
func NewBalance(f *factory.Factory) *balance {
	return &balance{f}
}
