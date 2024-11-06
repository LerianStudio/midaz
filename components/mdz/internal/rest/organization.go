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

type organization struct {
	Factory *factory.Factory
}

func (r *organization) Create(inp mmodel.CreateOrganizationInput) (*mmodel.Organization, error) {
	jsonData, err := json.Marshal(inp)
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

	var org mmodel.Organization
	if err := json.NewDecoder(resp.Body).Decode(&org); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return &org, nil
}

func (r *organization) Get(limit, page int) (*mmodel.Organizations, error) {
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

	var orgs mmodel.Organizations
	if err := json.NewDecoder(resp.Body).Decode(&orgs); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return &orgs, nil
}

func (r *organization) GetByID(organizationID string) (*mmodel.Organization, error) {
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

	var org mmodel.Organization
	if err := json.NewDecoder(resp.Body).Decode(&org); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return &org, nil
}

func (r *organization) Update(organizationID string, inp mmodel.UpdateOrganizationInput) (*mmodel.Organization, error) {
	payloadBytes, err := json.Marshal(inp)
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

	var org mmodel.Organization
	if err := json.NewDecoder(resp.Body).Decode(&org); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return &org, nil
}

func (r *organization) Delete(organizationID string) error {
	uri := fmt.Sprintf("%s/v1/organizations/%s", r.Factory.Env.URLAPILedger, organizationID)

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

	if resp.StatusCode != http.StatusNoContent {
		if resp.StatusCode == http.StatusUnauthorized {
			return errors.New("unauthorized invalid credentials")
		}

		return fmt.Errorf("failed to update organization, status code: %d",
			resp.StatusCode)
	}

	return nil
}

func NewOrganization(f *factory.Factory) *organization {
	return &organization{f}
}
