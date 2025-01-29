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

type cluster struct {
	Factory *factory.Factory
}

func (r *cluster) Create(organizationID, ledgerID string, inp mmodel.CreateClusterInput) (*mmodel.Cluster, error) {
	jsonData, err := json.Marshal(inp)
	if err != nil {
		return nil, fmt.Errorf("marshalling JSON: %v", err)
	}

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/clusters",
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

	var clusterResp mmodel.Cluster
	if err := json.NewDecoder(resp.Body).Decode(&clusterResp); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return &clusterResp, nil
}

func (r *cluster) Get(organizationID, ledgerID string, limit, page int, sortOrder, startDate, endDate string) (*mmodel.Clusters, error) {
	baseURL := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/clusters",
		r.Factory.Env.URLAPILedger, organizationID, ledgerID)

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

	var clustersResp mmodel.Clusters
	if err := json.NewDecoder(resp.Body).Decode(&clustersResp); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return &clustersResp, nil
}

func (r *cluster) GetByID(organizationID, ledgerID, clusterID string) (*mmodel.Cluster, error) {
	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/clusters/%s",
		r.Factory.Env.URLAPILedger, organizationID, ledgerID, clusterID)

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

	var clusterResp mmodel.Cluster
	if err := json.NewDecoder(resp.Body).Decode(&clusterResp); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return &clusterResp, nil
}

func (r *cluster) Update(
	organizationID, ledgerID, clusterID string, inp mmodel.UpdateClusterInput,
) (*mmodel.Cluster, error) {
	jsonData, err := json.Marshal(inp)
	if err != nil {
		return nil, fmt.Errorf("marshalling JSON: %v", err)
	}

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/clusters/%s",
		r.Factory.Env.URLAPILedger, organizationID, ledgerID, clusterID)

	req, err := http.NewRequest(http.MethodPatch, uri, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, errors.New("creating request: " + err.Error())
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.Factory.Token)

	resp, err := r.Factory.HTTPClient.Do(req)
	if err != nil {
		return nil, errors.New("making PATCH request: " + err.Error())
	}
	defer resp.Body.Close()

	if err := checkResponse(resp, http.StatusOK); err != nil {
		return nil, err
	}

	var clusterResp mmodel.Cluster
	if err := json.NewDecoder(resp.Body).Decode(&clusterResp); err != nil {
		return nil, errors.New("decoding response JSON:" + err.Error())
	}

	return &clusterResp, nil
}

func (r *cluster) Delete(organizationID, ledgerID, clusterID string) error {
	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/clusters/%s",
		r.Factory.Env.URLAPILedger, organizationID, ledgerID, clusterID)

	req, err := http.NewRequest(http.MethodDelete, uri, nil)
	if err != nil {
		return errors.New("creating request: " + err.Error())
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.Factory.Token)

	resp, err := r.Factory.HTTPClient.Do(req)
	if err != nil {
		return errors.New("making Delete request: " + err.Error())
	}

	defer resp.Body.Close()

	if err := checkResponse(resp, http.StatusNoContent); err != nil {
		return err
	}

	return nil
}

func NewCluster(f *factory.Factory) *cluster {
	return &cluster{f}
}
