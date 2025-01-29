package rest

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/components/mdz/pkg/environment"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/mockutil"
	"github.com/LerianStudio/midaz/components/mdz/pkg/ptr"
	"github.com/LerianStudio/midaz/pkg/mmodel"

	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"
)

func Test_cluster_Create(t *testing.T) {
	clusterID := "0193271b-877f-7c98-a5a6-43b664d68982"
	ledgerID := "01932715-9f93-7432-90c3-4352bcfe464d"
	organizationID := "01931b04-964a-7caa-a422-c29a95387c00"

	name := "Cluster Refined Cotton Chair"
	code := "ACTIVE"
	description := ptr.StringPtr("Teste Cluster")

	metadata := map[string]any{
		"bitcoin": "3g9ofZcD7KRWL44BWdNa3PyM4PfzgqDG5P",
		"chave":   "metadata_chave",
		"boolean": true,
	}

	input := mmodel.CreateClusterInput{
		Name: name,
		Status: mmodel.Status{
			Code:        code,
			Description: description,
		},
		Metadata: metadata,
	}

	expectedResult := &mmodel.Cluster{
		ID:             clusterID,
		Name:           name,
		LedgerID:       ledgerID,
		OrganizationID: organizationID,
		Status: mmodel.Status{
			Code:        code,
			Description: description,
		},
		Metadata: metadata,
	}

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)
	defer httpmock.DeactivateAndReset()

	URIAPILedger := "http://127.0.0.1:3000"

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/clusters",
		URIAPILedger, organizationID, ledgerID)

	httpmock.RegisterResponder(http.MethodPost, uri,
		mockutil.MockResponseFromFile(http.StatusCreated, "./.fixtures/cluster_response_create.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPILedger: URIAPILedger,
		},
	}

	clusterServ := NewCluster(factory)

	result, err := clusterServ.Create(organizationID, ledgerID, input)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedResult.ID, result.ID)
	assert.Equal(t, expectedResult.Name, result.Name)
	assert.Equal(t, expectedResult.OrganizationID, result.OrganizationID)
	assert.Equal(t, expectedResult.LedgerID, result.LedgerID)
	assert.Equal(t, expectedResult.Status.Code, result.Status.Code)
	assert.Equal(t, expectedResult.Status.Description, result.Status.Description)
	assert.Equal(t, expectedResult.Metadata, result.Metadata)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["POST "+uri])
}

func Test_cluster_Get(t *testing.T) {
	organizationID := "01931b04-964a-7caa-a422-c29a95387c00"
	ledgerID := "01931b04-c2d1-7a41-83ac-c5d6d8a3c22c"

	limit := 2
	page := 1

	expectedResult := mmodel.Clusters{
		Page:  page,
		Limit: limit,
		Items: []mmodel.Cluster{
			{
				ID:   "01932727-1b5a-7540-98c0-6521ffe78ce6",
				Name: "Cluster Licensed Concrete Hat",
				Status: mmodel.Status{
					Code:        "ACTIVE",
					Description: ptr.StringPtr("Teste Cluster"),
				},
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				CreatedAt:      time.Date(2024, 11, 11, 18, 51, 33, 15793, time.UTC),
				UpdatedAt:      time.Date(2024, 11, 11, 18, 51, 33, 15793, time.UTC),
				DeletedAt:      nil,
			},
			{
				ID:   "0193271b-877f-7c98-a5a6-43b664d68982",
				Name: "Toy30 Portfolio",
				Status: mmodel.Status{
					Code:        "ACTIVE",
					Description: ptr.StringPtr("Teste Cluster"),
				},
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				CreatedAt:      time.Date(2024, 11, 11, 18, 51, 27, 447406, time.UTC),
				UpdatedAt:      time.Date(2024, 11, 11, 18, 51, 27, 447406, time.UTC),
				DeletedAt:      nil,
				Metadata: map[string]any{
					"bitcoin": "3g9ofZcD7KRWL44BWdNa3PyM4PfzgqDG5P",
					"chave":   "metadata_chave",
					"boolean": true,
				},
			},
		},
	}

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)
	defer httpmock.DeactivateAndReset()

	URIAPILedger := "http://127.0.0.1:3000"

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/clusters?limit=%d&page=%d",
		URIAPILedger, organizationID, ledgerID, limit, page)

	httpmock.RegisterResponder(http.MethodGet, uri,
		mockutil.MockResponseFromFile(http.StatusOK, "./.fixtures/cluster_response_list.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPILedger: URIAPILedger,
		},
	}

	cluster := NewCluster(factory)

	result, err := cluster.Get(organizationID, ledgerID, limit, page, "", "", "")

	assert.NoError(t, err)
	assert.NotNil(t, result)

	for i, v := range result.Items {
		assert.Equal(t, expectedResult.Items[i].ID, v.ID)
		assert.Equal(t, expectedResult.Items[i].Metadata, v.Metadata)
	}

	assert.Equal(t, expectedResult.Limit, limit)
	assert.Equal(t, expectedResult.Page, page)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["GET "+uri])
}

