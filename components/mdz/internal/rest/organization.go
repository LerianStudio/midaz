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

type organization struct {
	Factory *factory.Factory
}

func (r *organization) Create(org model.Organization) (*model.OrganizationCreate, error) {
	jsonData, err := json.Marshal(org)
	if err != nil {
		return nil, fmt.Errorf("marshalling JSON: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost,
		r.Factory.Env.URLAPILedger+"/v1/organizations", bytes.NewBuffer(jsonData))
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

	var orgResponse model.OrganizationCreate
	if err := json.NewDecoder(resp.Body).Decode(&orgResponse); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return &orgResponse, nil
}

func (r *organization) Get(limit, page int) (*model.OrganizationList, error) {
	uri := fmt.Sprintf("%s/v1/organizations?limit=%d&page=%d",
		r.Factory.Env.URLAPILedger, limit, page)

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

		return nil, fmt.Errorf("failed to list organization, status code: %d",
			resp.StatusCode)
	}

	var orgListResp model.OrganizationList
	if err := json.NewDecoder(resp.Body).Decode(&orgListResp); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return &orgListResp, nil
}

func (r *organization) GetByID(organizationID string) (*model.OrganizationItem, error) {
	uri := fmt.Sprintf("%s/v1/organizations/%s", r.Factory.Env.URLAPILedger, organizationID)

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

	var orgItemResp model.OrganizationItem
	if err := json.NewDecoder(resp.Body).Decode(&orgItemResp); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return &orgItemResp, nil
}

func (r *organization) Update(organizationID string, orgInput model.OrganizationUpdate) (*model.OrganizationItem, error) {
	payloadBytes, err := json.Marshal(orgInput)
	if err != nil {
		return nil, fmt.Errorf("marshalling JSON: %v", err)
	}

	body := bytes.NewReader(payloadBytes)

	uri := fmt.Sprintf("%s/v1/organizations/%s", r.Factory.Env.URLAPILedger, organizationID)

	req, err := http.NewRequest(http.MethodPatch, uri, body)
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

		return nil, fmt.Errorf("failed to update organization, status code: %d",
			resp.StatusCode)
	}

	var orgItemResp model.OrganizationItem
	if err := json.NewDecoder(resp.Body).Decode(&orgItemResp); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return &orgItemResp, nil
}

func NewOrganization(f *factory.Factory) *organization {
	return &organization{f}
}
