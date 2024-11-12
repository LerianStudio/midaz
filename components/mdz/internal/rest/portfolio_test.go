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

func Test_portfolio_Create(t *testing.T) {
	portfolioID := "01931b44-6e33-791a-bfad-27992fa15984"
	ledgerID := "01931b04-c2d1-7a41-83ac-c5d6d8a3c22c"
	organizationID := "01931b04-964a-7caa-a422-c29a95387c00"

	name := "Leslie_Spencer Portfolio"
	code := "ACTIVE"
	description := ptr.StringPtr("Teste Portfolio")
	allowSending := ptr.BoolPtr(true)
	allowReceiving := ptr.BoolPtr(false)

	metadata := map[string]any{
		"bitcoin": "3o5onPR55kL6ajk14dGL5Q1fEhAnvY",
		"chave":   "metadata_chave",
		"boolean": true,
	}

	input := mmodel.CreatePortfolioInput{
		Name: name,
		Status: mmodel.StatusAllow{
			Code:           code,
			Description:    description,
			AllowSending:   allowSending,
			AllowReceiving: allowReceiving,
		},
		Metadata: metadata,
	}

	expectedResult := &mmodel.Portfolio{
		ID:             portfolioID,
		Name:           name,
		LedgerID:       ledgerID,
		OrganizationID: organizationID,
		Status: mmodel.StatusAllow{
			Code:           code,
			Description:    description,
			AllowSending:   allowSending,
			AllowReceiving: allowReceiving,
		},
		Metadata: metadata,
	}

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)
	defer httpmock.DeactivateAndReset()

	URIAPILedger := "http://127.0.0.1:3000"

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/portfolios",
		URIAPILedger, organizationID, ledgerID)

	httpmock.RegisterResponder(http.MethodPost, uri,
		mockutil.MockResponseFromFile(http.StatusCreated, "./.fixtures/portfolio_response_create.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPILedger: URIAPILedger,
		},
	}

	portfolioServ := NewPortfolio(factory)

	result, err := portfolioServ.Create(organizationID, ledgerID, input)

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

func Test_portfolio_Get(t *testing.T) {
	organizationID := "01931b04-964a-7caa-a422-c29a95387c00"
	ledgerID := "01931b04-c2d1-7a41-83ac-c5d6d8a3c22c"

	limit := 2
	page := 1

	expectedResult := mmodel.Portfolios{
		Page:  page,
		Limit: limit,
		Items: []mmodel.Portfolio{
			{
				ID:       "01931c91-1fa5-7e3f-8a52-cea2f98bb9cd",
				Name:     "Daisha_Koepp11 Portfolio",
				EntityID: "bcc37474-577a-43b5-b9a6-3b5ac28473f8",
				Status: mmodel.StatusAllow{
					Code:           "ACTIVE",
					Description:    ptr.StringPtr("Teste Portfolio"),
					AllowSending:   ptr.BoolPtr(true),
					AllowReceiving: ptr.BoolPtr(false),
				},
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				CreatedAt:      time.Date(2024, 11, 11, 18, 51, 33, 15793, time.UTC),
				UpdatedAt:      time.Date(2024, 11, 11, 18, 51, 33, 15793, time.UTC),
				DeletedAt:      nil,
			},
			{
				ID:       "01931c91-0957-763d-af95-b2ee2a9aae75",
				Name:     "Toy30 Portfolio",
				EntityID: "f3c0c356-1d6b-4cb2-b45f-ee6ce047491e",
				Status: mmodel.StatusAllow{
					Code:           "ACTIVE",
					Description:    ptr.StringPtr("Teste Portfolio"),
					AllowSending:   ptr.BoolPtr(true),
					AllowReceiving: ptr.BoolPtr(false),
				},
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				CreatedAt:      time.Date(2024, 11, 11, 18, 51, 27, 447406, time.UTC),
				UpdatedAt:      time.Date(2024, 11, 11, 18, 51, 27, 447406, time.UTC),
				DeletedAt:      nil,
				Metadata: map[string]any{
					"bitcoin": "1fdFL8cxmWTbwzpiQ7K5PPBJoq7HV",
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

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/portfolios?limit=%d&page=%d",
		URIAPILedger, organizationID, ledgerID, limit, page)

	httpmock.RegisterResponder(http.MethodGet, uri,
		mockutil.MockResponseFromFile(http.StatusOK, "./.fixtures/portfolio_response_list.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPILedger: URIAPILedger,
		},
	}

	asset := NewPortfolio(factory)

	result, err := asset.Get(organizationID, ledgerID, limit, page)

	assert.NoError(t, err)
	assert.NotNil(t, result)

	// _, ok := expectedResult.Items.(*[]mmodel.Portfolio)
	// if !ok {
	// 	t.Fatalf("Failed to convert expectedResult.Items to []mmodel.Portfolio type")
	// }

	// if reflect.TypeOf(expectedResult.Items) == reflect.TypeOf([]mmodel.Portfolio{}) {
	// 	expectedItems := (expectedResult.Items.([]mmodel.Portfolio))
	// 	fmt.Println(expectedItems)
	// } else {
	// 	t.Fatalf("Failed to convert expectedResult.Items to []mmodel.Portfolio type")
	// }

	// _, ok := result.Items.([]mmodel.Portfolio)
	// if !ok {
	// 	t.Fatalf("Failed to convert result.Items to []mmodel.Portfolio type")
	// }

	// for i, v := range actualItems {
	// 	assert.Equal(t, expectedItems[i].ID, v.ID)
	// }

	for i, v := range result.Items {
		assert.Equal(t, expectedResult.Items[i].ID, v.ID)
		assert.Equal(t, expectedResult.Items[i].Metadata, v.Metadata)
	}

	assert.Equal(t, expectedResult.Limit, limit)
	assert.Equal(t, expectedResult.Page, page)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["GET "+uri])
}