func Test_cluster_GetByID(t *testing.T) {
	clusterID := "01932727-1b5a-7540-98c0-6521ffe78ce6"
	ledgerID := "01932715-9f93-7432-90c3-4352bcfe464d"
	organizationID := "01931b04-964a-7caa-a422-c29a95387c00"

	URIAPILedger := "http://127.0.0.1:3000"

	expectedResult := &mmodel.Cluster{
		ID:             clusterID,
		Name:           "Cluster Licensed Concrete Hat",
		LedgerID:       ledgerID,
		OrganizationID: organizationID,
		Status: mmodel.Status{
			Code:        "ACTIVE",
			Description: ptr.StringPtr("Teste Cluster"),
		},
		CreatedAt: time.Date(2024, 11, 13, 20, 11, 34, 617671000, time.UTC),
		UpdatedAt: time.Date(2024, 11, 13, 20, 11, 34, 617674000, time.UTC),
		DeletedAt: nil,
	}

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)
	defer httpmock.DeactivateAndReset()

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/clusters/%s",
		URIAPILedger, organizationID, ledgerID, clusterID)

	httpmock.RegisterResponder(http.MethodGet, uri,
		mockutil.MockResponseFromFile(http.StatusOK, "./.fixtures/cluster_response_get_by_id.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPILedger: URIAPILedger,
		},
	}

	cluster := NewCluster(factory)

	result, err := cluster.GetByID(organizationID, ledgerID, clusterID)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedResult.ID, result.ID)
	assert.Equal(t, expectedResult.Name, result.Name)
	assert.Equal(t, expectedResult.OrganizationID, result.OrganizationID)
	assert.Equal(t, expectedResult.LedgerID, result.LedgerID)
	assert.Equal(t, expectedResult.Status.Code, result.Status.Code)
	assert.Equal(t, expectedResult.Status.Description, result.Status.Description)
	assert.Equal(t, expectedResult.CreatedAt, result.CreatedAt)
	assert.Equal(t, expectedResult.UpdatedAt, result.UpdatedAt)
	assert.Equal(t, expectedResult.DeletedAt, result.DeletedAt)
	assert.Equal(t, expectedResult.Metadata, result.Metadata)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["GET "+uri])
}

func Test_cluster_Update(t *testing.T) {
	clusterID := "01932727-1b5a-7540-98c0-6521ffe78ce6"
	ledgerID := "01932715-9f93-7432-90c3-4352bcfe464d"
	organizationID := "01931b04-964a-7caa-a422-c29a95387c00"
	name := "Cluster Practical Metal Sausages BLOCKED"
	statusCode := "BLOCKED"
	statusDescription := ptr.StringPtr("Teste Cluster BLOCKED")

	metadata := map[string]any{
		"bitcoin": "35x7shF9VF1npqiTNjMsytJTRBNAoaAh",
		"chave":   "metadata_chave",
		"boolean": true,
	}

	inp := mmodel.UpdateClusterInput{
		Name: name,
		Status: mmodel.Status{
			Code:        statusCode,
			Description: statusDescription,
		},
		Metadata: metadata,
	}

	expectedResult := &mmodel.Cluster{
		ID:   clusterID,
		Name: name,
		Status: mmodel.Status{
			Code:        statusCode,
			Description: statusDescription,
		},
		Metadata: metadata,
	}

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)
	defer httpmock.DeactivateAndReset()

	URIAPILedger := "http://127.0.0.1:3000"

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/clusters/%s",
		URIAPILedger, organizationID, ledgerID, clusterID)

	httpmock.RegisterResponder(http.MethodPatch, uri,
		mockutil.MockResponseFromFile(http.StatusOK,
			"./.fixtures/cluster_response_update.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPILedger: URIAPILedger,
		},
	}

	cluster := NewCluster(factory)

	result, err := cluster.Update(organizationID, ledgerID, clusterID, inp)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedResult.ID, result.ID)
	assert.Equal(t, expectedResult.Name, result.Name)
	assert.Equal(t, expectedResult.Status.Code, result.Status.Code)
	assert.Equal(t, expectedResult.Status.Description, result.Status.Description)
	assert.Equal(t, expectedResult.Metadata, result.Metadata)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["PATCH "+uri])
}

func Test_cluster_Delete(t *testing.T) {
	clusterID := "01930219-2c25-7a37-a5b9-610d44ae0a27"
	ledgerID := "0192fc1e-14bf-7894-b167-6e4a878b3a95"
	organizationID := "0192fc1d-f34d-78c9-9654-83e497349241"
	URIAPILedger := "http://127.0.0.1:3000"

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)
	defer httpmock.DeactivateAndReset()

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/clusters/%s",
		URIAPILedger, organizationID, ledgerID, clusterID)

	httpmock.RegisterResponder(http.MethodDelete, uri,
		httpmock.NewStringResponder(http.StatusNoContent, ""))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPILedger: URIAPILedger,
		},
	}

	cluster := NewCluster(factory)

	err := cluster.Delete(organizationID, ledgerID, clusterID)

	assert.NoError(t, err)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["DELETE "+uri])
}
