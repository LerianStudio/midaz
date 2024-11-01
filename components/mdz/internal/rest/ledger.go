package rest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/LerianStudio/midaz/components/mdz/internal/model"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
)

type ledger struct {
	Factory *factory.Factory
}

func (r *ledger) Create(organizationID string, inp model.LedgerInput) (*model.LedgerCreate, error) {
	jsonData, err := json.Marshal(inp)
	if err != nil {
		return nil, fmt.Errorf("marshalling JSON: %v", err)
	}

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers", r.Factory.Env.URLAPILedger, organizationID)

	req, err := http.NewRequest(http.MethodPost, uri, bytes.NewBuffer(jsonData))
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

	if resp.StatusCode != http.StatusCreated {
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, errors.New("unauthorized invalid credentials")
		}

		return nil, fmt.Errorf("failed to create organization, status code: %d",
			resp.StatusCode)
	}

	var ledResp model.LedgerCreate
	if err := json.NewDecoder(resp.Body).Decode(&ledResp); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return &ledResp, nil
}

func (r *ledger) Get(organizationID string, limit, page int) (*model.LedgerList, error) {
	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers?limit=%d&page=%d",
		r.Factory.Env.URLAPILedger, organizationID, limit, page)

	req, err := http.NewRequest(http.MethodGet, uri, nil)
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

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, errors.New("unauthorized invalid credentials")
		}

		return nil, fmt.Errorf("failed to create organization, status code: %d",
			resp.StatusCode)
	}

	var ledResp model.LedgerList
	if err := json.NewDecoder(resp.Body).Decode(&ledResp); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return &ledResp, nil
}

func (r *ledger) GetByID(organizationID, ledgerID string) (*model.LedgerItems, error) {
	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s",
		r.Factory.Env.URLAPILedger, organizationID, ledgerID)

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

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, errors.New("unauthorized invalid credentials")
		}

		return nil, fmt.Errorf("failed to get organization, status code: %d",
			resp.StatusCode)
	}

	var ledItemResp model.LedgerItems
	if err := json.NewDecoder(resp.Body).Decode(&ledItemResp); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return &ledItemResp, nil
}

func NewLedger(f *factory.Factory) *ledger {
	return &ledger{f}
}
