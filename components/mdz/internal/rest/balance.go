package rest

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/pkg/mmodel"
)

type balance struct {
	Factory *factory.Factory
}

func (r *balance) Get(
	organizationID, ledgerID string,
	limit int,
	cursor, sortOrder, startDate, endDate string,
) (*mmodel.Balances, error) {
	baseURL := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/balances",
		r.Factory.Env.URLAPITransaction, organizationID, ledgerID)

	// Build URL with query parameters
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, errors.New("parsing URL: " + err.Error())
	}

	q := u.Query()
	q.Set("limit", strconv.Itoa(limit))

	if cursor != "" {
		q.Set("cursor", cursor)
	}

	if sortOrder != "" {
		q.Set("sort_order", sortOrder)
	}

	if startDate != "" {
		q.Set("start_date", startDate)
	}

	if endDate != "" {
		q.Set("end_date", endDate)
	}

	u.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
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
	limit int,
	cursor, sortOrder, startDate, endDate string,
) (*mmodel.Balances, error) {
	baseURL := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/accounts/%s/balances",
		r.Factory.Env.URLAPITransaction, organizationID, ledgerID, accountID)

	// Build URL with query parameters
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, errors.New("parsing URL: " + err.Error())
	}

	q := u.Query()
	q.Set("limit", strconv.Itoa(limit))

	if cursor != "" {
		q.Set("cursor", cursor)
	}

	if sortOrder != "" {
		q.Set("sort_order", sortOrder)
	}

	if startDate != "" {
		q.Set("start_date", startDate)
	}

	if endDate != "" {
		q.Set("end_date", endDate)
	}

	u.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
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

func NewBalance(f *factory.Factory) *balance {
	return &balance{f}
}
