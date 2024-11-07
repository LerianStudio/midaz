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
