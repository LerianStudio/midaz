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

func Test_balance_Get(t *testing.T) {
	organizationID := "0192fc1d-f34d-78c9-9654-83e497349241"
	ledgerID := "01930218-bfb7-74fe-ba00-e52a17e9fb4e"

	limit := 2
	page := 1

	expectedResult := mmodel.Balances{
		Page:  page,
		Limit: limit,
		Items: []mmodel.Balance{
			{
				ID:             "01932165-b21d-7e6a-b0fc-d5f625c42a72",
				AccountID:      "01932159-f4bd-7e0a-971e-52cc6e528312",
				AssetID:        "01930219-2c25-7a37-a5b9-610d44ae0a27",
				Amount:         "1000.00",
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				CreatedAt:      time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
				UpdatedAt:      time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
			},
			{
				ID:             "01932166-c32e-7f7b-c1fd-e6g737d53b83",
				AccountID:      "01932160-g5ce-7f1b-982f-63dd7f639423",
				AssetID:        "01930365-4d46-7a09-a503-b932714f85af",
				Amount:         "2500.50",
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

	URIAPILedger := "http://127.0.0.1:3000"

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/balances?limit=%d&page=%d",
		URIAPILedger, organizationID, ledgerID, limit, page)

	httpmock.RegisterResponder(http.MethodGet, uri,
		mockutil.MockResponseFromFile(http.StatusOK, "./.fixtures/balance_response_list.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPILedger: URIAPILedger,
		},
	}

	balance := NewBalance(factory)

	result, err := balance.Get(organizationID, ledgerID, limit, page, "", "", "")

	assert.NoError(t, err)
	assert.NotNil(t, result)

	for i, v := range result.Items {
		assert.Equal(t, expectedResult.Items[i].ID, v.ID)
		assert.Equal(t, expectedResult.Items[i].AccountID, v.AccountID)
		assert.Equal(t, expectedResult.Items[i].AssetID, v.AssetID)
		assert.Equal(t, expectedResult.Items[i].Amount, v.Amount)
	}
	assert.Equal(t, expectedResult.Limit, limit)
	assert.Equal(t, expectedResult.Page, page)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["GET "+uri])
}

func Test_balance_GetByID(t *testing.T) {
	balanceID := "01932165-b21d-7e6a-b0fc-d5f625c42a72"
	ledgerID := "01930218-bfb7-74fe-ba00-e52a17e9fb4e"
	organizationID := "0192fc1d-f34d-78c9-9654-83e497349241"
	accountID := "01932159-f4bd-7e0a-971e-52cc6e528312"
	assetID := "01930219-2c25-7a37-a5b9-610d44ae0a27"
	amount := "1000.00"

	URIAPILedger := "http://127.0.0.1:3000"

	expectedResult := &mmodel.Balance{
		ID:             balanceID,
		AccountID:      accountID,
		AssetID:        assetID,
		Amount:         amount,
		LedgerID:       ledgerID,
		OrganizationID: organizationID,
		CreatedAt:      time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
		UpdatedAt:      time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
	}

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)
	defer httpmock.DeactivateAndReset()

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/balances/%s",
		URIAPILedger, organizationID, ledgerID, balanceID)

	httpmock.RegisterResponder(http.MethodGet, uri,
		mockutil.MockResponseFromFile(http.StatusOK, "./.fixtures/balance_response_get_by_id.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPILedger: URIAPILedger,
		},
	}

	balance := NewBalance(factory)

	result, err := balance.GetByID(organizationID, ledgerID, balanceID)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedResult.ID, result.ID)
	assert.Equal(t, expectedResult.AccountID, result.AccountID)
	assert.Equal(t, expectedResult.AssetID, result.AssetID)
	assert.Equal(t, expectedResult.Amount, result.Amount)
	assert.Equal(t, expectedResult.OrganizationID, result.OrganizationID)
	assert.Equal(t, expectedResult.LedgerID, result.LedgerID)
	assert.Equal(t, expectedResult.CreatedAt, result.CreatedAt)
	assert.Equal(t, expectedResult.UpdatedAt, result.UpdatedAt)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["GET "+uri])
}

