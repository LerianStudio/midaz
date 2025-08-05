package rest

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/components/mdz/pkg/environment"
	"github.com/LerianStudio/midaz/v3/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/v3/components/mdz/pkg/mockutil"
	"github.com/LerianStudio/midaz/v3/components/mdz/pkg/ptr"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"

	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"
)

func Test_account_Create(t *testing.T) {
	organizationID := "01933f94-67b1-794c-bb13-6b75aed7591a"
	ledgerID := "01933f94-8a8f-7a1e-b4ab-98f35a5f8d61"
	portfolioID := "01933f94-d329-76fe-8de0-40559c7b282d"
	accountID := "01933f96-ed04-7c57-be5b-c091388830f8"

	name := "Investment Account"
	typeV := "creditCard"
	statusCode := "CREDIT"
	statusDescription := ptr.StringPtr("Teste Account")

	metadata := map[string]any{
		"bitcoinn": "1TkzMvuqVuCVvwG9CsgaZWB67xe8",
		"chave":    "metadata_chave",
		"boolean":  true,
	}

	input := mmodel.CreateAccountInput{
		Name: name,
		Type: typeV,
		Status: mmodel.Status{
			Code:        statusCode,
			Description: statusDescription,
		},
		PortfolioID: &portfolioID,
		Metadata:    metadata,
	}

	expectedResult := &mmodel.Account{
		ID:   accountID,
		Name: name,
		Type: typeV,
		Status: mmodel.Status{
			Code:        statusCode,
			Description: statusDescription,
		},
		LedgerID:       ledgerID,
		OrganizationID: organizationID,
		CreatedAt:      time.Date(2024, 11, 18, 14, 04, 35, 972791193, time.UTC),
		UpdatedAt:      time.Date(2024, 11, 18, 14, 04, 35, 972794057, time.UTC),
		DeletedAt:      nil,
		Metadata:       metadata,
	}

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)
	defer httpmock.DeactivateAndReset()

	URIAPILedger := "http://127.0.0.1:3000"

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/accounts",
		URIAPILedger, organizationID, ledgerID)

	httpmock.RegisterResponder(http.MethodPost, uri,
		mockutil.MockResponseFromFile(http.StatusCreated, "./.fixtures/account_response_create.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPILedger: URIAPILedger,
		},
	}

	account := NewAccount(factory)

	result, err := account.Create(organizationID, ledgerID, input)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedResult.ID, result.ID)
	assert.Equal(t, expectedResult.Name, result.Name)
	assert.Equal(t, expectedResult.Type, result.Type)
	assert.Equal(t, expectedResult.Status.Code, result.Status.Code)
	assert.Equal(t, *expectedResult.Status.Description, *result.Status.Description)
	assert.Equal(t, expectedResult.LedgerID, result.LedgerID)
	assert.Equal(t, expectedResult.OrganizationID, result.OrganizationID)
	assert.Equal(t, expectedResult.CreatedAt, result.CreatedAt)
	assert.Equal(t, expectedResult.UpdatedAt, result.UpdatedAt)
	assert.Equal(t, expectedResult.DeletedAt, result.DeletedAt)
	assert.Equal(t, expectedResult.Metadata, result.Metadata)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["POST "+uri])
}

