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

func Test_assetRate_Create(t *testing.T) {
	organizationID := "01933f94-67b1-794c-bb13-6b75aed7591a"
	ledgerID := "01933f94-8a8f-7a1e-b4ab-98f35a5f8d61"
	assetRateID := "01933f96-ed04-7c57-be5b-c091388830f8"

	fromAssetCode := "USD"
	toAssetCode := "BRL"
	rate := int64(52000)
	rateScale := int64(4)
	statusCode := "ACTIVE"
	statusDescription := ptr.StringPtr("Active asset rate")

	metadata := map[string]any{
		"source":     "central_bank",
		"timestamp":  "2024-11-18T14:00:00Z",
		"isOfficial": true,
	}

	input := mmodel.CreateAssetRateInput{
		FromAssetCode: fromAssetCode,
		ToAssetCode:   toAssetCode,
		Rate:          rate,
		RateScale:     rateScale,
		Metadata:      metadata,
	}

	expectedResult := &mmodel.AssetRate{
		ID:           assetRateID,
		FromAssetCode: fromAssetCode,
		ToAssetCode:   toAssetCode,
		Rate:          rate,
		RateScale:     rateScale,
		Status: &mmodel.Status{
			Code:        statusCode,
			Description: statusDescription,
		},
		CreatedAt: time.Date(2024, 11, 18, 14, 04, 35, 972791193, time.UTC),
		UpdatedAt: time.Date(2024, 11, 18, 14, 04, 35, 972794057, time.UTC),
		Metadata:  metadata,
	}

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)
	defer httpmock.DeactivateAndReset()

	URIAPITransaction := "http://127.0.0.1:3001"

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/asset-rates",
		URIAPITransaction, organizationID, ledgerID)

	httpmock.RegisterResponder(http.MethodPost, uri,
		mockutil.MockResponseFromFile(http.StatusCreated, "./.fixtures/asset_rate_response_create.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPITransaction: URIAPITransaction,
		},
	}

	assetRate := NewAssetRate(factory)

	result, err := assetRate.Create(organizationID, ledgerID, input)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedResult.ID, result.ID)
	assert.Equal(t, expectedResult.FromAssetCode, result.FromAssetCode)
	assert.Equal(t, expectedResult.ToAssetCode, result.ToAssetCode)
	assert.Equal(t, expectedResult.Rate, result.Rate)
	assert.Equal(t, expectedResult.RateScale, result.RateScale)
	assert.Equal(t, expectedResult.Status.Code, result.Status.Code)
	assert.Equal(t, *expectedResult.Status.Description, *result.Status.Description)
	assert.Equal(t, expectedResult.CreatedAt, result.CreatedAt)
	assert.Equal(t, expectedResult.UpdatedAt, result.UpdatedAt)
	assert.Equal(t, expectedResult.Metadata, result.Metadata)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["POST "+uri])
}

func Test_assetRate_Update(t *testing.T) {
	organizationID := "01933f94-67b1-794c-bb13-6b75aed7591a"
	ledgerID := "01933f94-8a8f-7a1e-b4ab-98f35a5f8d61"
	assetRateID := "01933f96-ed04-7c57-be5b-c091388830f8"

	rate := int64(53000)
	rateScale := int64(4)
	statusCode := "UPDATED"
	statusDescription := ptr.StringPtr("Updated asset rate")

	metadata := map[string]any{
		"source":     "central_bank",
		"timestamp":  "2024-11-18T15:00:00Z",
		"isOfficial": true,
		"lastUpdate": "2024-11-18T15:00:00Z",
	}

	input := mmodel.UpdateAssetRateInput{
		Rate:      rate,
		RateScale: rateScale,
		Status: mmodel.Status{
			Code:        statusCode,
			Description: statusDescription,
		},
		Metadata: metadata,
	}

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)
	defer httpmock.DeactivateAndReset()

	URIAPITransaction := "http://127.0.0.1:3001"

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/asset-rates/%s",
		URIAPITransaction, organizationID, ledgerID, assetRateID)

	httpmock.RegisterResponder(http.MethodPatch, uri,
		mockutil.MockResponseFromFile(http.StatusOK, "./.fixtures/asset_rate_response_update.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPITransaction: URIAPITransaction,
		},
	}

	assetRate := NewAssetRate(factory)

	result, err := assetRate.Update(organizationID, ledgerID, assetRateID, input)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, assetRateID, result.ID)
	assert.Equal(t, rate, result.Rate)
	assert.Equal(t, rateScale, result.RateScale)
	assert.Equal(t, statusCode, result.Status.Code)
	assert.Equal(t, *statusDescription, *result.Status.Description)
	assert.Equal(t, metadata, result.Metadata)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["PATCH "+uri])
}

