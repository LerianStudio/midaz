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

func (r *organization) Create(org model.Organization) (*model.OrganizationResponse, error) {
	jsonData, err := json.Marshal(org)
	if err != nil {
		return nil, fmt.Errorf("error marshalling JSON: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost,
		r.Factory.Env.URLAPILedger+"/v1/organizations", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, errors.New("error creating request: " + err.Error())
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer ")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.New("error making POST request: " + err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("failed to create organization, status code: %d", resp.StatusCode)
	}

	var orgResponse model.OrganizationResponse
	if err := json.NewDecoder(resp.Body).Decode(&orgResponse); err != nil {
		return nil, errors.New("error decoding response JSON:" + err.Error())
	}

	return &orgResponse, nil
}

func NewOrganization(f *factory.Factory) *organization {
	return &organization{f}
}
