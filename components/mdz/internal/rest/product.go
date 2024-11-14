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

type product struct {
	Factory *factory.Factory
}

func (r *product) Create(organizationID, ledgerID string, inp mmodel.CreateProductInput) (*mmodel.Product, error) {
	jsonData, err := json.Marshal(inp)
	if err != nil {
		return nil, fmt.Errorf("marshalling JSON: %v", err)
	}

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/products",
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

	var productResp mmodel.Product
	if err := json.NewDecoder(resp.Body).Decode(&productResp); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return &productResp, nil
}

func (r *product) Get(organizationID, ledgerID string, limit, page int) (*mmodel.Products, error) {
	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/products?limit=%d&page=%d",
		r.Factory.Env.URLAPILedger, organizationID, ledgerID, limit, page)

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

	var productsResp mmodel.Products
	if err := json.NewDecoder(resp.Body).Decode(&productsResp); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return &productsResp, nil
}

func NewProduct(f *factory.Factory) *product {
	return &product{f}
}
