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

type assetRate struct {
	Factory *factory.Factory
}

func (r *assetRate) Create(
	organizationID, ledgerID string,
	inp mmodel.CreateAssetRateInput,
) (*mmodel.AssetRate, error) {
	jsonData, err := json.Marshal(inp)
	if err != nil {
		return nil, fmt.Errorf("marshalling JSON: %v", err)
	}

	body := bytes.NewReader(jsonData)

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/asset-rates",
		r.Factory.Env.URLAPILedger, organizationID, ledgerID)

	req, err := http.NewRequest(http.MethodPut, uri, body)
	if err != nil {
		return nil, errors.New("creating request: " + err.Error())
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.Factory.Token)

	resp, err := r.Factory.HTTPClient.Do(req)
	if err != nil {
		return nil, errors.New("making PUT request: " + err.Error())
	}

	defer resp.Body.Close()

	if err := checkResponse(resp, http.StatusCreated); err != nil {
		return nil, err
	}

	var assetRateResp mmodel.AssetRate
	if err := json.NewDecoder(resp.Body).Decode(&assetRateResp); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return &assetRateResp, nil
}

func (r *assetRate) GetByExternalID(
	organizationID, ledgerID, externalID string) (*mmodel.AssetRate, error) {
	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/asset-rates/%s",
		r.Factory.Env.URLAPILedger, organizationID, ledgerID, externalID)

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

	var assetRateResp mmodel.AssetRate
	if err := json.NewDecoder(resp.Body).Decode(&assetRateResp); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return &assetRateResp, nil
}

func (r *assetRate) GetByAssetCode(
	organizationID, ledgerID, assetCode string,
	limit, page int,
	sortOrder, startDate, endDate string,
) (*mmodel.AssetRates, error) {
	baseURL := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/asset-rates/from/%s",
		r.Factory.Env.URLAPILedger, organizationID, ledgerID, assetCode)

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

	var assetRatesResp mmodel.AssetRates
	if err := json.NewDecoder(resp.Body).Decode(&assetRatesResp); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return &assetRatesResp, nil
}

// NewAssetRate creates a new asset rate REST client
func NewAssetRate(f *factory.Factory) *assetRate {
	return &assetRate{f}
}