func Test_balance_GetByAccount(t *testing.T) {
	organizationID := "0192fc1d-f34d-78c9-9654-83e497349241"
	ledgerID := "01930218-bfb7-74fe-ba00-e52a17e9fb4e"
	accountID := "01932159-f4bd-7e0a-971e-52cc6e528312"

	limit := 2
	page := 1

	expectedResult := mmodel.Balances{
		Page:  page,
		Limit: limit,
		Items: []mmodel.Balance{
			{
				ID:             "01932165-b21d-7e6a-b0fc-d5f625c42a72",
				AccountID:      accountID,
				AssetID:        "01930219-2c25-7a37-a5b9-610d44ae0a27",
				Amount:         "1000.00",
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				CreatedAt:      time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
				UpdatedAt:      time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
			},
			{
				ID:             "01932166-c32e-7f7b-c1fd-e6g737d53b83",
				AccountID:      accountID,
				AssetID:        "01930365-4d46-7a09-a503-b932714f85af",
				Amount:         "500.75",
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

	URIAPILedger := "http://127.0.0.1:3000"

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/accounts/%s/balances?limit=%d&page=%d",
		URIAPILedger, organizationID, ledgerID, accountID, limit, page)

	httpmock.RegisterResponder(http.MethodGet, uri,
		mockutil.MockResponseFromFile(http.StatusOK, "./.fixtures/balance_response_by_account.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPILedger: URIAPILedger,
		},
	}

	balance := NewBalance(factory)

	result, err := balance.GetByAccount(organizationID, ledgerID, accountID, limit, page, "", "", "")

	assert.NoError(t, err)
	assert.NotNil(t, result)

	for i, v := range result.Items {
		assert.Equal(t, expectedResult.Items[i].ID, v.ID)
		assert.Equal(t, expectedResult.Items[i].AccountID, v.AccountID)
		assert.Equal(t, expectedResult.Items[i].AssetID, v.AssetID)
		assert.Equal(t, expectedResult.Items[i].Amount, v.Amount)
	}
	assert.Equal(t, expectedResult.Limit, limit)
	assert.Equal(t, expectedResult.Page, page)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["GET "+uri])
}

func Test_balance_Update(t *testing.T) {
	balanceID := "01932165-b21d-7e6a-b0fc-d5f625c42a72"
	ledgerID := "01930218-bfb7-74fe-ba00-e52a17e9fb4e"
	organizationID := "0192fc1d-f34d-78c9-9654-83e497349241"
	accountID := "01932159-f4bd-7e0a-971e-52cc6e528312"
	assetID := "01930219-2c25-7a37-a5b9-610d44ae0a27"
	amount := "1500.00"

	inp := mmodel.UpdateBalance{
		Amount: amount,
	}

	expectedResult := &mmodel.Balance{
		ID:             balanceID,
		AccountID:      accountID,
		AssetID:        assetID,
		Amount:         amount,
		LedgerID:       ledgerID,
		OrganizationID: organizationID,
		CreatedAt:      time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
		UpdatedAt:      time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
	}

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)
	defer httpmock.DeactivateAndReset()

	URIAPILedger := "http://127.0.0.1:3000"

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/balances/%s",
		URIAPILedger, organizationID, ledgerID, balanceID)

	httpmock.RegisterResponder(http.MethodPatch, uri,
		mockutil.MockResponseFromFile(http.StatusOK,
			"./.fixtures/balance_response_update.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPILedger: URIAPILedger,
		},
	}

	balance := NewBalance(factory)

	result, err := balance.Update(organizationID, ledgerID, balanceID, inp)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedResult.ID, result.ID)
	assert.Equal(t, expectedResult.AccountID, result.AccountID)
	assert.Equal(t, expectedResult.AssetID, result.AssetID)
	assert.Equal(t, expectedResult.Amount, result.Amount)
	assert.Equal(t, expectedResult.OrganizationID, result.OrganizationID)
	assert.Equal(t, expectedResult.LedgerID, result.LedgerID)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["PATCH "+uri])
}

func Test_balance_Delete(t *testing.T) {
	balanceID := "01932165-b21d-7e6a-b0fc-d5f625c42a72"
	ledgerID := "01930218-bfb7-74fe-ba00-e52a17e9fb4e"
	organizationID := "0192fc1d-f34d-78c9-9654-83e497349241"
	URIAPILedger := "http://127.0.0.1:3000"

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)
	defer httpmock.DeactivateAndReset()

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/balances/%s",
		URIAPILedger, organizationID, ledgerID, balanceID)

	httpmock.RegisterResponder(http.MethodDelete, uri,
		httpmock.NewStringResponder(http.StatusNoContent, ""))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPILedger: URIAPILedger,
		},
	}

	balance := NewBalance(factory)

	err := balance.Delete(organizationID, ledgerID, balanceID)

	assert.NoError(t, err)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["DELETE "+uri])
}