func Test_assetRate_Get(t *testing.T) {
	organizationID := "01933f94-67b1-794c-bb13-6b75aed7591a"
	ledgerID := "01933f94-8a8f-7a1e-b4ab-98f35a5f8d61"
	limit := 10
	page := 1
	sortOrder := "desc"
	startDate := "2024-01-01"
	endDate := "2024-12-31"

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)
	defer httpmock.DeactivateAndReset()

	URIAPITransaction := "http://127.0.0.1:3001"

	// Let's use BuildPaginatedURL to ensure the URL is constructed correctly
	baseURL := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/asset-rates",
		URIAPITransaction, organizationID, ledgerID)
	
	uri, err := BuildPaginatedURL(baseURL, limit, page, sortOrder, startDate, endDate)
	assert.NoError(t, err)

	httpmock.RegisterResponder(http.MethodGet, uri,
		mockutil.MockResponseFromFile(http.StatusOK, "./.fixtures/asset_rate_response_get.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPITransaction: URIAPITransaction,
		},
	}

	assetRate := NewAssetRate(factory)

	result, err := assetRate.Get(organizationID, ledgerID, limit, page, sortOrder, startDate, endDate)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, len(result.Items))
	assert.Equal(t, "01933f96-ed04-7c57-be5b-c091388830f8", result.Items[0].ID)
	assert.Equal(t, "next_page_token", *result.Pagination.NextCursor)
	assert.Equal(t, "prev_page_token", *result.Pagination.PrevCursor)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["GET "+uri])
}

func Test_assetRate_GetByID(t *testing.T) {
	organizationID := "01933f94-67b1-794c-bb13-6b75aed7591a"
	ledgerID := "01933f94-8a8f-7a1e-b4ab-98f35a5f8d61"
	assetRateID := "01933f96-ed04-7c57-be5b-c091388830f8"

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)
	defer httpmock.DeactivateAndReset()

	URIAPITransaction := "http://127.0.0.1:3001"

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/asset-rates/%s",
		URIAPITransaction, organizationID, ledgerID, assetRateID)

	httpmock.RegisterResponder(http.MethodGet, uri,
		mockutil.MockResponseFromFile(http.StatusOK, "./.fixtures/asset_rate_response_get_by_id.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPITransaction: URIAPITransaction,
		},
	}

	assetRate := NewAssetRate(factory)

	result, err := assetRate.GetByID(organizationID, ledgerID, assetRateID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, assetRateID, result.ID)
	assert.Equal(t, "USD", result.FromAssetCode)
	assert.Equal(t, "BRL", result.ToAssetCode)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["GET "+uri])
}

func Test_assetRate_GetByAssetCode(t *testing.T) {
	organizationID := "01933f94-67b1-794c-bb13-6b75aed7591a"
	ledgerID := "01933f94-8a8f-7a1e-b4ab-98f35a5f8d61"
	assetCode := "USD"
	limit := 10
	page := 1
	sortOrder := "desc"
	startDate := "2024-01-01"
	endDate := "2024-12-31"

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)
	defer httpmock.DeactivateAndReset()

	URIAPITransaction := "http://127.0.0.1:3001"

	// Let's use BuildPaginatedURL to ensure the URL is constructed correctly
	baseURL := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/asset-rates/from/%s",
		URIAPITransaction, organizationID, ledgerID, assetCode)
	
	uri, err := BuildPaginatedURL(baseURL, limit, page, sortOrder, startDate, endDate)
	assert.NoError(t, err)

	httpmock.RegisterResponder(http.MethodGet, uri,
		mockutil.MockResponseFromFile(http.StatusOK, "./.fixtures/asset_rate_response_get_by_asset_code.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPITransaction: URIAPITransaction,
		},
	}

	assetRate := NewAssetRate(factory)

	result, err := assetRate.GetByAssetCode(organizationID, ledgerID, assetCode, limit, page, sortOrder, startDate, endDate)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 2, len(result.Items))
	assert.Equal(t, "USD", result.Items[0].FromAssetCode)
	assert.Equal(t, "BRL", result.Items[0].ToAssetCode)
	assert.Equal(t, "USD", result.Items[1].FromAssetCode)
	assert.Equal(t, "EUR", result.Items[1].ToAssetCode)
	assert.Equal(t, "next_page_token", *result.Pagination.NextCursor)
	assert.Equal(t, "prev_page_token", *result.Pagination.PrevCursor)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["GET "+uri])
}
