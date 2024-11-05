package rest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/LerianStudio/midaz/common/mmodel"
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

	if err := checkResponse(resp, http.StatusCreated); err != nil {
		return nil, err
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

	if err := checkResponse(resp, http.StatusOK); err != nil {
		return nil, err
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

	if err := checkResponse(resp, http.StatusOK); err != nil {
		return nil, err
	}

	var ledItemResp model.LedgerItems
	if err := json.NewDecoder(resp.Body).Decode(&ledItemResp); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return &ledItemResp, nil
}

func (r *ledger) Update(organizationID, ledgerID string, inp mmodel.UpdateLedgerInput) (*mmodel.Ledger, error) {
	jsonData, err := json.Marshal(inp)
	if err != nil {
		return nil, fmt.Errorf("marshalling JSON: %v", err)
	}

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s",
		r.Factory.Env.URLAPILedger, organizationID, ledgerID)

	req, err := http.NewRequest(http.MethodPatch, uri, bytes.NewBuffer(jsonData))
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

	var ledResp mmodel.Ledger
	if err := json.NewDecoder(resp.Body).Decode(&ledResp); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return &ledResp, nil
}

func NewLedger(f *factory.Factory) *ledger {
	return &ledger{f}
}
