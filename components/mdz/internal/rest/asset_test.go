package rest

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/common/mmodel"
	"github.com/LerianStudio/midaz/components/mdz/pkg/environment"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/mockutil"
	"github.com/LerianStudio/midaz/components/mdz/pkg/ptr"
	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"
)

func Test_asset_Create(t *testing.T) {
	organizationID := "0192e250-ed9d-7e5c-a614-9b294151b572"
	ledgerID := "0192e251-328d-7390-99f5-5c54980115ed"
	assetID := "01930219-2c25-7a37-a5b9-610d44ae0a27"

	name := "Brazilian Real"
	typeV := "currency"
	code := "BRL"
	statusCode := "ACTIVE"
	statusDescription := ptr.StringPtr("Teste asset 1")

	metadata := map[string]any{
		"bitcoin": "3oDTprwNG37nASsyLzQGLuUBzNac",
		"chave":   "metadata_chave",
		"boolean": false,
	}

	input := mmodel.CreateAssetInput{
		Name: name,
		Type: typeV,
		Code: code,
		Status: mmodel.Status{
			Code:        statusCode,
			Description: statusDescription,
		},
		Metadata: metadata,
	}

	expectedResult := &mmodel.Asset{
		ID:   assetID,
		Name: name,
		Type: typeV,
		Code: code,
		Status: mmodel.Status{
			Code:        statusCode,
			Description: statusDescription,
		},
		LedgerID:       ledgerID,
		OrganizationID: organizationID,
		CreatedAt:      time.Date(2024, 11, 06, 15, 30, 24, 421664681, time.UTC),
		UpdatedAt:      time.Date(2024, 11, 06, 15, 30, 24, 421664731, time.UTC),
		DeletedAt:      nil,
		Metadata:       metadata,
	}

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)
	defer httpmock.DeactivateAndReset()

	URIAPILedger := "http://127.0.0.1:3000"

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/assets",
		URIAPILedger, organizationID, ledgerID)

	httpmock.RegisterResponder(http.MethodPost, uri,
		mockutil.MockResponseFromFile(http.StatusCreated, "./.fixtures/asset_response_create.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPILedger: URIAPILedger,
		},
	}

	asset := NewAsset(factory)

	result, err := asset.Create(organizationID, ledgerID, input)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedResult.ID, result.ID)
	assert.Equal(t, expectedResult.Name, result.Name)
	assert.Equal(t, expectedResult.Type, result.Type)
	assert.Equal(t, expectedResult.Code, result.Code)
	assert.Equal(t, expectedResult.Status.Code, result.Status.Code)
	assert.Equal(t, *expectedResult.Status.Description, *result.Status.Description)
	assert.Equal(t, expectedResult.LedgerID, result.LedgerID)
	assert.Equal(t, expectedResult.OrganizationID, result.OrganizationID)
	assert.Equal(t, expectedResult.CreatedAt, result.CreatedAt)
	assert.Equal(t, expectedResult.UpdatedAt, result.UpdatedAt)
	assert.Equal(t, expectedResult.DeletedAt, result.DeletedAt)
	assert.Equal(t, expectedResult.Metadata, result.Metadata)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["POST http://127.0.0.1:3000/v1/organizations/0192e250-ed9d-7e5c-a614-9b294151b572/ledgers/0192e251-328d-7390-99f5-5c54980115ed/assets"])
}

