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

type asset struct {
	Factory *factory.Factory
}

func (r *asset) Create(organizationID, ledgerID string, inp mmodel.CreateAssetInput) (*mmodel.Asset, error) {
	jsonData, err := json.Marshal(inp)
	if err != nil {
		return nil, fmt.Errorf("marshalling JSON: %v", err)
	}

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/assets",
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

	var assetRest mmodel.Asset
	if err := json.NewDecoder(resp.Body).Decode(&assetRest); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return &assetRest, nil
}

func (r *asset) Get(organizationID, ledgerID string, limit, page int) (*mmodel.Assets, error) {
	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/assets?limit=%d&page=%d",
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

	var assetResp mmodel.Assets
	if err := json.NewDecoder(resp.Body).Decode(&assetResp); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return &assetResp, nil
}

func NewAsset(f *factory.Factory) *asset {
	return &asset{f}
}