func Test_account_Get(t *testing.T) {
	organizationID := "01933f94-67b1-794c-bb13-6b75aed7591a"
	ledgerID := "01933f94-8a8f-7a1e-b4ab-98f35a5f8d61"
	portfolioID := "01933f94-d329-76fe-8de0-40559c7b282d"

	limit := 10
	page := 1

	expectedResult := mmodel.Accounts{
		Page:  page,
		Limit: limit,
		Items: []mmodel.Account{
			{
				ID:   "01933f96-ed04-7c57-be5b-c091388830f8",
				Name: "Investment Account",
				Type: "creditCard",
				Status: mmodel.Status{
					Code:        "CREDIT",
					Description: ptr.StringPtr("Teste Account"),
				},
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				PortfolioID:    &portfolioID,
				CreatedAt:      time.Date(2024, 11, 18, 14, 04, 35, 972791000, time.UTC),
				UpdatedAt:      time.Date(2024, 11, 18, 14, 04, 35, 972794000, time.UTC),
				DeletedAt:      nil,
				Metadata: map[string]any{
					"bitcoin": "1TkzMvuqVuCVvwG9CsgaZWB67xe8",
					"chave":   "metadata_chave",
					"boolean": true,
				},
			},
			{
				ID:   "01933f96-cc04-7509-8c2f-0faf76be1999",
				Name: "Home Loan Account",
				Type: "currency",
				Status: mmodel.Status{
					Code:        "CREDIT",
					Description: ptr.StringPtr("Teste Account"),
				},
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				PortfolioID:    &portfolioID,
				CreatedAt:      time.Date(2024, 11, 18, 14, 04, 27, 524314000, time.UTC),
				UpdatedAt:      time.Date(2024, 11, 18, 14, 04, 27, 524316000, time.UTC),
				DeletedAt:      nil,
				Metadata: map[string]any{
					"bitcoin": "3T9HRnyWiZiFfWm98HjMnj5v8w1H5tK",
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

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/accounts?limit=%d&page=%d",
		URIAPILedger, organizationID, ledgerID, limit, page)

	httpmock.RegisterResponder(http.MethodGet, uri,
		mockutil.MockResponseFromFile(http.StatusOK, "./.fixtures/account_response_list.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPILedger: URIAPILedger,
		},
	}

	asset := NewAccount(factory)

	result, err := asset.Get(organizationID, ledgerID, limit, page, "", "", "")

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

func Test_account_GetByID(t *testing.T) {
	organizationID := "01933f94-67b1-794c-bb13-6b75aed7591a"
	ledgerID := "01933f94-8a8f-7a1e-b4ab-98f35a5f8d61"
	portfolioID := "01933f94-d329-76fe-8de0-40559c7b282d"
	accountID := "01933f96-ed04-7c57-be5b-c091388830f8"

	URIAPILedger := "http://127.0.0.1:3000"

	expectedResult := &mmodel.Account{
		ID:   accountID,
		Name: "Investment Account",
		Type: "creditCard",
		Status: mmodel.Status{
			Code:        "CREDIT",
			Description: ptr.StringPtr("Teste Account"),
		},
		LedgerID:       ledgerID,
		PortfolioID:    ptr.StringPtr(portfolioID),
		OrganizationID: organizationID,
		CreatedAt:      time.Date(2024, 11, 18, 14, 04, 35, 972791000, time.UTC),
		UpdatedAt:      time.Date(2024, 11, 18, 14, 04, 35, 972794000, time.UTC),
		DeletedAt:      nil,
		Metadata: map[string]any{
			"bitcoin": "1TkzMvuqVuCVvwG9CsgaZWB67xe8",
			"chave":   "metadata_chave",
			"boolean": true,
		},
	}

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)
	defer httpmock.DeactivateAndReset()

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/accounts/%s",
		URIAPILedger, organizationID, ledgerID, accountID)

	httpmock.RegisterResponder(http.MethodGet, uri,
		mockutil.MockResponseFromFile(http.StatusOK, "./.fixtures/account_response_describe.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPILedger: URIAPILedger,
		},
	}

	account := NewAccount(factory)

	result, err := account.GetByID(organizationID, ledgerID, accountID)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedResult.ID, result.ID)
	assert.Equal(t, expectedResult.Name, result.Name)
	assert.Equal(t, expectedResult.OrganizationID, result.OrganizationID)
	assert.Equal(t, expectedResult.LedgerID, result.LedgerID)
	assert.Equal(t, expectedResult.Type, result.Type)
	assert.Equal(t, expectedResult.Status.Code, result.Status.Code)
	assert.Equal(t, expectedResult.Status.Description, result.Status.Description)
	assert.Equal(t, expectedResult.PortfolioID, result.PortfolioID)
	assert.Equal(t, expectedResult.CreatedAt, result.CreatedAt)
	assert.Equal(t, expectedResult.UpdatedAt, result.UpdatedAt)
	assert.Equal(t, expectedResult.DeletedAt, result.DeletedAt)
	assert.Equal(t, expectedResult.Metadata, result.Metadata)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["GET "+uri])
}

func Test_account_Update(t *testing.T) {
	organizationID := "01933f94-67b1-794c-bb13-6b75aed7591a"
	ledgerID := "01933f94-8a8f-7a1e-b4ab-98f35a5f8d61"
	portfolioID := "01933f94-d329-76fe-8de0-40559c7b282d"
	accountID := "01933f96-ed04-7c57-be5b-c091388830f8"

	name := "Auto Loan Account"
	typev := "creditCard"
	statusCode := "ACTIVE"
	statusDescription := ptr.StringPtr("Teste Account")

	metadata := map[string]any{
		"bitcoin": "1qicmb2yk2mUg6ayeoYJpCguk",
		"chave":   "jacare",
		"boolean": false,
	}

	inp := mmodel.UpdateAccountInput{
		Name: name,
		Status: mmodel.Status{
			Code:        statusCode,
			Description: statusDescription,
		},
		Metadata: metadata,
	}

	expectedResult := &mmodel.Account{
		ID:             accountID,
		Name:           name,
		Type:           typev,
		PortfolioID:    &portfolioID,
		LedgerID:       ledgerID,
		OrganizationID: organizationID,
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

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/accounts/%s",
		URIAPILedger, organizationID, ledgerID, accountID)

	httpmock.RegisterResponder(http.MethodPatch, uri,
		mockutil.MockResponseFromFile(http.StatusOK,
			"./.fixtures/account_response_update.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPILedger: URIAPILedger,
		},
	}

	account := NewAccount(factory)

	result, err := account.Update(organizationID, ledgerID, accountID, inp)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedResult.ID, result.ID)
	assert.Equal(t, expectedResult.LedgerID, result.LedgerID)
	assert.Equal(t, expectedResult.PortfolioID, result.PortfolioID)
	assert.Equal(t, expectedResult.OrganizationID, result.OrganizationID)
	assert.Equal(t, expectedResult.Name, result.Name)
	assert.Equal(t, expectedResult.Type, result.Type)
	assert.Equal(t, expectedResult.Status.Code, result.Status.Code)
	assert.Equal(t, expectedResult.Status.Description, result.Status.Description)
	assert.Equal(t, expectedResult.Metadata, result.Metadata)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["PATCH "+uri])
}

func Test_account_Delete(t *testing.T) {
	organizationID := "01933f94-67b1-794c-bb13-6b75aed7591a"
	ledgerID := "01933f94-8a8f-7a1e-b4ab-98f35a5f8d61"
	accountID := "01933f96-ed04-7c57-be5b-c091388830f8"

	URIAPILedger := "http://127.0.0.1:3000"

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)
	defer httpmock.DeactivateAndReset()

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/accounts/%s",
		URIAPILedger, organizationID, ledgerID, accountID)

	httpmock.RegisterResponder(http.MethodDelete, uri,
		httpmock.NewStringResponder(http.StatusNoContent, ""))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPILedger: URIAPILedger,
		},
	}

	account := NewAccount(factory)

	err := account.Delete(organizationID, ledgerID, accountID)

	assert.NoError(t, err)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["DELETE "+uri])
}