func Test_asset_Get(t *testing.T) {
	organizationID := "0192fc1d-f34d-78c9-9654-83e497349241"
	ledgerID := "01930218-bfb7-74fe-ba00-e52a17e9fb4e"

	limit := 2
	page := 1

	expectedResult := mmodel.Assets{
		Page:  page,
		Limit: limit,
		Items: []mmodel.Asset{
			{
				ID:   "01930365-4d46-7a09-a503-b932714f85af",
				Name: "2Real",
				Type: "commodity",
				Code: "DOP",
				Status: mmodel.Status{
					Code:        "ACTIVE",
					Description: ptr.StringPtr("Teste asset 1"),
				},
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				CreatedAt:      time.Date(2024, 11, 06, 21, 33, 10, 854653000, time.UTC),
				UpdatedAt:      time.Date(2024, 11, 06, 21, 33, 10, 854653000, time.UTC),
				DeletedAt:      nil,
				Metadata: map[string]any{
					"bitcoin": "1RuuEjC8CziKy6XbYU6uwsNSYjU7H2Mft",
					"chave":   "metadata_chave",
					"boolean": false,
				},
			},
			{
				ID:   "01930219-2c25-7a37-a5b9-610d44ae0a27",
				Name: "Brazilian Real",
				Type: "currency",
				Code: "BRL",
				Status: mmodel.Status{
					Code:        "ACTIVE",
					Description: ptr.StringPtr("Teste asset 1"),
				},
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				CreatedAt:      time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
				UpdatedAt:      time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
				DeletedAt:      nil,
				Metadata: map[string]any{
					"bitcoin": "3oDTprwNG37nASsyLzQGLuUBzNac",
					"chave":   "metadata_chave",
					"boolean": false,
				},
			},
		},
	}

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)
	defer httpmock.DeactivateAndReset()

	URIAPILedger := "http://127.0.0.1:3000"

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/assets?limit=%d&page=%d",
		URIAPILedger, organizationID, ledgerID, limit, page)

	httpmock.RegisterResponder(http.MethodGet, uri,
		mockutil.MockResponseFromFile(http.StatusOK, "./.fixtures/asset_response_list.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPILedger: URIAPILedger,
		},
	}

	asset := NewAsset(factory)

	result, err := asset.Get(organizationID, ledgerID, limit, page)

	assert.NoError(t, err)
	assert.NotNil(t, result)

	for i, v := range result.Items {
		assert.Equal(t, expectedResult.Items[i].ID, v.ID)
		assert.Equal(t, expectedResult.Items[i].Metadata, v.Metadata)
	}
	assert.Equal(t, expectedResult.Limit, limit)
	assert.Equal(t, expectedResult.Page, page)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["GET http://127.0.0.1:3000/v1/organizations/0192fc1d-f34d-78c9-9654-83e497349241/ledgers/01930218-bfb7-74fe-ba00-e52a17e9fb4e/assets?limit=2&page=1"])
}

func Test_asset_GetByID(t *testing.T) {
	assetID := "01930365-4d46-7a09-a503-b932714f85af"
	ledgerID := "01930218-bfb7-74fe-ba00-e52a17e9fb4e"
	organizationID := "0192fc1d-f34d-78c9-9654-83e497349241"

	URIAPILedger := "http://127.0.0.1:3000"

	expectedResult := &mmodel.Asset{
		ID:   assetID,
		Name: "2Real",
		Type: "commodity",
		Code: "DOP",
		Status: mmodel.Status{
			Code:        "ACTIVE",
			Description: ptr.StringPtr("Teste asset 1"),
		},
		LedgerID:       ledgerID,
		OrganizationID: organizationID,
		CreatedAt:      time.Date(2024, 11, 06, 21, 33, 10, 854653000, time.UTC),
		UpdatedAt:      time.Date(2024, 11, 06, 21, 33, 10, 854653000, time.UTC),
		DeletedAt:      nil,
		Metadata: map[string]any{
			"bitcoin": "1RuuEjC8CziKy6XbYU6uwsNSYjU7H2Mft",
			"chave":   "metadata_chave",
			"boolean": false,
		},
	}

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)
	defer httpmock.DeactivateAndReset()

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/assets/%s",
		URIAPILedger, organizationID, ledgerID, assetID)

	httpmock.RegisterResponder(http.MethodGet, uri,
		mockutil.MockResponseFromFile(http.StatusOK, "./.fixtures/asset_response_get_by_id.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPILedger: URIAPILedger,
		},
	}

	asset := NewAsset(factory)

	result, err := asset.GetByID(organizationID, ledgerID, assetID)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedResult.ID, result.ID)
	assert.Equal(t, expectedResult.Name, result.Name)
	assert.Equal(t, expectedResult.OrganizationID, result.OrganizationID)
	assert.Equal(t, expectedResult.LedgerID, result.LedgerID)
	assert.Equal(t, expectedResult.Code, result.Code)
	assert.Equal(t, expectedResult.Type, result.Type)
	assert.Equal(t, expectedResult.Status.Code, result.Status.Code)
	assert.Equal(t, expectedResult.Status.Description, result.Status.Description)
	assert.Equal(t, expectedResult.CreatedAt, result.CreatedAt)
	assert.Equal(t, expectedResult.UpdatedAt, result.UpdatedAt)
	assert.Equal(t, expectedResult.DeletedAt, result.DeletedAt)
	assert.Equal(t, expectedResult.Metadata, result.Metadata)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["GET http://127.0.0.1:3000/v1/organizations/0192fc1d-f34d-78c9-9654-83e497349241/ledgers/01930218-bfb7-74fe-ba00-e52a17e9fb4e/assets/01930365-4d46-7a09-a503-b932714f85af"])
}
