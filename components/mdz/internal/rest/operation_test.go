package rest

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/LerianStudio/midaz/components/mdz/pkg/environment"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/mockutil"

	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"
)

func Test_operation_Get(t *testing.T) {
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

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/operations?limit=%d&page=%d&sort=%s&startDate=%s&endDate=%s",
		URIAPITransaction, organizationID, ledgerID, limit, page, sortOrder, startDate, endDate)

	httpmock.RegisterResponder(http.MethodGet, uri,
		mockutil.MockResponseFromFile(http.StatusOK, "./.fixtures/operation_response_get.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPITransaction: URIAPITransaction,
		},
	}

	operation := NewOperation(factory)

	result, err := operation.Get(organizationID, ledgerID, limit, page, sortOrder, startDate, endDate)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, len(result.Items))
	assert.Equal(t, "01933f96-ed04-7c57-be5b-c091388830f8", result.Items[0].ID)
	assert.Equal(t, "01933f94-67b1-794c-bb13-6b75aed7591b", result.Items[0].AccountID)
	assert.Equal(t, int64(1000), result.Items[0].Amount)
	assert.Equal(t, "USD", result.Items[0].AssetCode)
	assert.Equal(t, "next_page_token", result.Pagination.NextCursor)
	assert.Equal(t, "prev_page_token", result.Pagination.PrevCursor)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["GET "+uri])
}

func Test_operation_GetByID(t *testing.T) {
	organizationID := "01933f94-67b1-794c-bb13-6b75aed7591a"
	ledgerID := "01933f94-8a8f-7a1e-b4ab-98f35a5f8d61"
	operationID := "01933f96-ed04-7c57-be5b-c091388830f8"

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)
	defer httpmock.DeactivateAndReset()

	URIAPITransaction := "http://127.0.0.1:3001"

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/operations/%s",
		URIAPITransaction, organizationID, ledgerID, operationID)

	httpmock.RegisterResponder(http.MethodGet, uri,
		mockutil.MockResponseFromFile(http.StatusOK, "./.fixtures/operation_response_get_by_id.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPITransaction: URIAPITransaction,
		},
	}

	operation := NewOperation(factory)

	result, err := operation.GetByID(organizationID, ledgerID, operationID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, operationID, result.ID)
	assert.Equal(t, "01933f94-67b1-794c-bb13-6b75aed7591b", result.AccountID)
	assert.Equal(t, int64(1000), result.Amount)
	assert.Equal(t, "credit", result.Type)
	assert.Equal(t, "USD", result.AssetCode)
	assert.Equal(t, "Test Operation", result.Description)
	assert.Equal(t, "01933f94-67b1-794c-bb13-6b75aed7591a", result.TransactionID)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["GET "+uri])
}

func Test_operation_GetByAccount(t *testing.T) {
	organizationID := "01933f94-67b1-794c-bb13-6b75aed7591a"
	ledgerID := "01933f94-8a8f-7a1e-b4ab-98f35a5f8d61"
	accountID := "01933f94-67b1-794c-bb13-6b75aed7591b"
	limit := 10
	page := 1
	sortOrder := "desc"
	startDate := "2024-01-01"
	endDate := "2024-12-31"

	client := &http.Client{}
	httpmock.ActivateNonDefault(client)
	defer httpmock.DeactivateAndReset()

	URIAPITransaction := "http://127.0.0.1:3001"

	uri := fmt.Sprintf("%s/v1/organizations/%s/ledgers/%s/accounts/%s/operations?limit=%d&page=%d&sort=%s&startDate=%s&endDate=%s",
		URIAPITransaction, organizationID, ledgerID, accountID, limit, page, sortOrder, startDate, endDate)

	httpmock.RegisterResponder(http.MethodGet, uri,
		mockutil.MockResponseFromFile(http.StatusOK, "./.fixtures/operation_response_get_by_account.json"))

	factory := &factory.Factory{
		HTTPClient: client,
		Env: &environment.Env{
			URLAPITransaction: URIAPITransaction,
		},
	}

	operation := NewOperation(factory)

	result, err := operation.GetByAccount(organizationID, ledgerID, accountID, limit, page, sortOrder, startDate, endDate)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 2, len(result.Items))
	assert.Equal(t, "01933f96-ed04-7c57-be5b-c091388830f8", result.Items[0].ID)
	assert.Equal(t, "01933f96-ed04-7c57-be5b-c091388830f9", result.Items[1].ID)
	assert.Equal(t, int64(1000), result.Items[0].Amount)
	assert.Equal(t, int64(-500), result.Items[1].Amount)
	assert.Equal(t, "credit", result.Items[0].Type)
	assert.Equal(t, "debit", result.Items[1].Type)
	assert.Equal(t, "next_page_token", result.Pagination.NextCursor)
	assert.Equal(t, "prev_page_token", result.Pagination.PrevCursor)

	info := httpmock.GetCallCountInfo()
	assert.Equal(t, 1, info["GET "+uri])
}
