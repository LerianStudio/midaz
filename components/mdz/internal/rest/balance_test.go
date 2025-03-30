package rest

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/LerianStudio/midaz/components/mdz/pkg/environment"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/mockutil"

	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"
)

func Test_balance_Get(t *testing.T) {
	organizationID := "01933f94-67b1-794c-bb13-6b75aed7591a"
	ledgerID := "01933f94-8a8f-7a1e-b4ab-98f35a5f8d61"
	limit := 10
	cursor := ""
	sortOrder := "desc"
	startDate := "2024-01-01"
	endDate := "2024-12-31"

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)
	defer httpmock.DeactivateAndReset()

	URIAPITransaction := "http://127.0.0.1:3001"

	baseURL := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/balances",
		URIAPITransaction, organizationID, ledgerID)
	
	u, err := url.Parse(baseURL)
	assert.NoError(t, err)
	
	q := u.Query()
	q.Set("limit", fmt.Sprintf("%d", limit))
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	q.Set("sort_order", sortOrder)
	q.Set("start_date", startDate)
	q.Set("end_date", endDate)
	u.RawQuery = q.Encode()
	
	uri := u.String()

	httpmock.RegisterResponder(http.MethodGet, uri,
		mockutil.MockResponseFromFile(http.StatusOK, "./.fixtures/balance_response_get.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPITransaction: URIAPITransaction,
		},
	}

	balance := NewBalance(factory)

	result, err := balance.Get(organizationID, ledgerID, limit, cursor, sortOrder, startDate, endDate)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, len(result.Items))
	assert.Equal(t, "01933f96-ed04-7c57-be5b-c091388830f8", result.Items[0].ID)
	assert.Equal(t, "01933f94-67b1-794c-bb13-6b75aed7591b", result.Items[0].AccountID)
	assert.Equal(t, int64(1500), result.Items[0].Amount)
	assert.Equal(t, int64(2), result.Items[0].AmountScale)
	assert.Equal(t, "USD", result.Items[0].AssetCode)
	assert.Equal(t, "next_page_token", *result.Pagination.NextCursor)
	assert.Equal(t, "prev_page_token", *result.Pagination.PrevCursor)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["GET "+uri])
}

func Test_balance_GetByID(t *testing.T) {
	organizationID := "01933f94-67b1-794c-bb13-6b75aed7591a"
	ledgerID := "01933f94-8a8f-7a1e-b4ab-98f35a5f8d61"
	balanceID := "01933f96-ed04-7c57-be5b-c091388830f8"

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)
	defer httpmock.DeactivateAndReset()

	URIAPITransaction := "http://127.0.0.1:3001"

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/balances/%s",
		URIAPITransaction, organizationID, ledgerID, balanceID)

	httpmock.RegisterResponder(http.MethodGet, uri,
		mockutil.MockResponseFromFile(http.StatusOK, "./.fixtures/balance_response_get_by_id.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPITransaction: URIAPITransaction,
		},
	}

	balance := NewBalance(factory)

	result, err := balance.GetByID(organizationID, ledgerID, balanceID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, balanceID, result.ID)
	assert.Equal(t, "01933f94-67b1-794c-bb13-6b75aed7591b", result.AccountID)
	assert.Equal(t, int64(1500), result.Amount)
	assert.Equal(t, int64(2), result.AmountScale)
	assert.Equal(t, "USD", result.AssetCode)
	// Status field doesn't exist in mmodel.Balance, removing these assertions
	// assert.Equal(t, "ACTIVE", result.Status.Code)
	// assert.Equal(t, "Active balance", *result.Status.Description)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["GET "+uri])
}

func Test_balance_GetByAccount(t *testing.T) {
	organizationID := "01933f94-67b1-794c-bb13-6b75aed7591a"
	ledgerID := "01933f94-8a8f-7a1e-b4ab-98f35a5f8d61"
	accountID := "01933f94-67b1-794c-bb13-6b75aed7591b"
	limit := 10
	cursor := ""
	sortOrder := "desc"
	startDate := "2024-01-01"
	endDate := "2024-12-31"

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)
	defer httpmock.DeactivateAndReset()

	URIAPITransaction := "http://127.0.0.1:3001"

	baseURL := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/accounts/%s/balances",
		URIAPITransaction, organizationID, ledgerID, accountID)
	
	u, err := url.Parse(baseURL)
	assert.NoError(t, err)
	
	q := u.Query()
	q.Set("limit", fmt.Sprintf("%d", limit))
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	q.Set("sort_order", sortOrder)
	q.Set("start_date", startDate)
	q.Set("end_date", endDate)
	u.RawQuery = q.Encode()
	
	uri := u.String()

	httpmock.RegisterResponder(http.MethodGet, uri,
		mockutil.MockResponseFromFile(http.StatusOK, "./.fixtures/balance_response_get_by_account.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPITransaction: URIAPITransaction,
		},
	}

	balance := NewBalance(factory)

	result, err := balance.GetByAccount(organizationID, ledgerID, accountID, limit, cursor, sortOrder, startDate, endDate)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 2, len(result.Items))
	assert.Equal(t, "01933f96-ed04-7c57-be5b-c091388830f8", result.Items[0].ID)
	assert.Equal(t, "01933f96-ed04-7c57-be5b-c091388830f9", result.Items[1].ID)
	assert.Equal(t, "USD", result.Items[0].AssetCode)
	assert.Equal(t, "EUR", result.Items[1].AssetCode)
	assert.Equal(t, int64(1500), result.Items[0].Amount)
	assert.Equal(t, int64(2000), result.Items[1].Amount)
	assert.Equal(t, "next_page_token", *result.Pagination.NextCursor)
	assert.Equal(t, "prev_page_token", *result.Pagination.PrevCursor)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["GET "+uri])
}

func Test_balance_Delete(t *testing.T) {
	organizationID := "01933f94-67b1-794c-bb13-6b75aed7591a"
	ledgerID := "01933f94-8a8f-7a1e-b4ab-98f35a5f8d61"
	balanceID := "01933f96-ed04-7c57-be5b-c091388830f8"

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)
	defer httpmock.DeactivateAndReset()

	URIAPITransaction := "http://127.0.0.1:3001"

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/balances/%s",
		URIAPITransaction, organizationID, ledgerID, balanceID)

	httpmock.RegisterResponder(http.MethodDelete, uri,
		httpmock.NewStringResponder(http.StatusNoContent, ""))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPITransaction: URIAPITransaction,
		},
	}

	balance := NewBalance(factory)

	err := balance.Delete(organizationID, ledgerID, balanceID)

	assert.NoError(t, err)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["DELETE "+uri])
}
