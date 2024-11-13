package rest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/LerianStudio/midaz/common/mmodel"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
)

type portfolio struct {
	Factory *factory.Factory
}

func (r *portfolio) Create(organizationID, ledgerID string, inp mmodel.CreatePortfolioInput) (*mmodel.Portfolio, error) {
	jsonData, err := json.Marshal(inp)
	if err != nil {
		return nil, fmt.Errorf("marshalling JSON: %v", err)
	}

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/portfolios",
		r.Factory.Env.URLAPILedger, organizationID, ledgerID)

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

	var portfolioResp mmodel.Portfolio
	if err := json.NewDecoder(resp.Body).Decode(&portfolioResp); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return &portfolioResp, nil
}

func (r *portfolio) Get(organizationID, ledgerID string, limit, page int) (*mmodel.Portfolios, error) {
	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/portfolios?limit=%d&page=%d",
		r.Factory.Env.URLAPILedger, organizationID, ledgerID, limit, page)

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

	var portfolioResp mmodel.Portfolios
	if err := json.NewDecoder(resp.Body).Decode(&portfolioResp); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return &portfolioResp, nil
}

func (r *portfolio) GetByID(organizationID, ledgerID, portfolioID string) (*mmodel.Portfolio, error) {
	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/portfolios/%s",
		r.Factory.Env.URLAPILedger, organizationID, ledgerID, portfolioID)

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

	var portfolioItemResp mmodel.Portfolio
	if err := json.NewDecoder(resp.Body).Decode(&portfolioItemResp); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return &portfolioItemResp, nil
}

func NewPortfolio(f *factory.Factory) *portfolio {
	return &portfolio{f}
}
