package rest

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/components/mdz/pkg/environment"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/mockutil"
	"github.com/LerianStudio/midaz/pkg/mmodel"

	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"
)

func Test_assetRate_Create(t *testing.T) {
	organizationID := "0192e250-ed9d-7e5c-a614-9b294151b572"
	ledgerID := "0192e251-328d-7390-99f5-5c54980115ed"
	assetCode := "BRL"
	targetAssetCode := "USD"
	value := 4.97
	externalID := "ext-rate-12345"

	input := mmodel.CreateAssetRateInput{
		From:       assetCode,
		To:         targetAssetCode,
		Rate:       int(value * 100), // Convert to integer with scale 2
		Scale:      2,
		ExternalID: &externalID,
	}

	expectedResult := &mmodel.AssetRate{
		From:           assetCode,
		To:             targetAssetCode,
		Rate:           value,
		ExternalID:     externalID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
	}

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)
	defer httpmock.DeactivateAndReset()

	URIAPIOnboarding := "http://127.0.0.1:3000"

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/asset-rates",
		URIAPIOnboarding, organizationID, ledgerID)

	httpmock.RegisterResponder(http.MethodPut, uri,
		mockutil.MockResponseFromFile(http.StatusCreated, "./.fixtures/assetrate_response_create.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPIOnboarding: URIAPIOnboarding,
		},
	}

	assetRate := NewAssetRate(factory)

	result, err := assetRate.Create(organizationID, ledgerID, input)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedResult.From, result.From)
	assert.Equal(t, expectedResult.To, result.To)
	assert.Equal(t, expectedResult.Rate, result.Rate)
	assert.Equal(t, expectedResult.ExternalID, result.ExternalID)
	assert.Equal(t, expectedResult.LedgerID, result.LedgerID)
	assert.Equal(t, expectedResult.OrganizationID, result.OrganizationID)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["PUT "+uri])
}

func Test_assetRate_GetByExternalID(t *testing.T) {
	organizationID := "0192e250-ed9d-7e5c-a614-9b294151b572"
	ledgerID := "0192e251-328d-7390-99f5-5c54980115ed"
	externalID := "ext-rate-12345"
	assetCode := "BRL"
	targetAssetCode := "USD"
	value := 4.97

	URIAPIOnboarding := "http://127.0.0.1:3000"

	expectedResult := &mmodel.AssetRate{
		From:           assetCode,
		To:             targetAssetCode,
		Rate:           value,
		ExternalID:     externalID,
		LedgerID:       ledgerID,
		OrganizationID: organizationID,
	}

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)
	defer httpmock.DeactivateAndReset()

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/asset-rates/%s",
		URIAPIOnboarding, organizationID, ledgerID, externalID)

	httpmock.RegisterResponder(http.MethodGet, uri,
		mockutil.MockResponseFromFile(http.StatusOK, "./.fixtures/assetrate_response_get_by_id.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPIOnboarding: URIAPIOnboarding,
		},
	}

	assetRate := NewAssetRate(factory)

	result, err := assetRate.GetByExternalID(organizationID, ledgerID, externalID)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedResult.From, result.From)
	assert.Equal(t, expectedResult.To, result.To)
	assert.Equal(t, expectedResult.Rate, result.Rate)
	assert.Equal(t, expectedResult.ExternalID, result.ExternalID)
	assert.Equal(t, expectedResult.OrganizationID, result.OrganizationID)
	assert.Equal(t, expectedResult.LedgerID, result.LedgerID)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["GET "+uri])
}

func Test_assetRate_GetByAssetCode(t *testing.T) {
	organizationID := "0192e250-ed9d-7e5c-a614-9b294151b572"
	ledgerID := "0192e251-328d-7390-99f5-5c54980115ed"
	assetCode := "BRL"

	limit := 2
	page := 1

	expectedResult := mmodel.AssetRates{
		Page:  page,
		Limit: limit,
		Items: []mmodel.AssetRate{
			{
				From:           "BRL",
				To:             "USD",
				Rate:           4.97,
				ExternalID:     "ext-rate-12345",
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				CreatedAt:      time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
				UpdatedAt:      time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
			},
			{
				From:           "BRL",
				To:             "EUR",
				Rate:           5.43,
				ExternalID:     "ext-rate-67890",
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				CreatedAt:      time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
				UpdatedAt:      time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
			},
		},
	}

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)
	defer httpmock.DeactivateAndReset()

	URIAPIOnboarding := "http://127.0.0.1:3000"

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/asset-rates/from/%s?limit=%d&page=%d",
		URIAPIOnboarding, organizationID, ledgerID, assetCode, limit, page)

	httpmock.RegisterResponder(http.MethodGet, uri,
		mockutil.MockResponseFromFile(http.StatusOK, "./.fixtures/assetrate_response_list.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPIOnboarding: URIAPIOnboarding,
		},
	}

	assetRate := NewAssetRate(factory)

	result, err := assetRate.GetByAssetCode(organizationID, ledgerID, assetCode, limit, page, "", "", "")

	assert.NoError(t, err)
	assert.NotNil(t, result)

	for i, v := range result.Items {
		assert.Equal(t, expectedResult.Items[i].From, v.From)
		assert.Equal(t, expectedResult.Items[i].To, v.To)
		assert.Equal(t, expectedResult.Items[i].Rate, v.Rate)
		assert.Equal(t, expectedResult.Items[i].ExternalID, v.ExternalID)
	}
	assert.Equal(t, expectedResult.Limit, limit)
	assert.Equal(t, expectedResult.Page, page)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["GET "+uri])
}
